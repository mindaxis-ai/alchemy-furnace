# 012 实施与续跑计划

**Goal:** 完成白蓝 UI、输入框模型切换、金丹/人设修复、清理与桌面验证。
**Architecture:** 沿用 React ChatContext → Go ConversationCommand → Python 编排链路，模型覆盖限定当前会话发送请求；保留已有 API 默认行为。全局设计令牌统一配色，聊天组件局部重排。
**Tech Stack:** Next.js 16 / React 19 / Go / Python / Wails。
**Spec:** `specs/012-codex-chat-refresh/spec.md`

## 执行顺序

- [x] P1：修改 `frontend/app/globals.css` 令牌与暖色阴影；重排 `chat-view.tsx` 的消息和输入布局、`chat-message.tsx` 的用户/助手外观。
- [x] P1：在消息服务和 Go SSE/编排边界加入可选模型覆盖，先写回归测试验证选择影响实际请求且默认行为不变；在输入框右下角接入启用模型列表，流式过程中禁止切换，切换会话隔离选择。
- [x] P2：追踪实际提示词构建、启用金丹与人设来源，记录根因，用真实构建器回归测试复现后修复。
- [x] P3：在通过测试后清理受影响链路中的重复/废弃代码。
- [x] P4：运行前端测试/lint/typecheck/build、Go/Python 测试，构建并验证 Wails，记录证据和未验收项。

## 续跑记录

2026-09-06：已创建五分钟自动跟进，截止北京时间次日 10:00，禁止使用重置卡。初始五小时额度剩余 96%，刷新 2026-09-07 04:01:27。已从 master 3fa5442 创建隔离工作树。

2026-09-07：完成白蓝令牌与聊天交互重排；模型选择贯通 React、Go SSE 与编排快照；确认根因是新 LangGraph 快照绕过语言模式服务，并将人设与启用金丹合成提示词接回实际模型输入。删除 Chat 接口中不再由传输层使用的语言模式与凭据透传方法。

验证：Go 全量测试、Python 206 项（另 2 项跳过）、前端 496 项、lint、typecheck、生产构建均通过。Wails v2.14.0 已构建 macOS arm64 应用并启动实际桌面窗口；CoreGraphics 确认窗口存在，桌面 HTTP guard 与 Python 引擎健康检查正常。当前机器未覆盖 macOS Intel 与 Windows x64 原生运行。
