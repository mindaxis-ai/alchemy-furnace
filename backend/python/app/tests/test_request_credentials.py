# -*- coding: utf-8 -*-
"""
调用级凭证（api_key/base_url）单元测试 (T009)

覆盖：
1. 请求携带 api_key/base_url 时，该次调用构造的 OpenAI 客户端使用这些值
2. 未携带时回退到环境变量配置的共享客户端（向后兼容，不新建客户端）
3. api_key 为空但 base_url 指向本地服务（如 ollama）时，以占位符 "none"
   通过 OpenAI SDK 的非空校验
4. 合成服务调用级凭证；错误映射与密钥脱敏
（原流式路径用例随 chat_completion_stream 下线移除——Task 15 LangGraph 权威化，
 错误映射仍由 TestErrorMappingAndMasking 覆盖）

运行：cd backend/python && .venv/bin/python -m pytest app/tests/test_request_credentials.py -q
"""
import json
from types import SimpleNamespace

import pytest

from app.core.config import settings
from app.services import chat_service as chat_module
from app.services import language_synthesis_service as synthesis_module
from app.services.chat_service import ChatService, map_llm_error, mask_api_key
from app.services.language_synthesis_service import LanguageSynthesisService


# ==================== 测试夹具 ====================


def _fake_llm_response(content="道人应答"):
    """伪造非流式 chat.completions.create 返回值"""
    message = SimpleNamespace(content=content)
    choice = SimpleNamespace(message=message)
    usage = SimpleNamespace(prompt_tokens=10, completion_tokens=5, total_tokens=15)
    return SimpleNamespace(choices=[choice], usage=usage)


class _SyncClientFactory:
    """记录 OpenAI 构造参数，并返回桩客户端"""

    def __init__(self, create):
        self.calls = []
        self._create = create

    def __call__(self, **kwargs):
        self.calls.append(kwargs)
        client = SimpleNamespace()
        client.chat = SimpleNamespace(
            completions=SimpleNamespace(create=self._create)
        )
        client.close = lambda: None
        return client


# ==================== 1. 非流式对话：调用级凭证 ====================


class TestChatCompletionCredentials:
    def test_request_credentials_used_for_client(self, monkeypatch):
        """请求携带 api_key/base_url -> 该次调用的客户端使用这些值"""
        monkeypatch.setattr(settings, "openai_api_key", "")
        svc = ChatService(api_key="", base_url="")
        factory = _SyncClientFactory(create=lambda **kw: _fake_llm_response())
        monkeypatch.setattr(chat_module, "OpenAI", factory)

        result = svc.chat_completion(
            messages=[{"role": "user", "content": "求道"}],
            model="deepseek-chat",
            api_key="sk-request-key",
            base_url="https://api.deepseek.com/v1",
        )

        assert result["content"] == "道人应答"
        assert len(factory.calls) == 1
        call = factory.calls[0]
        assert call["api_key"] == "sk-request-key"
        assert call["base_url"] == "https://api.deepseek.com/v1"

    def test_env_fallback_reuses_shared_client(self, monkeypatch):
        """未携带凭证 -> 复用环境变量配置的共享客户端，不新建"""
        monkeypatch.setattr(settings, "openai_api_key", "sk-env-key")
        svc = ChatService(api_key="sk-env-key", base_url="http://env-host/v1")
        factory = _SyncClientFactory(create=lambda **kw: _fake_llm_response())
        monkeypatch.setattr(chat_module, "OpenAI", factory)

        captured = {}

        def fake_create(**kwargs):
            captured["kwargs"] = kwargs
            return _fake_llm_response()

        monkeypatch.setattr(svc.client.chat.completions, "create", fake_create)

        result = svc.chat_completion(
            messages=[{"role": "user", "content": "求道"}],
            model="gpt-4o",
        )

        assert result["content"] == "道人应答"
        assert "kwargs" in captured  # 共享客户端确实被调用
        assert factory.calls == []  # 未构造任何新客户端

    def test_local_service_without_api_key_uses_placeholder(self, monkeypatch):
        """api_key 为空 + 本地 base_url（如 ollama）-> 以占位符构造客户端"""
        monkeypatch.setattr(settings, "openai_api_key", "")
        svc = ChatService(api_key="", base_url="")
        factory = _SyncClientFactory(create=lambda **kw: _fake_llm_response())
        monkeypatch.setattr(chat_module, "OpenAI", factory)

        result = svc.chat_completion(
            messages=[{"role": "user", "content": "求道"}],
            model="llama3",
            base_url="http://localhost:11434/v1",
        )

        assert result["content"] == "道人应答"
        assert len(factory.calls) == 1
        call = factory.calls[0]
        assert call["base_url"] == "http://localhost:11434/v1"
        # OpenAI SDK 要求非空 api_key，本地服务用占位符
        assert call["api_key"] == "none"


