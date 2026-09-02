"""内部编排 SSE API 测试：POST /api/v1/orchestration/runs/*（Task 8）。

验证（设计文档 §8/§10）：
- 流式端点把 OrchestrationEvent 转码为 SSE（event:/data:），事件序稳定
  （锚点 test_stream_emits_typed_sse_in_order）；
- 请求体校验（Pydantic 422）；意外异常只暴露稳定码 internal_error，
  异常文本/凭据细节只进服务端日志；
- cancel/resume 端点语义：cancel 幂等记录、resume 未知 run → 404 稳定码；
- 客户端断开把取消传播给运行器（interrupted 语义，可续跑）；
- default_model_ref seam：运输层接受会话默认模型 ref（Supervisor 导演用），
  消费缺口记录在 service 层 docstring（契约扩展前不静默丢字段）；
- 真实 ConversationRuntime + 假模型网关的 HTTP 端到端：报数全链路零泄露、
  流中 /cancel → run_interrupted → /resume 差集续跑（无重复终稿/无幽灵重跑）。
"""

import asyncio
import json
from dataclasses import dataclass
from typing import Any

import pytest
from fastapi.testclient import TestClient
from langchain_core.language_models.chat_models import BaseChatModel
from langchain_core.messages import AIMessage, BaseMessage

from app.main import app
from app.orchestration.contracts import ModelCredential, ModelRef
from app.orchestration.events import OrchestrationEvent
from app.orchestration.model_gateway import ModelGateway
from app.orchestration.runtime import RunNotFoundError
from app.orchestration.service import OrchestrationRunRequest, OrchestrationService

SECRET = "sk-secret"

STREAM_URL = "/api/v1/orchestration/runs/stream"
CANCEL_URL = "/api/v1/orchestration/runs/{run_id}/cancel"
RESUME_URL = "/api/v1/orchestration/runs/{run_id}/resume"

#: 报数确定性指令（与 Task 7 锚点同文案：逐发言人按序报数，不调 Supervisor）。
ROLL_CALL_TEXT = "@全体成员 全体都有！报数！"
ROLL_CALL_AGENTS = [
    ("zhang", "张雪峰"),
    ("li", "李雪琴"),
    ("jia", "贾玲"),
    ("shen", "沈腾"),
]


# ---------------------------------------------------------------------------
# 替身运行器（端点依赖注入目标）
# ---------------------------------------------------------------------------


class FakeRuntime:
    """端点依赖的脚本化替身：事件序列 + 调用记录（挂在测试线程可见的列表）。

    协议镜像 OrchestrationService 的端点面：start/resume 为 async generator，
    ensure_resumable 预检续跑可达性（RunNotFoundError=404），cancel 幂等同步。
    """

    def __init__(self) -> None:
        self.stream_events: list[OrchestrationEvent] = []
        self.resume_events: list[OrchestrationEvent] = []
        self.start_calls: list[Any] = []
        self.resume_calls: list[str] = []
        self.ensure_calls: list[str] = []
        self.cancel_calls: list[str] = []
        self.raise_on_stream: Exception | None = None
        self.hold_after_events: bool = False
        self.ghost_runs: set[str] = {"run-ghost"}

    async def start(self, run_request: Any) -> Any:
        self.start_calls.append(run_request)
        if self.raise_on_stream is not None:
            raise self.raise_on_stream
        for event in self.stream_events:
            yield event
        while self.hold_after_events:
            await asyncio.sleep(3600)

    async def resume(self, run_id: str) -> Any:
        self.resume_calls.append(run_id)
        await self._ensure(run_id)
        for event in self.resume_events:
            yield event

    async def ensure_resumable(self, run_id: str) -> None:
        self.ensure_calls.append(run_id)
        await self._ensure(run_id)

    async def _ensure(self, run_id: str) -> None:
        if run_id in self.ghost_runs:
            raise RunNotFoundError(f"run 不存在或不可续跑: {run_id}")

    def cancel(self, run_id: str) -> None:
        self.cancel_calls.append(run_id)


def make_event(name: str, run_id: str = "run-http", **payload: Any) -> OrchestrationEvent:
    return OrchestrationEvent(run_id=run_id, name=name, payload=payload)


