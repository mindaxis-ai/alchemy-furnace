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
    Literal,
    Mapping,
    TypedDict,
    cast,
    operator,
)

from pydantic import BaseModel

if TYPE_CHECKING:  # 仅类型检查；实际类由 Task 3+ 图模块提供
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

    agent_id: str
    name: str
    model_ref: ModelRef


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
    """确定性指令分类结果。kind 为 open 时由 Supervisor 规划发言计划。"""

    kind: DirectiveKind
    mentioned_agent_ids: list[str] = []


class SpeakingPlan(BaseModel):
    """Supervisor 或确定性路由产出的发言计划：发言人顺序 + 收敛理由。"""

    agent_ids: list[str]
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
    """图节点运行期依赖：模型网关、按模型 ref 的凭据、调试开关与取消信号。"""

    model_gateway: "ModelGateway"
    credentials_by_model_ref: Mapping[str, ModelCredential]
    debug_enabled: bool = False
    cancellation: "CancellationToken" = field(default=None)  # type: ignore[assignment]
