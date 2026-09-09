"""确定性回复导演：把语义意图换成可执行的字数、句数与发言人数预算。"""

from __future__ import annotations

from dataclasses import dataclass

from app.orchestration.contracts import ResponseBudget, SemanticUnderstanding


@dataclass(frozen=True)
class IntentDefaults:
    target_chars: int
    max_chars: int
    max_sentences: int
    max_tokens: int
    allow_list: bool
    max_speakers: int


_DEFAULTS = {
    "casual": IntentDefaults(40, 120, 2, 128, False, 1),
    "vent": IntentDefaults(60, 120, 2, 128, False, 1),
    "factual": IntentDefaults(120, 220, 3, 256, False, 1),
    "advice": IntentDefaults(180, 320, 4, 384, True, 1),
    "task": IntentDefaults(320, 800, 8, 768, True, 2),
    "deep_dive": IntentDefaults(600, 1600, 12, 1400, True, 2),
}


def build_response_budget(
    understanding: SemanticUnderstanding,
    user_text: str,
    session_type: str,
) -> ResponseBudget:
    """按固定优先级生成预算；自由文本摘要从不参与数值计算。"""

    defaults = _DEFAULTS[understanding.intent]
    target = defaults.target_chars
    max_chars = defaults.max_chars
    max_sentences = defaults.max_sentences
    max_tokens = defaults.max_tokens
    allow_list = defaults.allow_list
    max_speakers = defaults.max_speakers if session_type == "group" else 1
    allow_follow_up = (
        understanding.wants_follow_up
        and "follow_up" not in understanding.avoid_behaviors
    )

    if understanding.detail_preference == "detailed":
        expanded = _DEFAULTS["deep_dive"]
        target = max(target, expanded.target_chars)
        max_chars = max(max_chars, expanded.max_chars)
        max_sentences = max(max_sentences, expanded.max_sentences)
        max_tokens = max(max_tokens, expanded.max_tokens)
        allow_list = True
        if session_type == "group":
            max_speakers = max(max_speakers, expanded.max_speakers)

    normalized = user_text.casefold()
    brief = any(
        signal in normalized
        for signal in ("简短", "一句话", "一句", "只说结论", "别展开", "不要展开")
    )
    if understanding.detail_preference == "brief" or brief:
        target = min(target, 80)
        max_chars = 120
        max_sentences = 2
        max_tokens = min(max_tokens, 128)
        allow_list = False

    if understanding.requested_chars is not None:
        requested = understanding.requested_chars
        target = requested
        max_chars = min(8000, requested + requested // 5)
        max_tokens = min(4096, max(defaults.max_tokens, requested * 2))

    if "每人一句" in normalized:
        max_sentences = 1

    if understanding.format_preference == "code":
        max_chars = 0
        max_sentences = 0
        allow_list = True

    if understanding.should_clarify:
        target = 60
        max_chars = 120
        max_sentences = 1
        max_tokens = 128
        allow_list = False
        allow_follow_up = True
        max_speakers = 1

    return ResponseBudget(
        target_chars=target,
        max_chars=max_chars,
        max_sentences=max_sentences,
        max_tokens=max_tokens,
        allow_list=allow_list,
        allow_follow_up=allow_follow_up,
        max_speakers=max_speakers,
    )
