"""DaoistGraph 集成测试：可复用单道人发言链路（假模型，无网络）。

验证：
- 机械任务约束（报数编号）进入系统提示，且高于人设/记忆/闲聊风格；
- 节点产物事件顺序：speaker_started → assistant_delta → assistant_final（恰一条）；
- 记忆只注入当前道人自己的；历史窗口与字符预算生效；
- 报数校验失败只重试当前道人（≤2 次），重试不重启发言人回合；
- prompt_debug 仅在 debug 开启时发出，且全事件负载无凭据泄漏。
"""

from dataclasses import dataclass
from typing import Any

import pytest
from langchain_core.language_models.chat_models import BaseChatModel
from langchain_core.messages import AIMessage, BaseMessage
from langchain_core.outputs import ChatGeneration, ChatResult

from app.orchestration.contracts import (
    AgentSnapshot,
    ConversationState,
    MemorySnapshot,
    MessageSnapshot,
    ModelCredential,
    ModelRef,
    ResponseBudget,
    RuntimeContext,
    SemanticUnderstanding,
    SpeakingPlan,
    SpeakingPlanItem,
    UserTurnSnapshot,
)
from app.orchestration.events import OrchestrationEvent
from app.orchestration.graphs.daoist import build_daoist_graph
from app.orchestration.model_gateway import ModelGateway

#: 报数任务原文（ordinal=2 的机械约束；含 persona 陷阱「考研建议」须被压住）。
ROLL_CALL_TASK = (
    "报数任务：回复必须以指定编号：2 开头并原样保留该编号，仅输出你的编号。"
    "不得省略编号、不得更改顺序、不得插入考研建议、"
    "不得插入与报数无关的建议或补充说明。"
)

SECRET = "sk-secret"


# ---------------------------------------------------------------------------
# 假模型与网关
# ---------------------------------------------------------------------------


@dataclass
class FakeCall:
    """一次模型调用记录：入参消息、模型引用与运行期凭据。"""

    messages: list[BaseMessage]
    model_ref: ModelRef
    credential: ModelCredential
    kwargs: dict[str, Any]


class FakeChatModel(BaseChatModel):
    """从响应队列弹出一条回复并记录调用（无网络、确定性）。

    responses/calls 用 Any 字段透传外部共享列表——pydantic v2 会重建 list 类型
    字段（拷贝），Any 则保持对象同一性，保证测试能读到调用记录。
    """

    responses: Any = None
    calls: Any = None
    ref: Any = None
    credential: Any = None

    @property
    def _llm_type(self) -> str:
        return "fake"

    def _generate(
        self,
        messages: list[BaseMessage],
        stop: list[str] | None = None,
        run_manager: Any = None,
        **kwargs: Any,
    ) -> ChatResult:
        self.calls.append(
            FakeCall(
                messages=list(messages),
                model_ref=self.ref,
                credential=self.credential,
                kwargs=dict(kwargs),
            )
        )
        text = self.responses.pop(0)
        if isinstance(text, BaseException):
            raise text
        return ChatResult(generations=[ChatGeneration(message=AIMessage(content=text))])


@pytest.fixture
def fake_gateway() -> ModelGateway:
    """注入 deepseek 工厂的网关；调用记录挂在 gateway.calls，响应队列在 .responses。"""

    def factory(ref: ModelRef, credential: ModelCredential) -> FakeChatModel:
        return FakeChatModel(
            responses=gateway.responses, calls=gateway.calls, ref=ref, credential=credential
        )

    gateway = ModelGateway(factories={"deepseek": factory})
    gateway.calls = []  # type: ignore[attr-defined]
    gateway.responses = []  # type: ignore[attr-defined]
    return gateway


# ---------------------------------------------------------------------------
# 状态构建与运行辅助
# ---------------------------------------------------------------------------


def agent(agent_id: str, name: str, system_prompt: str = "") -> AgentSnapshot:
    return AgentSnapshot(
        agent_id=agent_id,
        name=name,
        system_prompt=system_prompt,
        model_ref=ModelRef(provider_type="deepseek", name="deepseek-chat"),
    )


def memory(memory_id: str, agent_id: str, text: str) -> MemorySnapshot:
    return MemorySnapshot(memory_id=memory_id, agent_id=agent_id, text=text)


