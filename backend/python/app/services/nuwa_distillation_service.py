"""Nuwa-inspired public-evidence distillation.

Methodology reference: https://github.com/alchaincyf/nuwa-skill (MIT).
The upstream project is an Agent Skill rather than an importable SDK, so this
module adapts its documented research/extract contract to the application's
existing OpenAI-compatible model layer.

Failure semantics: research failures surface as :class:`DistillationError`
with the provider's stable code and stage "research"; model failures map to
``model_timeout`` / ``model_request_failed`` / ``model_invalid_output`` with
stage "distill". Success responses carry a ``research`` metadata summary
(evidence level, counts, warnings) that never includes excerpts or keys.
"""
from __future__ import annotations

import json
import logging
import re
import time
from dataclasses import asdict
from typing import Any, Optional

import httpx
from openai import APIError, APIStatusError, APITimeoutError, OpenAI

from app.core.config import settings
from app.services.research_provider import (
    EvidenceLevel,
    ResearchCredentials,
    ResearchDocument,
    ResearchError,
    ResearchProvider,
)

logger = logging.getLogger(__name__)


def _loggable_subject(subject: str) -> str:
    """subject 只记 80 字符并移除换行;brief 与凭证一律不记录。"""
    return " ".join(subject.split())[:80]


class DistillationError(RuntimeError):
    """Service-level failure with stable machine-readable fields.

    ``code``/``stage``/``retryable`` travel unchanged to the Go gateway and
    frontend; ``details`` carries non-secret context (attempts, counts).
    """

    def __init__(
        self,
        code: str,
        stage: str,
        message: str,
        retryable: bool = False,
        details: dict | None = None,
    ):
        super().__init__(message)
        self.code = code
        self.stage = stage
        self.message = message
        self.retryable = retryable
        self.details = details or {}


