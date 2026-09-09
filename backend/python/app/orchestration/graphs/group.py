"""GroupChatGraph：群聊混合导演编排（设计文档 §4/§5）。

链路：classify_directive → deterministic_router（条件：dispatch / supervisor / END）
      supervisor ────────────┘        dispatch_daoists → convergence

路由语义：
- 确定性指令（@提及/@全体/报数）在 deterministic_router 内直接成计划，
  **永不调用 Supervisor**；停/继续是回合级控制指令，本图不产出任何计划与发言
  （ConversationGraph 控制面处理），direct 到 END。
- 开放讨论（kind=open）经 supervisor 节点用会话默认模型（RuntimeContext.
  default_model_ref，非人设）产出结构化计划；无效/不可用 → supervisor 模块
  返回 None → 落确定性回退（主成员，列表首位）。
- dispatch_daoists 按计划顺序逐发言人串行子运行 DaoistGraph（每发言人一张
  干净的瞬态切片），单道人失败只跳过该发言人，不阻塞剩余计划成员
  （设计文档 §10）；事件经共享 event_sink 流出，顺序 = 发言顺序。
- 中断/恢复在发言人边界生效：dispatch 每次迭代先让出事件循环再查取消旗标
  （runtime.context.cancellation），被取消时带着「已完成回复 + 剩余待发言者」
  干净返回，convergence 标记 interrupted/cancelled。resume 重放时按
  「计划 + 已完成回复」的差集裁决——不重发 plan_created、不重复已完成
  发言人（ConversationGraph 在顶层检查点存这两者，见 conversation.py）。
- convergence 收束本回合：正常收尾清空待发言者并标记 completed；被取消时
  保留待发言者、以旗标 reason（interrupted/cancelled）为 outcome。

事件：任何成形的计划（deterministic/supervisor/fallback）恰发一条
plan_created；负载先经 redact_event_payload 脱敏。
"""

from __future__ import annotations

import asyncio
from typing import Any, Literal

from langgraph.graph import END, StateGraph
from langgraph.runtime import Runtime

from app.orchestration.contracts import (
    AgentSnapshot,
    ConversationState,
    Directive,
    ResponseBudget,
    RuntimeContext,
    SpeakingPlan,
    SpeakingPlanItem,
    UserTurnSnapshot,
)
from app.orchestration.directives import build_deterministic_plan, classify_directive
from app.orchestration.director import per_speaker_budget
from app.orchestration.events import OrchestrationEvent, redact_event_payload
from app.orchestration.graphs.daoist import _TRANSIENT_CHANNELS
from app.orchestration.supervisor import build_fallback_plan, plan_with_supervisor


def _emit(runtime: Runtime, run_id: str, name: str, payload: dict[str, Any]) -> None:
    """经运行期事件出口投递事件；负载统一脱敏后封装为 OrchestrationEvent。"""
    ctx = runtime.context
    if ctx.event_sink is None:
        return
    ctx.event_sink(
        OrchestrationEvent(run_id=run_id, name=name, payload=redact_event_payload(payload))
    )


def _plan_created(
    runtime: Runtime, state: ConversationState, source: str, plan: SpeakingPlan
) -> dict[str, Any]:
    _emit(
        runtime,
        state["run_id"],
        "plan_created",
        {"source": source, "plan": plan.model_dump()},
    )
    return {"speaking_plan": plan.model_dump()}


def classify_directive_node(state: ConversationState) -> dict[str, Any]:
    """分类用户轮：确定性指令 / 开放讨论，落进 state.directive。"""
    agents = [AgentSnapshot.model_validate(a) for a in state["agent_snapshots"]]
    text = UserTurnSnapshot.model_validate(state["user_turn"]).text
    return {"directive": classify_directive(text, agents).model_dump()}


def deterministic_router(
    state: ConversationState, runtime: Runtime[RuntimeContext]
) -> dict[str, Any] | None:
    """确定性指令直接成计划（永不委托模型）；停/继续/空范围不产计划。

    resume 重放守卫：speaking_plan 已存在时直接返回 None——上一代次已产过
    计划并广播 plan_created，续跑不得重规划/重发（计划经顶层检查点并回）。
    """
    if state.get("speaking_plan"):
        return None
    directive = Directive.model_validate(state["directive"])
    if directive.kind in ("stop", "continue", "open"):
        return None
    agents = [AgentSnapshot.model_validate(a) for a in state["agent_snapshots"]]
    plan = build_deterministic_plan(directive, agents)
    if plan is None:
        return None
    return _plan_created(runtime, state, "deterministic", plan)


def _route_after_router(
    state: ConversationState,
) -> Literal["dispatch", "supervisor", "end"]:
    """条件边：节点只落计划，路由读单一决策（计划有无优先，其次 directive.kind）。

    顺序有讲究：resume 重放时 speaking_plan 已存在（路由守卫返回 None 不会重
    规划），必须直接走 dispatch 按差集续跑——若先看 kind，kind=open 的重放
    会误入 supervisor 重调默认模型。
    """
    if state.get("speaking_plan"):
        return "dispatch"
    kind = state["directive"]["kind"]
    if kind == "open":
        return "supervisor"
    return "end"


