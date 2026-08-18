# -*- coding: utf-8 -*-
"""
炼丹炉 - 金丹融合路由 (Fusion API)
- POST /api/v1/fusion/fuse - 融合金丹(多枚金丹 -> 新金丹预览,不落库)
"""
import logging

from fastapi import APIRouter, HTTPException, status

from app.models.schemas import FuseRequest, FuseResponse
from app.services.fusion_service import FusionService

logger = logging.getLogger(__name__)

router = APIRouter(prefix="/fusion", tags=["融合 - 合丹为新"])


fusion_service = FusionService()


@router.post(
    "/fuse",
    response_model=FuseResponse,
    summary="融合金丹",
    description="随机抽融合算子,LLM 将 N 枚金丹融合为新金丹(预览,不落库)",
)
async def fuse(request: FuseRequest) -> FuseResponse:
    try:
        pills = [p.model_dump() for p in request.pills]
        result = fusion_service.fuse(
            pills=pills,
            model=request.model or None,
            api_key=request.api_key,
            base_url=request.base_url,
            exclude_operator_id=request.exclude_operator_id,
        )
        return FuseResponse(**result)
    except ValueError as e:
        raise HTTPException(status_code=status.HTTP_400_BAD_REQUEST, detail=str(e))
    except Exception as e:
        logger.error(f"金丹融合失败: {e}")
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            detail=f"Fusion failed: {e}",
        )