class NuwaDistillationService:
    def __init__(self, research_provider: ResearchProvider) -> None:
        self.research_provider = research_provider

    def distill(
        self,
        subject: str,
        brief: str,
        model: Optional[str] = None,
        api_key: Optional[str] = None,
        base_url: Optional[str] = None,
        locale: str = "zh-CN",
        request_id: Optional[str] = None,
    ) -> dict[str, Any]:
        research_credentials = ResearchCredentials(
            model=model or settings.synthesis_model or settings.default_model,
            base_url=(base_url or settings.openai_base_url or "").strip(),
            api_key=(api_key or settings.openai_api_key or "").strip(),
        )
        started = time.monotonic()
        try:
            report = self.research_provider.collect(
                subject,
                brief,
                locale,
                credentials=research_credentials,
            )
        except ResearchError as exc:
            self._log_completion(
                request_id, subject, started, time.monotonic() - started, None,
                exc.attempts, 0, None, exc.code,
            )
            raise DistillationError(
                code=exc.code,
                stage=exc.stage,
                message=exc.message,
                retryable=exc.retryable,
                details={"attempts": [asdict(item) for item in exc.attempts]},
            ) from exc
        research_seconds = time.monotonic() - started
        attempts = report.attempts
        document_count = len(report.documents)
        evidence_value = report.evidence_level.value
        if report.evidence_level is EvidenceLevel.INSUFFICIENT:
            self._log_completion(
                request_id, subject, started, research_seconds, None,
                attempts, document_count, evidence_value, "research_insufficient_evidence",
            )
            raise DistillationError(
                code="research_insufficient_evidence",
                stage="research",
                message="找到的有效公开资料过少，请补充人物全名、领域或更明确的目标",
                retryable=False,
                details=report.public_summary(),
            )
        documents = report.documents

        effective_key = research_credentials.api_key
        effective_url = research_credentials.base_url or None
        is_openai_cloud = not effective_url or "api.openai.com" in effective_url.lower()
        if not effective_key and is_openai_cloud:
            self._log_completion(
                request_id, subject, started, time.monotonic() - started, None,
                attempts, document_count, evidence_value, "model_not_configured",
            )
            raise DistillationError(
                code="model_not_configured",
                stage="model",
                message="未配置可用于智能炼制的模型，请先到设置中配置模型供应商",
                retryable=False,
                details={},
            )

        client = OpenAI(
            api_key=effective_key or "none",
            base_url=effective_url,
            http_client=httpx.Client(timeout=120.0),
        )
        try:
            model_started = time.monotonic()
            try:
                response = client.chat.completions.create(
                    model=research_credentials.model,
                    temperature=0.25,
                    max_tokens=4096,
                    messages=[
                        {"role": "system", "content": self._system_prompt(locale)},
                        {
                            "role": "user",
                            "content": self._research_prompt(
                                subject, brief, documents, report.evidence_level
                            ),
                        },
                    ],
                )
            except APITimeoutError as exc:
                self._log_completion(
                    request_id, subject, started, research_seconds,
                    time.monotonic() - model_started, attempts,
                    document_count, evidence_value, "model_timeout",
                )
                raise DistillationError(
                    "model_timeout", "distill", "模型响应超时，请重试", True
                ) from exc
            except APIStatusError as exc:
                self._log_completion(
                    request_id, subject, started, research_seconds,
                    time.monotonic() - model_started, attempts,
                    document_count, evidence_value, f"model_request_failed_{exc.status_code}",
                )
                raise DistillationError(
                    "model_request_failed",
                    "distill",
                    "模型调用失败，请检查模型配置",
                    exc.status_code >= 500,
                ) from exc
            except APIError as exc:
                self._log_completion(
                    request_id, subject, started, research_seconds,
                    time.monotonic() - model_started, attempts,
                    document_count, evidence_value, "model_request_failed",
                )
                raise DistillationError(
                    "model_request_failed",
                    "distill",
                    "模型调用失败，请检查供应商连接",
                    True,
                ) from exc
            model_seconds = time.monotonic() - model_started
            content = response.choices[0].message.content or ""
            try:
                result = self._parse_json(content)
                self._validate(result)
            except ValueError as exc:
                self._log_completion(
                    request_id, subject, started, research_seconds, model_seconds,
                    attempts, document_count, evidence_value, "model_invalid_output",
                )
                raise DistillationError(
                    "model_invalid_output",
                    "distill",
                    str(exc),
                    True,
                ) from exc
            result["sources"] = [
                {"title": d.title, "url": d.url, "dimension": d.dimension}
                for d in documents
            ]
            result["model"] = research_credentials.model
            result["research"] = {
                "evidence_level": report.evidence_level.value,
                "document_count": len(report.documents),
                "domain_count": report.domain_count,
                "total_characters": report.total_characters,
                "warnings": report.warnings,
            }
            self._log_completion(
                request_id, subject, started, research_seconds, model_seconds,
                attempts, document_count, evidence_value, "success",
            )
            return result
        finally:
            client.close()

    def _log_completion(
        self,
        request_id: Optional[str],
        subject: str,
        started: float,
        research_seconds: float,
        model_seconds: Optional[float],
        attempts: list,
        document_count: int,
        evidence_value: Optional[str],
        result_status: str,
    ) -> None:
        """结构化完成日志：不含 brief、excerpt 与任何凭证。"""
        logger.info(
            "distillation complete request_id=%s subject=%s result=%s research_ms=%d "
            "model_ms=%s providers=%s candidates=%d accepted=%d evidence=%s",
            request_id or "-",
            _loggable_subject(subject),
            result_status,
            round(research_seconds * 1000),
            f"{round(model_seconds * 1000)}" if model_seconds is not None else "-",
            [f"{a.provider}:{a.status}" for a in attempts],
            sum(a.discovered for a in attempts),
            document_count,
            evidence_value or "-",
        )

    @staticmethod
    def _system_prompt(locale: str) -> str:
        language = "English" if locale == "en" else "简体中文"
        return f"""你是女娲蒸馏器，按 nuwa-skill 的公开方法论从证据中提取认知架构，而不是角色扮演。
输出语言：{language}。只输出 JSON，不要 Markdown。
规则：
1. 心智模型必须有跨场景证据、预测力和独特性；不满足则不要写。
2. 区分事实、合理推断和未知；不得编造引语、经历或来源。
3. 提取 3-7 个 mental_models、5-10 个 decision_heuristics，并给出 expression_dna。
4. 必须写 values、anti_patterns、honest_limits 和 2-4 组 example_dialogues。
5. source_evidence 只引用输入资料中的 URL；诚实边界必须说明资料空白。
JSON 结构：{{"name":string,"description":string,"persona_summary":string,"tags":string[],"skill_schema":{{"identity_card":string,"expression_dna":{{"sentence_length":"short|medium|long|mixed","formality":number,"vocabulary":string[],"taboo_words":string[],"rhythm":string,"humor_type":string,"certainty_style":string,"citation_habit":string}},"mental_models":[{{"name":string,"one_liner":string,"source_evidence":string[],"application":string,"detection_questions":string[],"limitations":string[]}}],"decision_heuristics":[{{"condition":string,"action":string,"case":string}}],"values":string[],"anti_patterns":string[],"honest_limits":string[],"example_dialogues":[{{"user":string,"assistant":string}}]}}}}"""

    @staticmethod
    def _research_prompt(
        subject: str,
        brief: str,
        documents: list[ResearchDocument],
        evidence_level: EvidenceLevel,
    ) -> str:
        evidence = "\n\n".join(
            f"[{index}] dimension={doc.dimension}\ntitle={doc.title}\nurl={doc.url}\nexcerpt={doc.excerpt}"
            for index, doc in enumerate(documents, 1)
        )
        limited = (
            "证据等级为 limited：只输出证据能支持的结论，并在 honest_limits 明确资料空白。\n\n"
            if evidence_level is EvidenceLevel.LIMITED
            else ""
        )
        return f"蒸馏对象：{subject}\n用户目标：{brief}\n\n{limited}公开资料：\n{evidence}"

    @staticmethod
    def _parse_json(content: str) -> dict[str, Any]:
        cleaned = content.strip()
        fenced = re.search(r"```(?:json)?\s*(\{.*\})\s*```", cleaned, re.DOTALL)
        if fenced:
            cleaned = fenced.group(1)
        else:
            start, end = cleaned.find("{"), cleaned.rfind("}")
            if start >= 0 and end > start:
                cleaned = cleaned[start : end + 1]
        try:
            return json.loads(cleaned)
        except json.JSONDecodeError as exc:
            raise ValueError("模型未返回有效的结构化丹方，请重试") from exc

    @staticmethod
    def _validate(result: dict[str, Any]) -> None:
        schema = result.get("skill_schema")
        if not isinstance(schema, dict) or not isinstance(schema.get("expression_dna"), dict):
            raise ValueError("蒸馏结果缺少表达 DNA，请重试")
        if not result.get("name") or not result.get("persona_summary"):
            raise ValueError("蒸馏结果不完整，请重试")
