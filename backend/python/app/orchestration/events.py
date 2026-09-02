"""编排内部事件：Go 将映射为公开 SSE 契约，前端消费。

事件名与设计文档 §8 逐字一致。所有事件负载在发出前必须经过
redact_event_payload 脱敏——API key、授权头等敏感键永不流出 Python 边界。
"""

from __future__ import annotations

from typing import Any, Literal

from pydantic import BaseModel

EventName = Literal[
    "run_started",
    "plan_created",
    "prompt_debug",
    "speaker_started",
    "assistant_delta",
    "assistant_final",
    "memory_proposed",
    "run_interrupted",
    "permission_required",
    "run_completed",
    "run_error",
]

#: 键名（case-insensitive）匹配即脱敏。值统一替换为 [REDACTED]。
_SENSITIVE_KEYS = frozenset(
    {
        "api_key",
        "apikey",
        "authorization",
        "credential",
        "credentials",
        "secret",
        "token",
        "x-api-key",
        "password",
        "cookie",
        "set-cookie",
    }
)


class OrchestrationEvent(BaseModel):
    """一条内部编排事件：run_id 关联轮次，payload 为已脱敏的任意结构。"""

    run_id: str
    name: EventName
    payload: dict[str, Any] = {}


def redact_event_payload(payload: dict[str, Any]) -> dict[str, Any]:
    """递归脱敏事件负载：敏感键替换为 [REDACTED]，不修改输入 dict。"""

    def redact(value: Any) -> Any:
        if isinstance(value, dict):
            return {
                key: (
                    "[REDACTED]"
                    if str(key).casefold() in _SENSITIVE_KEYS
                    else redact(item)
                )
                for key, item in value.items()
            }
        if isinstance(value, list):
            return [redact(item) for item in value]
        return value

    return redact(payload)  # type: ignore[return-value]
