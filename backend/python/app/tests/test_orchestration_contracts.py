"""编排契约测试：线格式类型、图状态剥离凭据、事件脱敏。"""

import json

import pytest

from app.orchestration.contracts import (
    AgentSnapshot,
    MemorySnapshot,
    ModelCredential,
    ModelRef,
    OrchestrationRequest,
    PermissionRequest,
    UserTurnSnapshot,
)
from app.orchestration.events import redact_event_payload


@pytest.fixture
def sample_request() -> OrchestrationRequest:
    return OrchestrationRequest(
        run_id="run-1",
        session_id="session-1",
        session_type="group",
        user_turn=UserTurnSnapshot(
            message_id="m1", text="全体都有", mentioned_agent_ids=[]
        ),
        history_snapshot=[],
        agent_snapshots=[
            AgentSnapshot(
                agent_id="a1",
                name="阿一",
                model_ref=ModelRef(provider_type="deepseek", name="deepseek-chat"),
            )
        ],
        memory_snapshots=[MemorySnapshot(memory_id="mem1", agent_id="a1", text="戒骄戒躁")],
        credentials={"a1": ModelCredential(api_key="sk-secret", base_url="https://api.deepseek.com")},
    )


def test_runtime_credentials_do_not_serialize_into_state(sample_request):
    state = sample_request.to_initial_state()
    dumped = json.dumps(state)
    assert "sk-secret" not in dumped
    assert "api_key" not in dumped


def test_event_redaction_removes_nested_secrets():
    payload = {"authorization": "Bearer x", "nested": {"api_key": "sk-x"}, "model_id": "m1"}
    assert redact_event_payload(payload) == {
        "authorization": "[REDACTED]",
        "nested": {"api_key": "[REDACTED]"},
        "model_id": "m1",
    }


def test_permission_request_is_serializable_without_tool_runtime():
    request = PermissionRequest(request_id="p1", action="tool.execute", summary="读取本地文件")
    assert request.model_dump() == {
        "request_id": "p1", "action": "tool.execute", "summary": "读取本地文件"
    }