# ==================== 2. 合成服务：调用级凭证 ====================


class TestSynthesisCredentials:
    def _llm_json_create(self, **kwargs):
        content = json.dumps(
            {"emergence_rules": ["涌现规则甲"]},
            ensure_ascii=False,
        )
        return _fake_llm_response(content=content)

    def test_request_credentials_used_for_client(self, monkeypatch):
        """合成请求携带凭证 -> 涌现推导客户端使用这些值（即使环境未配置密钥）"""
        monkeypatch.setattr(settings, "openai_api_key", "")
        svc = LanguageSynthesisService(api_key="", base_url="")
        factory = _SyncClientFactory(create=self._llm_json_create)
        monkeypatch.setattr(synthesis_module, "OpenAI", factory)

        result = svc.combine(
            personality="沉稳内敛",
            pills=[],
            model="deepseek-chat",
            api_key="sk-request-key",
            base_url="https://api.deepseek.com/v1",
        )

        # LLM 路径被使用（非降级）
        assert result["emergence_rules"] == ["涌现规则甲"]
        assert result["degraded"] is False
        assert len(factory.calls) == 1
        call = factory.calls[0]
        assert call["api_key"] == "sk-request-key"
        assert call["base_url"] == "https://api.deepseek.com/v1"

    def test_env_fallback_reuses_shared_client(self, monkeypatch):
        """合成请求未携带凭证 -> 复用共享客户端"""
        monkeypatch.setattr(settings, "openai_api_key", "sk-env-key")
        svc = LanguageSynthesisService(api_key="sk-env-key", base_url="http://env-host/v1")
        factory = _SyncClientFactory(create=self._llm_json_create)
        monkeypatch.setattr(synthesis_module, "OpenAI", factory)
        monkeypatch.setattr(
            svc.client.chat.completions, "create", self._llm_json_create
        )

        result = svc.combine(personality="沉稳内敛", pills=[], model="gpt-4o-mini")

        assert result["emergence_rules"] == ["涌现规则甲"]
        assert result["degraded"] is False
        assert factory.calls == []

    def test_no_credentials_anywhere_degrades(self, monkeypatch):
        """请求与环境均无凭证 -> 降级(空涌现层),不构造客户端"""
        monkeypatch.setattr(settings, "openai_api_key", "")
        svc = LanguageSynthesisService(api_key="", base_url="")
        factory = _SyncClientFactory(create=self._llm_json_create)
        monkeypatch.setattr(synthesis_module, "OpenAI", factory)

        result = svc.combine(personality="沉稳内敛", pills=[])

        assert result["degraded"] is True
        assert result["degraded_reason"] == "no_credentials"
        assert result["emergence_rules"] == []
        assert "system_prompt" not in result
        assert factory.calls == []


# ==================== 3. 错误映射与密钥脱敏 ====================


class TestErrorMappingAndMasking:
    def test_map_llm_error_timeout(self):
        import httpx

        message, code = map_llm_error(httpx.TimeoutException("t"))
        assert (message, code) == ("语言引擎响应超时，请稍后重试", "TIMEOUT")

    def test_map_llm_error_status_codes(self):
        class FakeError(Exception):
            def __init__(self, status_code):
                super().__init__("err")
                self.status_code = status_code

        assert map_llm_error(FakeError(401))[1] == "AUTH_FAILED"
        assert map_llm_error(FakeError(403))[1] == "AUTH_FAILED"
        assert map_llm_error(FakeError(404))[0] == "模型不存在或不可用"
        assert map_llm_error(FakeError(500))[1] == "LLM_ERROR"

    def test_map_llm_error_generic(self):
        message, code = map_llm_error(RuntimeError("boom"))
        assert code == "LLM_ERROR"
        assert message  # 可读中文

    def test_mask_api_key_never_plaintext(self):
        assert mask_api_key(None) == "(none)"
        assert mask_api_key("") == "(none)"
        assert mask_api_key("sk-1234567890abcd") == "sk-****abcd"
        assert "1234567890" not in mask_api_key("sk-1234567890abcd")
