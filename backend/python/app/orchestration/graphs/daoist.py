"""可复用 DaoistGraph：单道人的完整发言链路（设计文档 §4 子图）。

链路：select_memories → compile_prompt → invoke_model → validate_draft
      invoke_model ←──────────────────────────────┘（机械失败重试 ≤2 次）
      validate_draft → humanize_reply → validate_final_reply → propose_memory
                       humanize_reply ←──────────────┘（无效改写重试 ≤1 次）

子图边界（设计文档 §4/§10）：
- 只执行单次发言任务，不决定计划/轮次/收敛；单聊一次运行，群聊每道人一次运行。
- 机械任务约束逐字进入系统提示（prompts.compile_messages 原样保留任务文本）。
- 凭据只经 RuntimeContext 进 ModelGateway 构造模型；节点状态、检查点与事件
  永不携带凭据。事件经 _emit 统一走 redact_event_payload 后才投递。
- 发言事件顺序：speaker_started（compile 阶段，恰一次）→ assistant_delta（每次
  模型调用）→ assistant_final（恰一次，validate 通过或重试耗尽后）。
  当前阶段重试耗尽保留最后一稿（道人不出声会拖垮 UI 回合），后续 Task 的
  群聊收敛层再裁决单道人的机械失败。
"""

from __future__ import annotations

import re
from typing import Any, Literal
from uuid import uuid4

from langchain_core.language_models.chat_models import BaseChatModel
from langchain_core.messages import AIMessage, BaseMessage, HumanMessage, SystemMessage
from langgraph.graph import END, StateGraph
from langgraph.runtime import Runtime

from app.orchestration.contracts import (
    AgentReply,
    AgentSnapshot,
    ConversationState,
    MemorySnapshot,
    MessageSnapshot,
    ModelCredential,
    ResponseBudget,
    RuntimeContext,
    SemanticUnderstanding,
    UserTurnSnapshot,
)
from app.orchestration.events import OrchestrationEvent, redact_event_payload
from app.orchestration.humanizer import (
    build_humanizer_messages,
    constrain_to_budget,
    validate_humanized,
)
from app.orchestration.prompts import MAX_MEMORIES, MAX_MEMORY_CHARS, compile_messages

#: 报数回复校验：回复必须以整数开头（任务模板要求「开头并原样保留」）。
_ORDINAL_PREFIX_RE = re.compile(r"^\s*(\d+)")

#: 一次发言最多重试 2 次（合计最多 3 次模型调用），只针对当前道人的机械失败。
MAX_VALIDATION_RETRIES = 2

#: 校验失败后的纠错指令：逐字重申强制编号，作为追加 system 消息进入下一次调用。
_RETRY_INSTRUCTION = (
    "上一条回复不符合本回合任务要求。请重新回复：必须原样保留并只输出"
    "你的指定编号：{ordinal}，不得输出任何其他内容。"
)

#: prompt_messages 通道的 JSON 形态键值 -> LangChain 消息类。
_MESSAGE_TYPES = {
    "system": SystemMessage,
    "user": HumanMessage,
    "assistant": AIMessage,
}

#: LangChain 消息类型名 -> 通道形态 role。
_ROLE_NAMES = {"system": "system", "human": "user", "ai": "assistant"}

#: DaoistGraph 运行期瞬时通道：发言人切片之间不得残留上一道人的中间产物。
#: GroupChatGraph.dispatch_daoists / SingleChatGraph.speak / ConversationGraph
#: 都剔除这些通道后另起干净状态（reducer 会把已完成回复并入子运行结果，
#: 造成重复终稿——这是发言人级隔离的公共词汇，归属 DaoistGraph 边界）。
_TRANSIENT_CHANNELS = (
    "selected_memories",
    "prompt_messages",
    "validation_retries",
    "draft_reply",
    "humanized_reply",
    "humanizer_retries",
    "retry_pending",
)


def _emit(runtime: Runtime, run_id: str, name: str, payload: dict[str, Any]) -> None:
    """经运行期事件出口投递事件；负载统一脱敏后封装为 OrchestrationEvent。"""
    ctx = runtime.context
    if ctx.event_sink is None:
        return
    ctx.event_sink(
        OrchestrationEvent(run_id=run_id, name=name, payload=redact_event_payload(payload))
    )


def _current_agent(state: ConversationState) -> AgentSnapshot:
    """当前发言道人：pending_agent_ids 首元素在快照中定位。"""
    agents = [AgentSnapshot.model_validate(a) for a in state["agent_snapshots"]]
    target = state["pending_agent_ids"][0]
    return next(a for a in agents if a.agent_id == target)


def _plan_item(state: ConversationState) -> dict[str, Any] | None:
    """当前发言人在发言计划中的条目（任务/编号来源）。"""
    plan = state.get("speaking_plan")
    if not plan:
        return None
    current = state["pending_agent_ids"][0]
    for item in plan["items"]:
        if item["agent_id"] == current:
            return item
    return None


