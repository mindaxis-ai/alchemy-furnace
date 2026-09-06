// 幂等操作包装（§3.1 通用事务规则）
// PillOperation.PillOperationID 全局幂等键；PayloadHash = kind + 标准化参数 SHA-256。
// 执行顺序：验证格式 → 查已提交操作 → 校验同 key 的 kind/hash →
// 事务（插入唯一占位 → 业务变更 → 写完整 ResultJSON）→ 提交。
// 占位与业务同一事务，外部读不到"空结果的成功操作"；SQLite BUSY 最多重试 3 次。
package pill_inventory_service

import (
	"context"
	stderrors "errors"
	"strings"
	"time"

	"github.com/alchemy-furnace/server/internal/dao"
	"github.com/alchemy-furnace/server/internal/errors"
	"github.com/alchemy-furnace/server/internal/interface/service"
	"github.com/alchemy-furnace/server/model"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// busyBackoff SQLite BUSY 重试间隔（§3.1：25/50/100ms）
var busyBackoff = []time.Duration{25 * time.Millisecond, 50 * time.Millisecond, 100 * time.Millisecond}

// operationFn 业务变更；返回的结果 DTO 由包装层写为完整 ResultJSON
type operationFn func(tx *gorm.DB, op *model.PillOperation) (*service.PillOperationResult, error)

// runOperation 幂等执行库存写操作。任何写方法都必须经此包装，不得绕过。
func (s *Inventory) runOperation(ctx context.Context, key uuid.UUID, kind, hash string, fn operationFn) (*service.PillOperationResult, errors.Error) {
	if key == uuid.Nil {
		return nil, errors.New(errors.ErrorTypeInvalidRequest, "pill.invalid_request", "缺少幂等键")
	}

	// 1) 事务外快路径：已提交直接校验返回
	if op, err := dao.PillOperationByUUID(s.db, key); err == nil {
		return s.resolveCommitted(op, kind, hash)
	} else if !stderrors.Is(err, gorm.ErrRecordNotFound) {
		return nil, errors.ErrorServerInternalError("pill.operation.query_failed")
	}

	// 2) 事务执行；BUSY 重试 3 次，仍失败 503（同 key 可重试）
	var result *service.PillOperationResult
	var committed *model.PillOperation
	for attempt := 0; ; attempt++ {
		runErr := s.db.Transaction(func(tx *gorm.DB) error {
			// 事务内重查：覆盖快路径之后的并发提交窗口
			if op, err := dao.PillOperationByUUID(tx, key); err == nil {
				committed = op
				return nil
			} else if !stderrors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
			// 唯一占位：UUID 约束承担并发兜底（唯一冲突方回滚后重读已提交结果）。
			// 必须显式带上幂等键——否则 BeforeCreate 会另生成随机 UUID，幂等键即丢失
			op := &model.PillOperation{PillOperationID: key.String(), Kind: kind, PayloadHash: hash, ResultJSON: model.JSONMap{}}
			if err := dao.CreatePillOperation(tx, op); err != nil {
				if !isUniqueViolation(err) {
					return err
				}
				op2, err2 := dao.PillOperationByUUID(tx, key)
				if err2 == nil {
					committed = op2
					return nil
				}
				return err2
			}
			res, err := fn(tx, op)
			if err != nil {
				return err
			}
			result = res
			return dao.SetPillOperationResult(tx, op.PillOperationID, resultToJSON(res))
		})
		if runErr == nil {
			if committed != nil {
				return s.resolveCommitted(committed, kind, hash)
			}
			if result != nil {
				return result, nil
			}
			return nil, errors.ErrorServerInternalError("pill.operation.invalid_state")
		}
		if !isBusy(runErr) {
			if ie, ok := runErr.(errors.Error); ok {
				return nil, ie
			}
			return nil, errors.ErrorServerInternalError("pill.operation.tx_failed")
		}
		if attempt >= len(busyBackoff) {
			return nil, errors.New(errors.ErrorTypeServiceUnavailable, "pill.storage_busy", "数据库繁忙，请保留同一操作键稍后重试")
		}
		select {
		case <-time.After(busyBackoff[attempt]):
		case <-ctx.Done():
			return nil, errors.ErrorServerInternalError("pill.operation.cancelled")
		}
	}
}

// resolveCommitted 同 key 已提交结果校验：kind/hash 必须一致，否则 409
func (s *Inventory) resolveCommitted(op *model.PillOperation, kind, hash string) (*service.PillOperationResult, errors.Error) {
	if op.Kind != kind || op.PayloadHash != hash {
		return nil, errors.New(errors.ErrorTypeConflict, "pill.operation_payload_mismatch",
			"相同操作键的请求内容不同，请更换操作键后重试")
	}
	return resultFromJSON(op.ResultJSON)
}

// GetOperation 读取已提交操作结果（断线恢复：前端先查 operation，未知时同 key 重试）
func (s *Inventory) GetOperation(ctx context.Context, id uuid.UUID) (*service.PillOperationResult, errors.Error) {
	if id == uuid.Nil {
		return nil, errors.New(errors.ErrorTypeInvalidRequest, "pill.invalid_request", "缺少操作键")
	}
	op, err := dao.PillOperationByUUID(s.db, id)
	if stderrors.Is(err, gorm.ErrRecordNotFound) {
		return nil, errors.ErrorRecordNotFound("pill.operation.not_found")
	}
	if err != nil {
		return nil, errors.ErrorServerInternalError("pill.operation.query_failed")
	}
	return resultFromJSON(op.ResultJSON)
}

// isBusy SQLite 写锁检测（glebarez/sqlite）
func isBusy(err error) bool {
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "database is locked") || strings.Contains(msg, "sqlite_busy")
}

// isUniqueViolation 唯一约束冲突检测（并发幂等兜底路径）
func isUniqueViolation(err error) bool {
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "unique constraint") || strings.Contains(msg, "duplicate key")
}
