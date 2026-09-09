"""编排服务门面：向 API 层暴露 ConversationRuntime 的进程级单例面。

职责：
- 持有 ConversationRuntime 与模型网关（缺省官方注册表），组合成服务；
- 请求体 OrchestrationRunRequest = 契约 OrchestrationRequest + 传输层字段；
- start/resume 产出 OrchestrationEvent 流（async generator，端点转码 SSE）；
- cancel 同步幂等；ensure_resumable 预检续跑可达性——runtime.resume 是惰性
  async generator，错误要等首迭代才抛，HTTP 层已 200 无法改判 404，
  因此续跑端点必须先走本预检。

default_model_ref 是正式契约字段，由运行器注入 RuntimeContext，供每轮语义
理解和群聊 Supervisor 共用；它不进入图状态或检查点。
"""

from __future__ import annotations

from pathlib import Path
from typing import AsyncIterator

from app.orchestration.contracts import OrchestrationRequest
from app.orchestration.events import OrchestrationEvent
from app.orchestration.model_gateway import ModelGateway
from app.orchestration.runtime import ConversationRuntime, RunNotFoundError

#: interrupted 语义（runtime 模块常量；此处避免引私有名，只做语义比对）。
_RESUMABLE_OUTCOME = "interrupted"


class OrchestrationRunRequest(OrchestrationRequest):
    """内部编排 API 请求体；保留独立类型以稳定端点签名。"""

    def to_runtime_request(self) -> OrchestrationRequest:
        """转成运行器契约，完整保留默认模型引用。"""
        return OrchestrationRequest.model_validate(self.model_dump())


class OrchestrationService:
    """编排服务：一个进程一个实例（API 层惰性单例；测试可注入假件/真实件）。

    start/resume 与 ConversationRuntime 同签名同语义（async generator 逐条
    产出事件到终态收尾）；cancel 幂等同步；ensure_resumable 为端点 404 预检。
    """

    def __init__(
        self,
        model_gateway: ModelGateway | None = None,
        checkpoint_path: Path | None = None,
    ) -> None:
        self._gateway = model_gateway if model_gateway is not None else ModelGateway()
        self._runtime = ConversationRuntime(
            model_gateway=self._gateway,
            checkpoint_path=checkpoint_path,
        )

    async def start(self, run_request: OrchestrationRunRequest) -> AsyncIterator[OrchestrationEvent]:
        """开始一次新 run。"""
        async for event in self._runtime.start(run_request.to_runtime_request()):
            yield event

    async def resume(self, run_id: str) -> AsyncIterator[OrchestrationEvent]:
        """续跑 interrupted run（须先过 ensure_resumable，见模块 docstring）。"""
        async for event in self._runtime.resume(run_id):
            yield event

    async def ensure_resumable(self, run_id: str) -> None:
        """预检 run 是否可续跑（幂等）；未知/终态 → RunNotFoundError（端点 404）。"""
        state = await self._runtime.checkpoint_state(run_id)
        if state is None or state.get("outcome") != _RESUMABLE_OUTCOME:
            raise RunNotFoundError(f"run 不存在或没有可续跑的 interrupted 检查点: {run_id}")

    def cancel(self, run_id: str) -> None:
        """请求中断活跃 run（interrupted，可续跑）。幂等；未知 run 为 no-op。"""
        self._runtime.cancel(run_id)
