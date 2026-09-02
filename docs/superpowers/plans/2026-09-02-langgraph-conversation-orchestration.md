# LangGraph Conversation Orchestration Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace Go-owned single/group chat orchestration with a resumable Python LangGraph runtime while Go remains the authoritative CRUD, credential, and persistence backend.

**Architecture:** The frontend continues to stream through Go. Go saves business data, creates immutable execution snapshots, and proxies a typed internal SSE protocol; Python runs one `ConversationGraph` containing single-chat, group-chat, and reusable `DaoistGraph` paths. Secrets travel only in runtime context, LangGraph checkpoints contain serializable execution state, and the legacy Go engine exists only behind a temporary rollback flag.

**Tech Stack:** Go 1.x, Gin, GORM, Python 3.12.11, FastAPI, Pydantic 2, LangGraph, LangChain Core/provider adapters, SQLite checkpointer, Next.js 16, React 19, TypeScript, Wails.

**Spec:** `docs/superpowers/specs/2026-09-02-langgraph-conversation-orchestration-design.md`

## Global Constraints

- Wails desktop is the only product delivery target; browser development mode is not acceptance evidence.
- Go owns sessions, messages, agents, pills, memories, models, providers, encrypted credentials, and final persistence.
- Python/LangGraph owns intent classification, speaker selection, prompt construction, model calls, recovery, and memory proposals.
- API keys and decrypted credentials must never enter graph state, checkpoints, logs, debug payloads, or persisted messages.
- Use official provider adapters for OpenAI, DeepSeek, and Ollama; retain one lightweight OpenAI-compatible fallback.
- Do not introduce a complete LangChain Agent, AutoGen, RAG, or real tools in this phase.
- Explicit mentions, all-member commands, roll call, stop, and continue are deterministic and never delegated to the Supervisor.
- Mechanical task constraints outrank persona, pills, memory, and conversational style.
- Write a failing test first, verify the failure, add the minimum implementation, rerun focused and package tests, then commit only the task files.
- Do not overwrite, clean, reformat, or commit pre-existing worktree changes. Stop if a task must touch a file that was already dirty before that task.
- Do not commit generated `frontend/out`, `backend/go/internal/webui/out`, or build artifacts.
- The temporary `orchestration_engine=legacy|langgraph` switch is removed after LangGraph becomes the verified default.

## File Map

| Area | Files | Responsibility |
|---|---|---|
| Python contract | `backend/python/app/orchestration/contracts.py`, `events.py` | Typed state, runtime input, plans, replies, and events |
| Model access | `backend/python/app/orchestration/model_gateway.py` | Official adapter registry and unknown-provider fallback |
| Policies | `backend/python/app/orchestration/directives.py`, `prompts.py` | Deterministic directives and final prompt compilation |
| Graphs | `backend/python/app/orchestration/graphs/*.py` | Daoist, single, group, and top-level graphs |
| Runtime | `backend/python/app/orchestration/runtime.py`, `service.py` | SQLite checkpointer, streaming, cancellation, and resume |
| Python API | `backend/python/app/api/orchestration.py` | Internal orchestration SSE endpoint |
| Go contract/client | `backend/go/internal/service/orchestration/*.go` | Snapshot/event types and Python stream client |
| Go persistence | `backend/go/model/models.go`, chat DAO/service files | Run lifecycle and idempotent final reply persistence |
| Go handlers | `backend/go/server/http/gateway/web/handler/chat/impl_sse_*.go` | Public SSE mapping and migration switch |
| Frontend | existing chat service/context/message files | Run identity, stop/resume, and prompt-debug rendering |

---

### Task 1: Pin LangGraph and provider dependencies

**Files:**
- Modify: `backend/python/requirements.txt`
- Create: `backend/python/app/tests/test_langgraph_dependencies.py`

**Interfaces:**
- Consumes: Python 3.12.11 runtime built by `scripts/build-python-runtime.sh`.
- Produces: Importable, mutually compatible LangGraph and LangChain packages.

- [ ] **Step 1: Add an import smoke test**

```python
def test_langgraph_dependencies_import():
    from langgraph.graph import StateGraph
    from langgraph.checkpoint.sqlite.aio import AsyncSqliteSaver
    from langchain_core.language_models.chat_models import BaseChatModel
    from langchain_openai import ChatOpenAI
    from langchain_deepseek import ChatDeepSeek
    from langchain_ollama import ChatOllama

    assert StateGraph and AsyncSqliteSaver and BaseChatModel
    assert ChatOpenAI and ChatDeepSeek and ChatOllama
```

- [ ] **Step 2: Verify the smoke test fails because dependencies are absent**

Run: `cd backend/python && .venv/bin/pytest app/tests/test_langgraph_dependencies.py -q`

Expected: import failure for `langgraph` or a provider adapter.

- [ ] **Step 3: Pin direct dependencies**

Append exactly these direct requirements and update the existing OpenAI pin:

```text
openai==1.109.1
langgraph==0.6.11
langgraph-checkpoint-sqlite==2.0.11
langchain-core==0.3.86
langchain-openai==0.3.35
langchain-deepseek==0.1.4
langchain-ollama==0.3.10
aiosqlite==0.20.0
```

Run: `cd backend/python && .venv/bin/pip install -r requirements.txt`

