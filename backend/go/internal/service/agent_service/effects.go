// 能力编排（任务 5,effect UUID 语义）
// ListEffects 读道人活跃能力(含来源实例/版本对外标识);
// UpdateEffects 全量编排提交: 提交集必须等于活跃集,任何缺失/重复/外部能力 → 409,
// expectedEffectsRevision 乐观锁过期同 409,幂等性天然成立(重复执行结果相同);
// RemoveEffect 显式移除能力(软删,原实例不返还)。
package agent_service

import (
	"context"
	"time"

	"github.com/alchemy-furnace/server/internal/errors"
	idao "github.com/alchemy-furnace/server/internal/interface/dao"
	"github.com/alchemy-furnace/server/internal/interface/service"
	"github.com/alchemy-furnace/server/model"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// ListEffects 道人活跃能力列表(按 sort_order 升序) + effects_revision（前端乐观锁）
func (s *Agent) ListEffects(ctx context.Context, agentUID uuid.UUID) ([]*service.EffectWithSource, int, errors.Error) {
	agent, err := s.agent.TakeAgentByUUID(ctx, agentUID)
	if err != nil {
		return nil, 0, err.Relation(errors.ErrorRecordNotFound("service.agent.list_effects_take"))
	}
	effects, err := s.agent.ListActiveEffects(ctx, agent.UUID.String())
	if err != nil {
		return nil, 0, err.Relation(errors.ErrorServerInternalError("service.agent.list_effects"))
	}
	out := make([]*service.EffectWithSource, 0, len(effects))
	for _, ef := range effects {
		out = append(out, &service.EffectWithSource{
			Effect:       ef.Effect,
			ItemUUID:     ef.ItemUUID,
			RevisionUUID: ef.RevisionUUID,
		})
	}
	return out, agent.EffectsRevision, nil
}

// UpdateEffects 全量编排提交:
// 1) 读取活跃集快照(ListEffects 同源);
// 2) 集合校验: 提交集必须等于活跃集(数量一致 + 无重复 + 全部存在,含外部道人能力) → 409;
// 3) 乐观锁提交(CAS),失败(并发已提交) → 409;成功返回更新后的道人(含新 EffectsRevision)。
// 校验与提交间无写并发窗口: 中间若有并发变更,effects_revision 已被递增,CAS 0 行拦截。
func (s *Agent) UpdateEffects(ctx context.Context, agentUID uuid.UUID, expectedEffectsRevision int, items []service.EffectUpdateItem) (*model.DaoAgent, errors.Error) {
	agent, err := s.agent.TakeAgentByUUID(ctx, agentUID)
	if err != nil {
		return nil, err.Relation(errors.ErrorRecordNotFound("service.agent.update_effects_take"))
	}
	active, err := s.agent.ListActiveEffects(ctx, agent.UUID.String())
	if err != nil {
		return nil, err.Relation(errors.ErrorServerInternalError("service.agent.update_effects_list"))
	}

	if err := validateEffectSet(active, items); err != nil {
		return nil, err
	}

	writes := make([]idao.EffectWrite, 0, len(items))
	for _, it := range items {
		for _, ef := range active {
			if ef.Effect.UUID == it.EffectID {
				writes = append(writes, idao.EffectWrite{
					EffectUUID: ef.Effect.UUID.String(),
					Weight:     it.Weight,
					SortOrder:  it.SortOrder,
				})
				break
			}
		}
	}

	ok, err := s.agent.UpdateActiveEffectsCAS(ctx, agent.UUID.String(), expectedEffectsRevision, writes)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, errors.ErrorConflict("service.agent.effects_conflict",
			"能力编排已变更,请刷新后重试")
	}

	updated, err := s.agent.TakeAgentByUUID(ctx, agentUID)
	if err != nil {
		return nil, err.Relation(errors.ErrorServerInternalError("service.agent.update_effects_reload"))
	}
	return updated, nil
}

// validateEffectSet 提交集校验: 数量一致 + 无重复 + 全部属于活跃集(外部/幽灵能力拒绝)
func validateEffectSet(active []idao.EffectWithSource, items []service.EffectUpdateItem) errors.Error {
	if len(items) != len(active) {
		return errors.ErrorConflict("service.agent.effects_conflict",
			"提交的能力集合与当前活跃能力不一致")
	}
	activeSet := make(map[uuid.UUID]struct{}, len(active))
	for _, ef := range active {
		activeSet[ef.Effect.UUID] = struct{}{}
	}
	seen := make(map[uuid.UUID]struct{}, len(items))
	for _, it := range items {
		if _, dup := seen[it.EffectID]; dup {
			return errors.ErrorConflict("service.agent.effects_conflict",
				"提交的能力集合与当前活跃能力不一致")
		}
		seen[it.EffectID] = struct{}{}
		if _, exists := activeSet[it.EffectID]; !exists {
			return errors.ErrorConflict("service.agent.effects_conflict",
				"提交的能力集合与当前活跃能力不一致")
		}
	}
	return nil
}

// RemoveEffect 显式移除能力(按能力 UUID,任务 5):
// 单事务: 软删能力(removed_at 保留历史) → EffectsRevision++ → 缓存失效。
// 原实例保持 consumed_by_agent 不返还库存;不存在/已移除/跨道人返回 404。
func (s *Agent) RemoveEffect(ctx context.Context, agentUID uuid.UUID, effectUUID uuid.UUID) errors.Error {
	agent, err := s.agent.TakeAgentByUUID(ctx, agentUID)
	if err != nil {
		return err.Relation(errors.ErrorRecordNotFound("service.agent.remove_effect_take"))
	}
	if err := s.agent.RemoveAgentPillEffectByUUID(ctx, agent.UUID.String(), effectUUID, time.Now()); err != nil {
		return err
	}
	zap.L().Info("[炼丹炉] 道人移除已吸收能力",
		zap.String("agent_uuid", agentUID.String()),
		zap.String("effect_uuid", effectUUID.String()))
	return nil
}
