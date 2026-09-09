"""人物提示编译：人格、示例、历史和原始 Query 的优先级与顺序。"""

import pytest

from app.orchestration.contracts import (
    AgentSnapshot,
    MemorySnapshot,
    MessageSnapshot,
    ModelRef,
    ResponseBudget,
    SemanticUnderstanding,
    UserTurnSnapshot,
)
from app.orchestration.prompts import compile_messages


def understanding(intent="casual", **updates):
    return SemanticUnderstanding.model_validate(
        {
            "source": "model",
            "intent": intent,
            "core_request": "只是自然回应，不要扩写",
            "emotion": "neutral",
            "complexity": "tiny",
            "detail_preference": "brief",
            "requested_chars": None,
            "format_preference": "plain",
            "wants_advice": False,
            "wants_follow_up": False,
            "should_clarify": False,
            "avoid_behaviors": ["lecture", "summary", "list", "follow_up"],
            **updates,
        }
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


def person():
    return AgentSnapshot(
        agent_id="person-1",
        name="鲁迅",
        system_prompt="【身份与性格】\n姓名：鲁迅\n基础性格：冷峻克制。",
        model_ref=ModelRef(provider_type="deepseek", name="person-model"),
        example_dialogues=[
            {"user": "沉默是什么？", "assistant": "沉默呵，沉默罢了。"},
            {"user": "要乐观吗？", "assistant": "自然可以，只别把眼睛蒙上。"},
        ],
    )


def compile_for(intent="casual", *, task=None, semantic_updates=None, budget_updates=None):
    return compile_messages(
        agent=person(),
        user_turn=UserTurnSnapshot(message_id="now", text="今天天气不错"),
        history_snapshot=[
            MessageSnapshot(message_id="h1", role="user", text="昨日下雨"),
            MessageSnapshot(
                message_id="h2", role="assistant", text="是的。", agent_id="person-1"
            ),
        ],
        selected_memories=[
            MemorySnapshot(memory_id="mem", agent_id="person-1", text="用户喜欢短答")
        ],
        understanding=understanding(intent, **(semantic_updates or {})),
        budget=budget(**(budget_updates or {})),
        task=task,
    )


def test_message_order_keeps_examples_before_history_and_raw_query_last():
    messages = compile_for()
    assert [message.type for message in messages] == [
        "system", "human", "ai", "human", "ai", "human", "ai", "human"
    ]
    assert [message.content for message in messages[1:5]] == [
        "沉默是什么？", "沉默呵，沉默罢了。", "要乐观吗？", "自然可以，只别把眼睛蒙上。"
    ]
    assert [message.content for message in messages[5:7]] == ["昨日下雨", "是的。"]
    assert messages[-1].content == "今天天气不错"
    assert sum("今天天气不错" in str(message.content) for message in messages) == 1


def test_casual_policy_requires_short_plain_reply():
    system = str(compile_for()[0].content)
    assert "1～2句" in system
    assert "硬上限：120个字符" in system
    assert "不使用标题、列表或总结" in system
    assert "不要主动追问" in system


def test_vent_and_factual_policies_match_user_need():
    vent = str(
        compile_for(
            "vent",
            semantic_updates={"emotion": "frustrated"},
        )[0].content
    )
    factual = str(
        compile_for(
            "factual",
            semantic_updates={"detail_preference": "normal"},
            budget_updates={"target_chars": 120, "max_chars": 220, "max_sentences": 3, "max_tokens": 256},
        )[0].content
    )
    assert "先接住情绪" in vent
    assert "未求助时不说教" in vent
    assert "第一句直接回答" in factual


@pytest.mark.parametrize("intent", ["task", "deep_dive"])
def test_task_and_deep_dive_allow_useful_structure(intent):
    system = str(
        compile_for(
            intent,
            semantic_updates={"detail_preference": "detailed"},
            budget_updates={
                "target_chars": 600,
                "max_chars": 1600,
                "max_sentences": 12,
                "max_tokens": 1400,
                "allow_list": True,
                "max_speakers": 2,
            },
        )[0].content
    )
    assert "允许使用必要的列表或步骤" in system


def test_mechanical_task_precedes_identity_and_style_examples():
    system = str(compile_for(task="必须只输出编号：2")[0].content)
    assert system.index("【回合任务】") < system.index("【身份与性格】")
    assert "必须只输出编号：2" in system
    assert "机械任务优先于人物风格" in system
