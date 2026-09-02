package chat_service

import "context"

type promptDebugContextKey struct{}

// WithPromptDebug 将请求级 Prompt 调试开关传入群聊编排器，不改变持久化状态。
func WithPromptDebug(ctx context.Context, enabled bool) context.Context {
	return context.WithValue(ctx, promptDebugContextKey{}, enabled)
}

func promptDebugEnabled(ctx context.Context) bool {
	enabled, _ := ctx.Value(promptDebugContextKey{}).(bool)
	return enabled
}
