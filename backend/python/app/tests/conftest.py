# -*- coding: utf-8 -*-
"""
测试环境第三方依赖打桩

本环境未安装 fastapi/openai/httpx/pydantic，
在被测模块（app.services.language_synthesis_service ->
app.core.config）导入之前，向 sys.modules 注入最小可用的桩模块，
使 `python3 -m pytest app/tests/ -q` 可在无网络、无依赖环境下运行。
"""
import sys
import types
import importlib.util


def _install_pydantic_stubs() -> None:
    """桩住 pydantic / pydantic_settings，仅满足 app.core.config 的用法"""
    if "pydantic" in sys.modules or importlib.util.find_spec("pydantic") is not None:
        return

    pydantic = types.ModuleType("pydantic")

    def Field(default=None, default_factory=None, **kwargs):  # noqa: N802
        if default_factory is not None:
            return default_factory()
        return default

    pydantic.Field = Field
    sys.modules["pydantic"] = pydantic

    pydantic_settings = types.ModuleType("pydantic_settings")

    def SettingsConfigDict(**kwargs):  # noqa: N802
        return dict(kwargs)

    class BaseSettings:
        """极简 Settings：按类注解收集默认值，支持 kwargs 覆盖"""

        def __init__(self, **kwargs):
            annotations = {}
            for klass in reversed(type(self).__mro__):
                annotations.update(getattr(klass, "__annotations__", {}))
            for name in annotations:
                if name in kwargs:
                    setattr(self, name, kwargs[name])
                elif hasattr(type(self), name):
                    setattr(self, name, getattr(type(self), name))

    pydantic_settings.BaseSettings = BaseSettings
    pydantic_settings.SettingsConfigDict = SettingsConfigDict
    sys.modules["pydantic_settings"] = pydantic_settings


def _install_httpx_stub() -> None:
    if "httpx" in sys.modules or importlib.util.find_spec("httpx") is not None:
        return
    httpx = types.ModuleType("httpx")

    class Client:
        def __init__(self, *args, **kwargs):
            self.kwargs = kwargs

        def close(self):
            pass

    class AsyncClient:
        def __init__(self, *args, **kwargs):
            self.kwargs = kwargs

        async def aclose(self):
            pass

    class TimeoutException(Exception):
        pass

    httpx.Client = Client
    httpx.AsyncClient = AsyncClient
    httpx.TimeoutException = TimeoutException
    sys.modules["httpx"] = httpx


def _install_openai_stub() -> None:
    if "openai" in sys.modules or importlib.util.find_spec("openai") is not None:
        return
    openai = types.ModuleType("openai")

    class _Completions:
        def create(self, *args, **kwargs):
            raise NotImplementedError(
                "OpenAI stub: tests must monkeypatch client.chat.completions.create"
            )

    class _Chat:
        def __init__(self):
            self.completions = _Completions()

    class OpenAI:
        def __init__(self, *args, **kwargs):
            self.kwargs = kwargs
            self.chat = _Chat()

        def close(self):
            pass

    class AsyncOpenAI:
        def __init__(self, *args, **kwargs):
            self.kwargs = kwargs
            self.chat = _Chat()

        async def close(self):
            pass

    class APIError(Exception):
        pass

    class APIStatusError(APIError):
        def __init__(self, message="", status_code=None, *args, **kwargs):
            super().__init__(message)
            self.status_code = status_code

    class APITimeoutError(APIError):
        pass

    openai.OpenAI = OpenAI
    openai.AsyncOpenAI = AsyncOpenAI
    openai.APIError = APIError
    openai.APIStatusError = APIStatusError
    openai.APITimeoutError = APITimeoutError
    sys.modules["openai"] = openai


_install_pydantic_stubs()
_install_httpx_stub()
_install_openai_stub()