- [ ] **Step 4: Verify dependency and existing request-credential tests**

Run: `cd backend/python && .venv/bin/pytest app/tests/test_langgraph_dependencies.py app/tests/test_request_credentials.py -q`

Expected: all selected tests pass.

- [ ] **Step 5: Commit**

```bash
git add backend/python/requirements.txt backend/python/app/tests/test_langgraph_dependencies.py
git commit -m "build(python): add LangGraph runtime dependencies"
```

---

### Task 2: Define the orchestration contract and safe events

**Files:**
- Create: `backend/python/app/orchestration/__init__.py`
- Create: `backend/python/app/orchestration/contracts.py`
- Create: `backend/python/app/orchestration/events.py`
- Create: `backend/python/app/tests/test_orchestration_contracts.py`

**Interfaces:**
- Produces: `OrchestrationRequest`, `ConversationState`, `RuntimeContext`, `SpeakingPlan`, `AgentReply`, `PermissionRequest`, `OrchestrationEvent`, and `redact_event_payload`.

- [ ] **Step 1: Write failing serialization and redaction tests**

```python
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
```

- [ ] **Step 2: Run and verify missing-module failure**

Run: `cd backend/python && .venv/bin/pytest app/tests/test_orchestration_contracts.py -q`

- [ ] **Step 3: Implement typed contracts**

Use Pydantic models for wire data and `TypedDict` for graph state. Keep credentials only in runtime context:

```python
class ConversationState(TypedDict):
    run_id: str
    session_id: str
    session_type: Literal["single", "group"]
    user_turn: UserTurnSnapshot
    history_snapshot: list[MessageSnapshot]
    agent_snapshots: list[AgentSnapshot]
    memory_snapshots: list[MemorySnapshot]
    directive: Directive | None
    speaking_plan: SpeakingPlan | None
    pending_agent_ids: list[str]
    replies: Annotated[list[AgentReply], operator.add]
    memory_proposals: Annotated[list[MemoryProposal], operator.add]
    outcome: RunOutcome | None


@dataclass(frozen=True)
class RuntimeContext:
    model_gateway: "ModelGateway"
    credentials_by_model_ref: Mapping[str, ModelCredential]
    debug_enabled: bool
    cancellation: "CancellationToken"
```

Define event names exactly as specified: `run_started`, `plan_created`, `prompt_debug`, `speaker_started`, `assistant_delta`, `assistant_final`, `memory_proposed`, `run_interrupted`, `permission_required`, `run_completed`, and `run_error`. `PermissionRequest` is only a stable future interrupt contract; no Task in this plan executes a real tool.

- [ ] **Step 4: Run contract tests**

Run: `cd backend/python && .venv/bin/pytest app/tests/test_orchestration_contracts.py -q`

- [ ] **Step 5: Commit**

```bash
git add backend/python/app/orchestration backend/python/app/tests/test_orchestration_contracts.py
git commit -m "feat(orchestration): define state and event contracts"
```

---

### Task 3: Implement ModelGateway and provider adapters

**Files:**
- Create: `backend/python/app/orchestration/model_gateway.py`
- Create: `backend/python/app/tests/test_model_gateway.py`

**Interfaces:**
- Consumes: `ModelRef`, `ModelCredential`, LangChain `BaseChatModel`.
- Produces: `ModelGateway.create(model_ref, credential) -> BaseChatModel` and `OpenAICompatibleAdapter`.

- [ ] **Step 1: Write adapter-selection failures**

```python
@pytest.mark.parametrize(
    ("provider_type", "expected"),
    [("openai", "openai"), ("deepseek", "deepseek"), ("ollama", "ollama"), ("acme", "compatible")],
)
def test_gateway_selects_adapter(provider_type, expected):
    gateway = ModelGateway(factories=recording_factories())
    gateway.create(model_ref(provider_type), credential())
    assert gateway.last_factory == expected
```

Also assert that official DeepSeek receives provider-specific options, Ollama accepts an empty key, and fallback receives the configured Base URL.

- [ ] **Step 2: Run and verify failures**

Run: `cd backend/python && .venv/bin/pytest app/tests/test_model_gateway.py -q`

- [ ] **Step 3: Implement the registry and fallback**

```python
class ModelGateway:
    def create(self, ref: ModelRef, credential: ModelCredential) -> BaseChatModel:
        provider = ref.provider_type.lower()
        if provider == "deepseek":
            return ChatDeepSeek(model=ref.name, api_key=credential.api_key, api_base=credential.base_url)
        if provider == "ollama":
            return ChatOllama(model=ref.name, base_url=credential.base_url)
        if provider == "openai":
            return ChatOpenAI(model=ref.name, api_key=credential.api_key, base_url=credential.base_url)
        return OpenAICompatibleAdapter.from_config(ref, credential)
```

The fallback must subclass or wrap `BaseChatModel`, support async streaming, and translate chunks into `AIMessageChunk` without exposing provider response objects.

- [ ] **Step 4: Run model gateway and legacy credential tests**

Run: `cd backend/python && .venv/bin/pytest app/tests/test_model_gateway.py app/tests/test_request_credentials.py -q`

- [ ] **Step 5: Commit**

