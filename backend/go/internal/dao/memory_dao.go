// Package dao 道人数据访问实现(新架构 internal 分层;UUID 边界在此解析,内部联结仍用自增 ID)
package dao

import (
	"context"
	"time"

	"github.com/alchemy-furnace/server/internal/errors"
	"github.com/alchemy-furnace/server/model"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// MemoryDao dao.Memory 接口实现
type MemoryDao struct{}

// NewMemoryDao 构造记忆 DAO
func NewMemoryDao() *MemoryDao {
	return &MemoryDao{}
}

// ListMemories 按道人列出记忆(kind 为空不过滤;onlyActive=true 仅 active),新近优先
func (d *MemoryDao) ListMemories(ctx context.Context, agentID string, kind string, onlyActive bool) ([]*model.AgentMemory, errors.Error) {
	db := GetDB().WithContext(ctx).Model(&model.AgentMemory{}).Where("agent_id = ?", agentID)
	if kind != "" {
		db = db.Where("kind = ?", kind)
	}
	if onlyActive {
		db = db.Where("status = ?", "active")
	}
	var list []*model.AgentMemory
	if err := db.Order("created_at DESC").Find(&list).Error; err != nil {
		return nil, errors.ErrorServerInternalError("dao.memory.list_memories")
	}
	return list, nil
}

// GetMemory 按记忆 UUID 文本查询记忆(011 业务键,列 agent_memories.uuid)
func (d *MemoryDao) GetMemory(ctx context.Context, memoryUID string) (*model.AgentMemory, errors.Error) {
	var m model.AgentMemory
	if err := GetDB().WithContext(ctx).Where("uuid = ?", memoryUID).First(&m).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, errors.ErrorRecordNotFound("dao.memory.get_memory")
		}
		return nil, errors.ErrorServerInternalError("dao.memory.get_memory")
	}
	return &m, nil
}

// TakeMemoryByUUID 按对外 UUID 查询记忆
func (d *MemoryDao) TakeMemoryByUUID(ctx context.Context, uid uuid.UUID) (*model.AgentMemory, errors.Error) {
	var m model.AgentMemory
	if err := GetDB().WithContext(ctx).Where("uuid = ?", uid.String()).First(&m).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, errors.ErrorRecordNotFound("dao.memory.take_by_uuid")
		}
		return nil, errors.ErrorServerInternalError("dao.memory.take_by_uuid")
	}
	return &m, nil
}

// SaveMemory 新建记忆
func (d *MemoryDao) SaveMemory(ctx context.Context, m *model.AgentMemory) errors.Error {
	if err := GetDB().WithContext(ctx).Create(m).Error; err != nil {
		return errors.ErrorServerInternalError("dao.memory.save_memory")
	}
	return nil
}

// UpdateMemory 全字段更新既有记忆
func (d *MemoryDao) UpdateMemory(ctx context.Context, m *model.AgentMemory) errors.Error {
	if err := GetDB().WithContext(ctx).Save(m).Error; err != nil {
		return errors.ErrorServerInternalError("dao.memory.update_memory")
	}
	return nil
}

// DeleteMemory 物理删除单条记忆(spec §10.2:用户删除=物理删除);memoryUID 为记忆 UUID 文本
func (d *MemoryDao) DeleteMemory(ctx context.Context, memoryUID string) errors.Error {
	if err := GetDB().WithContext(ctx).Where("uuid = ?", memoryUID).Delete(&model.AgentMemory{}).Error; err != nil {
		return errors.ErrorServerInternalError("dao.memory.delete_memory")
	}
	return nil
}

// DeleteMemoriesByAgent 物理清空道人全部记忆
func (d *MemoryDao) DeleteMemoriesByAgent(ctx context.Context, agentID string) (int64, errors.Error) {
	result := GetDB().WithContext(ctx).Where("agent_id = ?", agentID).Delete(&model.AgentMemory{})
	if result.Error != nil {
		return 0, errors.ErrorServerInternalError("dao.memory.delete_by_agent")
	}
	return result.RowsAffected, nil
}

// FindActiveByContentHash 按内容哈希查 active 记忆(无命中返回 nil,nil)
func (d *MemoryDao) FindActiveByContentHash(ctx context.Context, agentID string, hash string) (*model.AgentMemory, errors.Error) {
	var m model.AgentMemory
	if err := GetDB().WithContext(ctx).
		Where("agent_id = ? AND content_hash = ? AND status = ?", agentID, hash, "active").
		First(&m).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, errors.ErrorServerInternalError("dao.memory.find_active_by_hash")
	}
	return &m, nil
}

// SupersedeMemory 将记忆置为 superseded(冲突置替,spec §10.2);memoryUID 为记忆 UUID 文本
func (d *MemoryDao) SupersedeMemory(ctx context.Context, memoryUID string) errors.Error {
	if err := GetDB().WithContext(ctx).Model(&model.AgentMemory{}).
		Where("uuid = ?", memoryUID).
		Update("status", "superseded").Error; err != nil {
		return errors.ErrorServerInternalError("dao.memory.supersede")
	}
	return nil
}

// TouchMemory 更新最近检索时间(LastAccessedAt);memoryUID 为记忆 UUID 文本
func (d *MemoryDao) TouchMemory(ctx context.Context, memoryUID string) errors.Error {
	if err := GetDB().WithContext(ctx).Model(&model.AgentMemory{}).
		Where("uuid = ?", memoryUID).
		Update("last_accessed_at", time.Now()).Error; err != nil {
		return errors.ErrorServerInternalError("dao.memory.touch")
	}
	return nil
}
