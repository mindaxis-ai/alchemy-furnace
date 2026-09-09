"""Humanizer：只改表达习惯，保留事实、人物声音与完整结构。"""

import json

import pytest

from app.orchestration.contracts import (
    AgentSnapshot,
    ModelRef,
    ResponseBudget,
    SemanticUnderstanding,
)
from app.orchestration.humanizer import (
    build_humanizer_messages,
    constrain_to_budget,
    validate_humanized,
)


def person():
    return AgentSnapshot(
        agent_id="person",
        name="鲁迅",
        system_prompt=(
            "【身份与性格】姓名：鲁迅。冷峻克制。\n"
            "【炼丹炉中的既定记录】已服用金丹：文学批评丹。用户询问时如实回答。"
        ),
        model_ref=ModelRef(provider_type="deepseek", name="person-model"),
        example_dialogues=[{"user": "沉默是什么？", "assistant": "沉默呵，沉默罢了。"}],
    )


def understanding():
    return SemanticUnderstanding(
        source="model",
        intent="casual",
        core_request="回答用户关于服丹记录的问题",
        emotion="neutral",
        complexity="simple",
        detail_preference="brief",
        requested_chars=None,
        format_preference="plain",
        wants_advice=False,
        wants_follow_up=False,
        should_clarify=False,
        avoid_behaviors=["lecture", "summary", "list", "follow_up"],
    )


def budget(**updates):
    return ResponseBudget.model_validate(
        {
            "target_chars": 40,
            "max_chars": 120,
            "max_sentences": 2,
            "max_tokens": 128,
            "allow_list": False,
            "allow_follow_up": False,
            "max_speakers": 1,
            **updates,
        }
    )


def test_humanizer_prompt_removes_ai_habits_and_preserves_voice_and_facts():
    messages = build_humanizer_messages(
        person(),
        understanding(),
        budget(),
        "你吃过什么丹？",
        "当然可以。我的记录是文学批评丹，详情见 https://example.test，编号 2026。",
    )
    system = str(messages[0].content)
    for required in [
        "舞台式开场", "不只是 X，而是 Y", "重复总结", "机械三段式",
        "装饰性标题", "客服式收尾", "事实", "名字", "数字", "URL", "代码",
        "引用", "不确定性", "人物的词汇、节奏、幽默和立场",
        "示例对白中的有意表达优先",
    ]:
        assert required in system
    assert "用户询问或原稿已经提及时" in system
    assert "不得自行添加古风、道教、修仙或炼丹措辞" in system
    envelope = json.loads(str(messages[1].content))
    assert envelope["user_query"] == "你吃过什么丹？"
    assert envelope["draft"].startswith("当然可以")
    assert envelope["persona"]["examples"][0]["assistant"] == "沉默呵，沉默罢了。"


@pytest.mark.parametrize(
    ("draft", "final", "reason"),
    [
        ("原稿", "", "empty"),
        ("短稿", "长" * 121, "length"),
        ("一。二。三。", "一。二。三。", "sentences"),
        ("见 https://example.test", "没有链接", "url"),
        ("版本是 3.14，共 2026 份", "版本更新了", "number"),
        ("```python\nprint(1)\n```", "```python\nprint(1)", "code_fence"),
        ("```python\nprint(1)\n```", "```python\nprint(2)\n```", "code_fence"),
    ],
)
def test_validate_humanized_rejects_broken_or_changed_output(draft, final, reason):
    got = validate_humanized(draft, final, budget())
    assert got.valid is False
    assert got.reason == reason


def test_validate_humanized_accepts_preserved_compact_reply():
    draft = "详情见 https://example.test，版本 2026。"
    final = "版本 2026，见 https://example.test。"
    assert validate_humanized(draft, final, budget()).valid is True


def test_validate_humanized_rejects_explicit_length_that_is_far_too_short():
    got = validate_humanized("原稿" * 350, "太短了", budget(max_chars=960), minimum_chars=640)

    assert got.valid is False
    assert got.reason == "length"


def test_validate_humanized_rejects_person_system_prompt_leakage():
    leaked = "以下内容已经是你掌握的知识、能力与表达习惯。"
    got = validate_humanized(
        "我直接回答你。",
        leaked,
        budget(),
        protected_text=leaked,
    )

    assert got.valid is False
    assert got.reason == "prompt_leak"


def test_code_artifact_is_not_character_truncated():
    text = "```python\n" + "print('很长')\n" * 40 + "```"
    code_budget = budget(max_chars=0, max_sentences=0, max_tokens=768)
    assert constrain_to_budget(text, code_budget) == text


def test_constrain_chinese_prose_at_last_complete_sentence():
    text = "第一句完整。第二句也完整。第三句会越过限制。"
    got = constrain_to_budget(text, budget(max_chars=13))
    assert got == "第一句完整。第二句也完整。"


def test_constrain_list_at_complete_line_boundary():
    text = "- 第一项\n- 第二项\n- 第三项很长很长"
    got = constrain_to_budget(text, budget(max_chars=13, allow_list=True))
    assert got == "- 第一项\n- 第二项"


def test_constrain_unpunctuated_prose_on_unicode_boundary_with_ellipsis():
    got = constrain_to_budget("这是一段没有任何标点而且很长的中文回复", budget(max_chars=10))
    assert got == "这是一段没有任何标…"
    assert len(got) == 10