```bash
git add backend/python/app/orchestration/model_gateway.py backend/python/app/tests/test_model_gateway.py
git commit -m "feat(orchestration): add provider-aware model gateway"
```

---

### Task 4: Implement deterministic directive classification

**Files:**
- Create: `backend/python/app/orchestration/directives.py`
- Create: `backend/python/app/tests/test_directives.py`

**Interfaces:**
- Consumes: user text and ordered `AgentSnapshot` list.
- Produces: `classify_directive(text, agents) -> Directive` and `build_deterministic_plan(directive, agents) -> SpeakingPlan | None`.

- [ ] **Step 1: Write the roll-call and mention tests**

```python
def test_all_member_roll_call_assigns_stable_ordinals(agents):
    directive = classify_directive("@全体成员 全体都有！报数！", agents)
    plan = build_deterministic_plan(directive, agents)
    assert [(item.agent_id, item.ordinal) for item in plan.items] == [
        ("zhang", 1), ("li", 2), ("jia", 3), ("shen", 4)
    ]
    assert plan.requires_supervisor is False
```

Cover single mention, multiple mentions, `@所有人`, `everyone`, stop, continue, ordinary discussion, duplicated names, and false positives such as prose containing the character `停`.

- [ ] **Step 2: Run and verify failures**

Run: `cd backend/python && .venv/bin/pytest app/tests/test_directives.py -q`

- [ ] **Step 3: Implement normalized rule matching**

Use ordered members as the only source of roll-call numbering. Return a structured directive with `kind`, mentioned IDs, all-member flag, and exact constraints. The roll-call task text must say that output begins with and preserves the assigned integer and forbids unrelated advice.

- [ ] **Step 4: Run directive tests**

Run: `cd backend/python && .venv/bin/pytest app/tests/test_directives.py -q`

- [ ] **Step 5: Commit**

```bash
git add backend/python/app/orchestration/directives.py backend/python/app/tests/test_directives.py
git commit -m "feat(orchestration): route explicit group directives"
```

---

### Task 5: Build prompts and the reusable DaoistGraph

**Files:**
- Create: `backend/python/app/orchestration/prompts.py`
- Create: `backend/python/app/orchestration/graphs/__init__.py`
- Create: `backend/python/app/orchestration/graphs/daoist.py`
- Create: `backend/python/app/tests/test_daoist_graph.py`

**Interfaces:**
- Consumes: `AgentTask`, snapshots, selected memories, and `RuntimeContext`.
- Produces: `compile_messages(...) -> list[BaseMessage]` and `build_daoist_graph() -> StateGraph`.

- [ ] **Step 1: Write failing priority and streaming tests**

```python
async def test_roll_call_constraint_overrides_persona(fake_gateway, roll_call_state):
    events = [event async for event in run_daoist_for_test(roll_call_state, fake_gateway)]
    prompt = fake_gateway.calls[0].messages
    assert "指定编号：2" in prompt[0].content
    assert "不得插入考研建议" in prompt[0].content
    assert final_content(events).strip().startswith("2")
```

Also verify message order, memory filtering, sentence/token budgets, `speaker_started`, deltas, one `assistant_final`, retry count, and secret-free `prompt_debug`.

- [ ] **Step 2: Run and verify failures**

Run: `cd backend/python && .venv/bin/pytest app/tests/test_daoist_graph.py -q`

- [ ] **Step 3: Implement focused nodes**

Implement nodes named `select_memories`, `compile_prompt`, `invoke_model`, `validate_reply`, and `propose_memory`. `invoke_model` uses `runtime.context.model_gateway`; no credential is copied into state. The validator rejects a roll-call reply whose first integer differs from the assigned ordinal and performs only the current-agent retry.

- [ ] **Step 4: Run focused tests**

Run: `cd backend/python && .venv/bin/pytest app/tests/test_daoist_graph.py app/tests/test_model_gateway.py -q`

- [ ] **Step 5: Commit**

```bash
git add backend/python/app/orchestration/prompts.py backend/python/app/orchestration/graphs backend/python/app/tests/test_daoist_graph.py
git commit -m "feat(orchestration): add reusable Daoist graph"
```

---

### Task 6: Implement Supervisor planning and GroupChatGraph

**Files:**
- Create: `backend/python/app/orchestration/supervisor.py`
- Create: `backend/python/app/orchestration/graphs/group.py`
- Create: `backend/python/app/tests/test_group_graph.py`

**Interfaces:**
- Consumes: deterministic directives, default Supervisor model reference, and `DaoistGraph`.
- Produces: `build_group_graph(daoist_graph) -> StateGraph` and validated `SpeakingPlan`.

- [ ] **Step 1: Write failing hybrid-director tests**

```python
async def test_roll_call_never_calls_supervisor(group_runner, fake_gateway):
    replies = await group_runner("@全体成员 报数")
    assert fake_gateway.supervisor_calls == 0
    assert [reply.content.strip() for reply in replies] == ["1", "2", "3", "4"]


async def test_open_discussion_uses_supervisor(group_runner, fake_gateway):
    await group_runner("你们怎么看这个选择？")
    assert fake_gateway.supervisor_calls == 1
```

Test invalid structured output, unavailable default model, one Daoist failure, ordered dispatch, and no duplicate completed reply.

