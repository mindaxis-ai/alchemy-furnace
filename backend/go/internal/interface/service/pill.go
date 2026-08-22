// Package service 业务逻辑接口定义(对齐 Luna-CY 模板 internal/interface/service)
// 对外标识一律为 UUID;错误一律为 errors.Error 类型化错误
package service

import (
	"context"

	"github.com/alchemy-furnace/server/internal/errors"
	"github.com/alchemy-furnace/server/model"
	"github.com/google/uuid"
)

// Pill 金丹业务逻辑接口
type Pill interface {
	// ListPills 分页查询金丹列表
	ListPills(ctx context.Context, page int, size int, keyword string, isBuiltin *bool) (int64, []*model.ElixirPill, errors.Error)

	// GetPillByUUID 按 UUID 获取金丹详情,不存在返回 ErrorTypeRecordNotFound
	GetPillByUUID(ctx context.Context, uid uuid.UUID) (*model.ElixirPill, errors.Error)

	// CreatePill 创建金丹;skill_schema 校验失败返回 ErrorTypeInvalidRequest
	CreatePill(ctx context.Context, name string, description string, skillSchema model.JSONMap, tags model.JSONList, author string, version string) (*model.ElixirPill, errors.Error)

	// UpdatePill 按 UUID 更新金丹(nil/空字段不更新),并失效相关道人语言模式缓存;内置金丹只读返回 service.pill.builtin_readonly
	UpdatePill(ctx context.Context, uid uuid.UUID, name *string, description *string, skillSchema model.JSONMap, tags model.JSONList, author *string, version *string) (*model.ElixirPill, errors.Error)

	// DeletePill 按 UUID 删除金丹(级联删服用记录并失效缓存);内置金丹只读返回 service.pill.builtin_readonly
	DeletePill(ctx context.Context, uid uuid.UUID) errors.Error

	// ClonePill 深复制金丹为自定义副本:全新 UUID、is_builtin=false、名称追加" 副本",schema/tags 不共享引用
	ClonePill(ctx context.Context, uid uuid.UUID) (*model.ElixirPill, errors.Error)
}
