# -*- coding: utf-8 -*-
"""
炼丹炉 - 语言模式合成路由 (Synthesis API)
- POST /api/v1/synthesis/combine - 合成语言模式（性格 + 金丹 -> 系统提示词）
"""
import logging

from fastapi import APIRouter, HTTPException, status

from app.models.schemas import CombineRequest, CombineResponse
from app.services.language_synthesis_service import LanguageSynthesisService

logger = logging.getLogger(__name__)

router = APIRouter(prefix="/synthesis", tags=["合成 - 化丹为性"])


synthesis_service = LanguageSynthesisService()


@router.post(
    "/combine",
    response_model=CombineResponse,
    summary="合成语言模式",
    description="将道人基础性格与已服用金丹合成为统一系统提示词",
)
async def combine(request: CombineRequest) -> CombineResponse:
    """
    化丹为性 - 语言模式合成

    流程：
    1. 结构化合并表达 DNA / 心智模型 / 启发式
    2. 检测丹性相冲（inner_tensions）
    3. LLM 涌现推导，生成最终系统提示词
    """
    try:
        pills = [p.model_dump() for p in request.pills]
        result = synthesis_service.combine(
            personality=request.personality,
            pills=pills,
            model=request.model or None,
            temperature=request.temperature,
            max_tokens=request.max_tokens,
            api_key=request.api_key,
            base_url=request.base_url,
        )
        return CombineResponse(**result)
    except Exception as e:
        logger.error(f"化丹为性失败: {e}")
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            detail=f"Synthesis failed: {e}",
        )
