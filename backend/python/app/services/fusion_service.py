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
import re
from typing import Any, Dict, List, Optional, Tuple

from app.services.fusion_operators import (
    FUSION_OPERATORS, FusionOperator, sample_operator,
)

logger = logging.getLogger(__name__)

_REQUIRED_SCHEMA_SECTIONS = [
    "identity_card", "expression_dna", "mental_models", "decision_heuristics",
    "values", "anti_patterns", "honest_limits", "example_dialogues",
]

_SYSTEM_PROMPT = (
    "你是炼丹炉的金丹融合大师。"
    "严格只输出合法 JSON,不要任何解释、注释、markdown 代码块包裹或多余文字。"
    "JSON 必须是可被 json.loads 直接解析的完整对象。"
)

# ─────────────────────────────────────────────────────────────
# 提示词压缩:全量 skill_schema 太大,多枚时易超 8K 上下文
# 只保留融合所需的「人格本质」: identity_card + 表达 DNA 摘要 + 关键心智模型
# 这能让 5 枚金丹的输入从 ~7500 tokens 压到 ~1500 tokens
# ─────────────────────────────────────────────────────────────
_PILL_ESSENCE_FIELDS = ("identity_card",)
_DNA_SUMMARY_FIELDS = (
    "sentence_length", "formality", "rhythm", "humor_type",
    "certainty_style", "citation_habit",
)
_MODEL_SUMMARY_FIELDS = ("name", "one_liner")
_DNA_SAFE_LIST_LIMIT = 8


def _summarize_dna(dna: Dict[str, Any]) -> Dict[str, Any]:
    """保留表达 DNA 的关键维度;vocabulary/taboo_words 仅取前 N 项"""
    summary: Dict[str, Any] = {}
    for k in _DNA_SUMMARY_FIELDS:
        v = dna.get(k)
        if v not in (None, "", []):
            summary[k] = v
    vocab = dna.get("vocabulary") or []
    taboo = dna.get("taboo_words") or []
    if vocab:
        summary["vocabulary"] = list(vocab)[:_DNA_SAFE_LIST_LIMIT]
    if taboo:
        summary["taboo_words"] = list(taboo)[:_DNA_SAFE_LIST_LIMIT]
    return summary


def _summarize_models(models: List[Dict[str, Any]]) -> List[Dict[str, Any]]:
    """心智模型只留 name + one_liner,前 3 条"""
    out: List[Dict[str, Any]] = []
    for m in models[:3]:
        if not isinstance(m, dict):
            continue
        item = {k: m.get(k) for k in _MODEL_SUMMARY_FIELDS if m.get(k)}
        if item:
            out.append(item)
    return out


def _pill_essence(pill: Dict[str, Any]) -> Dict[str, Any]:
    """从单枚金丹抽出融合所需的人格本质字段"""
    schema = pill.get("skill_schema") or {}
    if not isinstance(schema, dict):
        schema = {}
    essence: Dict[str, Any] = {}
    for k in _PILL_ESSENCE_FIELDS:
        v = schema.get(k)
        if v:
            essence[k] = v
    dna = schema.get("expression_dna")
    if isinstance(dna, dict) and dna:
        dna_summary = _summarize_dna(dna)
        if dna_summary:
            essence["expression_dna"] = dna_summary
    models = schema.get("mental_models")
    if isinstance(models, list) and models:
        essence["mental_models"] = _summarize_models(models)
    return essence


# 兜底:LLM 偶尔仍会用 ```json ... ``` 围栏,统一剥离
_FENCE_RE = re.compile(r"^```(?:json)?\s*\n?(.*?)\n?```\s*$", re.DOTALL | re.IGNORECASE)


def _parse_llm_json(content: str) -> Dict[str, Any]:
    """
    多策略解析 LLM 返回的 JSON,容忍围栏、杂质前缀/后缀、不可见字符

    顺序: 直接 loads → 剥离 ``` 围栏 → 取首个 { 到末个 } 子串 → 报错
    """
    if content is None:
        raise ValueError("LLM 返回为空")
    text = str(content).strip()
    if not text:
        raise ValueError("LLM 返回为空")

    # 1) 直接解析(绝大多数 response_format=json_object 时命中此分支)
    try:
        return json.loads(text)
    except json.JSONDecodeError:
        pass

    # 2) 剥离 markdown 围栏(```json ... ``` 或 ``` ... ```)
    m = _FENCE_RE.match(text)
    if m:
        try:
            return json.loads(m.group(1).strip())
        except json.JSONDecodeError:
            pass
    # 兜底手剥:首行是 ```/```json,末行是 ```
    if text.startswith("```"):
        lines = text.split("\n")
        if lines and lines[-1].strip().startswith("```"):
            lines = lines[:-1]
        # 跳过首行围栏标记
        inner = "\n".join(lines[1:]).strip() if lines[0].strip().startswith("```") else "\n".join(lines).strip()
        try:
            return json.loads(inner)
        except json.JSONDecodeError:
            pass

    # 3) 取首个 { 到末个 } 的子串再解析(处理前后杂质)
    start = text.find("{")
    end = text.rfind("}")
    if start != -1 and end > start:
        candidate = text[start:end + 1]
        try:
            return json.loads(candidate)
        except json.JSONDecodeError:
            pass

    raise ValueError(
        f"无法从 LLM 输出解析 JSON(长度={len(text)}): {text[:200]!r}"
    )

_TASK_TEMPLATE = """## 融合算子
{op_name}: {op_instruction}

## 原料金丹 (共 {n} 枚,仅含人格本质字段)
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

硬性约束(违反任何一条都视为输出失败):
- 严格输出合法 JSON,不得用 ``` 围栏包裹,不得附加任何解释
- 顶层必须是 {{ 开始、}} 结束的对象,字段名双引号
- example_dialogues 至少 2 条;mental_models / decision_heuristics 各至少 1 条
- 所有文本用中文,字段值内不可包含未配对的引号或换行干扰 JSON 闭合
- 控制总长度在 2500 字以内,避免末尾被截断导致 JSON 不闭合"""


def _build_pills_block(pills: List[Dict[str, Any]]) -> str:
    """仅输出融合所需的人格本质字段,避免 prompt 撑爆 8K 上下文"""
    parts = []
    for i, p in enumerate(pills, 1):
        essence = _pill_essence(p)
        parts.append(
            f"### 金丹 {i}: {p.get('name', '')}\n"
            f"描述: {p.get('description', '')}\n"
            f"人格本质: {json.dumps(essence, ensure_ascii=False)}"
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
        # 不强制 response_format=json_object:实测 deepseek-v4-flash 不支持该模式,
        # 传了会直接返回空 content。改靠「强 prompt + 多策略解析」保证 JSON 合规。
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
        # 暴露 LLM 原始 content 长度,便于事后诊断「返回为空/截断/被围栏」类问题
        logger.info(
            "融合 LLM 响应: model=%s, content_len=%d",
            resp.get("model", ""), len(content),
        )
        payload = _parse_llm_json(content)
        return payload, str(resp.get("model", ""))

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
