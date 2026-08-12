# -*- coding: utf-8 -*-
"""
金丹融合服务 - 合丹为新

随机抽融合算子(Promptbreeder 风格),构建 prompt 让 LLM 把 N 枚金丹的
完整 skill_schema 创造性融合为一枚全新金丹。
失败重试 1 次(重抽算子),再失败走结构化保底(degraded=True)。
"""
import json
import logging
import random
from typing import Any, Dict, List, Optional, Tuple

from app.services.fusion_operators import (
    FUSION_OPERATORS, FusionOperator, sample_operator,
)

logger = logging.getLogger(__name__)

_REQUIRED_SCHEMA_SECTIONS = [
    "identity_card", "expression_dna", "mental_models", "decision_heuristics",
    "values", "anti_patterns", "honest_limits", "example_dialogues",
]

_SYSTEM_PROMPT = "你是炼丹炉的金丹融合大师。你只输出合法 JSON,不要输出任何其他文字。"

_TASK_TEMPLATE = """## 融合算子
{op_name}: {op_instruction}

## 原料金丹 (共 {n} 枚)
{pills_block}

## 任务
按算子指令,将上述 {n} 枚金丹融合为一枚全新金丹。模型自由发挥,不必忠实于任何单一原料。

输出 JSON,结构如下:
{{
  "name": "全新名字(不得复用任何原料名,不超过 8 字)",
  "description": "简介,含触发语与反触发语,100 字内",
  "skill_schema": {{
    "identity_card": "...",
    "expression_dna": {{
      "sentence_length": "short|medium|long|mixed",
      "formality": 0.0 到 1.0 的小数,
      "vocabulary": ["..."], "taboo_words": ["..."],
      "rhythm": "...", "humor_type": "...",
      "certainty_style": "...", "citation_habit": "..."
    }},
    "mental_models": [{{"name": "...", "one_liner": "...", "application": "...", "limitations": ["..."]}}],
    "decision_heuristics": [{{"condition": "...", "action": "...", "case": "..."}}],
    "values": ["..."],
    "anti_patterns": ["..."],
    "honest_limits": ["..."],
    "example_dialogues": [{{"user": "...", "assistant": "..."}}]
  }}
}}

约束:
- example_dialogues 至少 2 条
- mental_models / decision_heuristics 各至少 1 条
- 所有文本用中文"""


def _build_pills_block(pills: List[Dict[str, Any]]) -> str:
    parts = []
    for i, p in enumerate(pills, 1):
        parts.append(
            f"### 金丹 {i}: {p.get('name', '')}\n"
            f"描述: {p.get('description', '')}\n"
            f"skill_schema: {json.dumps(p.get('skill_schema') or {}, ensure_ascii=False)}"
        )
    return "\n\n".join(parts)


def _validate_payload(payload: Dict[str, Any]) -> bool:
    """校验 LLM 返回结构: name/description 非空 + schema 关键 section 存在"""
    if not isinstance(payload, dict):
        return False
    if not str(payload.get("name") or "").strip():
        return False
    if not str(payload.get("description") or "").strip():
        return False
    schema = payload.get("skill_schema")
    if not isinstance(schema, dict):
        return False
    return all(section in schema for section in _REQUIRED_SCHEMA_SECTIONS)


