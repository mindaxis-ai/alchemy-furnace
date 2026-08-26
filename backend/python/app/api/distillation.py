"""Nuwa distillation API."""
import logging

from fastapi import APIRouter, HTTPException, status

from app.models.schemas import DistillRequest, DistillResponse
from app.services.duckduckgo_research_provider import DuckDuckGoResearchProvider
from app.services.nuwa_distillation_service import DistillationError, NuwaDistillationService

logger = logging.getLogger(__name__)
router = APIRouter(prefix="/distillation", tags=["女娲蒸馏"])

distillation_service = NuwaDistillationService(DuckDuckGoResearchProvider())


@router.post("/nuwa", response_model=DistillResponse, summary="从公开资料蒸馏金丹草稿")
def distill_nuwa(request: DistillRequest) -> DistillResponse:
    try:
        return DistillResponse(**distillation_service.distill(**request.model_dump()))
    except DistillationError as exc:
        # 稳定错误协议: detail 为结构化对象,Go 网关按 code/stage/retryable 透传
        http_status = (
            status.HTTP_503_SERVICE_UNAVAILABLE
            if exc.retryable
            else status.HTTP_422_UNPROCESSABLE_ENTITY
        )
        raise HTTPException(
            status_code=http_status,
            detail={
                "code": exc.code,
                "stage": exc.stage,
                "message": exc.message,
                "retryable": exc.retryable,
                "details": exc.details,
            },
        ) from exc
    except ValueError as exc:
        raise HTTPException(status_code=status.HTTP_400_BAD_REQUEST, detail=str(exc)) from exc
    except Exception as exc:
        logger.exception("女娲蒸馏失败")
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            detail={
                "code": "distillation_internal_error",
                "stage": "unknown",
                "message": "蒸馏服务内部错误",
                "retryable": False,
                "details": {},
            },
        ) from exc
