// Package agent_service 道人业务逻辑实现(新架构 internal 分层)
// 处理道人增删改查与道人-金丹绑定(服用/解除);UUID 为唯一对外标识
package agent_service

import (
	"context"

	"github.com/alchemy-furnace/server/internal/errors"
	"github.com/alchemy-furnace/server/internal/interface/dao"
	"github.com/alchemy-furnace/server/internal/interface/service"
	"github.com/alchemy-furnace/server/model"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// Agent service.Agent 接口实现
type Agent struct {
	agent dao.Agent
	pill  dao.Pill
	model dao.Model
}

// New 构造道人业务实例
func New(agent dao.Agent, pill dao.Pill, model dao.Model) *Agent {
	return &Agent{agent: agent, pill: pill, model: model}
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
// proactivity 合法区间 0-100;nil=不更新
func (s *Agent) UpdateAgent(ctx context.Context, uid uuid.UUID, name *string, avatar *string, personality *string, modelName *string, status *string, proactivity *int) (*model.DaoAgent, errors.Error) {
	if modelName != nil && *modelName != "" {
		if err := s.validateModelName(ctx, *modelName); err != nil {
			return nil, err
		}
	}
	if proactivity != nil && (*proactivity < 0 || *proactivity > 100) {
		return nil, errors.New(errors.ErrorTypeInvalidRequest, "service.agent.update_proactivity", "主动性需在 0-100 之间")
	}

	agent, err := s.agent.TakeAgentByUUID(ctx, uid)
	if err != nil {
		return nil, err.Relation(errors.ErrorRecordNotFound("service.agent.update_take"))
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

	if len(updates) > 0 {
		if err := s.agent.UpdateAgent(ctx, agent, updates); err != nil {
			return nil, err.Relation(errors.ErrorServerInternalError("service.agent.update"))
		}
		// 基础性格变化时失效语言模式缓存
		if _, ok := updates["personality"]; ok {
			s.invalidatePattern(ctx, agent.ID)
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
func (s *Agent) DeleteAgent(ctx context.Context, uid uuid.UUID) errors.Error {
	agent, err := s.agent.TakeAgentByUUID(ctx, uid)
	if err != nil {
		return err.Relation(errors.ErrorRecordNotFound("service.agent.delete_take"))
	}
	if err := s.agent.DeleteAgent(ctx, agent); err != nil {
		return err.Relation(errors.ErrorServerInternalError("service.agent.delete"))
	}

	zap.L().Info("[炼丹炉] 道人已归隐", zap.String("uuid", uid.String()), zap.String("name", agent.Name))
	return nil
}

// BindPill 道人服用金丹
func (s *Agent) BindPill(ctx context.Context, agentUID uuid.UUID, pillUID uuid.UUID, weight float64, sortOrder int) errors.Error {
	agent, pill, err := s.takeAgentAndPill(ctx, agentUID, pillUID, "bind")
	if err != nil {
		return err
	}

	// 已绑定检查
	if _, err := s.agent.TakeAgentPill(ctx, agent.ID, pill.ID); err == nil {
		return errors.ErrorConflict("service.agent.bind.duplicate", "道人已经服用过这枚金丹了")
	} else if !err.IsType(errors.ErrorTypeRecordNotFound) {
		return err.Relation(errors.ErrorServerInternalError("service.agent.bind_check"))
	}

	if weight <= 0 {
		weight = 1.0
	}
	if sortOrder <= 0 {
		maxOrder, err := s.agent.MaxAgentPillSortOrder(ctx, agent.ID)
		if err != nil {
			return err.Relation(errors.ErrorServerInternalError("service.agent.bind_max_order"))
		}
		sortOrder = maxOrder + 1
	}

	if err := s.agent.SaveAgentPill(ctx, &model.AgentPill{
		AgentID:   agent.ID,
		PillID:    pill.ID,
		Weight:    weight,
		SortOrder: sortOrder,
	}); err != nil {
		return err.Relation(errors.ErrorServerInternalError("service.agent.bind_save"))
	}

	s.invalidatePattern(ctx, agent.ID)

	zap.L().Info("[炼丹炉] 道人服用金丹",
		zap.String("agent_uuid", agentUID.String()),
		zap.String("pill_uuid", pillUID.String()),
		zap.Float64("weight", weight),
		zap.Int("sort_order", sortOrder),
		zap.String("agent_name", agent.Name),
		zap.String("pill_name", pill.Name))
	return nil
}

// UpdateAgentPill 更新服用记录(权重/顺序),并失效缓存
func (s *Agent) UpdateAgentPill(ctx context.Context, agentUID uuid.UUID, pillUID uuid.UUID, weight *float64, sortOrder *int) errors.Error {
	agent, pill, err := s.takeAgentAndPill(ctx, agentUID, pillUID, "uap")
	if err != nil {
		return err
	}

	existing, err := s.agent.TakeAgentPill(ctx, agent.ID, pill.ID)
	if err != nil {
		return err.Relation(errors.ErrorRecordNotFound("service.agent.uap_take_binding"))
	}

	updates := map[string]any{}
	if weight != nil {
		updates["weight"] = *weight
	}
	if sortOrder != nil {
		updates["sort_order"] = *sortOrder
	}
	if len(updates) == 0 {
		return nil
	}

	if err := s.agent.UpdateAgentPill(ctx, existing, updates); err != nil {
		return err.Relation(errors.ErrorServerInternalError("service.agent.uap_update"))
	}

	s.invalidatePattern(ctx, agent.ID)
	return nil
}

// UnbindPill 解除金丹绑定
func (s *Agent) UnbindPill(ctx context.Context, agentUID uuid.UUID, pillUID uuid.UUID) errors.Error {
	agent, pill, err := s.takeAgentAndPill(ctx, agentUID, pillUID, "unbind")
	if err != nil {
		return err
	}

	affected, err := s.agent.DeleteAgentPill(ctx, agent.ID, pill.ID)
	if err != nil {
		return err.Relation(errors.ErrorServerInternalError("service.agent.unbind_delete"))
	}
	if affected == 0 {
		return errors.ErrorRecordNotFound("service.agent.unbind_not_bound")
	}

	s.invalidatePattern(ctx, agent.ID)

	zap.L().Info("[炼丹炉] 道人解除金丹绑定",
		zap.String("agent_uuid", agentUID.String()),
		zap.String("pill_uuid", pillUID.String()))
	return nil
}

// ListAgentPills 道人已服用金丹列表
func (s *Agent) ListAgentPills(ctx context.Context, agentUID uuid.UUID) ([]*model.ElixirPill, errors.Error) {
	agent, err := s.agent.TakeAgentByUUID(ctx, agentUID)
	if err != nil {
		return nil, err.Relation(errors.ErrorRecordNotFound("service.agent.list_pills_take"))
	}
	pills, err := s.agent.FindPillsByAgentID(ctx, agent.ID)
	if err != nil {
		return nil, err.Relation(errors.ErrorServerInternalError("service.agent.list_pills"))
	}
	return pills, nil
}

// ReplacePillComposition 用完整服丹编排一次性替换道人服用关系(原子)
// 顺序: 先全部校验(权重区间/UUID 去重/批量解析金丹),通过后才调一次原子 DAO;成功回读详情
func (s *Agent) ReplacePillComposition(ctx context.Context, agentUID uuid.UUID, items []service.PillCompositionItem) (*model.DaoAgent, errors.Error) {
	// 1) 解析道人
	agent, err := s.agent.TakeAgentByUUID(ctx, agentUID)
	if err != nil {
		return nil, err.Relation(errors.ErrorRecordNotFound("service.agent.replace_pills_take_agent"))
	}

	// 2) 校验权重区间 + UUID 去重,收集待解析的 UUID
	seen := make(map[uuid.UUID]struct{}, len(items))
	uids := make([]uuid.UUID, 0, len(items))
	for _, item := range items {
		if item.Weight <= 0 || item.Weight > 10 {
			return nil, errors.New(errors.ErrorTypeInvalidRequest, "service.agent.replace_pills_weight", "剂量需在 0-10 之间")
		}
		if _, dup := seen[item.PillUUID]; dup {
			return nil, errors.New(errors.ErrorTypeInvalidRequest, "service.agent.replace_pills_duplicate", "同一枚金丹不能重复服用")
		}
		seen[item.PillUUID] = struct{}{}
		uids = append(uids, item.PillUUID)
	}

	// 3) 批量解析金丹 UUID → 内部 ID(一次查询,避免逐项往返)
	pills, err := s.pill.FindPillsByUUIDs(ctx, uids)
	if err != nil {
		return nil, err.Relation(errors.ErrorServerInternalError("service.agent.replace_pills_find"))
	}
	byUUID := make(map[uuid.UUID]*model.ElixirPill, len(pills))
	for _, p := range pills {
		byUUID[p.UUID] = p
	}

	// 4) 按请求顺序映射为 DAO 输入;任一缺失即 404(整体不落库)
	daoInputs := make([]dao.AgentPillInput, 0, len(items))
	for _, item := range items {
		p, ok := byUUID[item.PillUUID]
		if !ok {
			return nil, errors.ErrorRecordNotFound("service.agent.replace_pills_pill_not_found")
		}
		daoInputs = append(daoInputs, dao.AgentPillInput{PillID: p.ID, Weight: item.Weight})
	}

	// 5) 一次原子替换(事务内删旧写新 + 失效缓存)
	if err := s.agent.ReplaceAgentPills(ctx, agent.ID, daoInputs); err != nil {
		return nil, err.Relation(errors.ErrorServerInternalError("service.agent.replace_pills"))
	}

	// 6) 回读服务端确认详情
	detail, err := s.agent.TakeAgentDetailByUUID(ctx, agentUID)
	if err != nil {
		return nil, err.Relation(errors.ErrorServerInternalError("service.agent.replace_pills_retake"))
	}

	zap.L().Info("[炼丹炉] 道人服丹编排已更新",
		zap.String("agent_uuid", agentUID.String()),
		zap.String("agent_name", agent.Name),
		zap.Int("pill_count", len(daoInputs)))
	return detail, nil
}

// takeAgentAndPill 绑定类操作公共前置: 按 UUID 解析道人与金丹
func (s *Agent) takeAgentAndPill(ctx context.Context, agentUID uuid.UUID, pillUID uuid.UUID, op string) (*model.DaoAgent, *model.ElixirPill, errors.Error) {
	agent, err := s.agent.TakeAgentByUUID(ctx, agentUID)
	if err != nil {
		return nil, nil, err.Relation(errors.ErrorRecordNotFound("service.agent." + op + "_take_agent"))
	}
	pill, err := s.pill.TakePillByUUID(ctx, pillUID)
	if err != nil {
		return nil, nil, err.Relation(errors.ErrorRecordNotFound("service.agent." + op + "_take_pill"))
	}
	return agent, pill, nil
}

// invalidatePattern 失效道人语言模式缓存;失败仅告警不阻塞主流程
func (s *Agent) invalidatePattern(ctx context.Context, agentID uint) {
	if err := s.agent.InvalidateLanguagePattern(ctx, agentID); err != nil {
		zap.L().Warn("[炼丹炉] 失效语言模式缓存失败", zap.Uint("agent_id", agentID), zap.Error(err))
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
