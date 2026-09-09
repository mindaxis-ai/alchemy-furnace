"""ConversationGraph + ConversationRuntime 集成测试：会话路由、检查点与恢复语义。

验证（设计文档 §4/§7/§10，Task 7 锚点）：
- single/group 会话路由：单聊直达 DaoistGraph（无 plan_created），群聊按确定性
  指令成计划后逐发言人发言；
- 中断恢复差集：用户停止后同一 run 续跑，不重复已完成发言人的回复
  （锚点 test_resume_skips_completed_speakers）；
- 新用户消息取代旧轮：同会话新 run 取消活跃旧 run，绝不继承旧待发言计划；
- 检查点状态不含 api_key（凭据只活在 RuntimeContext，永不落盘）；
- 终态检查点清理选择：completed/cancelled 的线程删除，interrupted 保留，
  续跑至完成后再清理。
"""

import asyncio
import json
from dataclasses import dataclass
from types import SimpleNamespace
from typing import Any

import pytest
from langchain_core.language_models.chat_models import BaseChatModel
from langchain_core.messages import AIMessage, BaseMessage

from app.orchestration.contracts import (
    AgentSnapshot,
    ModelCredential,
    ModelRef,
    OrchestrationRequest,
    UserTurnSnapshot,
)
from app.orchestration.events import OrchestrationEvent
from app.orchestration.model_gateway import ModelGateway
from app.orchestration.runtime import ConversationRuntime, RunNotFoundError

SECRET = "sk-secret"

#: 报数确定性指令：逐发言人按序报数，永不调 Supervisor。
ROLL_CALL_TEXT = "@全体成员 全体都有！报数！"


@dataclass
class FakeCall:
    """一次模型调用记录：入参消息、模型引用与运行期凭据。"""

    messages: list[BaseMessage]
    model_ref: ModelRef
    credential: ModelCredential


