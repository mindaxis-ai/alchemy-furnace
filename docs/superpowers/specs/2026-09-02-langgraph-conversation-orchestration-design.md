# LangGraph Conversation Orchestration Migration Design

**Status:** Approved in conversation; awaiting written-spec review

**Date:** 2026-09-02

**Scope:** Desktop chat orchestration in `alchemy-furnace`

## 1. Purpose

Move all single-chat and group-chat orchestration from Go into a Python LangGraph runtime so future agent behavior can grow without turning the Go API into an agent framework.

The migration must fix the current class of coordination failures, including a simple group command such as `@全体成员 全体都有！报数！` being treated as casual conversation, limited to one speaker, and overridden by persona or memory instructions.

After the migration:

- Go remains the authoritative backend for authentication, business data, encrypted credentials, and CRUD.
- Python owns model-facing behavior and all conversation orchestration through LangGraph.
- Single chat and group chat share one top-level graph and one reusable per-agent graph.
- The desktop application remains the only product delivery target.

## 2. Goals and Non-goals

### Goals

- Migrate all single-chat and group-chat planning, routing, prompt construction, model calls, and memory strategy to LangGraph.
- Support deterministic commands and model-planned open discussion in one graph.
- Make executions resumable and idempotent.
- Use official LangChain provider adapters where they preserve provider behavior.
- Keep a small OpenAI-compatible fallback for unknown providers.
- Preserve Go as the source of truth for messages, agents, pills, memories, models, and providers.
- Expose safe prompt and orchestration diagnostics when Prompt Debug is enabled.
- Reserve stable graph and event boundaries for future tools, approvals, and multi-agent collaboration.

### Non-goals for the first phase

- No complete LangChain Agent or prebuilt agent abstraction.
- No real tool integrations in the first phase.
- No migration of business data ownership from Go to Python.
- No knowledge-base or RAG subsystem.
- No long-term operation of both the legacy Go orchestrator and LangGraph.
- No independent web or cloud deployment product.

## 3. Ownership Boundary

### Go owns

- Desktop/API authentication and request validation.
- Sessions, messages, agents, pills, behavior source data, and formal memories.
- Provider configuration, model configuration, Base URLs, and encrypted credentials.
- Saving user messages and final assistant messages.
- Validating and persisting memory proposals.
- Translating internal Python events into the public frontend SSE contract.
- Run bookkeeping and business-level idempotency.

Go does not choose speakers, classify dialogue intent, compose model prompts, or enforce conversational turn policy after the migration.

### Python/LangGraph owns

- Session routing between single and group chat.
- Directive classification and deterministic command handling.
- Supervisor planning for ambiguous or open-ended group discussion.
- Per-agent prompt construction, context selection, model invocation, and response budgets.
- Speaker ordering, convergence, retries, and orchestration recovery.
- Memory retrieval strategy, distillation decisions, and memory proposals.
- Future tool routing, permission interrupts, planning, and agent collaboration.

## 4. Graph Architecture

One compiled `ConversationGraph` is the stable orchestration entry point:

```text
ConversationGraph
├── hydrate_context
├── route_session
├── SingleChatGraph
│   └── DaoistGraph
└── GroupChatGraph
    ├── classify_directive
    ├── deterministic_router
    ├── supervisor
    ├── dispatch_daoists
    │   └── DaoistGraph × N
    ├── convergence
    └── propose_memory
```

### ConversationGraph

Normalizes the request snapshot, selects single or group execution, emits lifecycle events, and produces one terminal outcome.

### SingleChatGraph

Selects the target Daoist, prepares its relevant context, invokes `DaoistGraph`, and proposes memory after a successful final response.

### GroupChatGraph

First classifies the user turn. Explicit directives follow deterministic code paths; ambiguous discussion is planned by the Supervisor. It dispatches isolated per-agent work, collects results, and decides whether a convergence response is needed.

### DaoistGraph

A reusable subgraph that receives a concrete task rather than deciding the group plan. It selects allowed memories, compiles persona and pill behavior, builds the final messages, invokes `ModelGateway`, validates the response, and emits streaming and final events.

The subgraph boundary ensures that single chat and every group participant use the same prompt and model pipeline.

## 5. Group Direction Strategy

The design uses a hybrid director.

### Deterministic directives

These commands must not be delegated to a language model:

- Direct mention of one or more members.
- `@全体成员` and equivalent all-member addressing.
- Roll call or ordered counting.
- Stop, cancel, and continue.

For a roll call, the router constructs one immutable task per member containing the assigned ordinal. Persona, memory, casual-style rules, and generated commentary may affect tone only; they cannot change the number, omit a member, add unrelated advice, or alter the requested order.

### Supervisor-planned discussion

Ambiguous or open-ended group turns use the currently configured default model as a non-persona Supervisor. It returns a validated structured plan containing selected speakers, order, task per speaker, budgets, and convergence policy.

The Supervisor is not a visible group member and does not write user-facing prose. If it is unavailable or produces an invalid plan, the graph falls back deterministically to explicitly mentioned members or the group's primary member.

Single chats use the selected Daoist's configured model. Explicit group directives do not spend a Supervisor model call.

## 6. Model Gateway

The Python `ModelGateway` creates chat-model instances from Go-provided model references and runtime credentials.

Adapter selection follows this order:

1. Use a provider-specific official LangChain adapter when the provider type is recognized.
2. Use `ChatOpenAI` for OpenAI and ordinary OpenAI-compatible endpoints whose behavior matches the OpenAI contract.
3. Use a small `OpenAICompatibleAdapter` for unknown suppliers and vendor extensions that cannot be represented safely by an official adapter.

Initial recognized mappings cover the provider types already relevant to the application: OpenAI, DeepSeek, and Ollama. The registry is extensible without changing graph nodes. Unknown providers continue to work through the fallback adapter.

Provider-specific options are normalized by the selected adapter. Unsupported metadata must not be silently assumed to survive a generic OpenAI adapter. The exact dependency versions are pinned together during implementation and validated against the existing Python runtime and Apple Silicon packaging pipeline.

The application does not introduce a complete LangChain Agent. Graph nodes and transitions remain explicitly defined by this project.

## 7. State, Runtime Context, and Persistence

Graph state is typed, serializable, and free of secrets:

```text
ConversationState
├── run_id
├── session_id
├── session_type
├── user_turn
├── history_snapshot
├── agent_snapshots
├── memory_snapshots
├── directive
├── speaking_plan
├── pending_agent_ids
├── replies
├── memory_proposals
└── outcome
```

Runtime-only dependencies use LangGraph `context_schema`:

```text
RuntimeContext
├── model_gateway
├── credentials_by_model_ref
├── debug_enabled
└── cancellation
```

API keys, authorization headers, decrypted credentials, and live model clients must never enter graph state, SQLite checkpoints, emitted events, or logs.

LangGraph uses a local asynchronous SQLite checkpointer. Its thread identifier is derived from the session, while `run_id` identifies one user turn. Checkpoints support execution recovery only. Go remains the source of truth for formal conversation history and sends the current authoritative snapshot when starting a new run.

Formal memories remain Go-owned because they support UI CRUD, pinning, editing, deletion, and source tracking. LangGraph chooses which supplied memories to inject and emits proposals for new or updated memories. Go validates and persists accepted proposals.

## 8. Request and Event Contract

The frontend continues to call Go and consume Go's SSE stream. Go starts an internal streaming request to a new Python orchestration endpoint.

The internal request contains identifiers and immutable snapshots, including:

- `run_id`, session identifier, and session type.
- Current user turn and authoritative history snapshot.
- Agent, pill, behavior-source, and memory snapshots.
- Model references and non-secret generation settings.
- Runtime-only provider credentials transmitted over the existing loopback boundary.
- Debug and cancellation context.

Python emits typed internal events:

```text
run_started
plan_created
prompt_debug
speaker_started
assistant_delta
assistant_final
memory_proposed
run_interrupted
run_completed
run_error
```

Go maps these to the existing public SSE format. Streaming deltas are display-only. Go persists an assistant message only after `assistant_final`, keyed by `run_id` and `reply_id`. It validates and persists `memory_proposed` separately.

## 9. Prompt Debug

When Prompt Debug is disabled, graph diagnostics are neither emitted to the frontend nor retained as business data.

When enabled, diagnostics may show:

- Traversed graph nodes.
- Directive classification and routing reason.
- Supervisor speaking plan.
- Final model messages and non-secret generation parameters for each Daoist.
- Memory selection and rejection reasons.
- Retry count, duration, adapter identity, and sanitized error category.

