"""编排契约类型：线格式快照、图状态、运行期上下文与中断契约。

- Pydantic 模型 = 跨 Go↔Python 边界的线数据（可序列化、有校验）。
- ConversationState = TypedDict，LangGraph 图状态；只含可序列化执行数据，
  永不携带凭据（凭据仅存 RuntimeContext，随 context_schema 注入）。
- RuntimeContext = frozen dataclass，运行期依赖注入面。
"""

from __future__ import annotations

from dataclasses import dataclass, field
from typing import (
    TYPE_CHECKING,
    Annotated,
    Callable,
    Literal,
    Mapping,
    NotRequired,
    TypedDict,
    cast,
    operator,
)

from pydantic import BaseModel, ConfigDict, Field

if TYPE_CHECKING:  # 仅类型检查；实际类由 Task 3+ 图模块提供
    from orchestration.events import OrchestrationEvent
    from orchestration.model_gateway import ModelGateway
    from orchestration.runtime import CancellationToken

# ---------------------------------------------------------------------------
# 线格式快照（Go 权威数据的一次性不可变快照）
# ---------------------------------------------------------------------------


class ModelRef(BaseModel):
    """模型引用：provider_type 决定适配器；name 为供应商侧模型名。"""

    provider_type: str
    name: str


class ModelCredential(BaseModel):
    """运行期解密凭据（仅 loopback 边界随请求传递）。api_key 可为空（如 Ollama）。"""

    api_key: str | None = None
    base_url: str | None = None


class DialogueExample(BaseModel):
    """人物的短示例对白，只提供语感，不携带控制字段。"""

    model_config = ConfigDict(extra="forbid")

    user: str
    assistant: str


class SemanticUnderstanding(BaseModel):
    """语义模型的严格结构化输出；自由文本只保留受限的需求摘要。"""

    model_config = ConfigDict(extra="forbid")

    source: Literal["model", "fallback"]
    intent: Literal["casual", "vent", "factual", "advice", "task", "deep_dive"]
    core_request: str = Field(max_length=400)
    emotion: Literal["neutral", "positive", "sad", "frustrated", "angry", "anxious"]
    complexity: Literal["tiny", "simple", "moderate", "complex"]
    detail_preference: Literal["brief", "normal", "detailed"]
    requested_chars: int | None = Field(default=None, ge=80, le=8000)
    format_preference: Literal["plain", "list", "steps", "code", "creative"]
    wants_advice: bool
    wants_follow_up: bool
    should_clarify: bool
    avoid_behaviors: list[
        Literal["lecture", "repeat", "summary", "list", "follow_up"]
    ]


class ResponseBudget(BaseModel):
    """导演层给生成与最终校验共同使用的硬预算。"""

    model_config = ConfigDict(extra="forbid")

    target_chars: int = Field(ge=1, le=8000)
    max_chars: int = Field(ge=0, le=8000)
    max_sentences: int = Field(ge=0, le=24)
    max_tokens: int = Field(ge=32, le=4096)
    allow_list: bool
    allow_follow_up: bool
    max_speakers: int = Field(ge=1, le=32)


class UserTurnSnapshot(BaseModel):
    """当前用户轮。mentioned_agent_ids 保留显式 @ 命中，供确定性路由。"""

    message_id: str
    text: str
    mentioned_agent_ids: list[str] = []


class MessageSnapshot(BaseModel):
    """一条历史消息。assistant 消息带 agent_id 以区分发言人。"""

    message_id: str
    role: Literal["user", "assistant"]
    text: str
    agent_id: str | None = None


class AgentSnapshot(BaseModel):
    """参与会话的道人（含其模型引用）。"""

    model_config = ConfigDict(protected_namespaces=())  # 允许 model_ref 等字段名

    agent_id: str
    name: str
    system_prompt: str = ""
    model_ref: ModelRef
    example_dialogues: list[DialogueExample] = Field(default_factory=list)


class MemorySnapshot(BaseModel):
    """Go 侧正式记忆快照；图只选择注入哪些，不改写。"""

    memory_id: str
    agent_id: str
    text: str


# ---------------------------------------------------------------------------
# 指令 / 发言计划 / 回复 / 记忆提案
# ---------------------------------------------------------------------------

DirectiveKind = Literal[
    "direct_mention", "all_members", "roll_call", "stop", "continue", "open"
]


class Directive(BaseModel):
    """确定性指令分类结果。kind 为 open 时由 Supervisor 规划发言计划。

    all_members 标记 @全体 等全量地址；mentioned_agent_ids 为显式 @ 命中
    （按 agents 传入顺序去重保序）。
    """

    kind: DirectiveKind
    mentioned_agent_ids: list[str] = []
    all_members: bool = False


class SpeakingPlanItem(BaseModel):
    """计划中的一条发言任务。

    ordinal 仅报数任务携带（有序成员序号 1 起）；task 为该发言人的机械
    约束文本（可空 = 常规回应）。机械约束高于人设/记忆/闲聊风格。
    """

    agent_id: str
    ordinal: int | None = None
    task: str | None = None