def build_state(
    *,
    agents: list[AgentSnapshot],
    user_text: str,
    pending: list[str] | None = None,
    speaking_plan: SpeakingPlan | None = None,
    history: list[MessageSnapshot] | None = None,
    memories: list[MemorySnapshot] | None = None,
) -> ConversationState:
    state = ConversationState(
        run_id="run-test",
        session_id="session-test",
        session_type="group",
        user_turn=UserTurnSnapshot(message_id="m-user", text=user_text).model_dump(),
        history_snapshot=[m.model_dump() for m in (history or [])],
        agent_snapshots=[a.model_dump() for a in agents],
        memory_snapshots=[m.model_dump() for m in (memories or [])],
        directive=None,
        speaking_plan=speaking_plan.model_dump() if speaking_plan else None,
        pending_agent_ids=pending or [a.agent_id for a in agents],
        replies=[],
        memory_proposals=[],
        outcome=None,
    )
    state["semantic_understanding"] = SemanticUnderstanding(
        source="fallback",
        intent="task",
        core_request=user_text[:400],
        emotion="neutral",
        complexity="moderate",
        detail_preference="normal",
        requested_chars=None,
        format_preference="plain",
        wants_advice=False,
        wants_follow_up=False,
        should_clarify=False,
        avoid_behaviors=["repeat"],
    ).model_dump()
    state["response_budget"] = ResponseBudget(
        target_chars=320,
        max_chars=800,
        max_sentences=8,
        max_tokens=768,
        allow_list=True,
        allow_follow_up=False,
        max_speakers=2,
    ).model_dump()
    return state


@pytest.fixture
def roll_call_state() -> ConversationState:
    """zhang（张雪峰）领到 ordinal=2 的报数任务；li 在场但不发言。"""
    zhang = agent("zhang", "张雪峰")
    li = agent("li", "李雪琴")
    plan = SpeakingPlan(
        items=[SpeakingPlanItem(agent_id="zhang", ordinal=2, task=ROLL_CALL_TASK)],
        requires_supervisor=False,
        reason="确定性报数任务",
    )
    return build_state(
        agents=[zhang, li],
        user_text="@全体成员 全体都有！报数！",
        pending=["zhang"],
        speaking_plan=plan,
    )


async def run_daoist_for_test(
    state: ConversationState, gateway: ModelGateway, *, debug_enabled: bool = False
):
    """跑完整个 DaoistGraph 并逐条回放收集到的事件。"""
    collected: list[OrchestrationEvent] = []
    credentials = {a["agent_id"]: ModelCredential(api_key=SECRET) for a in state["agent_snapshots"]}
    context = RuntimeContext(
        model_gateway=gateway,
        credentials_by_model_ref=credentials,
        debug_enabled=debug_enabled,
        event_sink=collected.append,
    )
    graph = build_daoist_graph().compile()
    await graph.ainvoke(dict(state), context=context)
    for event in collected:
        yield event


def events_of(events: list[OrchestrationEvent], name: str) -> list[OrchestrationEvent]:
    return [e for e in events if e.name == name]


def final_content(events: list[OrchestrationEvent]) -> str:
    """assistant_final 正文（事件契约为恰一条终稿）。"""
    finals = events_of(events, "assistant_final")
    assert len(finals) == 1
    return str(finals[0].payload["text"])


def delta_texts(events: list[OrchestrationEvent]) -> list[str]:
    return [str(e.payload["text"]) for e in events_of(events, "assistant_delta")]


# ---------------------------------------------------------------------------
# 测试
# ---------------------------------------------------------------------------


@pytest.mark.asyncio
async def test_roll_call_constraint_overrides_persona(fake_gateway, roll_call_state):
    fake_gateway.responses.extend(["2", "2"])
    events = [event async for event in run_daoist_for_test(roll_call_state, fake_gateway)]

    prompt = fake_gateway.calls[0].messages
    assert "指定编号：2" in prompt[0].content
    assert "不得插入考研建议" in prompt[0].content
    assert final_content(events).strip().startswith("2")


@pytest.mark.asyncio
async def test_composed_persona_and_pill_prompt_reaches_real_model_input(fake_gateway):
    composed = (
        "你是云游道人。\n"
        "【基础人设】说话简洁、温和，但会指出逻辑漏洞。\n"
        "【能力模块：代码审查】先定位根因，再给最小修复。"
    )
    daoist = agent("daoist", "云游道人", composed)
    plan = SpeakingPlan(
        items=[SpeakingPlanItem(agent_id="daoist", ordinal=1, task="回答用户问题。")],
        requires_supervisor=False,
        reason="single",
    )
    state = build_state(
        agents=[daoist],
        user_text="请检查这段代码",
        pending=["daoist"],
        speaking_plan=plan,
    )
    fake_gateway.responses.extend(["1", "1"])

    [event async for event in run_daoist_for_test(state, fake_gateway, debug_enabled=True)]

    system = fake_gateway.calls[0].messages[0].content
    assert "【基础人设】说话简洁、温和" in system
    assert "【能力模块：代码审查】先定位根因" in system
    assert system.count("你是云游道人") == 1


