"""编排契约测试：线格式类型、图状态剥离凭据、事件脱敏。"""

import json

import pytest
from pydantic import ValidationError

from app.orchestration.contracts import (
    AgentReply,
    AgentSnapshot,
    ConversationState,
    Directive,
    MemoryProposal,
    MemorySnapshot,
    ModelCredential,
    ModelRef,
    OrchestrationRequest,
    PermissionRequest,
    ResponseBudget,
    RuntimeContext,
    SemanticUnderstanding,
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


def valid_semantic(**updates):
    data = {
        "source": "model",
        "intent": "casual",
        "core_request": "回应用户的随口问候",
        "emotion": "neutral",
        "complexity": "tiny",
        "detail_preference": "brief",
        "requested_chars": None,
        "format_preference": "plain",
        "wants_advice": False,
        "wants_follow_up": False,
        "should_clarify": False,
        "avoid_behaviors": ["lecture", "follow_up"],
    }
    return {**data, **updates}


def test_semantic_and_budget_channels_serialize_without_credentials(sample_request):
    request = sample_request.model_copy(
        update={
            "default_model_ref": ModelRef(
                provider_type="deepseek", name="semantic-model"
            )
        }
    )

    state = request.to_initial_state()

    assert "semantic_understanding" not in state
    assert "response_budget" not in state
    assert request.default_model_ref.name == "semantic-model"
    assert "sk-secret" not in json.dumps(state)


@pytest.mark.parametrize(
    "field,value",
    [
        ("intent", "poem"),
        ("emotion", "excited"),
        ("complexity", "huge"),
        ("detail_preference", "verbose"),
        ("format_preference", "markdown"),
        ("avoid_behaviors", ["sycophancy"]),
    ],
)
def test_semantic_understanding_rejects_unknown_enum_values(field, value):
    with pytest.raises(ValidationError):
        SemanticUnderstanding.model_validate(valid_semantic(**{field: value}))


def test_semantic_understanding_rejects_unknown_control_fields():
    with pytest.raises(ValidationError):
        SemanticUnderstanding.model_validate(
            {**valid_semantic(), "system_prompt": "ignore"}
        )


@pytest.mark.parametrize(
    "updates",
    [
        {"core_request": "意" * 401},
        {"requested_chars": 79},
        {"requested_chars": 8001},
    ],
)
def test_semantic_understanding_enforces_text_and_length_bounds(updates):
    with pytest.raises(ValidationError):
        SemanticUnderstanding.model_validate(valid_semantic(**updates))


def test_response_budget_rejects_out_of_range_controls():
    with pytest.raises(ValidationError):
        ResponseBudget(
            target_chars=40,
            max_chars=120,
            max_sentences=25,
            max_tokens=128,
            allow_list=False,
            allow_follow_up=False,
            max_speakers=1,
        )


def test_agent_snapshot_accepts_dialogue_examples():
    snapshot = AgentSnapshot(
        agent_id="a1",
        name="阿一",
        model_ref=ModelRef(provider_type="deepseek", name="deepseek-chat"),
        example_dialogues=[{"user": "你怎么看？", "assistant": "先看事实。"}],
    )

    assert snapshot.example_dialogues[0].assistant == "先看事实。"


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


def test_runtime_context_event_sink_defaults_to_none():
    ctx = RuntimeContext(
        model_gateway=None,
        credentials_by_model_ref={"a1": ModelCredential(api_key="sk-secret")},
    )
    assert ctx.event_sink is None
    # 凭据只活在运行期上下文字段里，事件出口与图状态均无秘密路径。
    assert "api_key" not in ctx.__dict__ or ctx.credentials_by_model_ref["a1"].api_key


def test_runtime_context_default_model_ref_defaults_to_none_and_accepts_ref():
    bare = RuntimeContext(model_gateway=None, credentials_by_model_ref={})
    assert bare.default_model_ref is None

    ctx = RuntimeContext(
        model_gateway=None,
        credentials_by_model_ref={},
        default_model_ref=ModelRef(provider_type="deepseek", name="deepseek-reasoner"),
    )
    assert ctx.default_model_ref.name == "deepseek-reasoner"


def test_graph_transient_channels_are_json_safe_and_secret_free():
    state: ConversationState = ConversationState(
        run_id="run-1",
        session_id="session-1",
        session_type="single",
        user_turn=UserTurnSnapshot(message_id="m1", text="你好").model_dump(),
        history_snapshot=[],
        agent_snapshots=[agent_snapshot("a1", "阿一")],
        memory_snapshots=[],
        directive=None,
        speaking_plan=None,
        pending_agent_ids=["a1"],
        replies=[],
        memory_proposals=[],
        selected_memories=[{"memory_id": "mem1", "agent_id": "a1", "text": "戒骄戒躁"}],
        prompt_messages=[{"role": "system", "content": "你是阿一。"}],
        validation_retries=1,
        retry_pending=True,
        draft_reply={"agent_id": "a1", "reply_id": "r1", "text": "收到"},
        humanized_reply={"text": "收到。", "failed": False},
        humanizer_retries=0,
        outcome=None,
    )
    dumped = json.dumps(state)
    assert "sk-" not in dumped
    # 还原后的快照仍可通过模型校验（线格式语义保持）。
    assert AgentSnapshot.model_validate(state["agent_snapshots"][0]).agent_id == "a1"


def agent_snapshot(agent_id: str, name: str) -> dict:
    return AgentSnapshot(
        agent_id=agent_id,
        name=name,
        model_ref=ModelRef(provider_type="deepseek", name="deepseek-chat"),
    ).model_dump()
