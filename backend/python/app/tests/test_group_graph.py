"""GroupChatGraph 集成测试：混合导演（确定性指令永不委托模型，开放讨论走 Supervisor）。

验证：
- 报数等确定性指令零 Supervisor 调用，按计划顺序逐道人发言、终稿不重复；
- 开放讨论恰一次调用默认模型 Supervisor，产出结构化发言计划；
- Supervisor 输出无效/默认模型不可用 → 确定性回退到主成员（不传染整轮）；
- 单个道人失败不阻塞后续发言人；停/继续控制指令不产出任何发言。
"""

import json
from dataclasses import dataclass
from typing import Any

import pytest
from langchain_core.language_models.chat_models import BaseChatModel
from langchain_core.messages import AIMessage, BaseMessage
from langchain_core.outputs import ChatGeneration, ChatResult

from app.orchestration.contracts import (
    AgentReply,
    AgentSnapshot,
    ConversationState,
    ModelCredential,
    ModelRef,
    RuntimeContext,
    UserTurnSnapshot,
)
from app.orchestration.events import OrchestrationEvent
from app.orchestration.graphs.daoist import build_daoist_graph
from app.orchestration.graphs.group import build_group_graph
from app.orchestration.model_gateway import ModelGateway

SECRET = "sk-secret"

#: 会话「默认模型」：Supervisor 导演用它；须与成员模型名可区分（deepseek-chat）。
SUPERVISOR_REF = ModelRef(provider_type="deepseek", name="deepseek-reasoner")

#: 无效的 Supervisor 结构化输出（任何不可解析/不合法的负载）。
GARBAGE_OUTPUT = "这根本不是 JSON 计划，我是道人的人设话术。"


@dataclass
class FakeCall:
    """一次模型调用记录：入参消息、模型引用与运行期凭据。"""

    messages: list[BaseMessage]
    model_ref: ModelRef
    credential: ModelCredential


class _StructuredFake:
    """with_structured_output 的确定性替身：经底层模型取文本并解析为 schema。"""

    def __init__(self, model: BaseChatModel, schema: type[Any]) -> None:
        self._model = model
        self._schema = schema

    async def ainvoke(self, messages: list[BaseMessage]) -> Any:
        reply = await self._model.ainvoke(messages)
        text = reply.content if isinstance(reply.content, str) else str(reply.content)
        return self._schema.model_validate_json(text)


