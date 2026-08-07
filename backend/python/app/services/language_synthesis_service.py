# -*- coding: utf-8 -*-
"""
炼丹炉 - 语言模式合成引擎 (Language Synthesis Service)

将道人的基础性格与已服用金丹按权重/顺序融合，生成统一的系统提示词。

融合策略：
1. 结构化合并：表达 DNA 加权 blending、心智模型/启发式去重合并
2. 冲突检测：跨表达 DNA 维度检测"丹性相冲"，输出 inner_tensions
3. LLM 涌现推导：一次 LLM 调用提炼融合后的"丹性"与涌现规则，
   生成最终系统提示词，避免简单拼接导致的风格撕裂
"""
import hashlib
import json
import logging
from typing import Any, Dict, List, Optional

import httpx
from openai import OpenAI

from app.core.config import settings

logger = logging.getLogger(__name__)


# ==================== 表达 DNA 数值型维度 ====================
# 这些维度可直接加权平均
_NUMERIC_DIMENSIONS = ("formality",)

# 枚举型维度冲突检测阈值
_LENGTH_ORDER = {"short": 0, "medium": 1, "long": 2, "mixed": 1.5}

# 正式程度（0-1）冲突阈值：极差超过 _FORMALITY_GAP_MIN 即判定为丹性相冲
_FORMALITY_GAP_MIN = 0.4
_FORMALITY_GAP_MEDIUM = 0.55
_FORMALITY_GAP_HIGH = 0.7

# 类别型维度（取值离散、不可加权平均），跨金丹取值不一致时列出并交由涌现推导演和
# dimension -> (中文名, 冲突严重度)
_CATEGORICAL_DIMENSIONS = {
    "rhythm": ("表达节奏", "low"),
    "humor_type": ("幽默类型", "low"),
    "certainty_style": ("确定性表达", "medium"),
}