- [ ] **Step 2: Run and verify failures**

Run: `cd backend/python && .venv/bin/pytest app/tests/test_group_graph.py -q`

- [ ] **Step 3: Implement Supervisor and group nodes**

The Supervisor uses `with_structured_output(SpeakingPlan)` and receives no persona. Implement nodes `classify_directive`, `deterministic_router`, `supervisor`, `dispatch_daoists`, and `convergence`. Invalid Supervisor output falls back to mentioned agents or the first ordered member.

- [ ] **Step 4: Run group tests**

Run: `cd backend/python && .venv/bin/pytest app/tests/test_group_graph.py app/tests/test_directives.py -q`

- [ ] **Step 5: Commit**

```bash
git add backend/python/app/orchestration/supervisor.py backend/python/app/orchestration/graphs/group.py backend/python/app/tests/test_group_graph.py
git commit -m "feat(orchestration): add hybrid group director"
```

---

### Task 7: Build ConversationGraph, checkpointing, and resume

**Files:**
- Create: `backend/python/app/orchestration/graphs/single.py`
- Create: `backend/python/app/orchestration/graphs/conversation.py`
- Create: `backend/python/app/orchestration/runtime.py`
- Create: `backend/python/app/tests/test_conversation_graph.py`
- Modify: `backend/python/app/core/config.py`

**Interfaces:**
- Consumes: single/group subgraphs and SQLite checkpoint location.
- Produces: `ConversationRuntime.start(request)`, `resume(run_id)`, `cancel(run_id)`, and async event iteration.

- [ ] **Step 1: Write failing routing and resume tests**

```python
async def test_resume_skips_completed_speakers(runtime, interrupted_group_request):
    first_events = await collect_until_interrupt(runtime.start(interrupted_group_request))
    resumed_events = await collect(runtime.resume(interrupted_group_request.run_id))
    assert completed_reply_ids(first_events).isdisjoint(new_reply_ids(resumed_events))
    assert all_expected_agents(first_events + resumed_events)
```

Also verify single routing, group routing, new-run supersession, checkpoint state contains no `api_key`, and terminal checkpoint cleanup selection.

- [ ] **Step 2: Run and verify failures**

Run: `cd backend/python && .venv/bin/pytest app/tests/test_conversation_graph.py -q`

- [ ] **Step 3: Implement runtime and graph**

Add configuration `orchestration_checkpoint_path`, defaulting beneath the existing writable application runtime/data directory rather than the source tree. Compile the graph with `AsyncSqliteSaver`; use session UUID as `thread_id` and `run_id` as checkpoint namespace. Maintain in-memory cancellation tokens only for active runs.

- [ ] **Step 4: Run graph tests**

Run: `cd backend/python && .venv/bin/pytest app/tests/test_conversation_graph.py app/tests/test_group_graph.py app/tests/test_daoist_graph.py -q`

- [ ] **Step 5: Commit**

```bash
git add backend/python/app/orchestration/graphs backend/python/app/orchestration/runtime.py backend/python/app/core/config.py backend/python/app/tests/test_conversation_graph.py
git commit -m "feat(orchestration): add resumable conversation graph"
```

---

### Task 8: Expose the internal Python orchestration SSE API

**Files:**
- Create: `backend/python/app/orchestration/service.py`
- Create: `backend/python/app/api/orchestration.py`
- Create: `backend/python/app/tests/test_orchestration_api.py`
- Modify: `backend/python/app/main.py`

**Interfaces:**
- Produces: `POST /api/v1/orchestration/runs/stream`, `POST /api/v1/orchestration/runs/{run_id}/resume`, and `POST /api/v1/orchestration/runs/{run_id}/cancel`.

- [ ] **Step 1: Write failing API event-order tests**

```python
def test_stream_emits_typed_sse_in_order(client, request_json, fake_runtime):
    response = client.post("/api/v1/orchestration/runs/stream", json=request_json)
    assert response.status_code == 200
    assert event_names(response.text) == [
        "run_started", "plan_created", "speaker_started",
        "assistant_delta", "assistant_final", "run_completed",
    ]
```

Test Pydantic rejection, sanitized errors, cancel, resume, and client disconnect propagation.

- [ ] **Step 2: Run and verify route-not-found failure**

Run: `cd backend/python && .venv/bin/pytest app/tests/test_orchestration_api.py -q`

- [ ] **Step 3: Implement the router and service**

Encode each event as `event: <name>\ndata: <json>\n\n`. Set `Cache-Control: no-cache` and `X-Accel-Buffering: no`. Errors expose stable codes only; raw provider exceptions stay server-side after sanitization.

- [ ] **Step 4: Run orchestration API and existing Python tests**

Run: `cd backend/python && .venv/bin/pytest app/tests/test_orchestration_api.py -q`

Run: `cd backend/python && .venv/bin/pytest -q`

- [ ] **Step 5: Commit**

```bash
git add backend/python/app/orchestration/service.py backend/python/app/api/orchestration.py backend/python/app/tests/test_orchestration_api.py backend/python/app/main.py
git commit -m "feat(orchestration): expose internal streaming API"
```

---

### Task 9: Add Go run persistence and reply idempotency

