# -*- coding: utf-8 -*-
"""
炼丹炉 · 金丹化性 - 对话业务逻辑层 (Chat Service)

处理对话流程：直接使用调用方提供的合成系统提示词，调用 LLM 生成回复。
不再进行向量检索——金丹化性系统中，"丹"已化为"性"（系统提示词）。
"""
import asyncio
import json
import logging
from typing import Any, AsyncGenerator, Dict, List, Optional, Tuple

import httpx
from openai import (
    APIStatusError,
    APITimeoutError,
    AsyncOpenAI,
    OpenAI,
)

from app.core.config import settings

logger = logging.getLogger(__name__)


# ==================== 统一错误映射（T032） ====================

_ERR_TIMEOUT = ("语言引擎响应超时，请稍后重试", "TIMEOUT")
_ERR_AUTH = ("模型凭证无效，请检查模型管理中的 API Key", "AUTH_FAILED")
_ERR_MODEL_NOT_FOUND = ("模型不存在或不可用", "MODEL_NOT_FOUND")
_ERR_GENERIC = ("对话生成失败，请稍后重试", "LLM_ERROR")


def map_llm_error(exc: Exception) -> Tuple[str, str]:
    """
    将 LLM 调用异常映射为 (可读中文消息, 错误码)

    - 超时           -> TIMEOUT
    - 401/403 鉴权   -> AUTH_FAILED
    - 404 模型不存在 -> MODEL_NOT_FOUND
    - 其他           -> LLM_ERROR
    """
    if isinstance(exc, (APITimeoutError, httpx.TimeoutException, asyncio.TimeoutError)):
        return _ERR_TIMEOUT
    status_code = getattr(exc, "status_code", None)
    if isinstance(exc, APIStatusError) or status_code is not None:
        if status_code in (401, 403):
            return _ERR_AUTH
        if status_code == 404:
            return _ERR_MODEL_NOT_FOUND
    return _ERR_GENERIC


def mask_api_key(api_key: Optional[str]) -> str:
    """API 密钥脱敏 - 日志中永不明文"""
    if not api_key:
        return "(none)"
    if len(api_key) <= 8:
        return "****"
    return f"{api_key[:3]}****{api_key[-4:]}"