Diagnostics must redact API keys, authorization headers, decrypted credentials, sensitive transport headers, and private provider response internals. Prompt-debug events reference model and provider IDs, never their credentials.

## 10. Failure, Cancellation, and Resume Semantics

Each turn has a globally unique `run_id`; each final Daoist reply has a unique `reply_id`. Go enforces persistence idempotency on those identifiers.

Run states are:

```text
pending -> running -> completed
                  -> failed
                  -> interrupted -> running
                  -> cancelled
```

Rules:

- A model failure retries only the current Daoist, at most two times by default.
- A Supervisor failure uses the deterministic fallback and does not fail the whole turn.
- One failed Daoist does not prevent remaining planned speakers from running.
- A Python process restart resumes from the latest safe checkpoint without repeating completed replies.
- A frontend disconnect causes Go to request cancellation. Completed replies remain; an incomplete streaming fragment is not saved as a formal message.
- User stop transitions the run to `interrupted` after cancellation or the current safe boundary and retains pending speakers.
- User continue resumes the same run and does not invoke completed speakers again.
- A new user message cancels the old run and creates a new `run_id`; it never inherits the old pending plan.
- Memory proposals use stable idempotency keys so retries cannot duplicate formal memories.
- Completed, failed, and cancelled checkpoints are removed according to a bounded local retention policy; formal messages and memories are unaffected.

## 11. Testing and Acceptance

### Node unit tests

- Directive variants: direct mention, multiple mention, all members, roll call, stop, and continue.
- Speaking-plan membership, ordering, assigned ordinals, budgets, and convergence.
- Mechanical task constraints outrank persona, memory, and casual response rules.
- Model adapter selection and fallback behavior.
- Memory selection, proposals, and secret redaction.

### Graph integration tests

Tests use deterministic fake models and make no external provider calls:

- Single chat invokes exactly the selected Daoist.
- All-member roll call assigns a distinct ordinal to every member.
- Open discussion invokes the default model as Supervisor.
- Invalid or failed Supervisor output uses deterministic fallback.
- One Daoist failure does not block the others.
- Resume does not repeat completed replies.
- Cancellation, continue, and new-message supersession follow the state contract.

### Go-Python contract tests

- Event order and schema are stable.
- Final messages and memory proposals are persisted once.
- Partial deltas are not persisted as final messages.
- Python disconnects, timeouts, and errors map to stable Go errors and SSE events.
- Prompt Debug contains useful orchestration evidence and no credentials.

### Desktop acceptance

Wails desktop behavior is authoritative. Acceptance covers Prompt Debug on/off, per-speaker diagnostic association, application restart and resume, preservation of existing data, and macOS Apple Silicon local execution and packaging. Release verification later extends to the repository's supported macOS Intel and Windows x64 targets.

The mandatory roll-call scenario is:

```text
Members: 张雪峰, 李雪琴, 贾玲, 沈腾
User: @全体成员 全体都有！报数！
```

Expected ordered replies are `1`, `2`, `3`, and `4`, one from each listed member. Persona may add only minimal compatible tone; it cannot change the assigned number, omit a member, introduce unrelated exam advice, or let the Supervisor rewrite the task.

## 12. Migration Strategy

Migration is incremental but ends with one implementation:

1. Introduce typed Python state, events, `ModelGateway`, and graph nodes behind tests.
2. Add the internal Go-Python orchestration stream contract and persistence idempotency.
3. Route single chat through LangGraph behind `orchestration_engine=legacy|langgraph`.
4. Route deterministic and Supervisor-based group chat through LangGraph.
5. Validate Prompt Debug, interruption, resume, restart, and desktop packaging.
6. Make LangGraph the default after automated and Wails acceptance tests pass.
7. Remove legacy Go prompt composition, turn policy, and group orchestration after a short rollback window.

The feature flag is a temporary migration control, not a permanent dual-engine product setting.

## 13. Resulting Extension Points

The first phase intentionally provides interfaces, not tool implementations, for:

- `ToolNode` execution.
- Human permission interrupts before sensitive tools.
- Long-running plans and resumable tasks.
- Additional provider adapters.
- Optional dedicated orchestrator-model selection beyond the initial default-model rule.
- More advanced multi-agent delegation and convergence policies.

These extensions can be added inside LangGraph without moving orchestration responsibilities back into Go or changing the frontend's public chat boundary.
