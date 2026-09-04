// Package dao 数据访问接口定义(对齐 Luna-CY 模板 internal/interface/dao)
// 接口入参为对外 UUID 或内部模型,返回内部模型;错误一律为 errors.Error 类型化错误
package dao

import (
	"context"

	"github.com/alchemy-furnace/server/internal/errors"
	"github.com/alchemy-furnace/server/model"
	"github.com/google/uuid"
)

// Memory 道人本地记忆数据访问接口(spec §10.1/§10.2)
// UUID 边界在此解析,内部联结仍用自增 ID;删除/清空为物理删除
type Memory interface {
	// ListMemories 按道人列出记忆(kind 为空不过滤;onlyActive=true 仅 active)
	ListMemories(ctx context.Context, agentID string, kind string, onlyActive bool) ([]*model.AgentMemory, errors.Error)

	// GetMemory 按内部自增 ID 查询记忆
	GetMemory(ctx context.Context, id uint) (*model.AgentMemory, errors.Error)

	// TakeMemoryByUUID 按对外 UUID 查询记忆,不存在返回 ErrorTypeRecordNotFound
	TakeMemoryByUUID(ctx context.Context, uid uuid.UUID) (*model.AgentMemory, errors.Error)

	// SaveMemory 新建记忆
	SaveMemory(ctx context.Context, m *model.AgentMemory) errors.Error

	// UpdateMemory 全字段更新既有记忆
	UpdateMemory(ctx context.Context, m *model.AgentMemory) errors.Error

	// DeleteMemory 物理删除单条记忆
	DeleteMemory(ctx context.Context, id uint) errors.Error

	// DeleteMemoriesByAgent 物理清空道人全部记忆,返回受影响行数
	DeleteMemoriesByAgent(ctx context.Context, agentID string) (int64, errors.Error)

	// FindActiveByContentHash 按内容哈希查 active 记忆(无命中返回 nil,nil)
	FindActiveByContentHash(ctx context.Context, agentID string, hash string) (*model.AgentMemory, errors.Error)

	// SupersedeMemory 将记忆置为 superseded(冲突置替,spec §10.2)
	SupersedeMemory(ctx context.Context, id uint) errors.Error

	// TouchMemory 更新最近检索时间(LastAccessedAt)
	TouchMemory(ctx context.Context, id uint) errors.Error
}