async def supervisor(
    state: ConversationState, runtime: Runtime[RuntimeContext]
) -> dict[str, Any]:
    """开放讨论：默认模型导演出计划；失败落确定性回退，导演错误不传染整轮。"""
    agents = [AgentSnapshot.model_validate(a) for a in state["agent_snapshots"]]
    text = UserTurnSnapshot.model_validate(state["user_turn"]).text
    plan = await plan_with_supervisor(runtime.context, agents, text)
    if plan is None:
        plan = build_fallback_plan(agents)
        source = "fallback"
    else:
        raw_budget = state.get("response_budget")
        if raw_budget is not None:
            try:
                max_speakers = ResponseBudget.model_validate(raw_budget).max_speakers
                plan = plan.model_copy(update={"items": plan.items[:max_speakers]})
            except Exception:
                pass
        source = "supervisor"
    return _plan_created(runtime, state, source, plan)


def convergence(
    state: ConversationState, runtime: Runtime[RuntimeContext]
) -> dict[str, Any]:
    """收束本回合：正常收尾清空待发言者并标记完成；被取消时保留差集依据。

    中断/被取代（旗标已置且 dispatch 留了 pending）→ 保留 pending 与已完成
    回复、以旗标 reason 为 outcome——顶层检查点由此携带「计划 + 已完成回复」，
    resume 重放时在差集上裁决（本层不改计划通道，回复 diff 在 dispatch）。
    """
    token = runtime.context.cancellation
    if token is not None and token.is_cancelled and state.get("pending_agent_ids"):
        return {"outcome": token.reason}
    return {"pending_agent_ids": [], "outcome": "completed"}


def build_group_graph(daoist_graph: StateGraph) -> StateGraph:
    """GroupChatGraph：一次用户轮的群聊编排，state_schema=ConversationState。

    复用 DaoistGraph 作子图：dispatch_daoists 串行子运行已编译的 DaoistGraph，
    每个发言人一张干净切片（pending=[该发言人]，瞬态通道清空，replies 从空
    累积——否则 reducer 会把已完成回复并入子运行结果造成重复终稿）。

    调用方负责 .compile()；本图不内嵌 DaoistGraph 节点，而是显式子调用，
    保证「单道人失败不阻塞剩余成员」的语义在节点内可捕获。
    """
    compiled_daoist = daoist_graph.compile()

    async def dispatch_daoists(
        state: ConversationState, runtime: Runtime[RuntimeContext]
    ) -> dict[str, Any]:
        plan = state.get("speaking_plan")
        items = [SpeakingPlanItem.model_validate(i) for i in plan["items"]] if plan else []
        token = runtime.context.cancellation
        # 已完成回复 = 差集裁决依据（resume 重放时 reducer 在顶层空态并入上一
        # 代次回复，本节点据此跳过已完成发言人，不重复发言）。
        done = {r["agent_id"] for r in state.get("replies", [])}
        new_replies: list[dict[str, Any]] = []
        remaining: list[str] = []
        shared_budget = ResponseBudget.model_validate(state["response_budget"])
        speaker_budget = per_speaker_budget(shared_budget, len(items))
        for index, item in enumerate(items):
            # 先让出事件循环再查旗标：取消请求（用户停止/新轮取代）只能在
            # await 点落进 token——让出后检查，取消恰好落在发言人边界。
            await asyncio.sleep(0)
            if token is not None and token.is_cancelled:
                remaining = [i.agent_id for i in items[index:] if i.agent_id not in done]
                break
            if item.agent_id in done:
                continue
            fresh: ConversationState = {
                k: v for k, v in state.items() if k not in _TRANSIENT_CHANNELS
            }
            fresh["pending_agent_ids"] = [item.agent_id]
            fresh["replies"] = []
            fresh["response_budget"] = speaker_budget.model_dump()
            try:
                result = await compiled_daoist.ainvoke(fresh, context=runtime.context)
            except Exception:
                # 单道人失败（模型超时/供应商错误）只跳过该发言人（设计文档 §10）；
                # DaoistGraph 内部已做 ≤2 次机械重试，此处不再重试。
                continue
            new_replies.extend(result.get("replies") or [])
        return {"replies": new_replies, "pending_agent_ids": remaining}

    graph = StateGraph(state_schema=ConversationState, context_schema=RuntimeContext)
    graph.add_node("classify_directive", classify_directive_node)
    graph.add_node("deterministic_router", deterministic_router)
    graph.add_node("supervisor", supervisor)
    graph.add_node("dispatch_daoists", dispatch_daoists)
    graph.add_node("convergence", convergence)

    graph.add_edge("classify_directive", "deterministic_router")
    graph.add_conditional_edges(
        "deterministic_router",
        _route_after_router,
        {"dispatch": "dispatch_daoists", "supervisor": "supervisor", "end": END},
    )
    graph.add_edge("supervisor", "dispatch_daoists")
    graph.add_edge("dispatch_daoists", "convergence")
    graph.add_edge("convergence", END)

    graph.set_entry_point("classify_directive")
    graph.set_finish_point("convergence")
    return graph
