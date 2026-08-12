# -*- coding: utf-8 -*-
"""
Demo Provider(演示模式) - 007-demo-mode

不调用任何 LLM;按 user prompt 关键字从 mock 池中挑一条回答,
30ms/chunk 异步 yield,1-2 字/分片,模拟真实流式体感。

SSE 输出格式与 RealChatProvider 保持完全一致:
    data: {"content": "..."}\n\n
    data: {"content": "..."}\n\n
    ...
    data: [DONE]\n\n
"""

import asyncio
import hashlib
import json
import logging
import re
import time
from typing import Any, AsyncGenerator, Dict, List, Optional

from app.services.providers.base import ChatProvider, FusionProvider, SynthesisProvider

logger = logging.getLogger(__name__)


# ==================== Chat Mock Pool ====================
# 按 user 消息关键词分组,匹配优先级:精确 > 模糊;无匹配走 generic
CHAT_MOCK_POOL: List[Dict[str, str]] = [
    {
        "match": "你好|hello|hi",
        "reply": (
            "道友安好,小道这厢有礼了。\n\n"
            "炼丹炉此刻已点燃,丹火温和而绵长。"
            "无论你是初入道途还是久炼成精,这里都为你敞开。"
            "请随我观炉中之景,我们一同体悟金丹化性之道。\n\n"
            "——守炉道人 敬上"
        ),
    },
    {
        "match": "道|丹|练|炼|炉",
        "reply": (
            "道可道,非常道;名可名,非常名。\n\n"
            "金丹化性之术,以「丹」为载体,以「性」为归宿。"
            "一丹一性,服用之后,丹方融入道人身心,语言模式自然生变。\n\n"
            "欲知更多,可问: 何为金丹? 何为道人? 化丹为性又作何解?"
        ),
    },
    {
        "match": "药|医|病|治",
        "reply": (
            "药医不死病,佛度有缘人。\n\n"
            "今人身心多郁,皆因欲念不调。"
            "本炉虽炼外丹,亦兼修内丹——性理平和,则气血自顺。\n\n"
            "若你正被某事困扰,可试问: 「何事让你寝食难安?」"
            "我们一起观其根源,寻其化解。"
        ),
    },
    {
        "match": "命|运|八字|算",
        "reply": (
            "命由我作,福自己求。\n\n"
            "八字命理虽有数,而修道之人贵在转命。"
            "金丹一服,性相一转,虽处困顿亦可逢生。\n\n"
            "你若想测一字,可报出你生辰的属相;若想求一签,可告诉我心中挂念之事。"
        ),
    },
    {
        "match": "佛|禅|经",
        "reply": (
            "佛说一切法,为度一切心;若无一切心,亦无一切法。\n\n"
            "金丹化性之术,本与佛家明心见性相通。"
            "我们不立宗门,但求一心澄澈,炉中自见真如。"
        ),
    },
    {
        "match": "推荐|建议|怎么",
        "reply": (
            "依小道之见,可分三步: 一曰观心,二曰试丹,三曰合炉。\n\n"
            "· 观心: 静坐片刻,问自己当下最想要什么。\n"
            "· 试丹: 从炉中取一二金丹,先观其名,再观其性。\n"
            "· 合炉: 与道人坐而论道,让其性合你的心。\n\n"
            "不必急,炉火常燃,你有的是时间。"
        ),
    },
    {
        "match": "戒|律|规",
        "reply": (
            "戒者,戒其心之放逸也。\n\n"
            "本炉有四戒: 戒贪、戒嗔、戒痴、戒我慢。"
            "贪则失本,嗔则伤气,痴则迷途,慢则招愆。\n\n"
            "然戒非束缚,乃守护。"
            "心如明镜,不染尘埃,则万象自显。"
        ),
    },
    {
        "match": "气|血|身",
        "reply": (
            "气血者,人之根本;一息尚存,道火不灭。\n\n"
            "今人多坐少动,气血多滞,金丹虽好,亦需身器配合。"
            "建议你: 晨起散步一刻,午后小憩一盏,夜间静坐一炷香。"
            "配合金丹化性,内外兼修,方见长效。"
        ),
    },
    {
        "match": "灵|魂|神",
        "reply": (
            "灵者不昧,魂者不散,神者不迷。\n\n"
            "三者本一,名相有三,皆指心性之妙用。"
            "金丹一服,灵台自清,识神自朗,魂魄自安。\n\n"
            "道友若有此感,正合本炉之化性之功。"
        ),
    },
    {
        "match": ".*",  # 兜底:任意匹配
        "reply": (
            "道友所问甚好,小道愿详说一二。\n\n"
            "本炉以「金丹化性」为宗,意在以丹为媒、以性为归。"
            "每一枚金丹都内含一套结构化语言模式,道人服用后,其应答风格、思维倾向、价值偏好皆随之微调。\n\n"
            "若你有具体情境,不妨描述一二;若仅是闲谈,小道亦可陪你观炉、论道、说古。"
            "本炉常燃,道友随时可来。"
        ),
    },
]


# ==================== DemoChatProvider ====================