**Files:**
- Modify: `backend/go/model/models.go`
- Modify: `backend/go/internal/interface/dao/chat_dao.go`
- Modify: `backend/go/internal/dao/chat_dao.go`
- Modify: `backend/go/internal/dao/chat_dao_test.go`
- Modify: `backend/go/internal/dao/migrate.go`
- Modify: `backend/go/internal/dao/migrate_smoke_test.go`

**Interfaces:**
- Produces: `model.ChatRun`, `ChatMessage.RunID`, `ChatMessage.ReplyID`, and DAO methods `CreateRun`, `UpdateRunStatus`, `TakeRunByUUID`, and `SaveFinalReplyOnce`.

- [ ] **Step 1: Write failing DAO tests**

```go
func TestSaveFinalReplyOnceIsIdempotent(t *testing.T) {
	first, err := dao.SaveFinalReplyOnce(ctx, run.UUID, "reply-1", message)
	require.NoError(t, err)
	second, err := dao.SaveFinalReplyOnce(ctx, run.UUID, "reply-1", message)
	require.NoError(t, err)
	require.Equal(t, first.ID, second.ID)
}
```

Test legal status transitions and uniqueness of `(run_id, reply_id)`.

- [ ] **Step 2: Run and verify missing-type/method failures**

Run: `cd backend/go && go test ./internal/dao -run 'TestSaveFinalReplyOnce|TestChatRun' -count=1`

- [ ] **Step 3: Implement the schema and DAO transaction**

`ChatRun` stores UUID, session ID, user message ID, status, engine, created/updated timestamps. `ChatMessage` stores nullable run UUID and reply ID. Use a unique composite index and return the existing row after a duplicate insert attempt.

- [ ] **Step 4: Run DAO and migration tests**

Run: `cd backend/go && go test ./internal/dao ./internal/migration/... -count=1`

- [ ] **Step 5: Commit**

```bash
git add backend/go/model/models.go backend/go/internal/interface/dao/chat_dao.go backend/go/internal/dao/chat_dao.go backend/go/internal/dao/chat_dao_test.go backend/go/internal/dao/migrate.go backend/go/internal/dao/migrate_smoke_test.go
git commit -m "feat(chat): persist orchestration runs idempotently"
```

---

### Task 10: Add the Go internal orchestration client

**Files:**
- Create: `backend/go/internal/service/orchestration/types.go`
- Create: `backend/go/internal/service/orchestration/client.go`
- Create: `backend/go/internal/service/orchestration/client_test.go`

**Interfaces:**
- Produces: `Client.Stream(ctx, Request, emit func(Event) error) error`, `Client.Resume`, and `Client.Cancel`.
- Consumes: Python event names from Task 2 and dynamic engine endpoint resolution.

- [ ] **Step 1: Write failing SSE parser tests**

```go
func TestClientStreamsTypedEvents(t *testing.T) {
	server := newSSEServer("run_started", "assistant_delta", "assistant_final", "run_completed")
	client := NewClient(engineendpoint.Static(server.URL))
	var names []string
	err := client.Stream(context.Background(), validRequest(), func(e Event) error {
		names = append(names, e.Name)
		return nil
	})
	require.NoError(t, err)
	require.Equal(t, []string{"run_started", "assistant_delta", "assistant_final", "run_completed"}, names)
}
```

Test lines larger than 64 KB, interrupted streams, context cancellation, unknown event rejection, and safe error mapping.

- [ ] **Step 2: Run and verify failures**

Run: `cd backend/go && go test ./internal/service/orchestration -count=1`

- [ ] **Step 3: Implement request/event types and client**

Use `bufio.Reader.ReadBytes('\n')`, not `bufio.Scanner`. JSON types must mirror Python wire names exactly. Never log or format the request because it contains runtime credentials.

- [ ] **Step 4: Run package tests**

Run: `cd backend/go && go test ./internal/service/orchestration -count=1`

- [ ] **Step 5: Commit**

```bash
git add backend/go/internal/service/orchestration
git commit -m "feat(chat): add LangGraph stream client"
```

---

### Task 11: Build authoritative Go execution snapshots

**Files:**
- Create: `backend/go/internal/service/chat_service/orchestration_snapshot.go`
- Create: `backend/go/internal/service/chat_service/orchestration_snapshot_test.go`
- Modify: `backend/go/internal/service/credential/credential.go`
- Modify: `backend/go/internal/service/credential/resolver.go`
- Modify: credential resolver tests adjacent to these files

**Interfaces:**
- Consumes: session, ordered members, history, behavior profiles, memories, default model, and resolved credentials.
- Produces: `BuildOrchestrationRequest(ctx, session, userMessage, run) (orchestration.Request, error)`.

- [ ] **Step 1: Write failing snapshot tests**

```go
func TestBuildOrchestrationRequestPreservesMemberOrderAndProviderType(t *testing.T) {
	req := buildSnapshotFixture(t)
	require.Equal(t, []string{"zhang", "li", "jia", "shen"}, agentIDs(req.Agents))
	require.Equal(t, "deepseek", req.Models[0].ProviderType)
	require.NotContains(t, mustJSON(req.StateProjection()), "sk-test")
}
```

Test single/group snapshots, default Supervisor resolution, memory-enabled filtering, retry history, and inactive-model failure.