class SpeakingPlan(BaseModel):
    """确定性路由或 Supervisor 产出的发言计划：有序条目 + 是否需要 Supervisor。

    requires_supervisor 供调度方区分计划来源；为 False 时节点不得再调模型改写。
    """

    items: list[SpeakingPlanItem] = []
    requires_supervisor: bool = False
    reason: str | None = None


class AgentReply(BaseModel):
    """单条道人最终回复。reply_id 全局唯一，Go 以此做幂等持久化。"""

    reply_id: str
    run_id: str
    agent_id: str
    text: str


class MemoryProposal(BaseModel):
    """记忆提案：proposal_id 幂等键，Go 校验后决定是否持久化为正式记忆。"""

    proposal_id: str
    agent_id: str
    content: str


class PermissionRequest(BaseModel):
    """未来工具的稳定人工审批中断契约。本阶段只定义与传输，不执行真实工具。"""

    request_id: str
    action: str
    summary: str


RunOutcome = Literal["completed", "failed", "interrupted", "cancelled"]

# ---------------------------------------------------------------------------
# 图状态（LangGraph 检查点载体，必须可 JSON 序列化且无秘密）
# ---------------------------------------------------------------------------


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

    # ---- DaoistGraph 运行期瞬时通道（NotRequired；不进线格式，Go 不感知）----
    # 节点间中间产物：已筛选记忆、编译好的 prompt 消息、重试计数、待校验回复，
    # 以及 validate_reply 的重试裁决（条件边的唯一依据，避免节点/路由谓词漂移）。
    # 全部保持普通 dict/list/bool 形态——可 JSON 序列化进检查点、永不携带凭据。
    selected_memories: NotRequired[list[dict]]
    prompt_messages: NotRequired[list[dict]]
    validation_retries: NotRequired[int]
    draft_reply: NotRequired[dict]
    humanized_reply: NotRequired[dict]
    humanizer_retries: NotRequired[int]
    retry_pending: NotRequired[bool]

    # ---- 每轮共享的语义与导演通道（严格模型校验后以普通 dict 入检查点）----
    semantic_understanding: NotRequired[dict]
    response_budget: NotRequired[dict]

    outcome: RunOutcome | None


# ---------------------------------------------------------------------------
# 请求入口（内部端点线格式）
# ---------------------------------------------------------------------------


class OrchestrationRequest(BaseModel):
    """一次用户轮的完整输入。凭据按模型 ref 携带，仅用于构建 RuntimeContext。"""

    run_id: str
    session_id: str
    session_type: Literal["single", "group"]
    user_turn: UserTurnSnapshot
    history_snapshot: list[MessageSnapshot]
    agent_snapshots: list[AgentSnapshot]
    memory_snapshots: list[MemorySnapshot]
    credentials: dict[str, ModelCredential]
    default_model_ref: ModelRef | None = None
    debug_enabled: bool = False

    def to_initial_state(self) -> ConversationState:
        """构造图初始状态：快照拍平为普通 dict，凭据被显式排除。"""
        return ConversationState(
            run_id=self.run_id,
            session_id=self.session_id,
            session_type=self.session_type,
            user_turn=cast(UserTurnSnapshot, self.user_turn.model_dump()),
            history_snapshot=[
                cast(dict, m.model_dump()) for m in self.history_snapshot
            ],
            agent_snapshots=[
                cast(dict, a.model_dump()) for a in self.agent_snapshots
            ],
            memory_snapshots=[
                cast(dict, m.model_dump()) for m in self.memory_snapshots
            ],
            directive=None,
            speaking_plan=None,
            pending_agent_ids=[a.agent_id for a in self.agent_snapshots],
            replies=[],
            memory_proposals=[],
            outcome=None,
        )


# ---------------------------------------------------------------------------
# 运行期上下文（不落检查点；随 LangGraph context_schema 注入）
# ---------------------------------------------------------------------------


@dataclass(frozen=True)
class RuntimeContext:
    """图节点运行期依赖：模型网关、按模型 ref 的凭据、调试开关、事件出口与取消信号。

    事件出口把节点产物以 OrchestrationEvent 发到边界（内部 SSE 或测试收集器）；
    事件负载须经 events.redact_event_payload 脱敏后再投递（由发射方保证）。
    default_model_ref 是会话「当前配置的默认模型」——群聊 Supervisor 导演用它
    （非人设、非群成员）；None 表示无默认模型，GroupChatGraph 不得调用 Supervisor，
    直接走确定性回退。
    """

    model_gateway: "ModelGateway"
    credentials_by_model_ref: Mapping[str, ModelCredential]
    default_model_ref: "ModelRef | None" = None
    debug_enabled: bool = False
    event_sink: Callable[["OrchestrationEvent"], None] | None = None
    cancellation: "CancellationToken" = field(default=None)  # type: ignore[assignment]
