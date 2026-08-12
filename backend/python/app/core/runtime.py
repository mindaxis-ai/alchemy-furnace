# -*- coding: utf-8 -*-
"""
运行时 Provider 装配(007-demo-mode)

在 FastAPI lifespan 阶段根据 is_demo() 选择 Real* 或 Demo*,
把结果写入 app.services.providers.base.providers 全局句柄。
service 层从此处取 Provider 实例,自身无需 import 具体类。
"""

import logging

from app.core.config import is_demo, mode as mode_str
from app.services.providers.base import providers
from app.services.providers.demo import DemoChatProvider, DemoFusionProvider, DemoSynthesisProvider
from app.services.providers.real import RealChatProvider, RealFusionProvider, RealSynthesisProvider

logger = logging.getLogger(__name__)


def setup_providers() -> None:
    """
    根据 is_demo() 注入真实或演示 Provider。
    - 真实模式:延迟构造 ChatService / LanguageSynthesisService(避免演示模式下重复构造 OpenAI 客户端)
    - 演示模式:直接 new DemoChatProvider / DemoSynthesisProvider
    """
    if is_demo():
        providers.set(
            chat=DemoChatProvider(),
            synthesis=DemoSynthesisProvider(),
            fusion=DemoFusionProvider(),
        )
        logger.info("🧪 演示模式 — Chat/Synthesis/Fusion 均为 Demo Provider")
    else:
        # 真实模式:按需 import 既有 service(避免演示模式启动时浪费)
        from app.services.chat_service import ChatService
        from app.services.language_synthesis_service import LanguageSynthesisService
        from app.services.fusion_service import FusionService

        providers.set(
            chat=RealChatProvider(ChatService()),
            synthesis=RealSynthesisProvider(LanguageSynthesisService()),
            fusion=RealFusionProvider(FusionService()),
        )
        logger.info("🔥 真实模式 — Chat/Synthesis/Fusion 均为 Real Provider")


def current_mode() -> str:
    """供 /health 端点使用:返回 'demo' 或 'real'"""
    return mode_str()


def get_chat_provider():
    """service 层取 ChatProvider 的便捷访问点"""
    return providers.chat()


def get_synthesis_provider():
    """service 层取 SynthesisProvider 的便捷访问点"""
    return providers.synthesis()


def get_fusion_provider():
    """service 层取 FusionProvider 的便捷访问点"""
    return providers.fusion()