class FakeChatModel(BaseChatModel):
    """从共享响应队列弹出一条回复并记录调用（无网络、确定性）。

    responses 元素可以是 Exception——弹出即抛，模拟单次模型调用失败。
    ainvoke 在记录前先让出事件循环（await asyncio.sleep(0)）：纯同步假模型会让
    整轮群聊在一个事件循环时间片内跑完，消费方（测试/runtime 取消）永远等不到
    发言人边界——让出后「第 N 条终稿到达 → 取消」的时序才可复现。
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
    ) -> Any:
        raise NotImplementedError("测试只走 ainvoke（需要让出事件循环的时机）")

    async def ainvoke(self, messages: list[BaseMessage], **kwargs: Any) -> AIMessage:
        await asyncio.sleep(0)
        self.calls.append(
            FakeCall(messages=list(messages), model_ref=self.ref, credential=self.credential)
        )
        item = self.responses.pop(0)
        if isinstance(item, BaseException):
            raise item
        return AIMessage(content=item)


@pytest.fixture
def fake_gateway() -> ModelGateway:
    """注入 deepseek 工厂的网关；调用记录挂在 .calls，响应队列在 .responses。"""

    def factory(ref: ModelRef, credential: ModelCredential) -> FakeChatModel:
        return FakeChatModel(
            responses=gateway.responses, calls=gateway.calls, ref=ref, credential=credential
        )

    gateway = ModelGateway(factories={"deepseek": factory})
    gateway.calls = []  # type: ignore[attr-defined]
    gateway.responses = []  # type: ignore[attr-defined]
    return gateway


@pytest.fixture
def runtime(fake_gateway: ModelGateway, tmp_path):
    """真实 ConversationRuntime：sqlite 检查点落在每测试独立临时目录。"""
    return ConversationRuntime(
        model_gateway=fake_gateway,
        checkpoint_path=tmp_path / "orchestration.sqlite",
    )


# ---------------------------------------------------------------------------
# 请求与事件辅助
# ---------------------------------------------------------------------------


def agent(agent_id: str, name: str) -> AgentSnapshot:
    return AgentSnapshot(
        agent_id=agent_id,
        name=name,
        model_ref=ModelRef(provider_type="deepseek", name="deepseek-chat"),
    )


GROUP_MEMBERS = [
    agent("zhang", "张雪峰"),
    agent("li", "李雪琴"),
    agent("jia", "贾玲"),
    agent("shen", "沈腾"),
]


def _credentials_for(agents: list[AgentSnapshot]) -> dict[str, ModelCredential]:
    return {a.agent_id: ModelCredential(api_key=SECRET) for a in agents}


@pytest.fixture
def single_request() -> OrchestrationRequest:
    daoist = agent("dan", "丹炉道长")
    return OrchestrationRequest(
        run_id="run-single",
        session_id="session-single",
        session_type="single",
        user_turn=UserTurnSnapshot(message_id="m-single", text="给我一句忠告"),
        history_snapshot=[],
        agent_snapshots=[daoist],
        memory_snapshots=[],
        credentials=_credentials_for([daoist]),
    )


@pytest.fixture
def group_request() -> OrchestrationRequest:
    return OrchestrationRequest(
        run_id="run-group",
        session_id="session-group",
        session_type="group",
        user_turn=UserTurnSnapshot(message_id="m-group", text=ROLL_CALL_TEXT),
        history_snapshot=[],
        agent_snapshots=GROUP_MEMBERS,
        memory_snapshots=[],
        credentials=_credentials_for(GROUP_MEMBERS),
    )


@pytest.fixture
def interrupted_group_request() -> OrchestrationRequest:
    """中断恢复锚点的群聊请求（报数 = 确定性计划，不依赖 Supervisor 队列）。"""
    return OrchestrationRequest(
        run_id="run-interrupted",
        session_id="session-interrupted",
        session_type="group",
        user_turn=UserTurnSnapshot(message_id="m-interrupted", text=ROLL_CALL_TEXT),
        history_snapshot=[],
        agent_snapshots=GROUP_MEMBERS,
        memory_snapshots=[],
        credentials=_credentials_for(GROUP_MEMBERS),
    )


async def collect(agen) -> list[OrchestrationEvent]:
    """吸干一次 start/resume 事件流（到终态事件为止）。"""
    events: list[OrchestrationEvent] = []
    async for e in agen:
        events.append(e)
    return events


async def collect_until_interrupt(runtime: ConversationRuntime, request: OrchestrationRequest):
    """启动 run 并消费事件；第 2 条 assistant_final 到达即取消，收集到中断为止。

    取消落在发言人边界（dispatch 每次迭代先让出事件循环再查旗标），
    因此 run 会带着「已完成发言人的回复」干净地转 interrupted。
    """
    events: list[OrchestrationEvent] = []
    finals = 0
    async for e in runtime.start(request):
        events.append(e)
        if e.name == "assistant_final":
            finals += 1
            if finals == 2:
                runtime.cancel(request.run_id)
        if e.name in ("run_interrupted", "run_completed", "run_error"):
            break
    return events


def events_of(events: list[OrchestrationEvent], name: str) -> list[OrchestrationEvent]:
    return [e for e in events if e.name == name]


def final_replies(events: list[OrchestrationEvent]) -> list[Any]:
    # assistant_final 负载无 run_id（在事件顶层）；补上以还原完整 AgentReply。
    return [
        dict(e.payload, run_id=e.run_id)
        for e in events_of(events, "assistant_final")
    ]


def completed_reply_ids(events: list[OrchestrationEvent]) -> set[str]:
    return {r["reply_id"] for r in final_replies(events)}


def new_reply_ids(events: list[OrchestrationEvent]) -> set[str]:
    return {r["reply_id"] for r in final_replies(events)}


def all_expected_agents(events: list[OrchestrationEvent]) -> set[str]:
    return {r["agent_id"] for r in final_replies(events)}


class CapturingRuntimeGraph:
    """只替换 LangGraph 执行边界，观察运行器给 start/resume 的真实 context。"""

    def __init__(self) -> None:
        self.contexts = []
        self.values: dict[str, Any] = {}

    async def ainvoke(self, state, *, config, context):
        self.contexts.append(context)
        outcome = "interrupted" if len(self.contexts) == 1 else "completed"
        if outcome == "interrupted":
            context.cancellation.cancel("interrupted")
        self.values = {**state, "outcome": outcome}
        return self.values

    async def aget_state(self, config):
        return SimpleNamespace(values=self.values)


# ---------------------------------------------------------------------------
# 测试
# ---------------------------------------------------------------------------


@pytest.mark.asyncio
async def test_runtime_preserves_default_model_ref_across_resume(
    runtime, single_request, monkeypatch
):
    graph = CapturingRuntimeGraph()

    async def use_capturing_graph():
        runtime._graph = graph

    monkeypatch.setattr(runtime, "_ensure_saver", use_capturing_graph)

    default_ref = ModelRef(provider_type="deepseek", name="semantic-model")
    request = single_request.model_copy(update={"default_model_ref": default_ref})
    await collect(runtime.start(request))
    await collect(runtime.resume(request.run_id))

    assert [ctx.default_model_ref for ctx in graph.contexts] == [default_ref, default_ref]


@pytest.mark.asyncio
async def test_single_session_routes_daoist_without_plan(runtime, fake_gateway, single_request):
    fake_gateway.responses.extend(["知行合一，莫问前程。"])

    events = await collect(runtime.start(single_request))

    # 单聊直达 DaoistGraph：无计划事件、恰一次发言人链路，终态 run_completed。
    assert [e.name for e in events] == [
        "run_started",
        "speaker_started",
        "assistant_delta",
        "assistant_final",
        "run_completed",
    ]
    assert events_of(events, "plan_created") == []
    assert len(fake_gateway.calls) == 1
    assert final_replies(events)[0]["agent_id"] == "dan"
    assert events[-1].name == "run_completed"


@pytest.mark.asyncio
async def test_group_roll_call_plans_then_speaks_in_order(runtime, fake_gateway, group_request):
    fake_gateway.responses.extend(["1", "2", "3", "4"])

    events = await collect(runtime.start(group_request))

    # 事件序：run_started → 计划 → 逐发言人 → run_completed 收尾。
    names = [e.name for e in events]
    assert names[0] == "run_started"
    assert names.index("plan_created") < names.index("speaker_started")
    assert events_of(events, "plan_created")[0].payload["source"] == "deterministic"
    assert final_replies(events) and all(r["text"] for r in final_replies(events))
    assert [r["agent_id"] for r in final_replies(events)] == ["zhang", "li", "jia", "shen"]
    assert len(fake_gateway.calls) == 4
    assert names[-1] == "run_completed"
    assert events_of(events, "run_interrupted") == []


@pytest.mark.asyncio
async def test_resume_skips_completed_speakers(runtime, fake_gateway, interrupted_group_request):
    """锚点：中断后续跑同一 run，不重复已完成发言人的回复、不重发计划。"""
    fake_gateway.responses.extend(["1", "2", "3", "4"])

    first_events = await collect_until_interrupt(runtime, interrupted_group_request)
    resumed_events = await collect(runtime.resume(interrupted_group_request.run_id))

    assert completed_reply_ids(first_events).isdisjoint(new_reply_ids(resumed_events))
    assert all_expected_agents(first_events + resumed_events) == {
        a.agent_id for a in GROUP_MEMBERS
    }
    # 中断语义：interrupted 而非完成；续跑不重规划、不重复任何已完成发言人。
    assert first_events[-1].name == "run_interrupted"
    assert first_events[-1].payload["reason"] == "interrupted"
    assert events_of(resumed_events, "plan_created") == []
    assert len(fake_gateway.calls) == len(GROUP_MEMBERS)
    assert resumed_events[-1].name == "run_completed"


@pytest.mark.asyncio
async def test_new_message_cancels_active_run_without_inheriting_plan(
    runtime, fake_gateway, group_request, interrupted_group_request
):
    """新用户消息取代旧轮：活跃旧 run 以 cancelled 终止，新 run 计划不受旧计划污染。"""
    fake_gateway.responses.extend(["1", "2", "3", "4", "备用甲", "备用乙"])

    follow_up = OrchestrationRequest(
        run_id="run-follow-up",
        session_id=group_request.session_id,  # 同一会话 → 取代旧轮
        session_type="group",
        user_turn=UserTurnSnapshot(
            message_id="m-follow-up",
            text="@张雪峰 你来收个尾",
            mentioned_agent_ids=["zhang"],
        ),
        history_snapshot=[],
        agent_snapshots=GROUP_MEMBERS,
        memory_snapshots=[],
        credentials=_credentials_for(GROUP_MEMBERS),
    )

    # 旧轮 A 先启动并消费到 plan_created（确定性计划已产出），再启动新轮 B。
    old_events: list[OrchestrationEvent] = []
    a_iter = runtime.start(group_request)
    async for e in a_iter:
        old_events.append(e)
        if e.name == "plan_created":
            break
    new_events = await collect(runtime.start(follow_up))
    # A 的余下事件：新轮启动即取代旧轮 → run_interrupted(reason=cancelled)。
    async for e in a_iter:
        old_events.append(e)
        if e.name in ("run_interrupted", "run_completed", "run_error"):
            break

    assert old_events[-1].name == "run_interrupted"
    assert old_events[-1].payload["reason"] == "cancelled"
    # 新轮只派 @张雪峰：旧轮 4 人计划不进入新 run 的执行。
    plan = events_of(new_events, "plan_created")[0].payload["plan"]
    assert [item["agent_id"] for item in plan["items"]] == ["zhang"]
    assert [r["agent_id"] for r in final_replies(new_events)] == ["zhang"]
    assert new_events[-1].name == "run_completed"


@pytest.mark.asyncio
async def test_checkpoint_state_contains_no_api_key(
    runtime, fake_gateway, interrupted_group_request
):
    fake_gateway.responses.extend(["1", "2", "3", "4"])

    events = await collect_until_interrupt(runtime, interrupted_group_request)
    assert events[-1].name == "run_interrupted"

    state = await runtime.checkpoint_state(interrupted_group_request.run_id)
    assert state is not None
    assert state["outcome"] == "interrupted"
    dumped = json.dumps(state)
    assert SECRET not in dumped
    assert "api_key" not in dumped


@pytest.mark.asyncio
async def test_terminal_checkpoint_cleanup_selection(
    runtime, fake_gateway, single_request, interrupted_group_request
):
    """终态清理：completed/interrupted 的选择性保留，续跑至完成再清理。"""
    fake_gateway.responses.extend(["你好呀"])

    # 1) 一次完整单聊 → completed 为终态，线程清理，检查点不可再读。
    await collect(runtime.start(single_request))
    assert await runtime.checkpoint_state(single_request.run_id) is None

    # 2) 中断的群聊 → interrupted 保留（可续跑）。
    fake_gateway.responses.extend(["1", "2", "3", "4"])
    first_events = await collect_until_interrupt(runtime, interrupted_group_request)
    assert first_events[-1].name == "run_interrupted"
    state = await runtime.checkpoint_state(interrupted_group_request.run_id)
    assert state is not None and state["outcome"] == "interrupted"

    # 3) 续跑至完成 → 终态清理删除全部代次线程。
    resumed = await collect(runtime.resume(interrupted_group_request.run_id))
    assert resumed[-1].name == "run_completed"
    assert await runtime.checkpoint_state(interrupted_group_request.run_id) is None

    # 4) 续跑未知 run → RunNotFoundError。
    with pytest.raises(RunNotFoundError):
        async for _e in runtime.resume("run-ghost"):
            pass
