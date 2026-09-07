package dao

import (
	"context"

	"github.com/alchemy-furnace/server/internal/errors"
	"github.com/alchemy-furnace/server/model"
	"github.com/google/uuid"
)

// Pill 金丹数据访问接口
type Pill interface {
	// TakePillByUUID 按对外 UUID 查询金丹,不存在返回 ErrorTypeRecordNotFound
	TakePillByUUID(ctx context.Context, uid uuid.UUID) (*model.ElixirPill, errors.Error)

	// FindPillsByUUIDs 按 UUID 批量查询金丹(试丹等场景;结果顺序不保证,调用方自行排序)
	FindPillsByUUIDs(ctx context.Context, uids []uuid.UUID) ([]*model.ElixirPill, errors.Error)

	// FindPills 分页查询金丹列表(keyword 模糊匹配名称/描述,isBuiltin 非 nil 时过滤)
	FindPills(ctx context.Context, page int, size int, keyword string, isBuiltin *bool) (int64, []*model.ElixirPill, errors.Error)

	// SavePill 新建金丹
	SavePill(ctx context.Context, pill *model.ElixirPill) errors.Error

	// UpdatePill 按字段 map 部分更新
	UpdatePill(ctx context.Context, pill *model.ElixirPill, updates map[string]any) errors.Error

	// DeletePill 删除金丹(服用记录由 FK CASCADE 清理)
	DeletePill(ctx context.Context, pill *model.ElixirPill) errors.Error

	// FindAgentIDsByPillID 查询服用了指定金丹的道人 UUID 文本列表(缓存失效用)
	FindAgentIDsByPillID(ctx context.Context, pillUID string) ([]string, errors.Error)

	// InvalidateLanguagePatternsByAgentIDs 批量失效道人(按 UUID 文本)的语言模式缓存
	InvalidateLanguagePatternsByAgentIDs(ctx context.Context, agentUIDs []string) errors.Error
}
