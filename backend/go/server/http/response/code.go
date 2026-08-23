// Package response 统一 HTTP 响应: {code, message, request_id, data}
// code 为业务状态码(0=成功);message 保留原命名兼容前端;request_id 为请求链路 ID
package response

// Code 业务状态码
type Code = int

// 常用业务码(沿用旧 pkg/response 的语义,前端零改动)
const (
	// Ok 成功
	Ok Code = 0
	// CodeCreated 创建成功(HTTP 201 语义标记,仅 Wrapper 识别)
	CodeCreated Code = 201
	// InvalidParams 请求参数错误
	InvalidParams Code = 400
	// NotFound 资源不存在
	NotFound Code = 404
	// Conflict 资源冲突
	Conflict Code = 409
	// ServerInternalError 服务器内部错误
	ServerInternalError Code = 500

	// CodeBindPillFailed 服用金丹失败(业务码,沿用)
	CodeBindPillFailed Code = 4001
	// CodeUnbindPillFailed 解除绑定失败(业务码,沿用)
	CodeUnbindPillFailed Code = 4002
	// CodeUpdateAgentPillFailed 更新服用记录失败(业务码,沿用)
	CodeUpdateAgentPillFailed Code = 4003
	// CodeReplacePillsFailed 完整服丹编排失败(业务码)
	CodeReplacePillsFailed Code = 4004
)