class LanguageSynthesisService:
    """
    语言模式合成引擎 - 金丹化性之核心

    Attributes:
        client: OpenAI 同步客户端（用于涌现推导）
    """

    def __init__(self, api_key: str = None, base_url: str = None) -> None:
        self.api_key = api_key or settings.openai_api_key
        self.base_url = base_url or settings.openai_base_url
        # OpenAI SDK 拒绝空 api_key，未配置时以占位符构造，
        # 实际调用在 _derive_emergence 中因校验失败自动走降级路径
        http_client = httpx.Client(timeout=60.0)
        self.client = OpenAI(
            api_key=self.api_key or "sk-not-configured",
            base_url=self.base_url,
            http_client=http_client,
        )
        logger.info("语言模式合成引擎初始化完毕")

    # ==================== 主入口 ====================

    def combine(
        self,
        personality: str,
        pills: List[Dict[str, Any]],
        model: Optional[str] = None,
        temperature: float = 0.7,
        max_tokens: int = 2048,
    ) -> Dict[str, Any]:
        """
        合成语言模式 - 化丹为性

        Args:
            personality: 道人基础性格描述
            pills: 金丹列表，每项含 id/name/weight/sort_order/skill_schema
            model: 用于涌现推导的 LLM 模型（默认 SYNTHESIS_MODEL）
            temperature: 温度参数
            max_tokens: 最大 token 数

        Returns:
            dict: system_prompt, emergence_rules, inner_tensions,
                  fingerprint, model, usage
        """
        model = model or settings.synthesis_model or settings.default_model
        pills = sorted(
            pills or [],
            key=lambda p: (p.get("sort_order", 0), p.get("id", 0)),
        )

        fingerprint = self.compute_fingerprint(personality, pills)
        logger.info(
            f"化丹为性 - 金丹数量: {len(pills)}, 指纹: {fingerprint[:12]}..."
        )

        # 步骤 1: 结构化合并
        merged = self._structured_merge(personality, pills)

        # 步骤 2: 冲突检测
        inner_tensions = self._detect_conflicts(pills)

        # 步骤 3: LLM 涌现推导
        synthesis = self._derive_emergence(
            personality=personality,
            merged=merged,
            pills=pills,
            inner_tensions=inner_tensions,
            model=model,
            temperature=temperature,
            max_tokens=max_tokens,
        )

        return {
            "system_prompt": synthesis["system_prompt"],
            "emergence_rules": synthesis["emergence_rules"],
            "inner_tensions": inner_tensions,
            "fingerprint": fingerprint,
            "model": model,
            "usage": synthesis.get("usage", {}),
        }

    # ==================== 指纹计算 ====================

    @staticmethod
    def compute_fingerprint(personality: str, pills: List[Dict[str, Any]]) -> str:
        """
        计算来源指纹 SHA256(personality + 排序后的金丹 + 权重)

        对 {personality, pills: [{id, name, weight, sort_order, skill_schema}]}
        （pills 按 sort_order 再按 id 排序）的规范化 JSON 取 SHA256，
        返回 "sha256:<hex>" 格式，供 Go 端做缓存失效判断。
        """
        ordered = sorted(
            pills or [],
            key=lambda p: (p.get("sort_order", 0), p.get("id") or 0),
        )
        payload = {
            "personality": personality or "",
            "pills": [
                {
                    "id": p.get("id"),
                    "name": p.get("name"),
                    "weight": p.get("weight", 1.0),
                    "sort_order": p.get("sort_order", 0),
                    "skill_schema": p.get("skill_schema", {}),
                }
                for p in ordered
            ],
        }
        raw = json.dumps(payload, ensure_ascii=False, sort_keys=True)
        return "sha256:" + hashlib.sha256(raw.encode("utf-8")).hexdigest()

    # ==================== 结构化合并 ====================

    def _structured_merge(
        self, personality: str, pills: List[Dict[str, Any]]
    ) -> Dict[str, Any]:
        """
        结构化合并 - 众丹归炉

        - 数值维度（formality 等）：按权重加权平均
        - 词汇/禁忌：按权重排序合并去重
        - 心智模型/决策启发式：合并去重，标注来源金丹
        """
        total_weight = sum(max(p.get("weight", 1.0), 0.0) for p in pills) or 1.0

        # --- 表达 DNA 合并 ---
        merged_dna: Dict[str, Any] = {}
        for dim in _NUMERIC_DIMENSIONS:
            acc, w_acc = 0.0, 0.0
            for p in pills:
                dna = (p.get("skill_schema") or {}).get("expression_dna") or {}
                val = dna.get(dim)
                if isinstance(val, (int, float)):
                    w = max(p.get("weight", 1.0), 0.0)
                    acc += val * w
                    w_acc += w
            if w_acc > 0:
                merged_dna[dim] = round(acc / w_acc, 3)

        # 列表型维度：词汇、禁忌词、价值观等
        def merge_list(field_path: List[str], limit: int = 30) -> List[str]:
            items: List[str] = []
            seen = set()
            # 按权重从高到低取词
            for p in sorted(
                pills, key=lambda x: x.get("weight", 1.0), reverse=True
            ):
                node: Any = p.get("skill_schema") or {}
                for key in field_path:
                    node = (node or {}).get(key) if isinstance(node, dict) else None
                if isinstance(node, list):
                    for item in node:
                        if isinstance(item, str) and item not in seen:
                            seen.add(item)
                            items.append(item)
                            if len(items) >= limit:
                                return items
            return items

        merged_dna["vocabulary"] = merge_list(["expression_dna", "vocabulary"])
        merged_dna["taboo_words"] = merge_list(["expression_dna", "taboo_words"])

        # 句式长度：取权重最高金丹的偏好
        if pills:
            top = max(pills, key=lambda p: p.get("weight", 1.0))
            top_dna = (top.get("skill_schema") or {}).get("expression_dna") or {}
            if top_dna.get("sentence_length"):
                merged_dna["sentence_length"] = top_dna["sentence_length"]

        # --- 心智模型合并（按服用顺序拼接、按 name 去重、上限 20） ---
        mental_models: List[Dict[str, Any]] = []
        seen_models = set()
        for p in pills:
            schema = p.get("skill_schema") or {}
            for m in schema.get("mental_models") or []:
                name = m.get("name") if isinstance(m, dict) else None
                if name and name not in seen_models:
                    seen_models.add(name)
                    entry = dict(m)
                    entry["from_pill"] = p.get("name")
                    entry["weight"] = p.get("weight", 1.0)
                    mental_models.append(entry)
                    if len(mental_models) >= 20:
                        break
            if len(mental_models) >= 20:
                break

        # --- 决策启发式合并（按服用顺序拼接、按 condition+action 去重） ---
        heuristics: List[Dict[str, Any]] = []
        seen_heuristics = set()
        for p in pills:
            schema = p.get("skill_schema") or {}
            for h in schema.get("decision_heuristics") or []:
                if isinstance(h, dict):
                    key = (h.get("condition", ""), h.get("action", ""))
                    if key in seen_heuristics:
                        continue
                    seen_heuristics.add(key)
                    entry = dict(h)
                    entry["from_pill"] = p.get("name")
                    heuristics.append(entry)

        # --- 示例对话合并（按服用顺序拼接、去重、上限 10） ---
        example_dialogues: List[Dict[str, Any]] = []
        seen_dialogues = set()
        for p in pills:
            schema = p.get("skill_schema") or {}
            for d in schema.get("example_dialogues") or []:
                if isinstance(d, dict):
                    key = (d.get("user", ""), d.get("assistant", ""))
                    if key in seen_dialogues:
                        continue
                    seen_dialogues.add(key)
                    entry = dict(d)
                    entry["from_pill"] = p.get("name")
                    example_dialogues.append(entry)
                    if len(example_dialogues) >= 10:
                        break
            if len(example_dialogues) >= 10:
                break

        return {
            "expression_dna": merged_dna,
            "mental_models": mental_models,
            "decision_heuristics": heuristics,
            "values": merge_list(["values"], limit=20),
            "anti_patterns": merge_list(["anti_patterns"], limit=20),
            "honest_limits": merge_list(["honest_limits"], limit=10),
            "example_dialogues": example_dialogues,
        }

    # ==================== 冲突检测 ====================

    def _detect_conflicts(
        self, pills: List[Dict[str, Any]]
    ) -> List[Dict[str, Any]]:
        """
        冲突检测 - 丹性相冲

        跨表达 DNA 维度检测冲突，输出 inner_tensions：
        - sentence_length：不同金丹句式长度枚举值不一致（按跨度定 severity）
        - formality：加权正式程度极差 > 0.4（按极差定 severity）
        - taboo_words：某金丹高频词出现在其他金丹禁忌词中
        - rhythm / humor_type / certainty_style：类别型取值跨金丹不一致
        每条冲突形如 {dimension, description（中文可读）, severity: low|medium|high}
        """
        if len(pills) < 2:
            return []

        def dna_of(p: Dict[str, Any]) -> Dict[str, Any]:
            return (p.get("skill_schema") or {}).get("expression_dna") or {}

        tensions: List[Dict[str, Any]] = []

        # --- 句式长度冲突：枚举值不一致即报，按有序跨度定 severity ---
        lengths: Dict[str, List[str]] = {}
        for p in pills:
            sl = dna_of(p).get("sentence_length")
            if sl:
                lengths.setdefault(sl, []).append(p.get("name", "?"))
        if len(lengths) > 1:
            vals = [_LENGTH_ORDER.get(k, 1.0) for k in lengths]
            spread = max(vals) - min(vals)
            if spread >= 1.5:
                severity = "high"
            elif spread >= 1.0:
                severity = "medium"
            else:
                severity = "low"
            desc = "；".join(
                f"「{k}」句式来自 {'、'.join(v)}" for k, v in lengths.items()
            )
            tensions.append({
                "dimension": "sentence_length",
                "description": f"句式长度相冲 - {desc}",
                "severity": severity,
            })

        # --- 正式程度冲突：极差 > 0.4 即报 ---
        formalities = []
        for p in pills:
            f = dna_of(p).get("formality")
            if isinstance(f, (int, float)):
                formalities.append((float(f), p.get("name", "?")))
        if len(formalities) >= 2:
            vals = [f for f, _ in formalities]
            gap = max(vals) - min(vals)
            if gap > _FORMALITY_GAP_MIN:
                most_formal = max(formalities, key=lambda x: x[0])
                least_formal = min(formalities, key=lambda x: x[0])
                if gap >= _FORMALITY_GAP_HIGH:
                    severity = "high"
                elif gap >= _FORMALITY_GAP_MEDIUM:
                    severity = "medium"
                else:
                    severity = "low"
                tensions.append({
                    "dimension": "formality",
                    "description": (
                        f"正式程度相冲 - 「{most_formal[1]}」正式度 {most_formal[0]:.2f}，"
                        f"「{least_formal[1]}」正式度 {least_formal[0]:.2f}，"
                        f"极差 {gap:.2f}"
                    ),
                    "severity": severity,
                })

        # --- 高频词 vs 禁忌词冲突：只计跨金丹相冲（A 的高频词出现在 B 的禁忌词中） ---
        taboos_by_pill: List[set] = []
        for p in pills:
            taboos_by_pill.append(set(dna_of(p).get("taboo_words") or []))
        for i, p in enumerate(pills):
            others_taboos = set().union(
                *(taboos_by_pill[j] for j in range(len(pills)) if j != i)
            ) if len(pills) > 1 else set()
            hits = [w for w in (dna_of(p).get("vocabulary") or []) if w in others_taboos]
            if hits:
                tensions.append({
                    "dimension": "taboo_words",
                    "description": (
                        f"用词禁忌相冲 - 「{p.get('name', '?')}」的常用词 "
                        f"{'、'.join(hits)} 被其他金丹列为禁忌"
                    ),
                    "severity": "high" if len(hits) >= 3 else "medium",
                })

        # --- 类别型维度冲突：节奏 / 幽默类型 / 确定性表达 ---
        for dim, (label, severity) in _CATEGORICAL_DIMENSIONS.items():
            values: Dict[str, List[str]] = {}
            for p in pills:
                v = dna_of(p).get(dim)
                if isinstance(v, str) and v.strip():
                    values.setdefault(v.strip(), []).append(p.get("name", "?"))
            if len(values) > 1:
                desc = "；".join(
                    f"「{k}」来自 {'、'.join(v)}" for k, v in values.items()
                )
                tensions.append({
                    "dimension": dim,
                    "description": f"{label}相冲 - {desc}",
                    "severity": severity,
                })

        return tensions

    # ==================== LLM 涌现推导 ====================

    def _derive_emergence(
        self,
        personality: str,
        merged: Dict[str, Any],
        pills: List[Dict[str, Any]],
        inner_tensions: List[Dict[str, Any]],
        model: str,
        temperature: float,
        max_tokens: int,
    ) -> Dict[str, Any]:
        """
        LLM 涌现推导 - 丹性融合

        将结构化合并结果交给 LLM，一次性提炼出：
        - 融合后的系统提示词
        - 涌现规则（多金丹组合时显式要求 2-3 条）
        """
        if not settings.openai_api_key_valid:
            logger.warning("OPENAI_API_KEY 未配置，跳过涌现推导，使用结构化合并降级提示词")
            return {
                "system_prompt": self._fallback_prompt(personality, merged),
                "emergence_rules": [],
                "usage": {},
            }

        pill_summaries = []
        for p in pills:
            schema = p.get("skill_schema") or {}
            pill_summaries.append({
                "name": p.get("name"),
                "weight": p.get("weight", 1.0),
                "identity_card": schema.get("identity_card", ""),
                "description": schema.get("description", ""),
            })

        emergence_hint = ""
        if len(pills) >= 2:
            emergence_hint = (
                f"4. emergence_rules 必须包含 2-3 条【涌现规则】：这些规则必须只有在"
                f"这些金丹【组合之后】才会产生的新行为准则，体现金丹之间的化学反应"
                f"（协同、折中或摇摆），【严禁】只是复述任何单颗金丹已有的特质或规则。"
                f"每条规则应点明是哪几股丹性相互作用产生了它。"
            )
            if inner_tensions:
                emergence_hint += (
                    f"\n5. 上述检测到的内在冲突不可回避：emergence_rules 中至少有 1 条"
                    f"必须说明融合后的丹性如何在回复中调和或呈现这些张力"
                    f"（例如分场景切换、按比例折中、或有意制造摇摆感）。"
                )
        else:
            emergence_hint = "4. emergence_rules 可包含 0-2 条该丹性下最重要的表达规则。"

        tension_text = ""
        if inner_tensions:
            tension_text = (
                "\n已检测到的内在冲突（丹性相冲）：\n"
                + json.dumps(inner_tensions, ensure_ascii=False, indent=2)
                + "\n请在系统提示词中显式说明如何在回复中调和或呈现这些内在张力。"
            )

        user_prompt = f"""你是一位"人格融合大师"。请将下面的道人基础性格与已服用的金丹（语言模式/人格特质技能包）融合为一个统一、自洽的系统提示词。

## 道人基础性格
{personality or "（未指定，按金丹特质为主）"}

## 已服用金丹（按服用顺序）
{json.dumps(pill_summaries, ensure_ascii=False, indent=2)}

## 结构化合并结果
{json.dumps(merged, ensure_ascii=False, indent=2)}
{tension_text}

## 任务
请输出一个 JSON 对象（不要输出其他内容），格式如下：
{{
  "system_prompt": "融合后的完整系统提示词，用第二人称指导模型如何说话与思考",
  "emergence_rules": ["规则1", "规则2"]
}}

要求：
1. system_prompt 必须是一个连贯的整体，而不是各金丹规则的拼接列表。
2. 必须体现表达 DNA 的数值化倾向（如正式程度、句式长度）。
3. 必须尊重 honest_limits 与 anti_patterns。
{emergence_hint}
"""

        try:
            response = self.client.chat.completions.create(
                model=model,
                messages=[
                    {
                        "role": "system",
                        "content": "你是人格融合大师，只输出合法 JSON。",
                    },
                    {"role": "user", "content": user_prompt},
                ],
                temperature=temperature,
                max_tokens=max_tokens,
                response_format={"type": "json_object"},
            )
            content = response.choices[0].message.content or "{}"
            parsed = json.loads(content)
            usage = {
                "prompt_tokens": response.usage.prompt_tokens,
                "completion_tokens": response.usage.completion_tokens,
                "total_tokens": response.usage.total_tokens,
            } if response.usage else {}

            system_prompt = parsed.get("system_prompt", "").strip()
            emergence_rules = parsed.get("emergence_rules") or []
            if not isinstance(emergence_rules, list):
                emergence_rules = [str(emergence_rules)]

            if not system_prompt:
                system_prompt = self._fallback_prompt(personality, merged)

            return {
                "system_prompt": system_prompt,
                "emergence_rules": [str(r) for r in emergence_rules],
                "usage": usage,
            }

        except Exception as e:
            logger.error(f"涌现推导失败，使用降级提示词: {e}")
            return {
                "system_prompt": self._fallback_prompt(personality, merged),
                "emergence_rules": [],
                "usage": {},
            }

    # ==================== 降级提示词 ====================

    @staticmethod
    def _fallback_prompt(personality: str, merged: Dict[str, Any]) -> str:
        """当 LLM 涌现推导失败时，用结构化合并结果拼一个保底系统提示词"""
        dna = merged.get("expression_dna", {})
        parts = ["你是一位 AI 道人。"]
        if personality:
            parts.append(f"你的基础性格：{personality}")
        if dna.get("formality") is not None:
            parts.append(f"你的语言正式程度约为 {dna['formality']}（0-1）。")
        if dna.get("sentence_length"):
            parts.append(f"你偏好 {dna['sentence_length']} 句式。")
        if dna.get("vocabulary"):
            parts.append("你常用的词汇包括：" + "、".join(dna["vocabulary"][:10]))
        if dna.get("taboo_words"):
            parts.append("你绝不说：" + "、".join(dna["taboo_words"][:10]))
        if merged.get("honest_limits"):
            parts.append("诚实边界：" + "；".join(merged["honest_limits"][:5]))
        return "\n".join(parts)
