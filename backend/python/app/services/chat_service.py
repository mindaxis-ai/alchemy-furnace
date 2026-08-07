# -*- coding: utf-8 -*-
"""
炼丹炉 · 金丹化性 - 对话业务逻辑层 (Chat Service)

处理对话流程：直接使用调用方提供的合成系统提示词，调用 LLM 生成回复。
不再进行向量检索——金丹化性系统中，"丹"已化为"性"（系统提示词）。
"""
import json
import logging
from typing import Any, AsyncGenerator, Dict, List

import httpx
from openai import AsyncOpenAI, OpenAI

from app.core.config import settings

logger = logging.getLogger(__name__)


class ChatService:
    """
    对话服务 - 道人智慧核心

    接收含合成系统提示词的完整消息列表，直接调用 LLM。
    支持流式（SSE）和非流式输出。

    Attributes:
        client: OpenAI 同步客户端
        async_client: OpenAI 异步客户端
    """

    def __init__(self, api_key: str = None, base_url: str = None) -> None:
        self.api_key = api_key or settings.openai_api_key
        self.base_url = base_url or settings.openai_base_url

        http_client = httpx.Client(timeout=120.0)
        self.client = OpenAI(
            api_key=self.api_key or "sk-not-configured",
            base_url=self.base_url,
            http_client=http_client,
        )

        async_http_client = httpx.AsyncClient(timeout=120.0)
        self.async_client = AsyncOpenAI(
            api_key=self.api_key or "sk-not-configured",
            base_url=self.base_url,
            http_client=async_http_client,
        )

        logger.info("道人智慧核心初始化完毕")

    @staticmethod
    def _normalize_messages(messages: List[Dict[str, str]]) -> List[Dict[str, str]]:
        """
        规范化消息列表 - 确保有 system 提示词

        调用方（Go 网关）应已注入合成后的 system 消息；
        若缺失，则补一条默认的人格提示。
        """
        if messages and messages[0].get("role") == "system":
            return messages
        return [
            {
                "role": "system",
                "content": (
                    "你是一位有个性的 AI 道人。请基于你的知识与人格设定，"
                    "以一致的语言风格回答用户的问题。"
                ),
            },
            *messages,
        ]

    def chat_completion(
        self,
        messages: List[Dict[str, str]],
        model: str = None,
        temperature: float = 0.7,
        max_tokens: int = 4096,
    ) -> Dict[str, Any]:
        """
        非流式对话 - 道人一次性回答

        Args:
            messages: 消息历史（含合成后的 system 消息）
            model: LLM 模型
            temperature: 温度参数
            max_tokens: 最大 token 数

        Returns:
            包含 content, model, usage 的字典
        """
        model = model or settings.default_model

        if not settings.openai_api_key_valid:
            raise RuntimeError(
                "对话生成失败: OPENAI_API_KEY 未配置，无法调用 LLM"
            )

        try:
            logger.info(f"求道之问 - 模型: {model}")
            chat_messages = self._normalize_messages(messages)

            response = self.client.chat.completions.create(
                model=model,
                messages=chat_messages,
                temperature=temperature,
                max_tokens=max_tokens,
            )

            content = response.choices[0].message.content
            usage = {
                "prompt_tokens": response.usage.prompt_tokens,
                "completion_tokens": response.usage.completion_tokens,
                "total_tokens": response.usage.total_tokens,
            } if response.usage else {}

            logger.info(f"道人回答完毕 - tokens: {usage.get('total_tokens')}")
            return {"content": content, "model": model, "usage": usage}

        except Exception as e:
            logger.error(f"道人回答失败: {e}")
            raise RuntimeError(f"对话生成失败: {e}")

    async def chat_completion_stream(
        self,
        messages: List[Dict[str, str]],
        model: str = None,
        temperature: float = 0.7,
        max_tokens: int = 4096,
    ) -> AsyncGenerator[str, None]:
        """
        流式对话 - 道人缓缓道来 (SSE)

        格式: data: {"content": "..."}\\n\\n ... data: [DONE]\\n\\n
        出错时: data: {"error": "..."} 后接 data: [DONE]
        """
        model = model or settings.default_model

        if not settings.openai_api_key_valid:
            error_data = json.dumps(
                {"error": "OPENAI_API_KEY 未配置，无法调用 LLM"},
                ensure_ascii=False,
            )
            yield f"data: {error_data}\n\n"
            yield "data: [DONE]\n\n"
            return

        try:
            logger.info(f"求道之问(流式) - 模型: {model}")
            chat_messages = self._normalize_messages(messages)

            stream = await self.async_client.chat.completions.create(
                model=model,
                messages=chat_messages,
                temperature=temperature,
                max_tokens=max_tokens,
                stream=True,
            )

            async for chunk in stream:
                if not chunk.choices:
                    continue
                delta = chunk.choices[0].delta
                if delta.content:
                    data = json.dumps({"content": delta.content}, ensure_ascii=False)
                    yield f"data: {data}\n\n"

            yield "data: [DONE]\n\n"
            logger.info("道人回答流式传输完毕")

        except Exception as e:
            logger.error(f"道人回答(流式)失败: {e}")
            error_data = json.dumps({"error": str(e)}, ensure_ascii=False)
            yield f"data: {error_data}\n\n"
            yield "data: [DONE]\n\n"
