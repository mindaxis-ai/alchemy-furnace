// Package router 提供 handler 包装器: 将 (code, data, err) 返回值统一写出为响应包络
// SSE 等流式 handler 自行写出,不经 Wrapper
package router

import (
	"github.com/alchemy-furnace/server/internal/errors"
	"github.com/alchemy-furnace/server/server/http/response"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// Handler 新风格 handler 签名: 返回业务码、数据、错误
type Handler func(c *gin.Context) (response.Code, any, error)

// Wrapper 将新风格 handler 包装为 gin.HandlerFunc
// 规则:
//   - err != nil: HTTP 状态码按错误类型映射(400/404/409/500),body code 用 handler 返回的业务码(0 则取 HTTP 状态码)
//   - err == nil 且 code == response.CodeCreated: 201 Created
//   - 其余: 200 Success
func Wrapper(h Handler) gin.HandlerFunc {
	return func(c *gin.Context) {
		code, data, err := h(c)
		if err != nil {
			status := errors.HTTPStatus(err)
			bodyCode := int(code)
			if bodyCode == 0 {
				bodyCode = status
			}
			errorCode := ""
			if internalError, ok := err.(errors.Error); ok {
				errorCode = internalError.GetCode()
			}
			// 5xx 不向前端暴露内部细节,统一文案并记日志;4xx 业务错误消息可直接返回
			message := err.Error()
			if status >= 500 {
				zap.L().Error("[炼丹炉] 内部错误",
					zap.String("request_id", c.GetString("X-Request-ID")),
					zap.Error(err))
				message = "服务器内部错误"
			}
			// 携带附加数据的错误(如 409 引用计数)写入响应 data 字段
			if ed, ok := err.(errors.ErrorWithData); ok {
				response.FailureWithDataAndErrorCode(c, status, bodyCode, errorCode, message, ed.GetData())
			} else {
				response.FailureWithErrorCode(c, status, bodyCode, errorCode, message)
			}
			return
		}

		if code == response.CodeCreated {
			response.Created(c, data)
			return
		}
		response.Success(c, data)
	}
}

// WrapperPage 分页专用包装: handler 返回 (total, page, pageSize, list, err)
func WrapperPage(h func(c *gin.Context) (int64, int, int, any, error)) gin.HandlerFunc {
	return func(c *gin.Context) {
		total, page, pageSize, list, err := h(c)
		if err != nil {
			status := errors.HTTPStatus(err)
			errorCode := ""
			if internalError, ok := err.(errors.Error); ok {
				errorCode = internalError.GetCode()
			}
			if status >= 500 {
				zap.L().Error("[炼丹炉] 内部错误",
					zap.String("request_id", c.GetString("X-Request-ID")),
					zap.Error(err))
				response.FailureWithErrorCode(c, status, status, errorCode, "服务器内部错误")
				return
			}
			response.FailureWithErrorCode(c, status, status, errorCode, err.Error())
			return
		}
		response.SuccessWithPage(c, total, page, pageSize, list)
	}
}
