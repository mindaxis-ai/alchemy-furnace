// 统一响应写出(全部经 request_id 中间件注入的 X-Request-ID)
package response

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// Response 统一响应结构体
type Response struct {
	Code      int         `json:"code"`                 // 业务状态码: 0=成功
	ErrorCode string      `json:"error_code,omitempty"` // 稳定业务错误定位码(仅错误响应)
	Message   string      `json:"message"`              // 提示信息(命名保留,前端依赖)
	RequestID string      `json:"request_id"`           // 请求链路 ID
	Data      interface{} `json:"data"`                 // 响应数据
}

// PageInfo 分页信息结构体
type PageInfo struct {
	Total    int64       `json:"total"`
	Page     int         `json:"page"`
	PageSize int         `json:"page_size"`
	List     interface{} `json:"list"`
}

// requestID 从 gin.Context 取请求 ID(由 request_id 中间件注入)
func requestID(c *gin.Context) string {
	return c.GetString("X-Request-ID")
}

// Success 返回成功响应(HTTP 200)
func Success(c *gin.Context, data interface{}) {
	c.JSON(http.StatusOK, Response{Code: 0, Message: "ok", RequestID: requestID(c), Data: data})
}

// SuccessWithPage 返回分页成功响应(HTTP 200)
func SuccessWithPage(c *gin.Context, total int64, page, pageSize int, list interface{}) {
	Success(c, PageInfo{Total: total, Page: page, PageSize: pageSize, List: list})
}

// Created 返回创建成功响应(HTTP 201)
func Created(c *gin.Context, data interface{}) {
	c.JSON(http.StatusCreated, Response{Code: 0, Message: "创建成功", RequestID: requestID(c), Data: data})
}

// Failure 返回错误响应(带 HTTP 状态码)
func Failure(c *gin.Context, httpStatus int, code Code, message string) {
	failureWithData(c, httpStatus, code, "", message, nil)
}

// FailureWithData 返回错误响应(带 HTTP 状态码与附加数据,如 409 引用计数)
func FailureWithData(c *gin.Context, httpStatus int, code Code, message string, data interface{}) {
	failureWithData(c, httpStatus, code, "", message, data)
}

// FailureWithErrorCode 返回错误响应(带稳定业务错误码)
func FailureWithErrorCode(c *gin.Context, httpStatus int, code Code, errorCode, message string) {
	failureWithData(c, httpStatus, code, errorCode, message, nil)
}

// FailureWithDataAndErrorCode 返回错误响应(带稳定业务错误码与附加数据)
func FailureWithDataAndErrorCode(c *gin.Context, httpStatus int, code Code, errorCode, message string, data interface{}) {
	failureWithData(c, httpStatus, code, errorCode, message, data)
}

func failureWithData(c *gin.Context, httpStatus int, code Code, errorCode, message string, data interface{}) {
	c.JSON(httpStatus, Response{Code: code, ErrorCode: errorCode, Message: message, RequestID: requestID(c), Data: data})
}

// BadRequest 400 / NotFoundResp 404 / ConflictResp 409 / InternalError 500 便捷函数
func BadRequest(c *gin.Context, message string) {
	Failure(c, http.StatusBadRequest, InvalidParams, message)
}
func NotFoundResp(c *gin.Context, message string) { Failure(c, http.StatusNotFound, NotFound, message) }
func ConflictResp(c *gin.Context, message string) { Failure(c, http.StatusConflict, Conflict, message) }
func InternalError(c *gin.Context, message string) {
	Failure(c, http.StatusInternalServerError, ServerInternalError, message)
}