class ChatService:
    """
    对话服务 - 道人智慧核心

    接收含合成系统提示词的完整消息列表，直接调用 LLM。
    支持流式（SSE）和非流式输出。

    凭证策略（T016）：调用级 api_key/base_url 优先；缺省回退到环境变量配置
    的共享客户端，不修改共享状态。api_key 为空但 base_url 指向本地服务
    （如 ollama）时，以占位符 "none" 通过 OpenAI SDK 的非空校验。

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

    # ==================== 凭证解析（T016） ====================

    def _resolve_credentials(
        self,
        api_key: Optional[str] = None,
        base_url: Optional[str] = None,
    ) -> Tuple[str, Optional[str], bool]:
        """
        解析本次调用的生效凭证

        Returns:
            (effective_api_key, effective_base_url, is_override)
            is_override 为 True 表示需构造临时客户端而非复用共享客户端
        """
        eff_key = (api_key or self.api_key or "").strip()
        eff_url = (base_url or self.base_url or "").strip() or None
        is_override = bool((api_key or "").strip() or (base_url or "").strip()) and (
            eff_key != (self.api_key or "") or eff_url != (self.base_url or None)
        )
        return eff_key, eff_url, is_override

    def _credentials_usable(self, eff_key: str, eff_url: Optional[str], is_override: bool) -> bool:
        """
        判断凭证是否可用于发起真实 LLM 调用

        - 调用级覆盖：提供了 api_key 或 base_url（本地服务可无密钥）即视为可用
        - 环境回退：沿用 openai_api_key_valid 校验
        """
        if is_override:
            return bool(eff_key or eff_url)
        return settings.openai_api_key_valid

    @staticmethod
    def _build_sync_client(api_key: str, base_url: Optional[str]) -> OpenAI:
        # OpenAI SDK 要求 api_key 非空；本地服务（如 ollama）无鉴权时用占位符
        return OpenAI(
            api_key=api_key or "none",
            base_url=base_url,
            http_client=httpx.Client(timeout=120.0),
        )

    @staticmethod
    def _build_async_client(api_key: str, base_url: Optional[str]) -> AsyncOpenAI:
        return AsyncOpenAI(
            api_key=api_key or "none",
            base_url=base_url,
            http_client=httpx.AsyncClient(timeout=120.0),
        )

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
        api_key: Optional[str] = None,
        base_url: Optional[str] = None,
        response_format: Optional[Dict[str, str]] = None,
    ) -> Dict[str, Any]:
        """
        非流式对话 - 道人一次性回答

        Args:
            messages: 消息历史（含合成后的 system 消息）
            model: LLM 模型
            temperature: 温度参数
            max_tokens: 最大 token 数
            api_key: 调用级覆盖的 API 密钥（缺省回退环境变量）
            base_url: 调用级覆盖的接口地址（缺省回退环境变量）
            response_format: 响应格式约束,例如 {"type": "json_object"} 强制 JSON 输出
                            (仅在目标模型支持时生效;不传则不约束)

        Returns:
            包含 content, model, usage 的字典
        """
        model = model or settings.default_model
        eff_key, eff_url, is_override = self._resolve_credentials(api_key, base_url)

        if not self._credentials_usable(eff_key, eff_url, is_override):
            raise RuntimeError(
                "对话生成失败: OPENAI_API_KEY 未配置，无法调用 LLM"
            )

        temp_client: Optional[OpenAI] = None
        client = self.client
        if is_override:
            temp_client = self._build_sync_client(eff_key, eff_url)
            client = temp_client
            logger.info(
                "使用调用级凭证 - base_url: %s, api_key: %s",
                eff_url, mask_api_key(eff_key),
            )

        try:
            logger.info(f"求道之问 - 模型: {model}")
            chat_messages = self._normalize_messages(messages)

            create_kwargs: Dict[str, Any] = {
                "model": model,
                "messages": chat_messages,
                "temperature": temperature,
                "max_tokens": max_tokens,
            }
            if response_format is not None:
                create_kwargs["response_format"] = response_format

            response = client.chat.completions.create(**create_kwargs)

            content = response.choices[0].message.content
            usage = {
                "prompt_tokens": response.usage.prompt_tokens,
                "completion_tokens": response.usage.completion_tokens,
                "total_tokens": response.usage.total_tokens,
            } if response.usage else {}

            logger.info(f"道人回答完毕 - tokens: {usage.get('total_tokens')}")
            return {"content": content, "model": model, "usage": usage}

        except Exception as e:
            message, code = map_llm_error(e)
            logger.error(f"道人回答失败 [{code}]: {e}")
            raise RuntimeError(f"{message} ({code})")
        finally:
            if temp_client is not None:
                try:
                    temp_client.close()
                except Exception:
                    logger.debug("关闭临时 OpenAI 客户端异常", exc_info=True)

    async def chat_completion_stream(
        self,
        messages: List[Dict[str, str]],
        model: str = None,
        temperature: float = 0.7,
        max_tokens: int = 4096,
        api_key: Optional[str] = None,
        base_url: Optional[str] = None,
    ) -> AsyncGenerator[str, None]:
        """
        流式对话 - 道人缓缓道来 (SSE)

        格式: data: {"content": "..."}\\n\\n ... data: [DONE]\\n\\n
        出错时: data: {"error": "...", "code": "..."} 后接 data: [DONE]

        取消语义（T024）：客户端断开 / asyncio.CancelledError / GeneratorExit 时，
        显式关闭上游 OpenAI 流以停止 token 消耗，finally 块释放全部资源。
        """
        model = model or settings.default_model
        eff_key, eff_url, is_override = self._resolve_credentials(api_key, base_url)

        if not self._credentials_usable(eff_key, eff_url, is_override):
            error_data = json.dumps(
                {"error": "OPENAI_API_KEY 未配置，无法调用 LLM", "code": "AUTH_FAILED"},
                ensure_ascii=False,
            )
            yield f"data: {error_data}\n\n"
            yield "data: [DONE]\n\n"
            return

        temp_client: Optional[AsyncOpenAI] = None
        client = self.async_client
        if is_override:
            temp_client = self._build_async_client(eff_key, eff_url)
            client = temp_client
            logger.info(
                "使用调用级凭证(流式) - base_url: %s, api_key: %s",
                eff_url, mask_api_key(eff_key),
            )

        stream = None
        try:
            logger.info(f"求道之问(流式) - 模型: {model}")
            chat_messages = self._normalize_messages(messages)

            stream = await client.chat.completions.create(
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

        except (asyncio.CancelledError, GeneratorExit):
            # 客户端断开 / Go 网关取消：安静收尾，不刷 traceback
            logger.info("流式对话被取消（客户端断开或收到停止指令），终止上游 LLM 流")
            raise
        except Exception as e:
            message, code = map_llm_error(e)
            logger.error(f"道人回答(流式)失败 [{code}]: {e}")
            error_data = json.dumps(
                {"error": message, "code": code}, ensure_ascii=False
            )
            yield f"data: {error_data}\n\n"
            yield "data: [DONE]\n\n"
        finally:
            # 释放上游流，停止 token 消耗
            if stream is not None:
                try:
                    close = getattr(stream, "aclose", None) or getattr(stream, "close", None)
                    if close is not None:
                        await close()
                except Exception:
                    logger.debug("关闭上游 LLM 流异常", exc_info=True)
            if temp_client is not None:
                try:
                    await temp_client.close()
                except Exception:
                    logger.debug("关闭临时 OpenAI 异步客户端异常", exc_info=True)
