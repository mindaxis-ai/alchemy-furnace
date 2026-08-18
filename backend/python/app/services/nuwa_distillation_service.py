"""Nuwa-inspired public-evidence distillation.

Methodology reference: https://github.com/alchaincyf/nuwa-skill (MIT).
The upstream project is an Agent Skill rather than an importable SDK, so this
module adapts its documented research/extraction contract to the application's
existing OpenAI-compatible model layer.
"""
from __future__ import annotations

import json
import re
from typing import Any, Optional

import httpx
from openai import OpenAI

from app.core.config import settings
from app.services.research_provider import ResearchDocument, ResearchProvider


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
    ) -> dict[str, Any]:
        documents = self.research_provider.collect(subject, brief)
        if len(documents) < 2:
            raise ValueError("公开资料不足，请补充更具体的人物全名、领域或目标描述")

        effective_key = (api_key or settings.openai_api_key or "").strip()
        effective_url = (base_url or settings.openai_base_url or "").strip() or None
        is_openai_cloud = not effective_url or "api.openai.com" in effective_url.lower()
        if not effective_key and is_openai_cloud:
            raise ValueError("未配置可用于智能炼制的模型，请先到设置中配置模型供应商")

        client = OpenAI(
            api_key=effective_key or "none",
            base_url=effective_url,
            http_client=httpx.Client(timeout=120.0),
        )
        try:
            response = client.chat.completions.create(
                model=model or settings.synthesis_model or settings.default_model,
                temperature=0.25,
                max_tokens=4096,
                messages=[
                    {"role": "system", "content": self._system_prompt(locale)},
                    {"role": "user", "content": self._research_prompt(subject, brief, documents)},
                ],
            )
            content = response.choices[0].message.content or ""
            result = self._parse_json(content)
            self._validate(result)
            result["sources"] = [
                {"title": d.title, "url": d.url, "dimension": d.dimension}
                for d in documents
            ]
            result["model"] = model or settings.synthesis_model or settings.default_model
            return result
        finally:
            client.close()

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
    def _research_prompt(subject: str, brief: str, documents: list[ResearchDocument]) -> str:
        evidence = "\n\n".join(
            f"[{index}] dimension={doc.dimension}\ntitle={doc.title}\nurl={doc.url}\nexcerpt={doc.excerpt}"
            for index, doc in enumerate(documents, 1)
        )
        return f"蒸馏对象：{subject}\n用户目标：{brief}\n\n公开资料：\n{evidence}"

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
