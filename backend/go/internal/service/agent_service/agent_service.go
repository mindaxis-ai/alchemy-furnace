// Package agent_service 道人业务逻辑实现(新架构 internal 分层)
// 处理道人增删改查与金丹服用/移除/调权(任务 3 起全部经过库存,不保留绕过库存的写路径);
// UUID 为唯一对外标识;服用的对外标识为金丹实例 UUID(非丹方/金丹 UUID)
package agent_service

import (
	"context"
	"time"

	"github.com/alchemy-furnace/server/internal/errors"
	"github.com/alchemy-furnace/server/internal/interface/dao"
	"github.com/alchemy-furnace/server/internal/interface/service"
	"github.com/alchemy-furnace/server/model"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// PillConsumer 服用所需的最小库存能力(任务 3 注入点):
// 定义为 Consume 单方法窄接口,不依赖任务 4 才实现的 ConfirmFusion;
// iservice.PillInventory 方法集包含本接口,wire 静态推导可注入完整实现。
type PillConsumer interface {
	// Consume 服用一枚金丹: available→consumed_by_agent + 能力快照(幂等)
	Consume(ctx context.Context, req service.ConsumePillRequest) (*service.PillOperationResult, errors.Error)
}

// Agent service.Agent 接口实现
type Agent struct {
	agent     dao.Agent
	model     dao.Model
	inventory PillConsumer // 服用的库存事实来源(幂等消费,任务 3)
}

// New 构造道人业务实例
func New(agent dao.Agent, model dao.Model, inventory PillConsumer) *Agent {
	return &Agent{agent: agent, model: model, inventory: inventory}
}

// ListAgents 分页查询道人列表
func (s *Agent) ListAgents(ctx context.Context, page int, size int, status string) (int64, []*model.DaoAgent, errors.Error) {
	if page < 1 {
		page = 1
	}
	if size < 1 {
		size = 10
	}
	if size > 100 {
		size = 100
	}
	return s.agent.FindAgents(ctx, page, size, status)
}

// GetAgentDetailByUUID 按 UUID 获取道人详情(含服用记录与语言模式缓存)
func (s *Agent) GetAgentDetailByUUID(ctx context.Context, uid uuid.UUID) (*model.DaoAgent, errors.Error) {
	agent, err := s.agent.TakeAgentDetailByUUID(ctx, uid)
	if err != nil {
		return nil, err.Relation(errors.ErrorRecordNotFound("service.agent.get_detail"))
	}
	return agent, nil
}

// CreateAgent 创建道人
// proactivity 为 nil 时取默认值 50;合法区间 0-100,越界返回 InvalidRequest
func (s *Agent) CreateAgent(ctx context.Context, name string, avatar string, personality string, modelName string, proactivity *int) (*model.DaoAgent, errors.Error) {
	if proactivity != nil && (*proactivity < 0 || *proactivity > 100) {
		return nil, errors.New(errors.ErrorTypeInvalidRequest, "service.agent.create_proactivity", "主动性需在 0-100 之间")
	}
	if err := s.validateModelName(ctx, modelName); err != nil {
		return nil, err
	}

	if modelName == "" {
		modelName = "gpt-4o"
	}
	proactivityVal := 50
	if proactivity != nil {
		proactivityVal = *proactivity
	}
	agent := &model.DaoAgent{
		Name:        name,
		Avatar:      avatar,
		Personality: personality,
		ModelName:   modelName,
		Status:      "active",
		Proactivity: proactivityVal,
	}
	if err := s.agent.SaveAgent(ctx, agent); err != nil {
		return nil, err.Relation(errors.ErrorServerInternalError("service.agent.create"))
	}

	zap.L().Info("[炼丹炉] 新道人下山历练", zap.String("name", agent.Name), zap.String("uuid", agent.UUID.String()))
	return agent, nil
}

// UpdateAgent 部分更新道人;性格变化时失效语言模式缓存
// proactivity 合法区间 0-100;nil=不更新;status 仅接受 active/inactive
// memoryEnabled nil=不更新;最终状态为 active 时,最终模型必须可用(即使本次未改 model_name)
func (s *Agent) UpdateAgent(ctx context.Context, uid uuid.UUID, name *string, avatar *string, personality *string, modelName *string, status *string, proactivity *int, memoryEnabled *bool) (*model.DaoAgent, errors.Error) {
	if proactivity != nil && (*proactivity < 0 || *proactivity > 100) {
		return nil, errors.New(errors.ErrorTypeInvalidRequest, "service.agent.update_proactivity", "主动性需在 0-100 之间")
	}
	// status 仅接受 active/inactive
	if status != nil && *status != "" && *status != "active" && *status != "inactive" {
		return nil, errors.New(errors.ErrorTypeInvalidRequest, "service.agent.update_status", "状态仅支持 active/inactive")
	}
	// 显式更换模型时校验新模型可用
	if modelName != nil && *modelName != "" {
		if err := s.validateModelName(ctx, *modelName); err != nil {
			return nil, err
		}
	}

	agent, err := s.agent.TakeAgentByUUID(ctx, uid)
	if err != nil {
		return nil, err.Relation(errors.ErrorRecordNotFound("service.agent.update_take"))
	}

	// 校验「更新后的最终状态 + 最终模型」: 最终为 active 时最终模型必须可用
	finalStatus := agent.Status
	if status != nil && *status != "" {
		finalStatus = *status
	}
	finalModel := agent.ModelName
	if modelName != nil && *modelName != "" {
		finalModel = *modelName
	}
	if finalStatus == "active" {
		if err := s.validateModelName(ctx, finalModel); err != nil {
			return nil, err
		}
	}

	updates := map[string]any{}
	if name != nil && *name != "" {
		updates["name"] = *name
	}
	if avatar != nil {
		updates["avatar"] = *avatar
	}
	if personality != nil {
		updates["personality"] = *personality
	}
	if modelName != nil && *modelName != "" {
		updates["model_name"] = *modelName
	}
	if status != nil && *status != "" {
		updates["status"] = *status
	}
	if proactivity != nil {
		updates["proactivity"] = *proactivity
	}
	if memoryEnabled != nil {
		updates["memory_enabled"] = *memoryEnabled
	}

	if len(updates) > 0 {
		if err := s.agent.UpdateAgent(ctx, agent, updates); err != nil {
			return nil, err.Relation(errors.ErrorServerInternalError("service.agent.update"))
		}
		// 基础性格变化时失效语言模式缓存
		if _, ok := updates["personality"]; ok {
			s.invalidatePattern(ctx, agent.UUID.String())
		}
	}

	fresh, err := s.agent.TakeAgentByUUID(ctx, uid)
	if err != nil {
		return nil, err.Relation(errors.ErrorServerInternalError("service.agent.update_retake"))
	}

	zap.L().Info("[炼丹炉] 道人信息已更新", zap.String("uuid", uid.String()), zap.String("name", fresh.Name))
	return fresh, nil
}

// DeleteAgent 删除道人
// 有会话历史(单聊/群聊)时禁止硬删除,返回 409 并携带 session_count,引导前端改沉睡
func (s *Agent) DeleteAgent(ctx context.Context, uid uuid.UUID) errors.Error {
	agent, err := s.agent.TakeAgentByUUID(ctx, uid)
	if err != nil {
		return err.Relation(errors.ErrorRecordNotFound("service.agent.delete_take"))
	}

	// 历史感知: 有会话历史只能沉睡不能删
	sessionCount, err := s.agent.CountSessionsByAgentID(ctx, agent.UUID.String())
	if err != nil {
		return err.Relation(errors.ErrorServerInternalError("service.agent.delete_count_sessions"))
	}
	if sessionCount > 0 {
		return errors.ErrorConflictWithData(
			"service.agent.delete_has_history",
			map[string]any{"session_count": sessionCount},
			"道人有 %d 段会话历史，只能沉睡不能删除", sessionCount)
	}

	if err := s.agent.DeleteAgent(ctx, agent); err != nil {
		return err.Relation(errors.ErrorServerInternalError("service.agent.delete"))
	}

	zap.L().Info("[炼丹炉] 道人已归隐", zap.String("uuid", uid.String()), zap.String("name", agent.Name))
	return nil
}

// BindPill 道人服用金丹实例(任务 3)
// 第二参数为金丹实例 UUID(非丹方/金丹 UUID);经库存幂等消费完成:
// available→consumed_by_agent + 能力快照 + EffectsRevision++ + 缓存失效(单事务)。
// 幂等键由本服务生成(旧入口无 Idempotency-Key 契约): 同一实例二次服用
// (不同幂等键)命中 CAS 0 行 → 409 pill.not_available;实例不存在/未知道人 → 404。
func (s *Agent) BindPill(ctx context.Context, agentUID uuid.UUID, itemUID uuid.UUID, weight float64, sortOrder int) errors.Error {
	if _, err := s.inventory.Consume(ctx, service.ConsumePillRequest{
		OperationID: uuid.New(),
		AgentID:     agentUID,
		ItemID:      itemUID,
		Weight:      weight,
		SortOrder:   sortOrder,
	}); err != nil {
		return err
	}

	zap.L().Info("[炼丹炉] 道人服用金丹",
		zap.String("agent_uuid", agentUID.String()),
		zap.String("item_uuid", itemUID.String()),
		zap.Float64("weight", weight),
		zap.Int("sort_order", sortOrder))
	return nil
}

// UpdateAgentPill 调整已吸收能力(权重/顺序,任务 3)
// 第二参数为金丹实例 UUID;单事务: 更新能力 → EffectsRevision++ → 缓存失效。
// 无活跃能力(未吸收/已移除)返回 404;weight/sortOrder 均为 nil 时仅校验存在。
func (s *Agent) UpdateAgentPill(ctx context.Context, agentUID uuid.UUID, itemUID uuid.UUID, weight *float64, sortOrder *int) errors.Error {
	agent, err := s.agent.TakeAgentByUUID(ctx, agentUID)
	if err != nil {
		return err.Relation(errors.ErrorRecordNotFound("service.agent.uap_take_agent"))
	}

	if err := s.agent.UpdateAgentPillEffect(ctx, agent.UUID.String(), itemUID, weight, sortOrder); err != nil {
		return err
	}
	return nil
}

// UnbindPill 移除道人的已吸收能力(任务 3)
// 第二参数为金丹实例 UUID;单事务: 软删能力(removed_at 保留历史) → EffectsRevision++ →
// 缓存失效。原实例保持 consumed_by_agent 不返还库存;无活跃能力返回 404。
func (s *Agent) UnbindPill(ctx context.Context, agentUID uuid.UUID, itemUID uuid.UUID) errors.Error {
	agent, err := s.agent.TakeAgentByUUID(ctx, agentUID)
	if err != nil {
		return err.Relation(errors.ErrorRecordNotFound("service.agent.unbind_take_agent"))
	}

	if err := s.agent.RemoveAgentPillEffect(ctx, agent.UUID.String(), itemUID, time.Now()); err != nil {
		return err
	}

	zap.L().Info("[炼丹炉] 道人移除已吸收能力",
		zap.String("agent_uuid", agentUID.String()),
		zap.String("item_uuid", itemUID.String()))
	return nil
}

// ListAgentPills 道人已服用金丹列表
func (s *Agent) ListAgentPills(ctx context.Context, agentUID uuid.UUID) ([]*model.ElixirPill, errors.Error) {
	agent, err := s.agent.TakeAgentByUUID(ctx, agentUID)
	if err != nil {
		return nil, err.Relation(errors.ErrorRecordNotFound("service.agent.list_pills_take"))
	}
	pills, err := s.agent.FindPillsByAgentID(ctx, agent.UUID.String())
	if err != nil {
		return nil, err.Relation(errors.ErrorServerInternalError("service.agent.list_pills"))
	}
	return pills, nil
}

// ReplacePillComposition 已下线(任务 3): 完整服丹编排允许一次写多条绑定,绕过库存
// (不消耗实例、不生成能力快照),不再保留。任意输入一律返回 410 pill.legacy_api_removed;
// 清空编排不是移除能力,改走 UnbindPill;HTTP 层路由封禁在任务 5。
func (s *Agent) ReplacePillComposition(ctx context.Context, agentUID uuid.UUID, items []service.PillCompositionItem) (*model.DaoAgent, errors.Error) {
	return nil, errors.ErrorGone("pill.legacy_api_removed",
		"完整服丹编排接口已下线(金丹消耗品重构),请改用服用/移除/融合接口")
}

// invalidatePattern 失效道人语言模式缓存;失败仅告警不阻塞主流程
func (s *Agent) invalidatePattern(ctx context.Context, agentUID string) {
	if err := s.agent.InvalidateLanguagePattern(ctx, agentUID); err != nil {
		zap.L().Warn("[炼丹炉] 失效语言模式缓存失败", zap.String("agent_uid", agentUID), zap.Error(err))
	}
}

// validateModelName 校验模型名引用了已启用供应商下的已启用模型配置
func (s *Agent) validateModelName(ctx context.Context, name string) errors.Error {
	if name == "" {
		return nil
	}
	count, err := s.model.CountEnabledModelByName(ctx, name)
	if err != nil {
		return err.Relation(errors.ErrorServerInternalError("service.agent.validate_model"))
	}
	if count == 0 {
		return errors.New(errors.ErrorTypeInvalidRequest, "service.agent.model_disabled", "模型「%s」不存在或已停用，请先在模型管理中配置", name)
	}
	return nil
}
