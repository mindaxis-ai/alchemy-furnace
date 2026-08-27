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

# ---------------------------------------------------------------------------
# 共享测试夹具（仅 stdlib，不引入第三方测试依赖）
# ---------------------------------------------------------------------------
from pathlib import Path

import httpx  # 打桩已在上方注册 sys.modules["httpx"]，此处取真包或桩模块
import pytest

from app.services.web_document_fetcher import FetchResult


def pytest_configure(config):
    """注册默认不运行的联网 smoke 标记（中国大陆发布前跑 network_cn）。"""
    config.addinivalue_line(
        "markers", "network_cn: 联网 smoke: 百度百科/千帆可达性（默认不运行）"
    )
    config.addinivalue_line(
        "markers", "network_global: 联网 smoke: Wikipedia/DDG 可达性（默认不运行）"
    )


def pytest_addoption(parser):
    parser.addoption(
        "--run-network",
        action="store_true",
        default=False,
        help="运行联网 smoke 测试（network_cn / network_global）",
    )


def pytest_collection_modifyitems(config, items):
    """未显式选择联网标记或 --run-network 时跳过联网 smoke，保证 CI 零公网依赖。"""
    if config.getoption("--run-network") or config.getoption("-m"):
        return
    skip_network = pytest.mark.skip(reason="联网 smoke：需 -m network_cn/network_global 或 --run-network")
    for item in items:
        if any(
            marker.name in {"network_cn", "network_global"}
            for marker in item.iter_markers()
        ):
            item.add_marker(skip_network)


def _timeout_exception():
    """httpx 超时异常：真环境取 ConnectTimeout，桩环境取 TimeoutException。"""
    return getattr(httpx, "ConnectTimeout", None) or getattr(
        httpx, "TimeoutException", None
    ) or Exception


class _FakeResponse:
    def __init__(self, status=200, text="", payload=None, headers=None, url=""):
        self.status_code = status
        self.text = text
        self.content = text.encode("utf-8")
        self._payload = payload
        self.headers = headers or {}
        self.url = url
        self.encoding = "utf-8"

    def raise_for_status(self):
        if self.status_code >= 400:
            raise RuntimeError(f"HTTP {self.status_code}")

    def json(self):
        return self._payload


class FakeHTTP:
    """可录制请求的假 httpx client：按 URL 或 FIFO 队列响应，支持触发超时。"""

    def __init__(self):
        self.by_url = {}
        self.queue = []
        self.timeout_now = False
        self.calls = []
        self.last_url = None
        self.last_headers = None
        self.last_json = None

    def add(self, url=None, status=200, text="", payload=None, headers=None):
        response = _FakeResponse(status=status, text=text, payload=payload, headers=headers)
        if url is None:
            self.queue.append(response)
        else:
            self.by_url[url] = response

    def add_json(self, payload, url=None):
        self.add(url=url, status=200, payload=payload, headers={"content-type": "application/json"})

    def raise_timeout(self):
        self.timeout_now = True

    def _record(self, url, kwargs):
        self.calls.append(url)
        self.last_url = url
        self.last_headers = kwargs.get("headers")
        self.last_json = kwargs.get("json")

    def _respond(self, url):
        if self.timeout_now:
            raise _timeout_exception()("simulated connect timeout")
        if url in self.by_url:
            return self.by_url[url]
        if self.queue:
            return self.queue.pop(0)
        raise AssertionError(f"未配置的假响应: {url}")

    def get(self, url, **kwargs):
        self._record(url, kwargs)
        return self._respond(url)

    def post(self, url, **kwargs):
        self._record(url, kwargs)
        return self._respond(url)


@pytest.fixture
def fake_http():
    return FakeHTTP()


@pytest.fixture
def load_fixture():
    fixture_dir = Path(__file__).parent / "fixtures"

    def _load(name: str) -> str:
        return (fixture_dir / name).read_text(encoding="utf-8")

    return _load


class _FakeResolver:
    """按调用次序返回 IP 列表的假 DNS resolver。"""

    def __init__(self, plan):
        self.plan = plan
        self.calls = 0

    def resolve(self, hostname):
        index = min(self.calls, len(self.plan) - 1)
        self.calls += 1
        return self.plan[index]


@pytest.fixture
def public_dns():
    return _FakeResolver([["93.184.216.34"]])


@pytest.fixture
def public_then_private_dns():
    return _FakeResolver([["93.184.216.34"], ["127.0.0.1"]])


class FakeFetcher:
    """假 WebDocumentFetcher：按 URL 返回摘录，或返回指定 status/reason。"""

    def __init__(self):
        self.by_url = {}
        self.fallback = None

    def add(self, url, excerpt):
        self.by_url[url] = FetchResult(url, excerpt, "ok", "")

    def add_result(self, status, reason):
        self.fallback = FetchResult("", "", status, reason)

    def fetch(self, url):
        if url in self.by_url:
            return self.by_url[url]
        if self.fallback is not None:
            return self.fallback
        return FetchResult(url, "", "failed", "no_fixture")


@pytest.fixture
def fake_fetcher():
    return FakeFetcher()


class FailIfCalled:
    """任何调用都失败的哨兵：验证“不应访问”的路径。"""

    def __getattr__(self, name):
        raise AssertionError(f"不应调用 {name}")