def _roll_call_ordinal(state: ConversationState) -> int | None:
    """当前发言人的报数编号；非报数任务返回 None（不做机械校验）。"""
    item = _plan_item(state)
    return item.get("ordinal") if item else None


def _violates_ordinal(state: ConversationState) -> bool:
    """草稿回复是否违反报数编号约束（无编号任务永远不违反）。"""
    ordinal = _roll_call_ordinal(state)
    if ordinal is None:
        return False
    text = str(state.get("draft_reply", {}).get("text", ""))
    match = _ORDINAL_PREFIX_RE.match(text)
    return bool(not match or int(match.group(1)) != ordinal)


def _to_messages(prompt_messages: list[dict]) -> list[BaseMessage]:
    return [_MESSAGE_TYPES[m["role"]](content=m["content"]) for m in prompt_messages]


# ---------------------------------------------------------------------------
# 节点
# ---------------------------------------------------------------------------


def select_memories(state: ConversationState) -> dict[str, Any]:
    """筛选当前道人的记忆进瞬时通道；其他道人的记忆一律不注入。"""
    current = state["pending_agent_ids"][0]
    own = [m for m in state["memory_snapshots"] if m.get("agent_id") == current]
    bounded = own[-MAX_MEMORIES:]
    selected = [
        {
            "memory_id": m["memory_id"],
            "agent_id": m["agent_id"],
            "text": (
                m["text"][: MAX_MEMORY_CHARS - 1] + "…"
                if len(m["text"]) > MAX_MEMORY_CHARS
                else m["text"]
            ),
        }
        for m in bounded
    ]
    return {"selected_memories": selected}


def compile_prompt(
    state: ConversationState, runtime: Runtime[RuntimeContext]
) -> dict[str, Any]:
    """编译最终消息列表（JSON 形态进状态），并宣告发言人开始。"""
    agent = _current_agent(state)
    item = _plan_item(state)
    task: str | None = item.get("task") if item else None
    memories = [
        MemorySnapshot.model_validate(m) for m in state.get("selected_memories", [])
    ]
    messages = compile_messages(
        agent=agent,
        user_turn=UserTurnSnapshot.model_validate(state["user_turn"]),
        history_snapshot=[
            MessageSnapshot.model_validate(m) for m in state["history_snapshot"]
        ],
        selected_memories=memories,
        understanding=SemanticUnderstanding.model_validate(
            state["semantic_understanding"]
        ),
        budget=ResponseBudget.model_validate(state["response_budget"]),
        task=task,
    )
    # 顺序约定：speaker_started 恰一次且先于任何模型产物（重试不重发）。
    _emit(
        runtime,
        state["run_id"],
        "speaker_started",
        {"agent_id": agent.agent_id, "task": task},
    )
    prompt_rows = [{"role": _ROLE_NAMES[m.type], "content": m.content} for m in messages]
    if runtime.context.debug_enabled:
        _emit(
            runtime,
            state["run_id"],
            "prompt_debug",
            {
                "agent_id": agent.agent_id,
                "task": task,
                "model_ref": agent.model_ref.model_dump(),
                "messages": prompt_rows,
            },
        )
    return {"prompt_messages": prompt_rows}


async def invoke_model(
    state: ConversationState, runtime: Runtime[RuntimeContext]
) -> dict[str, Any]:
    """经网关调当前道人模型，产出流式增量与待校验草稿。

    凭据只在运行期上下文里按当前道人 agent_id 解析；模型实例不进入状态。
    """
    ctx = runtime.context
    agent = _current_agent(state)
    model: BaseChatModel = ctx.model_gateway.create(
        agent.model_ref,
        ctx.credentials_by_model_ref.get(agent.agent_id) or ModelCredential(),
    )
    messages = _to_messages(state["prompt_messages"])
    budget = ResponseBudget.model_validate(state["response_budget"])
    reply = await model.ainvoke(messages, max_tokens=budget.max_tokens)
    text = reply.content if isinstance(reply.content, str) else str(reply.content)
    return {
        "draft_reply": {
            "agent_id": agent.agent_id,
            "reply_id": uuid4().hex,
            "text": text,
        }
    }


def validate_draft(
    state: ConversationState, runtime: Runtime[RuntimeContext]
) -> dict[str, Any] | None:
    """校验草稿并裁决机械重试；通过后只交给 Humanizer，不对外发出。

    retry_pending 是条件边的唯一依据——节点与路由不各自推算预算，
    避免「重试耗尽但无人收尾」的分叉（一次发言最多 MAX_VALIDATION_RETRIES 次重试）。
    """
    retries = int(state.get("validation_retries", 0))

    if _violates_ordinal(state) and retries < MAX_VALIDATION_RETRIES:
        ordinal = _roll_call_ordinal(state)
        return {
            "validation_retries": retries + 1,
            "retry_pending": True,
            "prompt_messages": [
                *state["prompt_messages"],
                {"role": "system", "content": _RETRY_INSTRUCTION.format(ordinal=ordinal)},
            ],
        }

    return {"retry_pending": False}