class FusionService:
    """金丹融合服务: 算子抽样 + prompt 构建 + LLM + 校验 + 保底"""

    def __init__(self) -> None:
        # 延迟导入,避免与 runtime.setup_providers 循环依赖
        from app.services.chat_service import ChatService
        self._chat = ChatService()

    def _pick_operator(self, exclude_operator_id: Optional[str]) -> FusionOperator:
        if not exclude_operator_id:
            return sample_operator()
        candidates = [op for op in FUSION_OPERATORS if op.id != exclude_operator_id]
        return random.choice(candidates)

    def _call_llm(
        self,
        pills: List[Dict[str, Any]],
        operator: FusionOperator,
        model: Optional[str],
        api_key: Optional[str],
        base_url: Optional[str],
    ) -> Tuple[Dict[str, Any], str]:
        user_prompt = _TASK_TEMPLATE.format(
            op_name=operator.name,
            op_instruction=operator.instruction,
            n=len(pills),
            pills_block=_build_pills_block(pills),
        )
        resp = self._chat.chat_completion(
            messages=[
                {"role": "system", "content": _SYSTEM_PROMPT},
                {"role": "user", "content": user_prompt},
            ],
            model=model,
            temperature=1.0,
            max_tokens=4096,
            api_key=api_key,
            base_url=base_url,
        )
        content = str(resp.get("content", "")).strip()
        # 容错: LLM 可能包了 ```json ... ``` 围栏
        if content.startswith("```"):
            content = content.strip("`")
            if content.startswith("json"):
                content = content[4:]
        return json.loads(content.strip()), str(resp.get("model", ""))

    def _fallback(self, pills: List[Dict[str, Any]], operator: FusionOperator) -> Dict[str, Any]:
        """保底方案: LLM 连续失败时,结构化拼接原料生成一枚保底金丹"""
        names = [str(p.get("name", "")) for p in pills]
        schemas = [p.get("skill_schema") or {} for p in pills]
        merged_dna: Dict[str, Any] = {}
        for s in schemas:
            dna = s.get("expression_dna") or {}
            for k, v in dna.items():
                if v and k not in merged_dna:
                    merged_dna[k] = v
        identity = "、".join(n for n in names if n)
        return {
            "name": f"合丹·{identity[:6]}",
            "description": f"由 {identity} 融合而成(保底方案,未经 LLM 创作)。",
            "skill_schema": {
                "identity_card": f"我是 {identity} 的融合体(保底形态)。",
                "expression_dna": merged_dna or {"sentence_length": "mixed", "formality": 0.5},
                "mental_models": [m for s in schemas for m in (s.get("mental_models") or [])][:5],
                "decision_heuristics": [h for s in schemas for h in (s.get("decision_heuristics") or [])][:5],
                "values": sorted({v for s in schemas for v in (s.get("values") or [])})[:10],
                "anti_patterns": sorted({v for s in schemas for v in (s.get("anti_patterns") or [])})[:10],
                "honest_limits": ["本次融合未调用大模型,产物为保底拼接。"],
                "example_dialogues": [d for s in schemas for d in (s.get("example_dialogues") or [])][:2],
            },
            "operator": {"id": operator.id, "name": operator.name},
            "model": "",
            "degraded": True,
        }

    def fuse(
        self,
        pills: List[Dict[str, Any]],
        model: Optional[str] = None,
        api_key: Optional[str] = None,
        base_url: Optional[str] = None,
        exclude_operator_id: Optional[str] = None,
    ) -> Dict[str, Any]:
        if len(pills) < 2:
            raise ValueError("融合至少需要 2 枚金丹")

        operator = self._pick_operator(exclude_operator_id)
        attempts = 2
        last_error: Optional[Exception] = None
        # 外部 exclude_operator_id 在重试时仍需排除,且各次尝试算子互不相同
        excluded_ids = {operator.id}
        if exclude_operator_id:
            excluded_ids.add(exclude_operator_id)
        for attempt in range(attempts):
            try:
                payload, used_model = self._call_llm(pills, operator, model, api_key, base_url)
                if _validate_payload(payload):
                    return {
                        "name": str(payload["name"]).strip(),
                        "description": str(payload["description"]).strip(),
                        "skill_schema": payload["skill_schema"],
                        "operator": {"id": operator.id, "name": operator.name},
                        "model": used_model,
                        "degraded": False,
                    }
                last_error = ValueError("LLM 返回结构不完整")
            except Exception as e:  # 融合失败走重试/保底,错误记录即可
                last_error = e
                logger.warning("融合第 %d 次尝试失败: %s", attempt + 1, e)
            # 重试: 换一个算子(保留外部排除语义)
            candidates = [op for op in FUSION_OPERATORS if op.id not in excluded_ids]
            operator = random.choice(candidates)
            excluded_ids.add(operator.id)

        logger.error("融合两次尝试均失败,走保底: %s", last_error)
        return self._fallback(pills, operator)