- [ ] **Step 2: Run and verify failures**

Run: `cd backend/go && go test ./internal/service/chat_service -run 'TestBuildOrchestrationRequest' -count=1`

- [ ] **Step 3: Add provider metadata and snapshot builder**

Extend `ModelCredentials` with non-secret `ModelRef`, `ProviderName`, and `ProviderType`; populate them in the resolver from `LLMProvider.Name` and `Protocol`. The snapshot builder sends raw structured behavior profiles and memory snippets, not a Go-composed system prompt.

- [ ] **Step 4: Run chat-service and credential tests**

Run: `cd backend/go && go test ./internal/service/chat_service ./internal/service/credential -count=1`

- [ ] **Step 5: Commit**

```bash
git add backend/go/internal/service/chat_service/orchestration_snapshot.go backend/go/internal/service/chat_service/orchestration_snapshot_test.go backend/go/internal/service/credential
git commit -m "feat(chat): build LangGraph execution snapshots"
```

---

### Task 12: Migrate single-chat streaming behind the engine switch

**Files:**
- Modify: `backend/go/internal/interface/service/chat.go`
- Modify: `backend/go/internal/service/chat_service/chat_service.go`
- Create: `backend/go/internal/service/chat_service/orchestration_turn.go`
- Create: `backend/go/internal/service/chat_service/orchestration_turn_test.go`
- Modify: `backend/go/server/http/gateway/web/handler/chat/impl_sse_chat.go`
- Modify: `backend/go/server/http/gateway/web/handler/chat/impl_sse_chat_test.go`
- Modify: `backend/go/internal/configuration/configuration.go`
- Modify: `backend/go/internal/configuration/loader/loader.go`
- Modify: `backend/go/internal/configuration/loader/loader_test.go`

**Interfaces:**
- Produces: `RunConversation(ctx, ConversationCommand, emit)` and temporary `orchestration_engine` setting.

- [ ] **Step 1: Write failing single-stream tests**

```go
func TestLangGraphSinglePersistsOnlyAssistantFinal(t *testing.T) {
	stream := events(delta("半"), delta("截"), final("reply-1", "完整回答"), completed())
	result := runLangGraphSingle(t, stream)
	require.Equal(t, []string{"完整回答"}, result.PersistedAssistantContents)
	require.Equal(t, 1, result.ReplyInsertCount)
}
```

Test accepted, chunk mapping, prompt debug, terminal errors, cancellation without partial persistence, retry idempotency, and legacy selection when explicitly configured.

- [ ] **Step 2: Run and verify failures**

Run: `cd backend/go && go test ./internal/service/chat_service ./server/http/gateway/web/handler/chat -run 'TestLangGraphSingle|TestSSEChat' -count=1`

- [ ] **Step 3: Implement service delegation and handler simplification**

The handler validates input and delegates. It must no longer call `turnpolicy.BuildTurnPlan`, `behavior.ComposeSystemPrompt`, memory retrieval, or `StreamChat` when the selected engine is LangGraph. Map internal `assistant_delta` to public `chunk` and persist only `assistant_final` through `SaveFinalReplyOnce`.

- [ ] **Step 4: Run focused Go tests**

Run: `cd backend/go && go test ./internal/service/chat_service ./server/http/gateway/web/handler/chat -count=1`

- [ ] **Step 5: Commit**

```bash
git add backend/go/internal/interface/service/chat.go backend/go/internal/service/chat_service backend/go/server/http/gateway/web/handler/chat/impl_sse_chat.go backend/go/server/http/gateway/web/handler/chat/impl_sse_chat_test.go backend/go/internal/configuration/configuration.go backend/go/internal/configuration/loader/loader.go backend/go/internal/configuration/loader/loader_test.go
git commit -m "feat(chat): route single chat through LangGraph"
```

---

### Task 13: Migrate group streaming, debug, and memory proposals

**Files:**
- Modify: `backend/go/internal/service/chat_service/group_orchestrator.go`
- Modify: `backend/go/internal/service/chat_service/group_orchestrator_test.go`
- Modify: `backend/go/server/http/gateway/web/handler/chat/impl_sse_group.go`
- Modify: `backend/go/server/http/gateway/web/handler/chat/impl_sse_chat_test.go`
- Modify: existing prompt-debug files introduced before this migration
- Create: `backend/go/internal/service/chat_service/memory_proposal.go`
- Create: `backend/go/internal/service/chat_service/memory_proposal_test.go`

**Interfaces:**
- Consumes: internal group events and Go memory service.
- Produces: existing public `speaker_start`, `chunk`, `speaker_done`, `prompt_debug`, `turn_done`, `stopped`, and `error` events.

- [ ] **Step 1: Write failing roll-call contract test**

```go
func TestLangGraphGroupRollCallMapsFourOrderedReplies(t *testing.T) {
	result := runGroupFixture(t, "@全体成员 全体都有！报数！")
	require.Equal(t, []string{"张雪峰:1", "李雪琴:2", "贾玲:3", "沈腾:4"}, result.SpeakerReplies)
	require.Equal(t, 0, result.SupervisorCalls)
}
```

Test per-speaker failure continuation, prompt-debug association, memory proposal validation/idempotency, stop, resume, and no partial-message persistence.

- [ ] **Step 2: Run and verify failures**

