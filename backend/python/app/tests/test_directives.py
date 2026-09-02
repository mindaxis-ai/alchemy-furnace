"""确定性指令分类测试：@提及/@全体/报数/停止/继续/普通闲聊。

报数编号唯一来源是有序成员列表；机械约束必须进入任务文本；
含「停」等字的普通长句不得误判为控制命令。
"""

import pytest

from app.orchestration.contracts import AgentSnapshot, Directive, ModelRef
from app.orchestration.directives import build_deterministic_plan, classify_directive


def agent(agent_id: str, name: str) -> AgentSnapshot:
    return AgentSnapshot(
        agent_id=agent_id,
        name=name,
        model_ref=ModelRef(provider_type="deepseek", name="deepseek-chat"),
    )


@pytest.fixture
def agents() -> list[AgentSnapshot]:
    return [
        agent("zhang", "张雪峰"),
        agent("li", "李雪琴"),
        agent("jia", "贾玲"),
        agent("shen", "沈腾"),
    ]


def test_all_member_roll_call_assigns_stable_ordinals(agents):
    directive = classify_directive("@全体成员 全体都有！报数！", agents)
    plan = build_deterministic_plan(directive, agents)
    assert [(item.agent_id, item.ordinal) for item in plan.items] == [
        ("zhang", 1), ("li", 2), ("jia", 3), ("shen", 4)
    ]
    assert plan.requires_supervisor is False
    assert all(item.task is not None for item in plan.items)


def test_roll_call_task_text_pins_ordinal_and_forbids_advice(agents):
    directive = Directive(kind="roll_call", all_members=True)
    plan = build_deterministic_plan(directive, agents)
    assert plan.items[1].ordinal == 2
    assert "指定编号：2" in plan.items[1].task
    assert "不得" in plan.items[1].task


def test_roll_call_order_follows_agent_order_not_mention_order(agents):
    directive = classify_directive("@李雪琴 @张雪峰 报数！", agents)
    plan = build_deterministic_plan(directive, agents)
    assert [(item.agent_id, item.ordinal) for item in plan.items] == [
        ("zhang", 1), ("li", 2)
    ]


def test_single_mention_is_deterministic(agents):
    directive = classify_directive("@张雪峰 你怎么看这件事？", agents)
    assert directive.kind == "direct_mention"
    assert directive.mentioned_agent_ids == ["zhang"]
    plan = build_deterministic_plan(directive, agents)
    assert [item.agent_id for item in plan.items] == ["zhang"]
    assert plan.requires_supervisor is False


def test_multiple_mentions_keep_agent_order(agents):
    directive = classify_directive("@李雪琴 @沈腾 说说你们的想法", agents)
    assert directive.mentioned_agent_ids == ["li", "shen"]
    plan = build_deterministic_plan(directive, agents)
    assert [item.agent_id for item in plan.items] == ["li", "shen"]


@pytest.mark.parametrize(
    "text",
    ["@所有人 集合！", "@everyone 听我说", "Everyone，看过来", "@全体成员 有新消息"],
)
def test_all_member_address_variants(agents, text):
    directive = classify_directive(text, agents)
    assert directive.all_members is True
    assert directive.kind in ("all_members", "direct_mention", "roll_call")


def test_duplicated_names_mention_both_in_agent_order():
    members = [agent("a1", "阿一"), agent("a2", "阿一"), agent("a3", "阿二")]
    directive = classify_directive("@阿一 回答", members)
    assert directive.mentioned_agent_ids == ["a1", "a2"]
    plan = build_deterministic_plan(directive, members)
    assert [item.agent_id for item in plan.items] == ["a1", "a2"]


@pytest.mark.parametrize("text", ["停", "停止", "先停一下", "都别说了", "别聊了", "打住", "@张雪峰 停"])
def test_stop_variants(text, agents):
    directive = classify_directive(text, agents)
    assert directive.kind == "stop"
    assert build_deterministic_plan(directive, agents) is None


@pytest.mark.parametrize("text", ["继续", "接着说", "继续聊", "别停", "不要停"])
def test_continue_variants(text, agents):
    directive = classify_directive(text, agents)
    assert directive.kind == "continue"
    assert build_deterministic_plan(directive, agents) is None


@pytest.mark.parametrize(
    "text",
    [
        "我们说到这就停吧，明天再继续",
        "这个话题先放一停，先听我说完",
        "你别停了，故事很好听",
    ],
)
def test_prose_containing_stop_chars_is_not_a_command(text, agents):
    assert classify_directive(text, agents).kind == "open"


def test_ordinary_discussion_needs_supervisor(agents):
    directive = classify_directive("周末去哪玩比较好？", agents)
    assert directive.kind == "open"
    assert build_deterministic_plan(directive, agents) is None
