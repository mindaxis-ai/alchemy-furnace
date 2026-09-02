"""内部编排 SSE API：暴露 ConversationRuntime 的流式面（Task 8）。

端点契约（Go 侧调用方在 Task 10 消费）：
- POST /api/v1/orchestration/runs/stream         — 发起一次新 run（SSE）
- POST /api/v1/orchestration/runs/{run_id}/resume — 续跑 interrupted run（SSE）
- POST /api/v1/orchestration/runs/{run_id}/cancel — 请求中断（interrupted，
  可续跑；同步幂等，未知 run 为 no-op，照常成功返回）

SSE 编码：每个 OrchestrationEvent 转码为一个块
``event: {name}\\ndata: {payload_json}\\n\\n``；禁缓冲头齐全
（Cache-Control: no-cache / X-Accel-Buffering: no，防代理/网关吞流）。

错误契约：请求体非法 → 422（FastAPI 校验）；未知/不可续跑 run → 404
静态稳定码 run_not_found；运行期异常 → 事件 run_error（只暴露稳定码
internal_error，异常文本/类名/凭据细节只进服务端日志——运行器层异常已
收敛为仅类名事件，越过运行器到达本层的属于端点契约内意外）。

断开传播：客户端提前断开 → 取消传入运行器（interrupted 语义，可续跑）。
终态事件（run_completed/run_interrupted/run_error）已流出的断开不再取消——
interrupted 之后可立即 resume，迟到的断开取消会误伤新代次的取消旗标
（runtime.resume 已换新 token）。
"""

from __future__ import annotations

import json
import logging
from typing import Any, AsyncIterator

from fastapi import APIRouter, Depends
from fastapi.responses import JSONResponse, StreamingResponse

from app.orchestration.events import OrchestrationEvent
from app.orchestration.runtime import RunNotFoundError
from app.orchestration.service import OrchestrationRunRequest, OrchestrationService

logger = logging.getLogger(__name__)

router = APIRouter(prefix="/orchestration", tags=["编排 - 内部"])

_SSE_HEADERS = {
    "Cache-Control": "no-cache",
    "Connection": "keep-alive",
    "X-Accel-Buffering": "no",
}

#: 终态事件：一旦流出，运行器的本轮生命周期即定，断开不再取消。
_TERMINAL_EVENT_NAMES = frozenset({"run_completed", "run_interrupted", "run_error"})

_RESUME_NOT_FOUND_BODY = {
    "code": "run_not_found",
    "message": "run 不存在或没有可续跑的 interrupted 检查点",
}

#: 进程级惰性单例（service 内封装官方网关 + 默认检查点路径）。
_service: OrchestrationService | None = None


def get_orchestration_service() -> OrchestrationService:
    """编排服务依赖（惰性单例）；测试经 app.dependency_overrides 替换。"""
    global _service
    if _service is None:
        _service = OrchestrationService()
    return _service


def _encode_sse(event: OrchestrationEvent) -> str:
    """一个 SSE 块：event: 名 + data: 负载 JSON（事件负载已由发射方脱敏）。"""
    return f"event: {event.name}\ndata: {json.dumps(event.payload, ensure_ascii=False)}\n\n"


async def _stream_run_events(
    run_id: str,
    service: OrchestrationService,
    events: AsyncIterator[OrchestrationEvent],
) -> AsyncIterator[str]:
    """把运行器事件流转码为 SSE；异常/断开时收敛（见模块 docstring）。"""
    finished = False
    terminal_streamed = False
    try:
        async for event in events:
            if event.name in _TERMINAL_EVENT_NAMES:
                terminal_streamed = True
            yield _encode_sse(event)
        finished = True
    except Exception:
        # 运行器层异常已收敛为 run_error 事件（仅类名）；到达这里的属端点
        # 契约内意外——只暴露稳定码，细节进服务端日志（凭据永不出引擎）。
        logger.exception("编排流内部异常 (run_id=%s)", run_id)
        if not terminal_streamed:
            yield _encode_sse(
                OrchestrationEvent(
                    run_id=run_id,
                    name="run_error",
                    payload={"error": "internal_error"},
                )
            )
    finally:
        if not finished and not terminal_streamed:
            # 客户端提前断开（生成器被取消）→ 把取消传播给运行器（interrupted，
            # 可续跑）；见模块 docstring 的终态不取消语义。
            try:
                service.cancel(run_id)
            except Exception:  # noqa: BLE001 — 断开路径的取消是尽力而为
                logger.warning("客户端断开后取消失败 (run_id=%s)", run_id)


@router.post("/runs/stream", summary="发起一轮编排 run（SSE）")
async def stream_run(
    run_request: OrchestrationRunRequest,
    service: OrchestrationService = Depends(get_orchestration_service),
) -> StreamingResponse:
    """内部编排：一次性用户轮的流式执行入口。请求体=OrchestrationRunRequest。"""
    return StreamingResponse(
        _stream_run_events(
            run_request.run_id,
            service,
            service.start(run_request),
        ),
        media_type="text/event-stream",
        headers=_SSE_HEADERS,
    )


@router.post(
    "/runs/{run_id}/resume",
    response_model=None,  # 404 JSON 与 SSE 二选一，不由注解生成响应模型
    summary="续跑 interrupted run（SSE）",
)
async def resume_run(
    run_id: str,
    service: OrchestrationService = Depends(get_orchestration_service),
) -> JSONResponse | StreamingResponse:
    """续跑：流已 200 后无法改判 404，先经 ensure_resumable 预检再开流。"""
    try:
        await service.ensure_resumable(run_id)
    except RunNotFoundError:
        logger.info("续跑请求未达 run: %s", run_id)
        return JSONResponse(status_code=404, content=_RESUME_NOT_FOUND_BODY)
    return StreamingResponse(
        _stream_run_events(run_id, service, service.resume(run_id)),
        media_type="text/event-stream",
        headers=_SSE_HEADERS,
    )


@router.post("/runs/{run_id}/cancel", summary="请求中断 run（可续跑；幂等）")
async def cancel_run(
    run_id: str,
    service: OrchestrationService = Depends(get_orchestration_service),
) -> dict[str, Any]:
    """请求中断活跃 run：同步置旗标，图在发言人边界收敛为 interrupted。

    幂等语义：未知/已终态 run 为 no-op，照常返回成功——Go 侧无需分支。
    """
    service.cancel(run_id)
    return {"code": 0, "message": "已请求中断（interrupted，可续跑）"}
