"""Nuwa distillation API."""
import logging

from fastapi import APIRouter, HTTPException, status

from app.models.schemas import DistillRequest, DistillResponse
from app.services.nuwa_distillation_service import NuwaDistillationService
from app.services.research_provider import DuckDuckGoResearchProvider

logger = logging.getLogger(__name__)
router = APIRouter(prefix="/distillation", tags=["女娲蒸馏"])

distillation_service = NuwaDistillationService(DuckDuckGoResearchProvider())


@router.post("/nuwa", response_model=DistillResponse, summary="从公开资料蒸馏金丹草稿")
def distill_nuwa(request: DistillRequest) -> DistillResponse:
    try:
        return DistillResponse(**distillation_service.distill(**request.model_dump()))
    except ValueError as exc:
        raise HTTPException(status_code=status.HTTP_400_BAD_REQUEST, detail=str(exc)) from exc
    except Exception as exc:
        logger.exception("女娲蒸馏失败")
        raise HTTPException(
            status_code=status.HTTP_502_BAD_GATEWAY,
            detail="公开资料收集或模型蒸馏失败，请稍后重试",
        ) from exc
