"""对话编排（LangGraph）包：契约类型、图状态、内部事件。

Go 仍是消息/会话/记忆/凭据的权威存储；本包定义穿越 Go↔Python 边界与
图执行内部的类型。API key 与解密凭据只允许存在于 RuntimeContext，
绝不落入 ConversationState / 检查点 / 事件。
"""