@pytest.mark.asyncio
async def test_event_sequence_and_single_final_reply(fake_gateway, roll_call_state):
    fake_gateway.responses.extend(["原始草稿 2", "2"])
    events = [event async for event in run_daoist_for_test(roll_call_state, fake_gateway)]

    names = [e.name for e in events]
    assert names == ["speaker_started", "assistant_delta", "assistant_final"]
    assert all(e.run_id == "run-test" for e in events)

    speaker = events_of(events, "speaker_started")[0]
    assert speaker.payload == {"agent_id": "zhang", "task": ROLL_CALL_TASK}

    assert delta_texts(events) == ["2"]
    assert "原始草稿" not in str([event.payload for event in events])
    final = events_of(events, "assistant_final")[0]
    assert final.payload["agent_id"] == "zhang"
    assert final.payload["reply_id"]
    assert final.payload["text"] == "2"
    # 本阶段不发记忆提案（蒸馏语义留待后续 Task）。
    assert "memory_proposed" not in names


@pytest.mark.asyncio
async def test_memory_selection_keeps_only_current_agent(fake_gateway, roll_call_state):
    fake_gateway.responses.extend(["2", "2"])
    state = roll_call_state
    state["memory_snapshots"] = [
        memory("mem-own-1", "zhang", "他喜欢研究高考志愿填报").model_dump(),
        memory("mem-own-2", "zhang", "最近在筹备新书").model_dump(),
        memory("mem-other", "li", "脱口秀巡演中").model_dump(),
    ]
    [event async for event in run_daoist_for_test(state, fake_gateway)]

    system = fake_gateway.calls[0].messages[0].content
    assert "喜欢研究高考志愿填报" in system
    assert "筹备新书" in system
    assert "脱口秀巡演中" not in system


@pytest.mark.asyncio
async def test_history_window_and_final_user_turn_order(fake_gateway, roll_call_state):
    fake_gateway.responses.extend(["2", "2"])
    history = [
        MessageSnapshot(message_id=f"h{i:02d}", role="user", text=f"hist-{i:02d}")
        for i in range(25)
    ]
    state = roll_call_state
    state["history_snapshot"] = [m.model_dump() for m in history]
    [event async for event in run_daoist_for_test(state, fake_gateway)]

    messages = fake_gateway.calls[0].messages
    assert messages[0].type == "system"
    contents = [m.content for m in messages]
    # 25 条历史只保留最近 20 条，旧消息被窗口丢弃。
    assert "hist-00" not in contents and "hist-04" not in contents
    assert contents[1] == "hist-05" and contents[20] == "hist-24"
    # 当前用户轮永远作为最后一条消息。
    assert len(messages) == 22
    assert messages[-1].type == "human"
    assert messages[-1].content == "@全体成员 全体都有！报数！"


@pytest.mark.asyncio
async def test_prompt_debug_is_gated_and_secret_free(fake_gateway, roll_call_state):
    fake_gateway.responses.extend(["2", "2"])
    events = [event async for event in run_daoist_for_test(roll_call_state, fake_gateway)]
    assert "prompt_debug" not in [e.name for e in events]

    # 第一次运行已消耗响应；补一条再跑 debug 模式（fixture 同为函数级作用域）。
    fake_gateway.responses.extend(["2", "2"])
    events = [
        event
        async for event in run_daoist_for_test(roll_call_state, fake_gateway, debug_enabled=True)
    ]
    debug = events_of(events, "prompt_debug")
    assert len(debug) == 1
    payload = debug[0].payload
    assert payload["agent_id"] == "zhang"
    assert "不得插入考研建议" in payload["messages"][0]["content"]
    assert payload["response_budget"] == roll_call_state["response_budget"]

    # 凭据到达网关（运行期边界允许），但任何事件负载都不携带。
    assert fake_gateway.calls[0].credential.api_key == SECRET
    import json

    dumped = json.dumps([e.payload for e in events])
    assert SECRET not in dumped


@pytest.mark.asyncio
async def test_roll_call_retry_corrects_wrong_ordinal(fake_gateway, roll_call_state):
    fake_gateway.responses.extend(["1", "2", "2"])
    events = [event async for event in run_daoist_for_test(roll_call_state, fake_gateway)]

    assert len(fake_gateway.calls) == 3
    second_prompt = fake_gateway.calls[1].messages
    # 纠错指令以追加消息进入第二次调用（role=system，含强制编号原文）。
    assert second_prompt[-1].type == "system"
    assert "指定编号：2" in second_prompt[-1].content

    assert final_content(events).strip() == "2"
    # 重试只多一次模型调用，不重启发言人回合。
    assert len(events_of(events, "speaker_started")) == 1
    assert delta_texts(events) == ["2"]
    assert len(events_of(events, "assistant_final")) == 1