def single_speaker_script(run_id: str = "run-http") -> list[OrchestrationEvent]:
    """最小确定性脚本：run_started → plan_created → 发言人链路 → run_completed。"""
    return [
        make_event("run_started", run_id=run_id),
        make_event("plan_created", run_id=run_id, source="deterministic", plan={}),
        make_event("speaker_started", run_id=run_id, agent_id="zhang", task=None),
        make_event("assistant_delta", run_id=run_id, agent_id="zhang", text="1"),
        make_event("assistant_final", run_id=run_id, agent_id="zhang", reply_id="rep-1", text="1"),
        make_event("run_completed", run_id=run_id),
    ]


@pytest.fixture
def fake_runtime() -> FakeRuntime:
    runtime = FakeRuntime()
    runtime.stream_events = single_speaker_script()
    runtime.resume_events = [
        make_event("run_started"),
        make_event("speaker_started", agent_id="zhang", task=None),
        make_event("assistant_delta", agent_id="zhang", text="1"),
        make_event("assistant_final", agent_id="zhang", reply_id="rep-1", text="1"),
        make_event("run_completed"),
    ]
    return runtime


@pytest.fixture
def client(fake_runtime: FakeRuntime):
    """端点依赖替换为替身运行器；每测试独立 TestClient，收尾清 overrides。"""
    from app.api.orchestration import get_orchestration_service

    app.dependency_overrides[get_orchestration_service] = lambda: fake_runtime
    with TestClient(app) as test_client:
        yield test_client
    app.dependency_overrides.clear()


# ---------------------------------------------------------------------------
# 请求体与 SSE 解析辅助
# ---------------------------------------------------------------------------


def roll_call_body(run_id: str = "run-http", session_id: str = "session-http") -> dict[str, Any]:
    members = [
        {
            "agent_id": agent_id,
            "name": name,
            "model_ref": {"provider_type": "deepseek", "name": "deepseek-chat"},
        }
        for agent_id, name in ROLL_CALL_AGENTS
    ]
    return {
        "run_id": run_id,
        "session_id": session_id,
        "session_type": "group",
        "user_turn": {"message_id": "m-http", "text": ROLL_CALL_TEXT},
        "history_snapshot": [],
        "agent_snapshots": members,
        "memory_snapshots": [],
        "credentials": {m["agent_id"]: {"api_key": SECRET} for m in members},
        "debug_enabled": False,
    }


def parse_sse_blocks(text: str) -> list[tuple[str, dict[str, Any]]]:
    """把整段 SSE 文本解析为 (event_name, payload) 序列。"""
    blocks: list[tuple[str, dict[str, Any]]] = []
    for block in text.split("\n\n"):
        name = ""
        payload: dict[str, Any] = {}
        for line in block.splitlines():
            if line.startswith("event: "):
                name = line[len("event: "):]
            elif line.startswith("data: "):
                payload = json.loads(line[len("data: "):])
        if name:
            blocks.append((name, payload))
    return blocks


def event_names(blocks: list[tuple[str, dict[str, Any]]]) -> list[str]:
    return [name for name, _ in blocks]


def finals(blocks: list[tuple[str, dict[str, Any]]]) -> list[dict[str, Any]]:
    """assistant_final 负载序列（保序）。"""
    return [payload for name, payload in blocks if name == "assistant_final"]


def reply_ids(blocks: list[tuple[str, dict[str, Any]]]) -> set[str]:
    return {p["reply_id"] for p in finals(blocks)}


# ---------------------------------------------------------------------------
# 测试：端点语义（替身运行器）
# ---------------------------------------------------------------------------


def test_stream_emits_typed_sse_in_order(client: TestClient):
    """锚点：事件序稳定（run_started → plan_created → speaker → run_completed）。"""
    resp = client.post(STREAM_URL, json=roll_call_body())
    assert resp.status_code == 200
    assert resp.headers["content-type"].startswith("text/event-stream")
    assert resp.headers["cache-control"] == "no-cache"
    assert resp.headers["x-accel-buffering"] == "no"
    assert event_names(parse_sse_blocks(resp.text)) == [
        "run_started", "plan_created", "speaker_started",
        "assistant_delta", "assistant_final", "run_completed",
    ]


