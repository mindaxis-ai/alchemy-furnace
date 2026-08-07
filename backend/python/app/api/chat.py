# -*- coding: utf-8 -*-
"""
炼丹炉 - 对话路由 (Chat API)
- POST /api/v1/chat/completions - 非流式对话
- POST /api/v1/chat/completions/stream - SSE 流式对话
犹如求道之殿，与道人论道
"""
import logging

from fastapi import APIRouter, HTTPException, status
from fastapi.responses import StreamingResponse

from app.models.schemas import (
    ChatCompletionRequest,
    BaseResponse,
)
from app.services.chat_service import ChatService

logger = logging.getLogger(__name__)

# 创建路由 - 对话之门户
router = APIRouter(prefix="/chat", tags=["对话 - 求道"])

# 服务实例 - 道人智慧核心
chat_service = ChatService()


@router.post(
    "/completions",
    response_model=BaseResponse,
    summary="非流式对话",
    description="发送消息列表（含合成后的 system 消息），获取完整的 AI 回答",
)
async def chat_completion(request: ChatCompletionRequest) -> BaseResponse:
    """
    非流式对话 - 一次性求道

    Request Body:
        - messages: 消息历史 [{role, content}]，首条应为合成后的 system 消息
        - model: LLM 模型，默认 gpt-4o
        - temperature: 温度参数
        - max_tokens: 最大 token 数
    """
    try:
        logger.info(f"收到求道之问 - 模型: {request.model or 'default'}")

        messages = [
            {"role": msg.role, "content": msg.content}
            for msg in request.messages
        ]

        result = chat_service.chat_completion(
            messages=messages,
            model=request.model or None,
            temperature=request.temperature,
            max_tokens=request.max_tokens,
        )

        return BaseResponse(
            code=0,
            message="道人回答完毕",
            data=result,
        )

    except ValueError as e:
        logger.error(f"求道参数错误: {e}")
        raise HTTPException(
            status_code=status.HTTP_400_BAD_REQUEST,
            detail=str(e),
        )
    except Exception as e:
        logger.error(f"求道失败: {e}")
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            detail=f"对话生成失败: {e}",
        )


@router.post(
    "/completions/stream",
    summary="SSE 流式对话",
    description="发送消息列表（含合成后的 system 消息），以 SSE 流式格式获取 AI 回答",
)
async def chat_completion_stream(request: ChatCompletionRequest):
    """
    SSE 流式对话 - 道人缓缓道来

    返回格式:
        data: {"content": "第"}\n\n
        data: {"content": "一"}\n\n
        ...
        data: [DONE]\n\n
    """
    try:
        logger.info(f"收到求道之问(流式) - 模型: {request.model or 'default'}")

        messages = [
            {"role": msg.role, "content": msg.content}
            for msg in request.messages
        ]

        return StreamingResponse(
            chat_service.chat_completion_stream(
                messages=messages,
                model=request.model or None,
                temperature=request.temperature,
                max_tokens=request.max_tokens,
            ),
            media_type="text/event-stream",
            headers={
                "Cache-Control": "no-cache",
                "Connection": "keep-alive",
                "X-Accel-Buffering": "no",  # 禁用 Nginx 缓冲
            },
        )

    except ValueError as e:
        logger.error(f"求道(流式)参数错误: {e}")
        raise HTTPException(
            status_code=status.HTTP_400_BAD_REQUEST,
            detail=str(e),
        )
    except Exception as e:
        logger.error(f"求道(流式)失败: {e}")
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            detail=f"流式对话失败: {e}",
        )