class FakeChatModel(BaseChatModel):
    """从响应队列弹出一条回复并记录调用（无网络、确定性）。

    responses 元素可以是 Exception——弹出即抛，模拟单次模型调用失败。
    字段全用 Any 透传外部共享列表（pydantic v2 会重建 list 类型字段，
    Any 保持对象同一性），保证测试能读到调用记录。
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
            FakeCall(messages=list(messages), model_ref=self.ref, credential=self.credential)
        )
        item = self.responses.pop(0)
        if isinstance(item, BaseException):
            raise item
        return ChatResult(generations=[ChatGeneration(message=AIMessage(content=item))])

    def with_structured_output(self, schema: type[Any], **kwargs: Any) -> _StructuredFake:
        return _StructuredFake(self, schema)


@pytest.fixture
def fake_gateway() -> ModelGateway:
    """注入 deepseek 工厂的网关；Supervisor 调用按 ref 单独计数。

    调用记录挂在 gateway.calls，响应队列在 .responses（pop 顺序 = 调用顺序）。
    """

    def factory(ref: ModelRef, credential: ModelCredential) -> FakeChatModel:
        if ref == SUPERVISOR_REF:
            gateway.supervisor_calls += 1  # type: ignore[attr-defined]
        return FakeChatModel(
            responses=gateway.responses, calls=gateway.calls, ref=ref, credential=credential
        )

    gateway = ModelGateway(factories={"deepseek": factory})
    gateway.calls = []  # type: ignore[attr-defined]
    gateway.responses = []  # type: ignore[attr-defined]
    gateway.supervisor_calls = 0  # type: ignore[attr-defined]
    return gateway


# ---------------------------------------------------------------------------
# 状态构建与运行辅助
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


@pytest.fixture
def group_runner(fake_gateway: ModelGateway):
    """跑完整轮群聊并回放事件；supervisor_ref 可覆盖（None=无默认模型）。"""

    async def run(
        text: str, *, supervisor_ref: ModelRef | None = SUPERVISOR_REF
    ) -> list[OrchestrationEvent]:
        collected: list[OrchestrationEvent] = []
        credentials = {m.agent_id: ModelCredential(api_key=SECRET) for m in GROUP_MEMBERS}
        if supervisor_ref is not None:
            credentials[supervisor_ref.name] = ModelCredential(api_key=SECRET)
        context = RuntimeContext(
            model_gateway=fake_gateway,
            credentials_by_model_ref=credentials,
            default_model_ref=supervisor_ref,
            event_sink=collected.append,
        )
        state: ConversationState = ConversationState(
            run_id="run-group",
            session_id="session-group",
            session_type="group",
            user_turn=UserTurnSnapshot(message_id="m-user", text=text).model_dump(),
            history_snapshot=[],
            agent_snapshots=[m.model_dump() for m in GROUP_MEMBERS],
            memory_snapshots=[],
            directive=None,
            speaking_plan=None,
            pending_agent_ids=[m.agent_id for m in GROUP_MEMBERS],
            replies=[],
            memory_proposals=[],
            outcome=None,
        )
        await build_group_graph(build_daoist_graph()).compile().ainvoke(
            dict(state), context=context
        )
        return collected

    return run


def events_of(events: list[OrchestrationEvent], name: str) -> list[OrchestrationEvent]:
    return [e for e in events if e.name == name]


def final_pairs(events: list[OrchestrationEvent]) -> list[tuple[str, str]]:
    """assistant_final 的 (agent_id, text)，按事件顺序 = 发言顺序。"""
    return [
        (str(e.payload["agent_id"]), str(e.payload["text"]).strip())
        for e in events_of(events, "assistant_final")
    ]


def final_replies(events: list[OrchestrationEvent]) -> list[AgentReply]:
    # assistant_final 负载无 run_id（在事件顶层）；补上以还原完整契约。
    return [
        AgentReply.model_validate({**e.payload, "run_id": e.run_id})
        for e in events_of(events, "assistant_final")
    ]


# ---------------------------------------------------------------------------
# 测试
# ---------------------------------------------------------------------------


@pytest.mark.asyncio
async def test_roll_call_never_calls_supervisor(group_runner, fake_gateway):
    fake_gateway.responses.extend(["1", "2", "3", "4"])
    events = await group_runner("@全体成员 全体都有！报数！")

    # 确定性报数：全程不建 Supervisor 模型，也不产出任何模型计划。
    assert fake_gateway.supervisor_calls == 0
    assert events_of(events, "plan_created")[0].payload["source"] == "deterministic"
    # 恰好每名成员被调用一次：无重试、无重复发言人、无重复终稿。
    assert len(fake_gateway.calls) == 4
    assert final_pairs(events) == [
        ("zhang", "1"), ("li", "2"), ("jia", "3"), ("shen", "4")
    ]
    assert len(final_replies(events)) == 4
    assert len({r.reply_id for r in final_replies(events)}) == 4


@pytest.mark.asyncio
async def test_open_discussion_uses_supervisor(group_runner, fake_gateway):
    plan = json.dumps(
        {
            "items": [{"agent_id": "zhang"}, {"agent_id": "li"}],
            "reason": "让主咖先起头，李雪琴接梗",
        }
    )
    fake_gateway.responses.extend([plan, "我先说说我的看法", "我补充一点"])
    events = await group_runner("你们怎么看这个选择？")

    assert fake_gateway.supervisor_calls == 1
    assert events_of(events, "plan_created")[0].payload["source"] == "supervisor"
    # 事件序：导演计划先于一切发言人产物；随后按计划顺序逐道人发言。
    names = [e.name for e in events]
    assert names.index("plan_created") < names.index("speaker_started")
    assert final_pairs(events) == [("zhang", "我先说说我的看法"), ("li", "我补充一点")]


@pytest.mark.asyncio
async def test_invalid_supervisor_output_falls_back_to_primary(group_runner, fake_gateway):
    fake_gateway.responses.extend([GARBAGE_OUTPUT, "收到"])
    events = await group_runner("你们怎么看这个选择？")

    # Supervisor 仍被调用一次，但输出无效 → 确定性回退到主成员（不失败整轮）。
    assert fake_gateway.supervisor_calls == 1
    assert events_of(events, "plan_created")[0].payload["source"] == "fallback"
    assert final_pairs(events) == [("zhang", "收到")]


@pytest.mark.asyncio
@pytest.mark.parametrize("supervisor_ref", [None, ModelRef(provider_type="acme", name="acme-x")])
async def test_unavailable_default_model_falls_back_without_model_call(
    group_runner, fake_gateway, supervisor_ref
):
    fake_gateway.responses.append("主成员兜底回复")
    events = await group_runner("你们怎么看这个选择？", supervisor_ref=supervisor_ref)

    # 无默认模型 / 默认模型无适配器：连 Supervisor 调用都不发生，直接确定性回退。
    assert fake_gateway.supervisor_calls == 0
    assert events_of(events, "plan_created")[0].payload["source"] == "fallback"
    assert final_pairs(events) == [("zhang", "主成员兜底回复")]


@pytest.mark.asyncio
async def test_one_daoist_failure_does_not_block_remaining(group_runner, fake_gateway):
    fake_gateway.responses.extend(
        ["1", RuntimeError("模型超时"), "3", "4"]
    )
    events = await group_runner("@全体成员 全体都有！报数！")

    # li 的模型调用失败：其余计划发言人照常发言，整轮不崩。
    assert fake_gateway.supervisor_calls == 0
    assert final_pairs(events) == [("zhang", "1"), ("jia", "3"), ("shen", "4")]


@pytest.mark.asyncio
@pytest.mark.parametrize("text", ["@张雪峰 停", "继续聊"])
async def test_stop_and_continue_produce_no_speakers(group_runner, fake_gateway, text):
    events = await group_runner(text)

    # 停/继续是回合级控制指令：不产出计划、不调用任何模型。
    assert fake_gateway.supervisor_calls == 0
    assert fake_gateway.calls == []
    assert events == []
