"""ConversationGraph：单次用户轮的会话级编排入口（设计文档 §4）。

链路：hydrate_context → dispatch_session（闭包式子运行已编译的
SingleChatGraph / GroupChatGraph，按 state.session_type 路由）。

执行语义：
- hydrate_context 兜底补全空 pending（外部残缺输入/直接喂原始状态的兜底）；
- dispatch_session 与子图共享同一 RuntimeContext：事件出口与取消旗标直通，
  中断粒度 = 子图发言人边界；
- 子图最终状态里与后续执行相关的通道（directive/speaking_plan/pending/
  outcome/replies/memory_proposals）显式并回顶层——顶层检查点因此携带
  「计划 + 已完成回复」，resume 重放时把顶层值作新代次输入，子图在差集上
  裁决：不重发 plan_created、不重调 Supervisor、不重复已完成发言人
  （守卫分布在 group.py：router 计划守卫 / 路由 plan 优先 / dispatch
  回复 diff / convergence 旗标裁决）。
- 本图不内嵌子图节点而是闭包式调用，与 GroupChatGraph.dispatch_daoists
  同一惯用法（单道人失败语义在子图内部按发言人粒度处理）。

调用方（ConversationRuntime）负责对整图 .compile(checkpointer=...)；
顶层检查点只落本图节点边界（hydrate / dispatch_session），运行期凭据
（RuntimeContext）不参与状态。
"""

from __future__ import annotations

from typing import Any

from langgraph.graph import END, StateGraph
from langgraph.runtime import Runtime

from app.orchestration.contracts import ConversationState, RuntimeContext
from app.orchestration.graphs.daoist import _TRANSIENT_CHANNELS, build_daoist_graph
from app.orchestration.graphs.group import build_group_graph
from app.orchestration.graphs.single import build_single_graph


def hydrate_context(state: ConversationState) -> dict[str, Any]:
    """补全空待发言列表：以全体道人作为未派发言人（残缺输入/续跑兜底）。"""
    if state.get("pending_agent_ids"):
        return {}
    return {"pending_agent_ids": [a["agent_id"] for a in state["agent_snapshots"]]}


def build_conversation_graph() -> StateGraph:
    """ConversationGraph：单/群聊一次用户轮，state_schema=ConversationState。

    以共享 DaoistGraph 装配 single/group 子图并在此编译；调用方
    （ConversationRuntime）再对整图 .compile(checkpointer=...)。
    """
    compiled_single = build_single_graph(build_daoist_graph()).compile()
    compiled_group = build_group_graph(build_daoist_graph()).compile()

    async def dispatch_session(
        state: ConversationState, runtime: Runtime[RuntimeContext]
    ) -> dict[str, Any]:
        ctx = runtime.context
        # 剔除瞬时通道：子图运行期产物（draft/记忆选择等）不并回顶层。
        slice_state: ConversationState = {
            k: v for k, v in state.items() if k not in _TRANSIENT_CHANNELS
        }
        compiled = compiled_single if state["session_type"] == "single" else compiled_group
        result = await compiled.ainvoke(slice_state, context=ctx)
        # 并回路由相关通道：顶层检查点携带计划与已完成回复（resume 差集依据），
        # 以及收束裁决（outcome/pending）——值以子图最终态为准。
        return {
            "replies": result.get("replies") or [],
            "memory_proposals": result.get("memory_proposals") or [],
            "pending_agent_ids": result.get("pending_agent_ids") or [],
            "directive": result.get("directive"),
            "speaking_plan": result.get("speaking_plan"),
            "outcome": result.get("outcome"),
        }

    graph = StateGraph(state_schema=ConversationState, context_schema=RuntimeContext)
    graph.add_node("hydrate_context", hydrate_context)
    graph.add_node("dispatch_session", dispatch_session)
    graph.add_edge("hydrate_context", "dispatch_session")
    graph.add_edge("dispatch_session", END)
    graph.set_entry_point("hydrate_context")
    graph.set_finish_point("dispatch_session")
    return graph
