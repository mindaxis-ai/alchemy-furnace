"""ConversationRuntime：LangGraph 会话编排的进程级运行器与检查点生命周期。

职责（设计文档 §4/§7/§10）：
- 编译 ConversationGraph（AsyncSqliteSaver 本地异步 SQLite 检查点）；
- start(request)：一次用户轮的新 run；
- resume(run_id)：从 interrupted run 的检查点重建输入，在新线程代次重放；
- cancel(run_id)：同步置旗标，图在发言人边界收敛为 interrupted；
- 事件：run_started 在每次流开头；子图事件（speaker_started/delta/final、
  plan_created…）直通；终态 run_completed / run_interrupted(reason) /
  run_error 由运行时收尾（每 run 一个 asyncio.Queue，sentinel 后排干）。

**线程模型与设计偏差（probe 验证，Task 7 报告记录）**：
计划原文「use session UUID as thread_id and run_id as checkpoint namespace」
在 langgraph 0.6.11 不可行——顶层运行总是写入 checkpoint_ns=''（namespace
配置被忽略），且同一 thread 的第二次 ainvoke 会重放嵌套子图步骤、把 reducer
通道值重复合并（幽灵回复）。因此：
- 线程 id 采用会话派生的复合代次：``f"{session_id}:{run_id}"``（gen 0）、
  续跑追加 ``:{gen}``（gen 1、2…）；同一 run 的所有代次共用前缀分区；
- **每个线程恰好 ainvoke 一次**：resume 永远是新代次线程。顶层检查点携带
  「计划 + 已完成回复」，重放时子图在差集上裁决（见 graphs/group.py 守卫）；
- 进程重启后的恢复：Go 侧重启引擎后调用方扫描 ``alist(None)`` 中
  ``<session>:<run>`` 形态的线程前缀重建 run 清单（Task 9 接线，本模块
  resume 以进程内注册表为准，interrupted 检查点保留在库中）。

终态清理：completed/failed/cancelled 到达即删除该 run 全部代次线程并弹出
注册表；interrupted 保留（可续跑）。取消语义：start 取代同会话活跃旧 run
（reason=cancelled，终态）；cancel() 请求中断（reason=interrupted，可续跑）。

凭据约定：request.credentials 只进 RuntimeContext（每代次记录内存保留，
供 resume 复用），永不进状态/检查点/事件——test_checkpoint_state_contains
_no_api_key 守护此不变量。
"""

from __future__ import annotations

import asyncio
from dataclasses import dataclass
from pathlib import Path
from typing import Any, AsyncIterator

from langgraph.checkpoint.sqlite.aio import AsyncSqliteSaver

from app.core.config import settings
from app.orchestration.contracts import OrchestrationRequest, RuntimeContext
from app.orchestration.events import OrchestrationEvent
from app.orchestration.graphs.conversation import build_conversation_graph
from app.orchestration.model_gateway import ModelGateway

#: 事件队列排干哨兵（_drain 见到即返回，不当作事件投给消费方）。
_DONE: object = object()

#: 取消 reason 合法值：interrupted=用户停止（可续跑）；cancelled=被新轮取代（终态）。
_INTERRUPTED = "interrupted"
_CANCELLED = "cancelled"

#: 终态 outcome：到达即清理线程与注册表记录。
_TERMINAL_OUTCOMES = ("completed", "failed", "cancelled")


class CancellationToken:
    """发言人间可见的同步取消旗标（随 RuntimeContext 注入，不落检查点）。

    语义：图在安全边界（dispatch/speak 迭代起点）轮询 is_cancelled；
    reason 决定收敛 outcome——"interrupted"（用户停止，可续跑）或
    "cancelled"（被新轮取代，终态）。首次 cancel 生效，后续调用为 no-op
    （先到者为准，恢复代次用新 token 重新武装）。
    """

    def __init__(self) -> None:
        self._reason: str | None = None

    @property
    def is_cancelled(self) -> bool:
        return self._reason is not None

    @property
    def reason(self) -> str | None:
        return self._reason

    def cancel(self, reason: str) -> None:
        if self._reason is None:
            self._reason = reason


class RunNotFoundError(LookupError):
    """resume 目标不可达：run 不存在，或没有可续跑的 interrupted 检查点。"""


@dataclass
class _RunRecord:
    """一个 run 的进程内登记：代次线程清单、取消旗标、驱动任务与凭据副本。"""

    run_id: str
    session_id: str
    thread_ids: list[str]
    token: CancellationToken
    credentials: dict[str, Any]
    debug_enabled: bool
    task: asyncio.Task | None = None


def _thread_id(session_id: str, run_id: str, gen: int) -> str:
    """代次线程 id：gen 0 无后缀，续跑追加 :N（见模块 docstring 偏差记录）。"""
    return f"{session_id}:{run_id}" if gen == 0 else f"{session_id}:{run_id}:{gen}"


