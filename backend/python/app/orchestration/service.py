"""编排服务门面：向 API 层暴露 ConversationRuntime 的进程级单例面。

职责：
- 持有 ConversationRuntime 与模型网关（缺省官方注册表），组合成服务；
- 请求体 OrchestrationRunRequest = 契约 OrchestrationRequest + 传输层字段；
- start/resume 产出 OrchestrationEvent 流（async generator，端点转码 SSE）；
- cancel 同步幂等；ensure_resumable 预检续跑可达性——runtime.resume 是惰性
  async generator，错误要等首迭代才抛，HTTP 层已 200 无法改判 404，
  因此续跑端点必须先走本预检。

**default_model_ref seam（记录在案，Task 8 报告）**：
OrchestrationRunRequest 已能携带会话默认模型 ref（Supervisor 导演语义：
RuntimeContext.default_model_ref 就为此预留），但 OrchestrationRequest
（契约层，Task 2 定版）无此字段，runtime.start 的 RuntimeContext 组装
（Task 7 定版）也只透传契约字段——当前该 ref 无法注入运行上下文，群聊开放
讨论会确定性回退到主道人模型。运输层在此接受并解析（请求合法、可序列化），
收到非空值打 warning 便于察觉回退；真正的接线需契约层与 runtime 同步扩展
（Task 12 default_model 全量接线）。Supervisor 凭据不受影响：可按
credentials 键（= ref.name）随请求送达，不进本 seam。
"""

from __future__ import annotations

import logging
from pathlib import Path
from typing import AsyncIterator

from app.orchestration.contracts import ModelRef, OrchestrationRequest
from app.orchestration.events import OrchestrationEvent
from app.orchestration.model_gateway import ModelGateway
from app.orchestration.runtime import ConversationRuntime, RunNotFoundError

logger = logging.getLogger(__name__)

#: interrupted 语义（runtime 模块常量；此处避免引私有名，只做语义比对）。
_RESUMABLE_OUTCOME = "interrupted"


class OrchestrationRunRequest(OrchestrationRequest):
    """内部编排 API 请求体：契约请求 + 传输层补充字段。

    default_model_ref：会话「当前配置的默认模型」引用（Supervisor 导演用），
    见模块 docstring 的 seam 记录——运输层接受并解析，消费在后续契约扩展。
    """

    default_model_ref: ModelRef | None = None

    def to_runtime_request(self) -> OrchestrationRequest:
        """剥掉传输层字段，落成运行器/契约层的 OrchestrationRequest。"""
        data = self.model_dump(exclude={"default_model_ref"})
        return OrchestrationRequest.model_validate(data)


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
        """开始一次新 run。default_model_ref seam：收到即记录，契约扩展前回退。"""
        if run_request.default_model_ref is not None:
            logger.warning(
                "default_model_ref 已收到但契约层尚不透传 (run_id=%s, ref=%s)——"
                "开放讨论将回退到主道人模型；Task 12 契约扩展接线",
                run_request.run_id,
                run_request.default_model_ref.name,
            )
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