Run: `cd backend/go && go test ./internal/service/chat_service ./server/http/gateway/web/handler/chat -run 'TestLangGraphGroup|TestMemoryProposal' -count=1`

- [ ] **Step 3: Replace LangGraph-path group decisions with event mapping**

`RunGroupTurn` becomes a compatibility entry that delegates to `RunConversation` under LangGraph. Go must not select candidates, calculate rounds, build prompts, or evaluate `[PASS]` on that path. Validate memory proposal target agent, size, source run, and duplicate key before calling the memory service.

- [ ] **Step 4: Run group and prompt-debug tests**

Run: `cd backend/go && go test ./internal/service/chat_service ./server/http/gateway/web/handler/chat -count=1`

- [ ] **Step 5: Commit**

```bash
git add backend/go/internal/service/chat_service backend/go/server/http/gateway/web/handler/chat
git commit -m "feat(chat): route group chat through LangGraph"
```

---

### Task 14: Add frontend run identity and resume behavior

**Files:**
- Modify: `frontend/services/types.ts`
- Modify: `frontend/services/chatService.ts`
- Modify: `frontend/services/chatService.test.ts`
- Modify: `frontend/contexts/ChatContext.tsx`
- Modify: `frontend/components/chat-message.tsx`
- Modify: `frontend/components/chat-message.test.tsx`

**Interfaces:**
- Consumes: accepted/run-interrupted/stopped public SSE payloads containing `run_id`.
- Produces: stop and continue actions that address the correct run.

- [ ] **Step 1: Write failing client/context tests**

```tsx
it('resumes the interrupted run without resending a new user message', async () => {
  seedInterruptedRun('run-1')
  await continueGeneration()
  expect(fetchMock).toHaveBeenCalledWith(expect.stringContaining('/resume'), expect.anything())
  expect(savedUserMessages()).toHaveLength(1)
})
```

Test that a new user message cancels the old run, completed replies remain, partial fragments are removed, and Prompt Debug still associates with the correct speaker.

- [ ] **Step 2: Run and verify failures**

Run: `cd frontend && pnpm vitest run services/chatService.test.ts components/chat-message.test.tsx`

- [ ] **Step 3: Implement minimal run-aware state**

Store only the active/interrupted `run_id` required for controls. Do not persist credentials, full prompt-debug payloads, or duplicate message content in browser storage.

- [ ] **Step 4: Run tests, lint, and production build**

Run: `cd frontend && pnpm vitest run services/chatService.test.ts components/chat-message.test.tsx`

Run: `cd frontend && pnpm lint`

Run: `cd frontend && pnpm build`

- [ ] **Step 5: Commit**

```bash
git add frontend/services/types.ts frontend/services/chatService.ts frontend/services/chatService.test.ts frontend/contexts/ChatContext.tsx frontend/components/chat-message.tsx frontend/components/chat-message.test.tsx
git commit -m "feat(frontend): resume interrupted LangGraph runs"
```

---

### Task 15: Make LangGraph authoritative and remove legacy orchestration

**Files:**
- Delete: `backend/go/internal/service/chat_service/group_orchestrator.go`
- Delete: `backend/go/internal/service/chat_service/group_orchestrator_test.go`
- Delete: `backend/go/internal/service/chat_service/group_prompt.go`
- Delete: `backend/go/internal/service/chat_service/group_prompt_test.go`
- Delete: `backend/go/internal/service/chat_service/sentence_limiter.go`
- Delete: `backend/go/internal/service/chat_service/sentence_limiter_test.go`
- Delete: `backend/go/internal/service/chat_service/stream_interruption_test.go`
- Delete: `backend/go/internal/behavior/activation.go`
- Delete: `backend/go/internal/behavior/activation_test.go`
- Delete: `backend/go/internal/behavior/fewshot.go`
- Delete: `backend/go/internal/behavior/fewshot_test.go`
- Delete: `backend/go/internal/behavior/render_dynamic.go`
- Delete: `backend/go/internal/behavior/render_dynamic_test.go`
- Delete: `backend/go/internal/service/turnpolicy/activated_rule.go`
- Delete: `backend/go/internal/service/turnpolicy/response_policy.go`
- Delete: `backend/go/internal/service/turnpolicy/response_policy_test.go`
- Delete: `backend/go/internal/service/turnpolicy/turn_intent.go`
- Delete: `backend/go/internal/service/turnpolicy/turn_intent_test.go`
- Delete: `backend/go/internal/service/turnpolicy/turn_plan.go`
- Delete: `backend/go/internal/service/turnpolicy/turn_plan_test.go`
- Delete: `backend/go/internal/service/turnpolicy/user_constraints.go`
- Delete: `backend/go/internal/service/turnpolicy/user_constraints_test.go`
- Modify: `backend/go/internal/service/chat_service/chat_service.go`
- Modify: `backend/go/internal/interface/service/chat.go`
- Modify: `backend/go/internal/interface/service/memory.go`
- Modify: `backend/go/internal/service/memory_service/memory_service.go`
- Modify: `backend/go/internal/service/memory_service/memory_service_test.go`
- Modify: `backend/go/server/http/gateway/web/handler/chat/impl_sse_chat.go`
- Modify: `backend/go/server/http/gateway/web/handler/chat/impl_sse_group.go`
- Modify: `backend/go/internal/configuration/configuration.go`
- Modify: `backend/go/internal/configuration/loader/loader.go`
- Modify: `backend/go/internal/configuration/loader/loader_test.go`
- Modify: `backend/python/app/services/chat_service.py`
- Modify: `backend/python/app/api/chat.py`
- Modify: `docs/architecture.md`