class ConversationRuntime:
    """会话编排运行器。一个进程一个实例；服务停止即丢失活跃 run 的内存态。"""

    def __init__(
        self,
        model_gateway: ModelGateway,
        checkpoint_path: Path | None = None,
    ) -> None:
        self._model_gateway = model_gateway
        self._checkpoint_path = Path(
            checkpoint_path if checkpoint_path is not None else settings.orchestration_checkpoint_path
        )
        # from_conn_string 是 @asynccontextmanager：CM 的 asyncgen 帧持有 aiosqlite
        # 连接。不能只取 __aenter__ 结果就丢 CM——asyncgen 被 GC 时事件循环会
        # aclose() 它，把连接关掉（报 "Cannot operate on a closed database"）。
        # CM 必须与实例同生命周期，aclose 时走它的 __aexit__。
        self._saver_cm: Any = None
        self._saver: AsyncSqliteSaver | None = None
        self._graph: Any = None
        self._runs: dict[str, _RunRecord] = {}

    # ------------------------------------------------------------------
    # 检查点库 / 图生命周期
    # ------------------------------------------------------------------

    async def _ensure_saver(self) -> None:
        """惰性打开进程级 AsyncSqliteSaver 并编译带检查点的会话图（幂等）。"""
        if self._saver is not None:
            return
        self._checkpoint_path.parent.mkdir(parents=True, exist_ok=True)
        self._saver_cm = AsyncSqliteSaver.from_conn_string(str(self._checkpoint_path))
        self._saver = await self._saver_cm.__aenter__()
        await self._saver.setup()
        self._graph = build_conversation_graph().compile(checkpointer=self._saver)

    async def aclose(self) -> None:
        """关闭检查点库（进程退出/引擎关停时调用）。"""
        if self._saver_cm is not None:
            await self._saver_cm.__aexit__(None, None, None)
            self._saver_cm = None
            self._saver = None
            self._graph = None

    # ------------------------------------------------------------------
    # 生命周期：start / resume / cancel
    # ------------------------------------------------------------------

    async def start(
        self, request: OrchestrationRequest
    ) -> AsyncIterator[OrchestrationEvent]:
        """开始一次新 run（async generator：逐条产出事件到终态收尾）。

        同一会话仍在跑的旧 run 会被取代：旧 token 置 cancelled（终态语义，
        图在发言人边界收敛后由旧驱动清理）；interrupted 旧记录不受影响。
        """
        await self._ensure_saver()
        run_id, session_id = request.run_id, request.session_id
        for rec in list(self._runs.values()):
            if (
                rec.session_id == session_id
                and rec.run_id != run_id
                and rec.task is not None
                and not rec.task.done()
            ):
                rec.token.cancel(_CANCELLED)
        token = CancellationToken()
        thread = _thread_id(session_id, run_id, 0)
        rec = _RunRecord(
            run_id=run_id,
            session_id=session_id,
            thread_ids=[thread],
            token=token,
            credentials=dict(request.credentials),
            debug_enabled=request.debug_enabled,
        )
        self._runs[run_id] = rec
        queue: asyncio.Queue[Any] = asyncio.Queue()
        context = RuntimeContext(
            model_gateway=self._model_gateway,
            credentials_by_model_ref=request.credentials,
            debug_enabled=request.debug_enabled,
            event_sink=lambda event: queue.put_nowait(event),
            cancellation=token,
        )
        rec.task = asyncio.create_task(
            self._drive(
                run_id=run_id,
                rec=rec,
                queue=queue,
                thread=thread,
                initial_state=request.to_initial_state(),
                context=context,
            )
        )
        async for event in self._drain(queue):
            yield event

    async def resume(self, run_id: str) -> AsyncIterator[OrchestrationEvent]:
        """续跑一个 interrupted run：新线程代次重放，已完成发言人不重复。

        输入来自最新检查点值（含已完成回复/计划/pending——子图在差集上
        裁决）；outcome 复位 None 让收束节点重新裁决。新代次线程是空态，
        reducer 把历史值并入 = 干净重放（同线程二次 ainvoke 会损坏 reducer
        通道，见模块 docstring）。凭据复用记录里的副本（内存态；进程重启后
        由 Task 9 Go 侧重供）。
        """
        await self._ensure_saver()
        rec = self._runs.get(run_id)
        if rec is None:
            raise RunNotFoundError(f"run 不存在或已终态清理: {run_id}")
        values = await self._latest_values(rec.thread_ids[-1])
        if values is None or values.get("outcome") != _INTERRUPTED:
            raise RunNotFoundError(f"run 没有可续跑的 interrupted 检查点: {run_id}")
        gen = len(rec.thread_ids)
        thread = _thread_id(rec.session_id, run_id, gen)
        rec.thread_ids.append(thread)
        token = CancellationToken()
        rec.token = token
        queue: asyncio.Queue[Any] = asyncio.Queue()
        context = RuntimeContext(
            model_gateway=self._model_gateway,
            credentials_by_model_ref=rec.credentials,
            debug_enabled=rec.debug_enabled,
            event_sink=lambda event: queue.put_nowait(event),
            cancellation=token,
        )
        replay_state = dict(values)
        replay_state["outcome"] = None
        rec.task = asyncio.create_task(
            self._drive(
                run_id=run_id,
                rec=rec,
                queue=queue,
                thread=thread,
                initial_state=replay_state,
                context=context,
            )
        )
        async for event in self._drain(queue):
            yield event

    def cancel(self, run_id: str) -> None:
        """请求中断一个活跃 run（用户停止，可续跑）。幂等；未知 run 为 no-op。"""
        rec = self._runs.get(run_id)
        if rec is not None:
            rec.token.cancel(_INTERRUPTED)

    async def checkpoint_state(self, run_id: str) -> dict[str, Any] | None:
        """读 run 最新检查点状态值（未知/已终态清理 → None；凭据永不在此）。"""
        await self._ensure_saver()
        rec = self._runs.get(run_id)
        if rec is None:
            return None
        return await self._latest_values(rec.thread_ids[-1])

    # ------------------------------------------------------------------
    # 驱动与清理
    # ------------------------------------------------------------------

    async def _drive(
        self,
        run_id: str,
        rec: _RunRecord,
        queue: asyncio.Queue[Any],
        thread: str,
        initial_state: dict[str, Any],
        context: RuntimeContext,
    ) -> None:
        """跑一次图执行并收尾：终态事件 + 终态清理（进程内单一驱动点）。

        图在两种形态下结束：
        1. 收敛节点裁决过（outcome 非 None）——照用；
        2. 图没走到收敛就自然结束（如停/继续的空轮、取消落在规划前）——
           outcome 缺失时以旗标为准：已取消 → 旗标 reason，否则 completed。
        """
        try:
            queue.put_nowait(
                OrchestrationEvent(run_id=run_id, name="run_started", payload={})
            )
            result = await self._graph.ainvoke(
                dict(initial_state),
                config={"configurable": {"thread_id": thread}},
                context=context,
            )
            outcome = result.get("outcome")
            if outcome not in _TERMINAL_OUTCOMES:
                if context.cancellation.is_cancelled:
                    outcome = context.cancellation.reason
                else:
                    outcome = "completed"
            if outcome == _INTERRUPTED:
                queue.put_nowait(
                    OrchestrationEvent(
                        run_id=run_id,
                        name="run_interrupted",
                        payload={"reason": _INTERRUPTED},
                    )
                )
            elif outcome == _CANCELLED:
                queue.put_nowait(
                    OrchestrationEvent(
                        run_id=run_id,
                        name="run_interrupted",
                        payload={"reason": _CANCELLED},
                    )
                )
            else:
                queue.put_nowait(
                    OrchestrationEvent(run_id=run_id, name="run_completed", payload={})
                )
            if outcome in _TERMINAL_OUTCOMES:
                await self._cleanup_run(rec, run_id)
        except Exception as exc:
            # 只报异常类名：异常文本可能夹带模型响应/凭据细节，不进事件。
            queue.put_nowait(
                OrchestrationEvent(
                    run_id=run_id,
                    name="run_error",
                    payload={"error": type(exc).__name__},
                )
            )
            await self._cleanup_run(rec, run_id)
        finally:
            queue.put_nowait(_DONE)

    async def _cleanup_run(self, rec: _RunRecord, run_id: str) -> None:
        """终态清理：删除该 run 全部代次线程；注册表按身份弹出（防覆盖误删）。"""
        if self._saver is not None:
            for thread in rec.thread_ids:
                try:
                    await self._saver.adelete_thread(thread)
                except Exception:
                    # 尽力而为：删除失败只留孤儿行，不影响终态语义。
                    pass
        if self._runs.get(run_id) is rec:
            self._runs.pop(run_id, None)

    async def _latest_values(self, thread: str) -> dict[str, Any] | None:
        """读线程最新根检查点的合并状态值；无行/不可读 → None。"""
        try:
            snapshot = await self._graph.aget_state(
                {"configurable": {"thread_id": thread}}
            )
        except Exception:
            return None
        return snapshot.values or None

    @staticmethod
    async def _drain(queue: asyncio.Queue[Any]) -> AsyncIterator[OrchestrationEvent]:
        while True:
            item = await queue.get()
            if item is _DONE:
                return
            yield item
