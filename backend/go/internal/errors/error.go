// Package errors 内部错误体系(对齐 Luna-CY 模板,简化掉 i18n,中文消息硬编码)
// 错误分两类语义: 系统内部错误(含定位码,仅内部流转)与业务边界错误(可直接返回前端)
// 定位码格式: {路径码}.{方法码}.{行号}{分}{秒} —— 全局可检索,并行开发无中心化冲突
package errors

import (
	"fmt"
)

// ErrorType 错误类型,Wrapper 据此映射 HTTP 状态码
type ErrorType int

const (
	// ErrorTypeRecordNotFound 记录不存在 → 404
	ErrorTypeRecordNotFound ErrorType = iota
	// ErrorTypeInvalidRequest 请求参数错误 → 400
	ErrorTypeInvalidRequest
	// ErrorTypeConflict 资源冲突(如被引用删除) → 409
	ErrorTypeConflict
	// ErrorTypeServiceUnavailable 受控的外部服务不可用(远端 503) → 503
	// 与内部错误不同: Wrapper 对这类错误保留公开 message,不替换为"服务器内部错误"
	ErrorTypeServiceUnavailable
	// ErrorTypeServerInternalError 服务器内部错误 → 500
	ErrorTypeServerInternalError
)

// Error 内部错误接口
type Error interface {
	error

	// GetCode 获取错误定位码
	GetCode() string

	// IsType 检查错误类型
	IsType(ErrorType) bool

	// Relation 将新的错误关联到当前错误(返回当前错误,支持链式)
	Relation(...Error) Error

	// Relations 获取所有关联的错误列表
	Relations() []Error
}

// ErrorWithData 携带附加响应数据的错误(如 409 冲突返回引用计数)
// Wrapper 据此把数据写入失败响应的 data 字段
type ErrorWithData interface {
	Error

	// GetData 获取附加响应数据
	GetData() any
}

// ie 内部错误实现
type ie struct {
	t         ErrorType // 错误类型
	code      string    // 定位码,硬编码在发生错误的地方
	message   any       // 消息内容,string 时为 fmt 模板
	values    []any     // 模板变量
	data      any       // 附加响应数据(可选,如 409 引用计数)
	relations []Error   // 关联错误
}

func (e *ie) Error() string {
	if len(e.values) > 0 {
		if tmpl, ok := e.message.(string); ok {
			return fmt.Sprintf(tmpl, e.values...)
		}
	}
	return fmt.Sprintf("%v", e.message)
}

func (e *ie) GetCode() string       { return e.code }
func (e *ie) IsType(t ErrorType) bool { return e.t == t }
func (e *ie) Relations() []Error    { return e.relations }
func (e *ie) GetData() any          { return e.data }

func (e *ie) Relation(errs ...Error) Error {
	e.relations = append(e.relations, errs...)
	return e
}

// New 创建内部错误;message 为模板字符串时 values 为模板变量
func New(t ErrorType, code string, message any, values ...any) Error {
	return &ie{t: t, code: code, message: message, values: values}
}

// NewWithData 创建携带附加响应数据的内部错误(如 409 冲突返回引用计数)
func NewWithData(t ErrorType, code string, data any, message any, values ...any) Error {
	return &ie{t: t, code: code, message: message, values: values, data: data}
}

// IsType 判断 err 是否(或关联错误中)包含指定类型
func IsType(err error, t ErrorType) bool {
	for err != nil {
		e, ok := err.(Error)
		if !ok {
			u, ok2 := err.(interface{ Unwrap() error })
			if !ok2 {
				return false
			}
			err = u.Unwrap()
			continue
		}
		if e.IsType(t) {
			return true
		}
		for _, r := range e.Relations() {
			if IsType(r, t) {
				return true
			}
		}
		return false
	}
	return false
}

// HTTPStatus 将错误映射为 HTTP 状态码(供 router.Wrapper 使用)
func HTTPStatus(err error) int {
	switch {
	case IsType(err, ErrorTypeRecordNotFound):
		return 404
	case IsType(err, ErrorTypeInvalidRequest):
		return 400
	case IsType(err, ErrorTypeConflict):
		return 409
	case IsType(err, ErrorTypeServiceUnavailable):
		return 503
	default:
		return 500
	}
}
