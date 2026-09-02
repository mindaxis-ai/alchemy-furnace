"""模型网关测试：适配器注册、官方选项透传、空 key 与回退。

仅构造适配器对象（不发任何网络请求）；真实模型调用由后续 Task 的
图集成测试以假模型覆盖。
"""

import pytest
from langchain_deepseek import ChatDeepSeek
from langchain_ollama import ChatOllama

from app.orchestration.contracts import ModelCredential, ModelRef
from app.orchestration.model_gateway import ModelGateway, OpenAICompatibleAdapter


def model_ref(provider_type: str) -> ModelRef:
    return ModelRef(provider_type=provider_type, name="unit-model")


def credential(api_key: str | None = "sk-test", base_url: str | None = "https://api.example.com/v1"):
    return ModelCredential(api_key=api_key, base_url=base_url)


def recording_factories() -> dict[str, object]:
    """注入式工厂：记录命中键与入参，返回哨兵，不触碰真实模型构造。"""

    def make(key: str):
        def factory(ref: ModelRef, cred: ModelCredential):
            factory.calls.append((key, ref, cred))
            return key

        factory.calls = []  # type: ignore[attr-defined]
        return factory

    return {provider: make(provider) for provider in ("openai", "deepseek", "ollama", "compatible")}


@pytest.mark.parametrize(
    ("provider_type", "expected"),
    [("openai", "openai"), ("deepseek", "deepseek"), ("ollama", "ollama"), ("acme", "compatible")],
)
def test_gateway_selects_adapter(provider_type, expected):
    gateway = ModelGateway(factories=recording_factories())
    gateway.create(model_ref(provider_type), credential())
    assert gateway.last_factory == expected


def test_gateway_records_arguments_on_injected_factory():
    factories = recording_factories()
    gateway = ModelGateway(factories=factories)
    gateway.create(model_ref("openai"), credential(api_key="sk-abc", base_url="https://api.openai.com/v1"))
    assert factories["openai"].calls == [
        ("openai", model_ref("openai"), credential(api_key="sk-abc", base_url="https://api.openai.com/v1"))
    ]


def test_official_deepseek_receives_provider_options():
    gateway = ModelGateway()
    chat = gateway.create(
        model_ref("deepseek"),
        credential(api_key="dk-abc", base_url="https://api.deepseek.com"),
    )
    assert isinstance(chat, ChatDeepSeek)
    assert chat.model_name == "unit-model"
    assert chat.api_base == "https://api.deepseek.com"
    assert chat.api_key.get_secret_value() == "dk-abc"


def test_ollama_accepts_empty_key():
    gateway = ModelGateway()
    chat = gateway.create(
        model_ref("ollama"),
        credential(api_key=None, base_url="http://localhost:11434"),
    )
    assert isinstance(chat, ChatOllama)


def test_fallback_receives_configured_base_url():
    gateway = ModelGateway()
    adapter = gateway.create(
        model_ref("acme"),
        credential(api_key="ak-9", base_url="http://acme.internal/v1"),
    )
    assert isinstance(adapter, OpenAICompatibleAdapter)
    assert adapter.base_url == "http://acme.internal/v1"
