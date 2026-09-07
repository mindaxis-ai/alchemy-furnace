"""可编排图实现：Daoist（单道人发言链路）、single/group（会话子图）、
conversation（顶层会话图，ConversationRuntime 的编译对象）。

本包每个模块导出一个构图入口；调用方（runtime/测试）负责 .compile() 与执行。
"""

from app.orchestration.graphs.conversation import build_conversation_graph
from app.orchestration.graphs.daoist import build_daoist_graph
from app.orchestration.graphs.group import build_group_graph
from app.orchestration.graphs.single import build_single_graph

__all__ = [
    "build_conversation_graph",
    "build_daoist_graph",
    "build_group_graph",
    "build_single_graph",
]