def _route_after_validate(state: ConversationState) -> Literal["retry", "proceed"]:
    """条件边只读取节点已经作出的重试裁决。"""
    return "retry" if state.get("retry_pending") else "proceed"


async def humanize_reply(
    state: ConversationState, runtime: Runtime[RuntimeContext]
) -> dict[str, Any]:
    """用当前人物自己的模型做表达编辑；异常标记后由最终节点回退原稿。"""
    ctx = runtime.context
    agent = _current_agent(state)
    budget = ResponseBudget.model_validate(state["response_budget"])
    understanding = SemanticUnderstanding.model_validate(
        state["semantic_understanding"]
    )
    draft_text = str(state.get("draft_reply", {}).get("text", ""))
    messages = build_humanizer_messages(
        agent,
        understanding,
        budget,
        UserTurnSnapshot.model_validate(state["user_turn"]).text,
        draft_text,
    )
    try:
        model: BaseChatModel = ctx.model_gateway.create(
            agent.model_ref,
            ctx.credentials_by_model_ref.get(agent.agent_id) or ModelCredential(),
        )
        reply = await model.ainvoke(messages, max_tokens=budget.max_tokens)
        text = reply.content if isinstance(reply.content, str) else str(reply.content)
        return {"humanized_reply": {"text": text, "failed": False}}
    except Exception:
        return {"humanized_reply": {"text": "", "failed": True}}


def validate_final_reply(
    state: ConversationState, runtime: Runtime[RuntimeContext]
) -> dict[str, Any]:
    """校验 Humanizer 结果；最多重试一次，随后按预算回退人物原稿。"""
    agent = _current_agent(state)
    draft = state.get("draft_reply", {})
    draft_text = str(draft.get("text", ""))
    humanized = state.get("humanized_reply", {})
    candidate = str(humanized.get("text", ""))
    failed = bool(humanized.get("failed"))
    budget = ResponseBudget.model_validate(state["response_budget"])
    validation = validate_humanized(draft_text, candidate, budget)
    retries = int(state.get("humanizer_retries", 0))
    if not failed and not validation.valid and retries < 1:
        return {"humanizer_retries": retries + 1, "retry_pending": True}

    text = candidate if validation.valid else constrain_to_budget(draft_text, budget)
    if not text.strip():
        raise ValueError("empty final reply")
    reply = AgentReply(
        reply_id=draft["reply_id"],
        run_id=state["run_id"],
        agent_id=agent.agent_id,
        text=text,
    )
    _emit(
        runtime,
        state["run_id"],
        "assistant_delta",
        {"agent_id": agent.agent_id, "text": reply.text},
    )
    _emit(
        runtime,
        state["run_id"],
        "assistant_final",
        {"agent_id": agent.agent_id, "reply_id": reply.reply_id, "text": reply.text},
    )
    return {"retry_pending": False, "replies": [reply.model_dump()]}


def propose_memory(state: ConversationState) -> None:
    """记忆提案接入点：本阶段不产提案（蒸馏语义由后续 Task 实现，Go 侧契约见
    memory_proposal）。保留节点以保证 DaoistGraph 链路形状稳定。"""
    return None


# ---------------------------------------------------------------------------
# 构图
# ---------------------------------------------------------------------------


def build_daoist_graph() -> StateGraph:
    """DaoistGraph：单道人发言链路，state_schema=ConversationState。

    调用方负责 .compile()：可用也可不用检查点（瞬时通道全部可 JSON 序列化）。
    """
    graph = StateGraph(state_schema=ConversationState, context_schema=RuntimeContext)
    graph.add_node("select_memories", select_memories)
    graph.add_node("compile_prompt", compile_prompt)
    graph.add_node("invoke_model", invoke_model)
    graph.add_node("validate_draft", validate_draft)
    graph.add_node("humanize_reply", humanize_reply)
    graph.add_node("validate_final_reply", validate_final_reply)
    graph.add_node("propose_memory", propose_memory)

    graph.add_edge("select_memories", "compile_prompt")
    graph.add_edge("compile_prompt", "invoke_model")
    graph.add_edge("invoke_model", "validate_draft")
    graph.add_conditional_edges(
        "validate_draft",
        _route_after_validate,
        {"retry": "invoke_model", "proceed": "humanize_reply"},
    )
    graph.add_edge("humanize_reply", "validate_final_reply")
    graph.add_conditional_edges(
        "validate_final_reply",
        _route_after_validate,
        {"retry": "humanize_reply", "proceed": "propose_memory"},
    )
    graph.add_edge("propose_memory", END)

    graph.set_entry_point("select_memories")
    graph.set_finish_point("propose_memory")
    return graph
