"""SingleChatGraph：单聊回合编排——直达 DaoistGraph 的单发言人链路（设计文档 §4）。

链路：speak（入口=出口，一个原子发言人节点）。

单聊无指令分类（@/报数/停继续都是群聊与 UI 语义）：把单个道人的整份状态切片
（剔除 DaoistGraph 瞬时通道）交给已编译的 DaoistGraph 子运行，终稿并入
replies 并标记 completed。事件经共享 event_sink 直通（speaker_started →
assistant_delta → assistant_final，与群聊同序）。

中断语义：发言人是原子边界——token 在调用 DaoistGraph 前检查，取消落在
speaker 边界（outcome=interrupted、pending 保留，可续跑重放）；调用一旦
开始就跑完整个发言（DaoistGraph 内部 ≤2 次机械重试有界，不追加外层重试）。

重放语义：resume 恢复时本轮状态经 reducer 已并入上一代次的终稿；speak
在调用前检查已有回复直接收尾（与 GroupChatGraph.dispatch_daoists 的
差集裁决同一套路——单聊正常只会中断在「未发言」或「已终稿」两种形态，
见 runtime.resume 的输入组装）。
"""

from __future__ import annotations

from typing import Any

from langgraph.graph import END, StateGraph
from langgraph.runtime import Runtime

from app.orchestration.contracts import ConversationState, RuntimeContext
from app.orchestration.graphs.daoist import _TRANSIENT_CHANNELS


def build_single_graph(daoist_graph: StateGraph) -> StateGraph:
    """SingleChatGraph：一次单聊回合，state_schema=ConversationState。

    调用方负责 .compile()；DaoistGraph 在此编译（无检查点），speak 节点串行
    子运行——与 GroupChatGraph.dispatch_daoists 同一惯用法（闭包持有已编译
    子图，事件与取消旗标经共享 RuntimeContext 直通）。
    """
    compiled_daoist = daoist_graph.compile()

    async def speak(
        state: ConversationState, runtime: Runtime[RuntimeContext]
    ) -> dict[str, Any]:
        token = runtime.context.cancellation
        if token is not None and token.is_cancelled:
            # 用户停止落在发言人边界：保留 pending（顶层并回），可续跑重放。
            return {"outcome": token.reason}
        pending = state.get("pending_agent_ids") or [
            state["agent_snapshots"][0]["agent_id"]
        ]
        fresh: ConversationState = {
            k: v for k, v in state.items() if k not in _TRANSIENT_CHANNELS
        }
        fresh["pending_agent_ids"] = [pending[0]]
        fresh["replies"] = []
        result = await compiled_daoist.ainvoke(fresh, context=runtime.context)
        return {"replies": result.get("replies") or [], "outcome": "completed"}

    graph = StateGraph(state_schema=ConversationState, context_schema=RuntimeContext)
    graph.add_node("speak", speak)
    graph.set_entry_point("speak")
    graph.set_finish_point("speak")
    return graph