def test_stream_rejects_malformed_body(client: TestClient, fake_runtime: FakeRuntime):
    """Pydantic 拒绝：缺必填字段/凭据类型错 → 422（不触碰运行器）。"""
    missing = roll_call_body()
    del missing["run_id"]
    resp = client.post(STREAM_URL, json=missing)
    assert resp.status_code == 422
    assert fake_runtime.start_calls == []

    bad_credentials = roll_call_body()
    bad_credentials["credentials"] = {"zhang": "not-an-object"}
    resp = client.post(STREAM_URL, json=bad_credentials)
    assert resp.status_code == 422


def test_stream_accepts_default_model_ref_seam(client: TestClient, fake_runtime: FakeRuntime):
    """default_model_ref seam：运输层接受并解析会话默认模型（Supervisor 导演用）。

    消费缺口（RuntimeContext 注入）记录在 service 层 docstring——契约扩展前
    不得静默丢字段，收到即打 warning 便于察觉回退。
    """
    body = roll_call_body()
    body["default_model_ref"] = {"provider_type": "deepseek", "name": "deepseek-reasoner"}
    resp = client.post(STREAM_URL, json=body)
    assert resp.status_code == 200
    received = fake_runtime.start_calls[0]
    assert isinstance(received, OrchestrationRunRequest)
    assert received.default_model_ref == ModelRef(provider_type="deepseek", name="deepseek-reasoner")


def test_stream_sanitizes_unexpected_errors(client: TestClient, fake_runtime: FakeRuntime):
    """意外异常只暴露稳定码：异常文本/凭据不进响应，细节进服务端日志。"""
    fake_runtime.raise_on_stream = RuntimeError(f"provider boom {SECRET}")

    resp = client.post(STREAM_URL, json=roll_call_body())

    assert resp.status_code == 200
    names = event_names(parse_sse_blocks(resp.text))
    assert names == ["run_error"]
    assert '"error": "internal_error"' in resp.text
    assert SECRET not in resp.text
    assert "boom" not in resp.text


def test_cancel_records_and_is_idempotent(client: TestClient, fake_runtime: FakeRuntime):
    resp = client.post(CANCEL_URL.format(run_id="run-http"))
    assert resp.status_code == 200
    assert resp.json()["code"] == 0
    assert fake_runtime.cancel_calls == ["run-http"]

    # 未知 run：运行器语义为幂等 no-op，端点照常成功返回（Go 侧无需分支）。
    resp = client.post(CANCEL_URL.format(run_id="run-ghost"))
    assert resp.status_code == 200
    assert fake_runtime.cancel_calls == ["run-http", "run-ghost"]


def test_resume_unknown_run_returns_stable_404(client: TestClient, fake_runtime: FakeRuntime):
    """resume 未知/不可续跑 run：预检即 404，静态稳定码，无泄漏文本。"""
    resp = client.post(RESUME_URL.format(run_id="run-ghost"))
    assert resp.status_code == 404
    body = resp.json()
    assert body["code"] == "run_not_found"
    assert "run-ghost" not in json.dumps(body["message"])
    assert fake_runtime.ensure_calls == ["run-ghost"]
    assert fake_runtime.resume_calls == []


def test_resume_streams_from_interrupted_run(client: TestClient, fake_runtime: FakeRuntime):
    resp = client.post(RESUME_URL.format(run_id="run-http"))
    assert resp.status_code == 200
    assert resp.headers["content-type"].startswith("text/event-stream")
    assert event_names(parse_sse_blocks(resp.text)) == [
        "run_started", "speaker_started", "assistant_delta",
        "assistant_final", "run_completed",
    ]
    assert fake_runtime.resume_calls == ["run-http"]