class DemoChatProvider:
    """演示模式对话 Provider:不调 LLM,按 mock 池流式产出"""

    CHUNK_INTERVAL_MS = 30
    CHARS_PER_CHUNK_MIN = 1
    CHARS_PER_CHUNK_MAX = 2

    def _pick_reply(self, messages: List[Dict[str, str]]) -> str:
        last_user = ""
        for m in reversed(messages):
            if m.get("role") == "user":
                last_user = m.get("content", "")
                break

        for entry in CHAT_MOCK_POOL:
            if re.search(entry["match"], last_user, flags=re.IGNORECASE):
                return entry["reply"]
        return CHAT_MOCK_POOL[-1]["reply"]

    def chat_completion(
        self,
        messages: List[Dict[str, str]],
        model: Optional[str] = None,
        temperature: float = 0.7,
        max_tokens: int = 4096,
        api_key: Optional[str] = None,
        base_url: Optional[str] = None,
    ) -> Dict[str, Any]:
        reply = self._pick_reply(messages)
        return {
            "content": reply,
            "model": model or "demo-mock",
            "usage": {
                "prompt_tokens": sum(len(m.get("content", "")) for m in messages),
                "completion_tokens": len(reply),
                "total_tokens": sum(len(m.get("content", "")) for m in messages) + len(reply),
            },
        }

    async def chat_completion_stream(
        self,
        messages: List[Dict[str, str]],
        model: Optional[str] = None,
        temperature: float = 0.7,
        max_tokens: int = 4096,
        api_key: Optional[str] = None,
        base_url: Optional[str] = None,
    ) -> AsyncGenerator[str, None]:
        reply = self._pick_reply(messages)
        start = time.monotonic()
        for ch in reply:
            await asyncio.sleep(self.CHUNK_INTERVAL_MS / 1000.0)
            data = json.dumps({"content": ch}, ensure_ascii=False)
            yield f"data: {data}\n\n"
        # 末帧:与 RealChatProvider 一致地以 [DONE] 收尾
        yield "data: [DONE]\n\n"
        logger.info(
            "demo stream 完毕 - chars=%d duration_ms=%d",
            len(reply),
            int((time.monotonic() - start) * 1000),
        )


# ==================== DemoSynthesisProvider ====================


class DemoSynthesisProvider:
    """演示模式合成 Provider:固定 prompt 串 + 100ms 模拟延迟,无 LLM"""

    def synthesize(
        self,
        personality: str,
        pills: List[Any],
    ) -> Dict[str, Any]:
        return self.combine(personality, pills)

    def combine(
        self,
        personality: str,
        pills: List[Any],
        model: Optional[str] = None,
        temperature: float = 0.7,
        max_tokens: int = 4096,
        api_key: Optional[str] = None,
        base_url: Optional[str] = None,
    ) -> Dict[str, Any]:
        """演示合成: 返回 CombineResponse 兼容结构(system_prompt/emergence_rules/inner_tensions/fingerprint)"""
        # 100ms 同步 sleep 模拟"LLM 合成中"延迟
        time.sleep(0.1)
        names = []
        for p in pills or []:
            name = getattr(p, "name", None) or (p.get("name") if isinstance(p, dict) else None)
            if name:
                names.append(str(name))
        joined = "、".join(names) if names else "(无)"
        prompt = (
            f"【演示模式 · 合成】\n"
            f"你是一位融合了以下金丹特性的道人:{joined}\n"
            f"基础性格: {personality or '(空)'}\n"
            f"请以温和、从容、富含古意的语气回答来客问询。\n"
        )
        fp = hashlib.sha256(prompt.encode("utf-8")).hexdigest()
        return {
            "system_prompt": prompt,
            "emergence_rules": [
                "语调温和,不疾不徐",
                "善用譬喻与典故",
                "遇不明之处坦诚相告",
            ],
            "inner_tensions": [],
            "fingerprint": fp,
            "model": model or "demo-mock",
            "usage": {},
        }


# ==================== DemoFusionProvider ====================


class DemoFusionProvider:
    """演示模式融合 Provider: 固定产物 + 100ms 延迟,无 LLM"""

    def fuse(
        self,
        pills: List[Any],
        model: Optional[str] = None,
        api_key: Optional[str] = None,
        base_url: Optional[str] = None,
        exclude_operator_id: Optional[str] = None,
    ) -> Dict[str, Any]:
        import time
        time.sleep(0.1)
        names = []
        for p in pills or []:
            name = getattr(p, "name", None) or (p.get("name") if isinstance(p, dict) else None)
            if name:
                names.append(str(name))
        joined = "、".join(names) if names else "(无)"
        return {
            "name": "演示合丹",
            "description": f"【演示模式】由 {joined} 融合而成的展示用金丹。",
            "skill_schema": {
                "identity_card": f"我是 {joined} 的演示融合体。",
                "expression_dna": {"sentence_length": "mixed", "formality": 0.5,
                                   "vocabulary": [], "taboo_words": [],
                                   "rhythm": "", "humor_type": "",
                                   "certainty_style": "", "citation_habit": ""},
                "mental_models": [], "decision_heuristics": [], "values": [],
                "anti_patterns": [], "honest_limits": ["演示模式产物,未经 LLM 创作"],
                "example_dialogues": [
                    {"user": "你好", "assistant": "此乃演示合丹,真实模式下方显真身。"},
                    {"user": "你会什么", "assistant": "演示而已,博君一笑。"},
                ],
            },
            "operator": {"id": "hyperbole", "name": "夸张突变"},
            "model": "demo-mock",
            "degraded": False,
        }
