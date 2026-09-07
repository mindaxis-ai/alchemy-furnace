"""模型网关：按 provider_type 选择官方适配器，未知供应商走兼容回退。

选路顺序（设计文档 §6）：
1. 已识别的供应商用官方 LangChain 适配器：openai / deepseek / ollama。
2. 其余按 OpenAI 兼容协议构造 OpenAICompatibleAdapter。

凭据只在此层用于构造模型实例；生成的实例不携带进任何检查点/事件。
"""

from __future__ import annotations

from typing import Any, Callable, Mapping, Optional

from langchain_core.language_models.chat_models import BaseChatModel
from langchain_core.messages import BaseMessage
from langchain_core.outputs import ChatGenerationChunk, ChatResult
from langchain_deepseek import ChatDeepSeek
from langchain_ollama import ChatOllama
from langchain_openai import ChatOpenAI

from app.orchestration.contracts import ModelCredential, ModelRef

#: 未识别供应商走此注册键（OpenAI 兼容回退）。
_FALLBACK_KEY = "compatible"

#: 工厂签名：模型引用 + 凭据 -> 已配置的聊天模型实例。
ProviderFactory = Callable[[ModelRef, ModelCredential], BaseChatModel]


def _official_factories() -> dict[str, ProviderFactory]:
    """默认官方注册表：provider_type(lower) -> 构造工厂。

    ChatDeepSeek 使用 api_base；ChatOllama 无需 api_key；
    ChatOpenAI 以 base_url 传端点（0.3.35 的别名）。
    """

    def openai(ref: ModelRef, cred: ModelCredential) -> BaseChatModel:
        return ChatOpenAI(
            model=ref.name, api_key=cred.api_key, base_url=cred.base_url
        )

    def deepseek(ref: ModelRef, cred: ModelCredential) -> BaseChatModel:
        return ChatDeepSeek(
            model=ref.name, api_key=cred.api_key, api_base=cred.base_url
        )

    def ollama(ref: ModelRef, cred: ModelCredential) -> BaseChatModel:
        return ChatOllama(model=ref.name, base_url=cred.base_url)

    def compatible(ref: ModelRef, cred: ModelCredential) -> BaseChatModel:
        return OpenAICompatibleAdapter.from_config(ref, cred)

    return {
        "openai": openai,
        "deepseek": deepseek,
        "ollama": ollama,
        _FALLBACK_KEY: compatible,
    }


class ModelGateway:
    """模型实例工厂。factories 可注入（测试用录制工厂），缺省用官方注册表。

    last_factory 记录最近一次命中的注册键（provider 名或 "compatible"）。
    """

    def __init__(
        self,
        factories: Mapping[str, ProviderFactory] | None = None,
    ) -> None:
        self._factories: dict[str, ProviderFactory] = dict(
            factories if factories is not None else _official_factories()
        )
        self.last_factory: str | None = None

    def create(self, ref: ModelRef, credential: ModelCredential) -> BaseChatModel:
        provider = ref.provider_type.lower()
        factory = self._factories.get(provider)
        hit = provider
        if factory is None:
            hit = _FALLBACK_KEY
            factory = self._factories.get(_FALLBACK_KEY)
        if factory is None:
            raise ValueError(
                f"provider {ref.provider_type!r} 无可用适配器（含兼容回退）"
            )
        self.last_factory = hit
        return factory(ref, credential)


class OpenAICompatibleAdapter(BaseChatModel):
    """未知供应商的轻量 OpenAI 兼容回退。

    内部委托 ChatOpenAI（OpenAI 兼容协议），把供应商响应规范化为
    langchain 消息对象；不把供应商私有响应对象暴露给调用方。
    流式经 langchain 标准桥接，产出 AIMessageChunk。
    """

    client: ChatOpenAI
    base_url: Optional[str] = None

    @classmethod
    def from_config(
        cls, ref: ModelRef, credential: ModelCredential
    ) -> "OpenAICompatibleAdapter":
        client = ChatOpenAI(
            model=ref.name,
            api_key=credential.api_key,
            base_url=credential.base_url,
        )
        return cls(client=client, base_url=credential.base_url)

    @property
    def _llm_type(self) -> str:
        return "openai-compatible"

    def _generate(
        self,
        messages: list[BaseMessage],
        stop: Optional[list[str]] = None,
        run_manager: Optional[Any] = None,
        **kwargs: Any,
    ) -> ChatResult:
        return self.client._generate(
            messages, stop=stop, run_manager=run_manager, **kwargs
        )

    def _stream(
        self,
        messages: list[BaseMessage],
        stop: Optional[list[str]] = None,
        run_manager: Optional[Any] = None,
        **kwargs: Any,
    ):
        yield from self.client._stream(
            messages, stop=stop, run_manager=run_manager, **kwargs
        )