**Interfaces:**
- Produces: one production chat engine: LangGraph.

- [ ] **Step 1: Prove both engines pass before removing rollback code**

Run the focused Python, Go, and frontend commands from Tasks 8, 13, and 14 with `orchestration_engine=langgraph`, then repeat legacy Go focused tests with the temporary setting set to `legacy`.

- [ ] **Step 2: Record the exact legacy references before deletion**

```bash
rg -n 'orchestration_engine|BuildTurnPlan|ComposeSystemPrompt|BuildGroupSystemPrompt|SentenceLimiter|/chat/completions/stream' backend/go backend/python/app
```

Expected before deletion: matches are limited to the temporary switch, legacy Go chat orchestration, its tests, and Python's legacy streaming route. Save this output in the task report so deletion scope is reviewable.

- [ ] **Step 3: Remove the flag and dead Go orchestration**

Delete the listed chat-only files and remove the temporary configuration switch. Move the surviving `MemorySnippet` value type from `turnpolicy` to `internal/interface/service/memory.go`, update memory service/chat snapshot references, then delete the now-unreferenced `turnpolicy` package. Remove only the streaming functions from `backend/python/app/services/chat_service.py` and `/completions/stream` from `backend/python/app/api/chat.py`; preserve non-streaming `/chat/completions`, which is still used by trials, memory distillation, provider tests, and fusion. Preserve static behavior-profile compilation used by language-pattern synthesis.

Run the same `rg` command again. Expected after deletion: no match for `orchestration_engine`, `BuildTurnPlan`, `ComposeSystemPrompt`, `BuildGroupSystemPrompt`, `SentenceLimiter`, or `/chat/completions/stream` in production files.

- [ ] **Step 4: Run full verification**

Run: `make test`

Run: `cd frontend && pnpm lint && pnpm build`

Run: `scripts/build-python-runtime.sh darwin-arm64`

Run the Wails desktop application and manually verify single chat, open group discussion, four-person roll call, Prompt Debug off/on, stop, resume, application restart recovery, and existing history/memory preservation. Record exact commands and any unrelated baseline failures without modifying them.

- [ ] **Step 5: Update architecture and commit**

Document the Go/Python ownership boundary, graph topology, event flow, checkpoint path, and secret boundary in `docs/architecture.md`.

```bash
git add backend/go/internal/service/chat_service/group_orchestrator.go backend/go/internal/service/chat_service/group_orchestrator_test.go backend/go/internal/service/chat_service/group_prompt.go backend/go/internal/service/chat_service/group_prompt_test.go backend/go/internal/service/chat_service/sentence_limiter.go backend/go/internal/service/chat_service/sentence_limiter_test.go backend/go/internal/service/chat_service/stream_interruption_test.go backend/go/internal/behavior/activation.go backend/go/internal/behavior/activation_test.go backend/go/internal/behavior/fewshot.go backend/go/internal/behavior/fewshot_test.go backend/go/internal/behavior/render_dynamic.go backend/go/internal/behavior/render_dynamic_test.go backend/go/internal/service/turnpolicy backend/go/internal/service/chat_service/chat_service.go backend/go/internal/interface/service/chat.go backend/go/internal/interface/service/memory.go backend/go/internal/service/memory_service/memory_service.go backend/go/internal/service/memory_service/memory_service_test.go backend/go/server/http/gateway/web/handler/chat/impl_sse_chat.go backend/go/server/http/gateway/web/handler/chat/impl_sse_group.go backend/go/internal/configuration/configuration.go backend/go/internal/configuration/loader/loader.go backend/go/internal/configuration/loader/loader_test.go backend/python/app/services/chat_service.py backend/python/app/api/chat.py docs/architecture.md
git diff --cached --name-only
git commit -m "refactor(chat): make LangGraph the conversation runtime"
```

Before committing, unstage generated artifacts and any file not intentionally changed by this task. Do not push, merge, tag, bump versions, or publish a release.

## Definition of Done

- Single and group chat production paths execute through `ConversationGraph`.
- `@全体成员 全体都有！报数！` yields ordered `1`, `2`, `3`, `4` replies with no Supervisor call and no unrelated persona advice.
- Open discussion uses the default model as a structured non-persona Supervisor and falls back deterministically on failure.
- Official OpenAI, DeepSeek, and Ollama adapters work; an unknown OpenAI-compatible provider uses the fallback.
- Go persists user messages, final replies, run state, and validated memories exactly once.
- Python checkpoints restore interrupted work without replaying completed speakers.
- Prompt Debug shows graph decisions and final model messages while exposing no credentials.
- Partial streamed content is not persisted as a formal reply.
- The temporary legacy switch and obsolete Go chat orchestration are removed.
- Focused tests, full Go/Python tests, frontend lint/build, Python runtime assembly, and Wails desktop acceptance have documented results.