@pytest.mark.asyncio
async def test_roll_call_retry_exhaustion_keeps_last_draft(fake_gateway, roll_call_state):
    fake_gateway.responses.extend(["1", "1", "1", "1"])
    events = [event async for event in run_daoist_for_test(roll_call_state, fake_gateway)]

    # 最多 2 次重试（合计 3 次模型调用），不无限循环。
    assert len(fake_gateway.calls) == 4
    assert final_content(events).strip() == "1"
    assert len(events_of(events, "assistant_final")) == 1


@pytest.mark.asyncio
async def test_draft_is_hidden_and_both_normal_calls_use_director_token_budget(
    fake_gateway, roll_call_state
):
    fake_gateway.responses.extend(["2。当然可以，下面详细说说。", "2。"])

    events = [event async for event in run_daoist_for_test(roll_call_state, fake_gateway)]

    dumped = str([event.payload for event in events])
    assert "当然可以" not in dumped
    assert final_content(events) == "2。"
    assert len(fake_gateway.calls) == 2
    assert [call.kwargs["max_tokens"] for call in fake_gateway.calls] == [768, 768]


@pytest.mark.asyncio
async def test_humanizer_exception_falls_back_to_constrained_draft(
    fake_gateway, roll_call_state
):
    fake_gateway.responses.extend(["2", RuntimeError("humanizer unavailable")])

    events = [event async for event in run_daoist_for_test(roll_call_state, fake_gateway)]

    assert final_content(events) == "2"
    assert len(fake_gateway.calls) == 2


@pytest.mark.asyncio
async def test_invalid_humanizer_output_retries_once(fake_gateway, roll_call_state):
    fake_gateway.responses.extend(["2", "", "2"])

    events = [event async for event in run_daoist_for_test(roll_call_state, fake_gateway)]

    assert final_content(events) == "2"
    assert len(fake_gateway.calls) == 3


@pytest.mark.asyncio
async def test_invalid_humanizer_retry_falls_back_to_draft(fake_gateway, roll_call_state):
    fake_gateway.responses.extend(["2", "", ""])

    events = [event async for event in run_daoist_for_test(roll_call_state, fake_gateway)]

    assert final_content(events) == "2"
    assert len(fake_gateway.calls) == 3


@pytest.mark.asyncio
async def test_humanizer_cannot_change_roll_call_number(fake_gateway, roll_call_state):
    fake_gateway.responses.extend(["2", "1 2", "2"])

    events = [
        event async for event in run_daoist_for_test(roll_call_state, fake_gateway)
    ]

    assert final_content(events) == "2"
    assert len(fake_gateway.calls) == 3


@pytest.mark.asyncio
async def test_explicit_length_retries_a_far_too_short_humanized_reply(fake_gateway):
    state = build_state(agents=[agent("writer", "写作者")], user_text="请写800字")
    state["semantic_understanding"] = SemanticUnderstanding(
        source="model",
        intent="task",
        core_request="写800字介绍",
        emotion="neutral",
        complexity="moderate",
        detail_preference="detailed",
        requested_chars=800,
        format_preference="creative",
        wants_advice=False,
        wants_follow_up=False,
        should_clarify=False,
        avoid_behaviors=["repeat"],
    ).model_dump()
    state["response_budget"] = ResponseBudget(
        target_chars=800,
        max_chars=960,
        max_sentences=12,
        max_tokens=1600,
        allow_list=True,
        allow_follow_up=False,
        max_speakers=1,
    ).model_dump()
    draft = "原稿内容" * 140
    final = "润色内容" * 170
    fake_gateway.responses.extend([draft, "太短", final])

    events = [event async for event in run_daoist_for_test(state, fake_gateway)]

    assert final_content(events) == final
    assert len(final_content(events)) >= 640
    assert len(fake_gateway.calls) == 3


@pytest.mark.asyncio
async def test_humanizer_system_prompt_leak_retries_before_emission(fake_gateway):
    protected = "以下内容已经是你掌握的知识、能力与表达习惯。"
    state = build_state(
        agents=[agent("person", "阿衡", system_prompt=protected)],
        user_text="你怎么看？",
    )
    fake_gateway.responses.extend(["我直接说看法。", protected, "我直接说看法。"])

    events = [event async for event in run_daoist_for_test(state, fake_gateway)]

    assert final_content(events) == "我直接说看法。"
    assert protected not in str([event.payload for event in events])
    assert len(fake_gateway.calls) == 3
