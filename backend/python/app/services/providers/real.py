# -*- coding: utf-8 -*-
"""
Real Provider(真实模式) - 007-demo-mode

包装现有 ChatService / LanguageSynthesisService,使其实现 Provider 协议。
service 层从此处导入 get_chat() / get_synthesis() 而非直接构造具体类,
由此达到 service 文件"零修改"通过 protocol 注入的目标。
"""

import logging
from typing import Any, AsyncGenerator, Dict, List, Optional

from app.services.providers.base import ChatProvider, FusionProvider, SynthesisProvider

logger = logging.getLogger(__name__)


class RealChatProvider:
    """包装 app.services.chat_service.ChatService,实现 ChatProvider 协议"""

    def __init__(self, chat_service: Any) -> None:
        self._svc = chat_service

    def chat_completion(
        self,
        messages: List[Dict[str, str]],
        model: Optional[str] = None,
        temperature: float = 0.7,
        max_tokens: int = 4096,
        api_key: Optional[str] = None,
        base_url: Optional[str] = None,
    ) -> Dict[str, Any]:
        return self._svc.chat_completion(
            messages=messages,
            model=model,
            temperature=temperature,
            max_tokens=max_tokens,
            api_key=api_key,
            base_url=base_url,
        )

    async def chat_completion_stream(
        self,
        messages: List[Dict[str, str]],
        model: Optional[str] = None,
        temperature: float = 0.7,
        max_tokens: int = 4096,
        api_key: Optional[str] = None,
        base_url: Optional[str] = None,
    ) -> AsyncGenerator[str, None]:
        async for chunk in self._svc.chat_completion_stream(
            messages=messages,
            model=model,
            temperature=temperature,
            max_tokens=max_tokens,
            api_key=api_key,
            base_url=base_url,
        ):
            yield chunk


class RealSynthesisProvider:
    """包装 app.services.language_synthesis_service,实现 SynthesisProvider 协议"""

    def __init__(self, synthesis_service: Any) -> None:
        self._svc = synthesis_service

    def synthesize(
        self,
        personality: str,
        pills: List[Any],
    ) -> Dict[str, Any]:
        if hasattr(self._svc, "synthesize"):
            return self._svc.synthesize(personality, pills)
        return {
            "system_prompt": personality or "",
            "emergence_rules": [],
            "inner_tensions": [],
            "source_fingerprint": "real-fallback",
        }

    def combine(
        self,
        personality: str,
        pills: List[Any],
        model: Optional[str] = None,
        temperature: float = 0.7,
        max_tokens: int = 4096,
        api_key: Optional[str] = None,
        base_url: Optional[str] = None,
    ) -> Dict[str, Any]:
        """委托给既有 LanguageSynthesisService.combine(签名对齐)"""
        return self._svc.combine(
            personality=personality,
            pills=pills,
            model=model,
            temperature=temperature,
            max_tokens=max_tokens,
            api_key=api_key,
            base_url=base_url,
        )


class RealFusionProvider:
    """包装 app.services.fusion_service.FusionService,实现 FusionProvider 协议"""

    def __init__(self, fusion_service: Any) -> None:
        self._svc = fusion_service

    def fuse(
        self,
        pills: List[Any],
        model: Optional[str] = None,
        api_key: Optional[str] = None,
        base_url: Optional[str] = None,
        exclude_operator_id: Optional[str] = None,
    ) -> Dict[str, Any]:
        return self._svc.fuse(
            pills=pills, model=model, api_key=api_key,
            base_url=base_url, exclude_operator_id=exclude_operator_id,
        )