@pytest.mark.asyncio
async def test_client_disconnect_cancels_run(fake_runtime: FakeRuntime):
    """客户端断开传播：SSE 流被提前关闭 → 端点把取消传给运行器（可续跑）。

    在端点 seam 上验证（HTTP 断开 → 生成器关闭是 Starlette 的契约，本测试
    验证我们的传播逻辑「关闭即取消」）：直接调 stream_run 处理器取
    body_iterator，消费首个事件后 aclose()，断言取消已传入运行器。

    不走 TestClient HTTP 层的记录在案偏差：starlette 1.6 的 httpx 版
    TestClient 在客户端提前关闭响应时不向 app 投递 http.disconnect，服务端
    生成器永不被取消 → TestClient 门户关闭会永远等待服务端任务（挂起），
    无法作为可执行测试；传播逻辑因此移到端点 seam 直接验证。
    """
    from app.api.orchestration import stream_run
    from app.orchestration.service import OrchestrationRunRequest

    fake_runtime.hold_after_events = True
    run_request = OrchestrationRunRequest.model_validate(roll_call_body())
    response = await stream_run(run_request, service=fake_runtime)
    first = await anext(response.body_iterator)
    assert first.startswith("event: run_started")
    await response.body_iterator.aclose()
    assert fake_runtime.cancel_calls == ["run-http"]


# ---------------------------------------------------------------------------
# 真实 ConversationRuntime 端到端（假模型网关 + 每测试独立检查点）
# ---------------------------------------------------------------------------


@dataclass
class FakeCall:
    """一次模型调用记录：入参消息、模型引用与运行期凭据。"""

    messages: list[BaseMessage]
    model_ref: ModelRef
    credential: ModelCredential


