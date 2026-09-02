"""可编排图实现：Daoist（单道人发言链路）与后续 single/group/conversation 图。

本包每个模块导出一个构图入口；调用方（runtime/测试）负责 .compile() 与执行。
"""

from app.orchestration.graphs.daoist import build_daoist_graph

__all__ = ["build_daoist_graph"]
