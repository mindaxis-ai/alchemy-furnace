"""导演预算是纯函数：由意图和用户明确约束决定回答规模。"""

import inspect

import pytest

from app.orchestration.contracts import SemanticUnderstanding
from app.orchestration.director import build_response_budget, per_speaker_budget


def understanding(intent, **updates):
    base = {
        "source": "model",
        "intent": intent,
        "core_request": "测试需求",
        "emotion": "neutral",
        "complexity": "simple",
        "detail_preference": "normal",
        "requested_chars": None,
        "format_preference": "plain",
        "wants_advice": False,
        "wants_follow_up": False,
        "should_clarify": False,
        "avoid_behaviors": [],
    }
    return SemanticUnderstanding.model_validate({**base, **updates})


@pytest.mark.parametrize(
    ("intent", "expected"),
    [
        ("casual", (40, 120, 2, 128, False, 1)),
        ("vent", (60, 120, 2, 128, False, 1)),
        ("factual", (120, 220, 3, 256, False, 1)),
        ("advice", (180, 320, 4, 384, True, 1)),
        ("task", (320, 800, 8, 768, True, 2)),
        ("deep_dive", (600, 1600, 12, 1400, True, 2)),
    ],
)
def test_intent_budget_matrix(intent, expected):
    got = build_response_budget(understanding(intent), "原始问题", "group")
    assert (
        got.target_chars,
        got.max_chars,
        got.max_sentences,
        got.max_tokens,
        got.allow_list,
        got.max_speakers,
    ) == expected


def test_explicit_brief_request_caps_length_and_sentences():
    got = build_response_budget(
        understanding("deep_dive", detail_preference="detailed"),
        "简短说，只说结论，别展开",
        "single",
    )
    assert got.target_chars <= 120
    assert got.max_chars == 120
    assert got.max_sentences == 2


def test_explicit_character_request_controls_target_and_hard_maximum():
    got = build_response_budget(
        understanding("task", requested_chars=800, detail_preference="detailed"),
        "请写800字",
        "single",
    )
    assert got.target_chars == 800
    assert got.max_chars == 960
    assert got.max_tokens == 1600


def test_clarification_is_one_short_question():
    got = build_response_budget(
        understanding("task", should_clarify=True), "帮我弄一下", "single"
    )
    assert (got.target_chars, got.max_chars, got.max_sentences) == (60, 120, 1)
    assert got.allow_follow_up is True
    assert got.allow_list is False


def test_code_artifact_uses_tokens_without_character_cut():
    got = build_response_budget(
        understanding("task", format_preference="code"), "写一段 Python 代码", "single"
    )
    assert got.max_chars == 0
    assert got.max_sentences == 0
    assert got.allow_list is True
    assert got.max_tokens == 768


def test_every_person_one_sentence_does_not_rewrite_speaker_budget():
    got = build_response_budget(
        understanding("task"), "请让群里每人一句", "group"
    )
    assert got.max_sentences == 1
    assert got.max_speakers == 2


def test_director_has_no_personality_or_proactivity_input():
    assert list(inspect.signature(build_response_budget).parameters) == [
        "understanding", "user_text", "session_type"
    ]


def test_group_budget_is_split_across_selected_speakers():
    total = build_response_budget(understanding("task"), "请两个人回答", "group")

    each = per_speaker_budget(total, 2)

    assert each.target_chars == 160
    assert each.max_chars == 400
    assert each.max_sentences == 4
    assert each.max_tokens == 384


def test_code_budget_remains_unbounded_by_characters_when_split():
    total = build_response_budget(
        understanding("task", format_preference="code"), "请两个人写代码", "group"
    )

    each = per_speaker_budget(total, 2)

    assert each.max_chars == 0
    assert each.max_sentences == 0