class SlowFakeChatModel(BaseChatModel):
    """async ainvoke 假模型：每次调用睡 30ms 让出事件循环。

    HTTP 测试的消费方在独立线程（TestClient portal），不像同协程测试那样能
    精确同步在事件间取消——30ms 的真实让出给 /cancel 请求留出往返窗口，保证
    取消落在发言人边界（中断前终稿 ∈ {2,3}，差集断言对任意切分 robust）。
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
        await asyncio.sleep(0.03)
        self.calls.append(
            FakeCall(messages=list(messages), model_ref=self.ref, credential=self.credential)
        )
        return AIMessage(content=self.responses.pop(0))


@pytest.fixture
def fake_gateway() -> ModelGateway:
    """注入 deepseek 工厂的网关；调用记录挂 .calls，响应队列在 .responses。"""

    def factory(ref: ModelRef, credential: ModelCredential) -> SlowFakeChatModel:
        return SlowFakeChatModel(
            responses=gateway.responses, calls=gateway.calls, ref=ref, credential=credential
        )

    gateway = ModelGateway(factories={"deepseek": factory})
    gateway.calls = []  # type: ignore[attr-defined]
    gateway.responses = []  # type: ignore[attr-defined]
    return gateway


@pytest.fixture
def real_service(fake_gateway: ModelGateway, tmp_path) -> OrchestrationService:
    """真实 OrchestrationService：假模型网关 + 每测试独立检查点目录。"""
    return OrchestrationService(
        model_gateway=fake_gateway,
        checkpoint_path=tmp_path / "orchestration.sqlite",
    )


@pytest.fixture
def real_client(real_service: OrchestrationService, fake_gateway: ModelGateway):
    """HTTP 依赖替换为真实服务；仅用于完整流（无中途动作）的端到端。"""
    from app.api.orchestration import get_orchestration_service

    app.dependency_overrides[get_orchestration_service] = lambda: real_service
    with TestClient(app) as test_client:
        yield test_client, fake_gateway
    app.dependency_overrides.clear()


def test_real_stream_roll_call_over_http(real_client):
    """真实运行器 HTTP 端到端：确定性报数全链路、凭据送达、响应零泄露。"""
    client, fake_gateway = real_client
    fake_gateway.responses.extend(["1", "2", "3", "4"])

    resp = client.post(STREAM_URL, json=roll_call_body())

    assert resp.status_code == 200
    assert resp.headers["content-type"].startswith("text/event-stream")
    blocks = parse_sse_blocks(resp.text)
    names = event_names(blocks)
    assert names[0] == "run_started"
    assert names.index("plan_created") < names.index("speaker_started")
    assert events_of(names, "run_interrupted") == []
    assert events_of(names, "run_error") == []
    assert [p["agent_id"] for p in finals(blocks)] == [
        a for a, _ in ROLL_CALL_AGENTS
    ]
    assert all(isinstance(p["text"], str) and p["text"] for p in finals(blocks))
    assert names[-1] == "run_completed"
    assert len(fake_gateway.calls) == len(ROLL_CALL_AGENTS)
    assert {c.credential.api_key for c in fake_gateway.calls} == {SECRET}
    assert SECRET not in resp.text
    assert "api_key" not in resp.text


@pytest.mark.asyncio
async def test_real_cancel_and_resume_over_seam(real_service, fake_gateway):
    """真实链路（seam 级）：流中取消 → run_interrupted → resume 差集续跑。

    中断落在发言人边界：先段终稿是成员序列的前缀（2 或 3 条），续跑只跑
    剩余后缀——reply_id 两段不相交（无重复终稿）、并集恰好 4 人（无幽灵
    回复）、总模型调用 == 4（无幽灵重跑）、凭据全程零泄漏。

    记录在案偏差（与断连测试同源）：starlette 1.6 TestClient 的流式响应
    是缓冲投递（事件要等服务端跑完才到达客户端），HTTP 层无法执行「流中
    经 /cancel 取消」——探针实测 cancel 永远落在 run 完成之后成为 no-op。
    因此真实运行器的取消/续跑链路在端点 seam 上验证（同一 service 面），
    HTTP 传输层由完整流测试 test_real_stream_roll_call_over_http 覆盖。
    """
    from app.orchestration.service import OrchestrationRunRequest

    fake_gateway.responses.extend(["1", "2", "3", "4"])
    run_id = "run-http-interrupted"
    members = [a for a, _ in ROLL_CALL_AGENTS]
    run_request = OrchestrationRunRequest.model_validate(roll_call_body(run_id=run_id))

    # 阶段 1：消费到第 2 条 assistant_final 后取消，读至中断收尾。
    first_blocks: list[tuple[str, dict[str, Any]]] = []
    cancel_posted = False
    async for event in real_service.start(run_request):
        first_blocks.append((event.name, event.payload))
        if not cancel_posted and event.name == "assistant_final":
            if len(finals(first_blocks)) == 2:
                real_service.cancel(run_id)
                cancel_posted = True
        if cancel_posted and event.name in ("run_interrupted", "run_completed", "run_error"):
            break

    first_finals = finals(first_blocks)
    assert first_blocks[-1][0] == "run_interrupted", first_blocks[-1]
    assert first_blocks[-1][1].get("reason") == "interrupted"
    assert len(first_finals) in (2, 3)
    assert [p["agent_id"] for p in first_finals] == members[: len(first_finals)]
    assert events_of(event_names(first_blocks), "run_error") == []

    # 阶段 2：resume —— 只跑未完成发言人；计划已在检查点，不重发。
    resumed_blocks: list[tuple[str, dict[str, Any]]] = []
    async for event in real_service.resume(run_id):
        resumed_blocks.append((event.name, event.payload))
    resumed_names = event_names(resumed_blocks)
    assert resumed_names[0] == "run_started"
    assert "plan_created" not in resumed_names
    resumed_finals = finals(resumed_blocks)
    assert [p["agent_id"] for p in resumed_finals] == members[len(first_finals):]
    assert resumed_names[-1] == "run_completed"
    assert reply_ids(first_blocks).isdisjoint(reply_ids(resumed_blocks))
    assert len(reply_ids(first_blocks) | reply_ids(resumed_blocks)) == len(members)
    assert len(fake_gateway.calls) == len(members)
    assert {c.credential.api_key for c in fake_gateway.calls} == {SECRET}
    dumped = json.dumps([p for _, p in first_blocks + resumed_blocks], ensure_ascii=False)
    assert SECRET not in dumped
    assert "api_key" not in dumped

    # 阶段 3：run 已终态收尾 → 预检 404 稳定码（可续跑点已清理）。
    with pytest.raises(RunNotFoundError):
        await real_service.ensure_resumable(run_id)


def events_of(names: list[str], name: str) -> list[str]:
    return [n for n in names if n == name]
