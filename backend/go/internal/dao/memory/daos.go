// memory 包的 5 个 DAO 实现(007-demo-mode)
// 与 internal/dao/* 的 GORM 实现满足同一组 internal/interface/dao 接口,
// wire 在 DEMO_MODE=true 时注入本组实现,service 层无感知。
package memory

import (
	"context"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/alchemy-furnace/server/internal/errors"
	"github.com/alchemy-furnace/server/model"
	"github.com/google/uuid"
)

// ==================== PillDao ====================

// PillDao dao.Pill 接口的内存实现
type PillDao struct {
	store *Store
}

// NewPillDao 构造金丹内存 DAO(注入共享 store 单例)
func NewPillDao() *PillDao { return &PillDao{store: SharedStore()} }

func (d *PillDao) TakePillByUUID(ctx context.Context, uid uuid.UUID) (*model.ElixirPill, errors.Error) {
	d.store.muPill.RLock()
	defer d.store.muPill.RUnlock()
	p, ok := d.store.pills[uid.String()]
	if !ok {
		return nil, errors.ErrorRecordNotFound("dao.pill.take_by_uuid")
	}
	return clonePill(p), nil
}

func (d *PillDao) FindPillsByUUIDs(ctx context.Context, uids []uuid.UUID) ([]*model.ElixirPill, errors.Error) {
	if len(uids) == 0 {
		return nil, nil
	}
	d.store.muPill.RLock()
	defer d.store.muPill.RUnlock()
	out := make([]*model.ElixirPill, 0, len(uids))
	for _, u := range uids {
		if p, ok := d.store.pills[u.String()]; ok {
			out = append(out, clonePill(p))
		}
	}
	return out, nil
}

func (d *PillDao) FindPills(ctx context.Context, page int, size int, keyword string, isBuiltin *bool) (int64, []*model.ElixirPill, errors.Error) {
	d.store.muPill.RLock()
	defer d.store.muPill.RUnlock()
	all := make([]*model.ElixirPill, 0, len(d.store.pills))
	for _, p := range d.store.pills {
		if keyword != "" {
			if !strings.Contains(p.Name, keyword) && !strings.Contains(p.Description, keyword) {
				continue
			}
		}
		if isBuiltin != nil && p.IsBuiltin != *isBuiltin {
			continue
		}
		all = append(all, clonePill(p))
	}
	sort.SliceStable(all, func(i, j int) bool {
		if all[i].IsBuiltin != all[j].IsBuiltin {
			return all[i].IsBuiltin
		}
		return all[i].UpdatedAt.After(all[j].UpdatedAt)
	})
	total := int64(len(all))
	if total == 0 || size <= 0 || int64((page-1)*size) >= total {
		return total, nil, nil
	}
	start := (page - 1) * size
	end := start + size
	if end > len(all) {
		end = len(all)
	}
	return total, all[start:end], nil
}

func (d *PillDao) SavePill(ctx context.Context, pill *model.ElixirPill) errors.Error {
	d.store.muPill.Lock()
	defer d.store.muPill.Unlock()
	d.store.nextPillID++
	pill.ID = d.store.nextPillID
	if pill.UUID == (uuid.UUID{}) {
		pill.UUID = uuid.New()
	}
	now := time.Now()
	pill.CreatedAt = now
	pill.UpdatedAt = now
	d.store.pills[pill.UUID.String()] = clonePill(pill)
	return nil
}

func (d *PillDao) UpdatePill(ctx context.Context, pill *model.ElixirPill, updates map[string]any) errors.Error {
	d.store.muPill.Lock()
	defer d.store.muPill.Unlock()
	p, ok := d.store.pills[pill.UUID.String()]
	if !ok {
		return errors.ErrorRecordNotFound("dao.pill.update_pill")
	}
	applyUpdates(p, updates)
	p.UpdatedAt = time.Now()
	*pill = *clonePill(p)
	return nil
}

func (d *PillDao) DeletePill(ctx context.Context, pill *model.ElixirPill) errors.Error {
	d.store.muPill.Lock()
	d.store.muAp.Lock()
	defer d.store.muPill.Unlock()
	defer d.store.muAp.Unlock()
	delete(d.store.pills, pill.UUID.String())
	for agentID, binds := range d.store.agentPill {
		if _, ok := binds[pill.ID]; ok {
			delete(binds, pill.ID)
			if len(binds) == 0 {
				delete(d.store.agentPill, agentID)
			}
		}
	}
	return nil
}

func (d *PillDao) FindAgentIDsByPillID(ctx context.Context, pillID uint) ([]uint, errors.Error) {
	d.store.muAp.RLock()
	defer d.store.muAp.RUnlock()
	out := make([]uint, 0)
	for agentID, binds := range d.store.agentPill {
		if _, ok := binds[pillID]; ok {
			out = append(out, agentID)
		}
	}
	return out, nil
}

func (d *PillDao) InvalidateLanguagePatternsByAgentIDs(ctx context.Context, agentIDs []uint) errors.Error {
	if len(agentIDs) == 0 {
		return nil
	}
	d.store.muPattern.Lock()
	defer d.store.muPattern.Unlock()
	for _, aid := range agentIDs {
		if pat, ok := d.store.patterns[aid]; ok {
			pat.IsValid = false
		}
	}
	return nil
}

// ==================== AgentDao ====================

// AgentDao dao.Agent 接口的内存实现
type AgentDao struct {
	store *Store
}

// NewAgentDao 构造道人内存 DAO
func NewAgentDao() *AgentDao { return &AgentDao{store: SharedStore()} }

func (d *AgentDao) TakeAgentByUUID(ctx context.Context, uid uuid.UUID) (*model.DaoAgent, errors.Error) {
	d.store.muAgent.RLock()
	defer d.store.muAgent.RUnlock()
	a, ok := d.store.agents[uid.String()]
	if !ok {
		return nil, errors.ErrorRecordNotFound("dao.agent.take_by_uuid")
	}
	return cloneAgent(a), nil
}

func (d *AgentDao) TakeAgentDetailByUUID(ctx context.Context, uid uuid.UUID) (*model.DaoAgent, errors.Error) {
	return d.takeDetail(uid.String(), "dao.agent.take_detail_by_uuid")
}

func (d *AgentDao) TakeAgentDetailByID(ctx context.Context, agentID uint) (*model.DaoAgent, errors.Error) {
	d.store.muAgent.RLock()
	var found string
	for k, a := range d.store.agents {
		if a.ID == agentID {
			found = k
			break
		}
	}
	d.store.muAgent.RUnlock()
	if found == "" {
		return nil, errors.ErrorRecordNotFound("dao.agent.take_detail_by_id")
	}
	return d.takeDetail(found, "dao.agent.take_detail_by_id")
}

// takeDetail 取道人并填充服用记录+语言模式缓存
func (d *AgentDao) takeDetail(uuidStr, code string) (*model.DaoAgent, errors.Error) {
	d.store.muAgent.RLock()
	a, ok := d.store.agents[uuidStr]
	d.store.muAgent.RUnlock()
	if !ok {
		return nil, errors.ErrorRecordNotFound(code)
	}
	out := cloneAgent(a)

	d.store.muAp.RLock()
	d.store.muPill.RLock()
	if binds, ok := d.store.agentPill[a.ID]; ok && len(binds) > 0 {
		out.AgentPills = make([]model.AgentPill, 0, len(binds))
		for _, ap := range binds {
			out.AgentPills = append(out.AgentPills, *ap)
		}
		sort.SliceStable(out.AgentPills, func(i, j int) bool {
			if out.AgentPills[i].SortOrder != out.AgentPills[j].SortOrder {
				return out.AgentPills[i].SortOrder < out.AgentPills[j].SortOrder
			}
			return out.AgentPills[i].ID < out.AgentPills[j].ID
		})
		for i := range out.AgentPills {
			for _, p := range d.store.pills {
				if p.ID == out.AgentPills[i].PillID {
					out.AgentPills[i].Pill = *clonePill(p)
					break
				}
			}
		}
	}
	d.store.muPill.RUnlock()
	d.store.muAp.RUnlock()

	d.store.muPattern.RLock()
	if pat, ok := d.store.patterns[a.ID]; ok {
		pc := *pat
		out.LanguagePattern = &pc
	}
	d.store.muPattern.RUnlock()

	return out, nil
}

func (d *AgentDao) FindAgents(ctx context.Context, page int, size int, status string) (int64, []*model.DaoAgent, errors.Error) {
	d.store.muAgent.RLock()
	defer d.store.muAgent.RUnlock()
	all := make([]*model.DaoAgent, 0, len(d.store.agents))
	for _, a := range d.store.agents {
		if status != "" && a.Status != status {
			continue
		}
		all = append(all, cloneAgent(a))
	}
	sort.SliceStable(all, func(i, j int) bool { return all[i].CreatedAt.After(all[j].CreatedAt) })
	total := int64(len(all))
	if total == 0 || size <= 0 || int64((page-1)*size) >= total {
		return total, nil, nil
	}
	start := (page - 1) * size
	end := start + size
	if end > len(all) {
		end = len(all)
	}
	return total, all[start:end], nil
}

func (d *AgentDao) SaveAgent(ctx context.Context, agent *model.DaoAgent) errors.Error {
	d.store.muAgent.Lock()
	defer d.store.muAgent.Unlock()
	d.store.nextAgentID++
	agent.ID = d.store.nextAgentID
	if agent.UUID == (uuid.UUID{}) {
		agent.UUID = uuid.New()
	}
	agent.CreatedAt = time.Now()
	d.store.agents[agent.UUID.String()] = cloneAgent(agent)
	return nil
}

func (d *AgentDao) UpdateAgent(ctx context.Context, agent *model.DaoAgent, updates map[string]any) errors.Error {
	d.store.muAgent.Lock()
	defer d.store.muAgent.Unlock()
	a, ok := d.store.agents[agent.UUID.String()]
	if !ok {
		return errors.ErrorRecordNotFound("dao.agent.update_agent")
	}
	applyUpdates(a, updates)
	*agent = *cloneAgent(a)
	return nil
}

func (d *AgentDao) DeleteAgent(ctx context.Context, agent *model.DaoAgent) errors.Error {
	d.store.muAgent.Lock()
	d.store.muAp.Lock()
	d.store.muPattern.Lock()
	defer d.store.muAgent.Unlock()
	defer d.store.muAp.Unlock()
	defer d.store.muPattern.Unlock()
	delete(d.store.agents, agent.UUID.String())
	delete(d.store.agentPill, agent.ID)
	delete(d.store.patterns, agent.ID)
	return nil
}

func (d *AgentDao) TakeAgentPill(ctx context.Context, agentID uint, pillID uint) (*model.AgentPill, errors.Error) {
	d.store.muAp.RLock()
	defer d.store.muAp.RUnlock()
	binds, ok := d.store.agentPill[agentID]
	if !ok {
		return nil, errors.ErrorRecordNotFound("dao.agent.take_agent_pill")
	}
	ap, ok := binds[pillID]
	if !ok {
		return nil, errors.ErrorRecordNotFound("dao.agent.take_agent_pill")
	}
	apc := *ap
	return &apc, nil
}

func (d *AgentDao) SaveAgentPill(ctx context.Context, ap *model.AgentPill) errors.Error {
	d.store.muAp.Lock()
	defer d.store.muAp.Unlock()
	d.store.nextApID++
	ap.ID = d.store.nextApID
	ap.CreatedAt = time.Now()
	if d.store.agentPill[ap.AgentID] == nil {
		d.store.agentPill[ap.AgentID] = map[uint]*model.AgentPill{}
	}
	apc := *ap
	d.store.agentPill[ap.AgentID][ap.PillID] = &apc
	return nil
}

func (d *AgentDao) UpdateAgentPill(ctx context.Context, ap *model.AgentPill, updates map[string]any) errors.Error {
	d.store.muAp.Lock()
	defer d.store.muAp.Unlock()
	binds, ok := d.store.agentPill[ap.AgentID]
	if !ok {
		return errors.ErrorRecordNotFound("dao.agent.update_agent_pill")
	}
	cur, ok := binds[ap.PillID]
	if !ok {
		return errors.ErrorRecordNotFound("dao.agent.update_agent_pill")
	}
	applyUpdates(cur, updates)
	*ap = *cur
	return nil
}

func (d *AgentDao) DeleteAgentPill(ctx context.Context, agentID uint, pillID uint) (int64, errors.Error) {
	d.store.muAp.Lock()
	defer d.store.muAp.Unlock()
	binds, ok := d.store.agentPill[agentID]
	if !ok {
		return 0, nil
	}
	if _, ok := binds[pillID]; !ok {
		return 0, nil
	}
	delete(binds, pillID)
	if len(binds) == 0 {
		delete(d.store.agentPill, agentID)
	}
	return 1, nil
}

func (d *AgentDao) MaxAgentPillSortOrder(ctx context.Context, agentID uint) (int, errors.Error) {
	d.store.muAp.RLock()
	defer d.store.muAp.RUnlock()
	max := 0
	if binds, ok := d.store.agentPill[agentID]; ok {
		for _, ap := range binds {
			if ap.SortOrder > max {
				max = ap.SortOrder
			}
		}
	}
	return max, nil
}

func (d *AgentDao) FindPillsByAgentID(ctx context.Context, agentID uint) ([]*model.ElixirPill, errors.Error) {
	d.store.muAp.RLock()
	d.store.muPill.RLock()
	defer d.store.muPill.RUnlock()
	defer d.store.muAp.RUnlock()
	binds, ok := d.store.agentPill[agentID]
	if !ok || len(binds) == 0 {
		return nil, nil
	}
	aps := make([]*model.AgentPill, 0, len(binds))
	for _, ap := range binds {
		aps = append(aps, ap)
	}
	sort.SliceStable(aps, func(i, j int) bool {
		if aps[i].SortOrder != aps[j].SortOrder {
			return aps[i].SortOrder < aps[j].SortOrder
		}
		return aps[i].ID < aps[j].ID
	})
	out := make([]*model.ElixirPill, 0, len(aps))
	for _, ap := range aps {
		for _, p := range d.store.pills {
			if p.ID == ap.PillID {
				out = append(out, clonePill(p))
				break
			}
		}
	}
	return out, nil
}

func (d *AgentDao) InvalidateLanguagePattern(ctx context.Context, agentID uint) errors.Error {
	d.store.muPattern.Lock()
	defer d.store.muPattern.Unlock()
	if pat, ok := d.store.patterns[agentID]; ok {
		pat.IsValid = false
	}
	return nil
}

func (d *AgentDao) SaveLanguagePattern(ctx context.Context, pattern *model.LanguagePattern) errors.Error {
	d.store.muPattern.Lock()
	defer d.store.muPattern.Unlock()
	if pattern.ID == 0 {
		d.store.nextPatternID++
		pattern.ID = d.store.nextPatternID
	}
	pattern.UpdatedAt = time.Now()
	pc := *pattern
	d.store.patterns[pattern.AgentID] = &pc
	return nil
}

// ==================== ChatDao ====================

// ChatDao dao.Chat 接口的内存实现
type ChatDao struct {
	store *Store
}

// NewChatDao 构造对话域内存 DAO
func NewChatDao() *ChatDao { return &ChatDao{store: SharedStore()} }

func (d *ChatDao) TakeSessionByUUID(ctx context.Context, uid uuid.UUID) (*model.ChatSession, errors.Error) {
	d.store.muChat.RLock()
	s, ok := d.store.sessions[uid.String()]
	d.store.muChat.RUnlock()
	if !ok {
		return nil, errors.ErrorRecordNotFound("dao.chat.take_session_by_uuid")
	}
	out := cloneSession(s)
	// 预加载道人(按 AgentID 反查)
	d.store.muAgent.RLock()
	for _, a := range d.store.agents {
		if s.AgentID != nil && a.ID == *s.AgentID {
			out.Agent = *cloneAgent(a)
			break
		}
	}
	d.store.muAgent.RUnlock()
	return out, nil
}

func (d *ChatDao) FindSessions(ctx context.Context, agentID uint, page int, size int) (int64, []*model.ChatSession, errors.Error) {
	d.store.muChat.RLock()
	defer d.store.muChat.RUnlock()
	all := make([]*model.ChatSession, 0, len(d.store.sessions))
	for _, s := range d.store.sessions {
		if agentID > 0 && (s.AgentID == nil || *s.AgentID != agentID) {
			continue
		}
		all = append(all, cloneSession(s))
	}
	sort.SliceStable(all, func(i, j int) bool { return all[i].UpdatedAt.After(all[j].UpdatedAt) })
	total := int64(len(all))
	if total == 0 || size <= 0 || int64((page-1)*size) >= total {
		return total, nil, nil
	}
	start := (page - 1) * size
	end := start + size
	if end > len(all) {
		end = len(all)
	}
	return total, all[start:end], nil
}

func (d *ChatDao) SaveSession(ctx context.Context, session *model.ChatSession) errors.Error {
	d.store.muChat.Lock()
	defer d.store.muChat.Unlock()
	d.store.nextSessionID++
	session.ID = d.store.nextSessionID
	if session.UUID == (uuid.UUID{}) {
		session.UUID = uuid.New()
	}
	now := time.Now()
	session.CreatedAt = now
	session.UpdatedAt = now
	d.store.sessions[session.UUID.String()] = cloneSession(session)
	return nil
}

func (d *ChatDao) UpdateSession(ctx context.Context, session *model.ChatSession, updates map[string]any) errors.Error {
	d.store.muChat.Lock()
	defer d.store.muChat.Unlock()
	s, ok := d.store.sessions[session.UUID.String()]
	if !ok {
		return errors.ErrorRecordNotFound("dao.chat.update_session")
	}
	applyUpdates(s, updates)
	s.UpdatedAt = time.Now()
	*session = *cloneSession(s)
	return nil
}

func (d *ChatDao) DeleteSession(ctx context.Context, session *model.ChatSession) errors.Error {
	d.store.muChat.Lock()
	delete(d.store.sessions, session.UUID.String())
	d.store.muChat.Unlock()

	d.store.muChat.Lock()
	delete(d.store.messages, session.ID)
	d.store.muChat.Unlock()
	return nil
}

func (d *ChatDao) FindMessages(ctx context.Context, sessionID uint, page int, size int) (int64, []*model.ChatMessage, errors.Error) {
	d.store.muChat.RLock()
	msgs := d.store.messages[sessionID]
	total := int64(len(msgs))
	if total == 0 || size <= 0 || int64((page-1)*size) >= total {
		return total, nil, nil
	}
	start := (page - 1) * size
	end := start + size
	if end > len(msgs) {
		end = len(msgs)
	}
	out := make([]*model.ChatMessage, 0, end-start)
	for i := start; i < end; i++ {
		mc := *msgs[i]
		out = append(out, &mc)
	}
	d.store.muChat.RUnlock() // 先释放,避免嵌套持锁

	// 预加载 Agent(两段式):AgentID 为 nil(用户/系统消息)跳过
	d.store.muAgent.RLock()
	for _, m := range out {
		if m.AgentID == nil {
			continue
		}
		for _, a := range d.store.agents {
			if a.ID == *m.AgentID {
				agent := *cloneAgent(a)
				m.Agent = &agent
				break
			}
		}
	}
	d.store.muAgent.RUnlock()

	return total, out, nil
}

func (d *ChatDao) SaveMessage(ctx context.Context, message *model.ChatMessage) errors.Error {
	d.store.muChat.Lock()
	defer d.store.muChat.Unlock()
	d.store.nextMessageID++
	message.ID = d.store.nextMessageID
	if message.UUID == (uuid.UUID{}) {
		message.UUID = uuid.New()
	}
	message.CreatedAt = time.Now()
	mc := *message
	d.store.messages[message.SessionID] = append(d.store.messages[message.SessionID], &mc)
	for _, s := range d.store.sessions {
		if s.ID == message.SessionID {
			s.UpdatedAt = time.Now()
			break
		}
	}
	return nil
}

// SaveMembers 批量写入群成员(调用方保证不重复;sort_order 由调用方赋值)
func (d *ChatDao) SaveMembers(ctx context.Context, members []*model.SessionMember) errors.Error {
	d.store.muChat.Lock()
	defer d.store.muChat.Unlock()
	for _, m := range members {
		d.store.nextMemberID++
		cp := *m
		cp.ID = d.store.nextMemberID
		cp.JoinedAt = time.Now()
		d.store.members[cp.SessionID] = append(d.store.members[cp.SessionID], &cp)
	}
	return nil
}

// FindMembers 按发言顺序(SortOrder ASC)查询群成员,预加载 Agent
func (d *ChatDao) FindMembers(ctx context.Context, sessionID uint) ([]*model.SessionMember, errors.Error) {
	d.store.muChat.RLock()
	src := d.store.members[sessionID]
	out := make([]*model.SessionMember, 0, len(src))
	for _, m := range src {
		cp := *m
		out = append(out, &cp)
	}
	d.store.muChat.RUnlock()

	// 预加载 Agent(两段式,避免嵌套持锁;照抄 TakeSessionByUUID 模式)
	d.store.muAgent.RLock()
	for _, m := range out {
		for _, a := range d.store.agents {
			if a.ID == m.AgentID {
				agent := *cloneAgent(a)
				m.Agent = agent
				break
			}
		}
	}
	d.store.muAgent.RUnlock()

	sort.SliceStable(out, func(i, j int) bool { return out[i].SortOrder < out[j].SortOrder })
	return out, nil
}

// DeleteMember 移出群成员;不存在返回 ErrorTypeRecordNotFound
func (d *ChatDao) DeleteMember(ctx context.Context, sessionID uint, agentID uint) errors.Error {
	d.store.muChat.Lock()
	defer d.store.muChat.Unlock()
	src := d.store.members[sessionID]
	for i, m := range src {
		if m.AgentID == agentID {
			d.store.members[sessionID] = append(src[:i], src[i+1:]...)
			return nil
		}
	}
	return errors.ErrorRecordNotFound("dao.chat.delete_member")
}

// ==================== ProviderDao ====================

// ProviderDao dao.Provider 接口的内存实现
type ProviderDao struct {
	store *Store
}

// NewProviderDao 构造供应商内存 DAO
func NewProviderDao() *ProviderDao { return &ProviderDao{store: SharedStore()} }

func (d *ProviderDao) TakeProviderByUUID(ctx context.Context, uid uuid.UUID) (*model.LLMProvider, errors.Error) {
	d.store.muProvider.RLock()
	defer d.store.muProvider.RUnlock()
	p, ok := d.store.providers[uid.String()]
	if !ok {
		return nil, errors.ErrorRecordNotFound("dao.provider.take_by_uuid")
	}
	return cloneProvider(p), nil
}

func (d *ProviderDao) TakeProviderByID(ctx context.Context, id uint) (*model.LLMProvider, errors.Error) {
	d.store.muProvider.RLock()
	defer d.store.muProvider.RUnlock()
	for _, p := range d.store.providers {
		if p.ID == id {
			return cloneProvider(p), nil
		}
	}
	return nil, errors.ErrorRecordNotFound("dao.provider.take_by_id")
}

func (d *ProviderDao) FindProviders(ctx context.Context, page, size int, enabled *bool) (int64, []*model.LLMProvider, errors.Error) {
	d.store.muProvider.RLock()
	defer d.store.muProvider.RUnlock()
	all := make([]*model.LLMProvider, 0, len(d.store.providers))
	for _, p := range d.store.providers {
		if enabled != nil && p.IsEnabled != *enabled {
			continue
		}
		all = append(all, cloneProvider(p))
	}
	sort.SliceStable(all, func(i, j int) bool {
		if all[i].SortOrder != all[j].SortOrder {
			return all[i].SortOrder < all[j].SortOrder
		}
		return all[i].ID < all[j].ID
	})
	total := int64(len(all))
	if total == 0 || size <= 0 || int64((page-1)*size) >= total {
		return total, nil, nil
	}
	start := (page - 1) * size
	end := start + size
	if end > len(all) {
		end = len(all)
	}
	return total, all[start:end], nil
}

func (d *ProviderDao) SaveProvider(ctx context.Context, provider *model.LLMProvider) errors.Error {
	d.store.muProvider.Lock()
	defer d.store.muProvider.Unlock()
	d.store.nextProviderID++
	provider.ID = d.store.nextProviderID
	if provider.UUID == (uuid.UUID{}) {
		provider.UUID = uuid.New()
	}
	now := time.Now()
	provider.CreatedAt = now
	provider.UpdatedAt = now
	d.store.providers[provider.UUID.String()] = cloneProvider(provider)
	return nil
}

func (d *ProviderDao) UpdateProvider(ctx context.Context, provider *model.LLMProvider, updates map[string]any) errors.Error {
	d.store.muProvider.Lock()
	defer d.store.muProvider.Unlock()
	p, ok := d.store.providers[provider.UUID.String()]
	if !ok {
		return errors.ErrorRecordNotFound("dao.provider.update")
	}
	applyUpdates(p, updates)
	p.UpdatedAt = time.Now()
	*provider = *cloneProvider(p)
	return nil
}

func (d *ProviderDao) DeleteProvider(ctx context.Context, provider *model.LLMProvider) errors.Error {
	d.store.muProvider.Lock()
	defer d.store.muProvider.Unlock()
	delete(d.store.providers, provider.UUID.String())
	return nil
}

func (d *ProviderDao) CountModelsByProvider(ctx context.Context, providerID uint) (int64, errors.Error) {
	d.store.muModel.RLock()
	defer d.store.muModel.RUnlock()
	var n int64
	for _, m := range d.store.models {
		if m.ProviderID == providerID {
			n++
		}
	}
	return n, nil
}

func (d *ProviderDao) CountProvidersByName(ctx context.Context, name string, excludeID uint) (int64, errors.Error) {
	d.store.muProvider.RLock()
	defer d.store.muProvider.RUnlock()
	var n int64
	for _, p := range d.store.providers {
		if p.Name == name && (excludeID == 0 || p.ID != excludeID) {
			n++
		}
	}
	return n, nil
}

// ==================== ModelDao ====================

// ModelDao dao.Model 接口的内存实现
type ModelDao struct {
	store *Store
}

// NewModelDao 构造模型内存 DAO
func NewModelDao() *ModelDao { return &ModelDao{store: SharedStore()} }

func (d *ModelDao) CountEnabledModelByName(ctx context.Context, name string) (int64, errors.Error) {
	d.store.muModel.RLock()
	d.store.muProvider.RLock()
	defer d.store.muProvider.RUnlock()
	defer d.store.muModel.RUnlock()
	var n int64
	for _, m := range d.store.models {
		if m.Name != name || !m.IsEnabled {
			continue
		}
		for _, p := range d.store.providers {
			if p.ID == m.ProviderID && p.IsEnabled {
				n++
				break
			}
		}
	}
	return n, nil
}

func (d *ModelDao) TakeModelByUUID(ctx context.Context, uid uuid.UUID) (*model.LLMModel, errors.Error) {
	d.store.muModel.RLock()
	d.store.muProvider.RLock()
	defer d.store.muProvider.RUnlock()
	defer d.store.muModel.RUnlock()
	m, ok := d.store.models[uid.String()]
	if !ok {
		return nil, errors.ErrorRecordNotFound("dao.model.take_by_uuid")
	}
	out := cloneModel(m)
	for _, p := range d.store.providers {
		if p.ID == m.ProviderID {
			out.Provider = *cloneProvider(p)
			break
		}
	}
	return out, nil
}

func (d *ModelDao) TakeModelByID(ctx context.Context, id uint) (*model.LLMModel, errors.Error) {
	d.store.muModel.RLock()
	d.store.muProvider.RLock()
	defer d.store.muProvider.RUnlock()
	defer d.store.muModel.RUnlock()
	for _, m := range d.store.models {
		if m.ID == id {
			out := cloneModel(m)
			for _, p := range d.store.providers {
				if p.ID == m.ProviderID {
					out.Provider = *cloneProvider(p)
					break
				}
			}
			return out, nil
		}
	}
	return nil, errors.ErrorRecordNotFound("dao.model.take_by_id")
}

func (d *ModelDao) FindModelsByProvider(ctx context.Context, providerID uint, page, size int) (int64, []*model.LLMModel, errors.Error) {
	d.store.muModel.RLock()
	defer d.store.muModel.RUnlock()
	all := make([]*model.LLMModel, 0)
	for _, m := range d.store.models {
		if m.ProviderID == providerID {
			all = append(all, cloneModel(m))
		}
	}
	sort.SliceStable(all, func(i, j int) bool {
		if all[i].SortOrder != all[j].SortOrder {
			return all[i].SortOrder < all[j].SortOrder
		}
		return all[i].ID < all[j].ID
	})
	total := int64(len(all))
	if total == 0 || size <= 0 || int64((page-1)*size) >= total {
		return total, nil, nil
	}
	start := (page - 1) * size
	end := start + size
	if end > len(all) {
		end = len(all)
	}
	return total, all[start:end], nil
}

func (d *ModelDao) CountModelsByNameInProvider(ctx context.Context, providerID uint, name string, excludeID uint) (int64, errors.Error) {
	d.store.muModel.RLock()
	defer d.store.muModel.RUnlock()
	var n int64
	for _, m := range d.store.models {
		if m.ProviderID == providerID && m.Name == name && (excludeID == 0 || m.ID != excludeID) {
			n++
		}
	}
	return n, nil
}

func (d *ModelDao) ModelNameExistsInProvider(ctx context.Context, providerID uint, name string, excludeID uint) (bool, errors.Error) {
	n, err := d.CountModelsByNameInProvider(ctx, providerID, name, excludeID)
	return n > 0, err
}

func (d *ModelDao) SaveModel(ctx context.Context, m *model.LLMModel) errors.Error {
	d.store.muModel.Lock()
	defer d.store.muModel.Unlock()
	if m.IsDefault {
		for _, other := range d.store.models {
			if other.ID != m.ID {
				other.IsDefault = false
			}
		}
	}
	if m.IsSynthesis {
		for _, other := range d.store.models {
			if other.ID != m.ID {
				other.IsSynthesis = false
			}
		}
	}
	if m.IsFusion {
		for _, other := range d.store.models {
			if other.ID != m.ID {
				other.IsFusion = false
			}
		}
	}
	d.store.nextModelID++
	m.ID = d.store.nextModelID
	if m.UUID == (uuid.UUID{}) {
		m.UUID = uuid.New()
	}
	now := time.Now()
	m.CreatedAt = now
	m.UpdatedAt = now
	d.store.models[m.UUID.String()] = cloneModel(m)
	return nil
}

func (d *ModelDao) UpdateModel(ctx context.Context, m *model.LLMModel, updates map[string]any) errors.Error {
	d.store.muModel.Lock()
	defer d.store.muModel.Unlock()
	cur, ok := d.store.models[m.UUID.String()]
	if !ok {
		return errors.ErrorRecordNotFound("dao.model.update")
	}
	if v, ok := updates["is_default"]; ok {
		if b, _ := v.(bool); b {
			for _, other := range d.store.models {
				if other.ID != cur.ID {
					other.IsDefault = false
				}
			}
		}
	}
	if v, ok := updates["is_synthesis"]; ok {
		if b, _ := v.(bool); b {
			for _, other := range d.store.models {
				if other.ID != cur.ID {
					other.IsSynthesis = false
				}
			}
		}
	}
	if v, ok := updates["is_fusion"]; ok {
		if b, _ := v.(bool); b {
			for _, other := range d.store.models {
				if other.ID != cur.ID {
					other.IsFusion = false
				}
			}
		}
	}
	applyUpdates(cur, updates)
	cur.UpdatedAt = time.Now()
	*m = *cloneModel(cur)
	return nil
}

func (d *ModelDao) DeleteModel(ctx context.Context, m *model.LLMModel) errors.Error {
	d.store.muModel.Lock()
	defer d.store.muModel.Unlock()
	delete(d.store.models, m.UUID.String())
	return nil
}

func (d *ModelDao) CountAgentReferencesByName(ctx context.Context, name string) (int64, errors.Error) {
	d.store.muAgent.RLock()
	defer d.store.muAgent.RUnlock()
	var n int64
	for _, a := range d.store.agents {
		if a.ModelName == name {
			n++
		}
	}
	return n, nil
}

func (d *ModelDao) CountAgentReferencesByNames(ctx context.Context, names []string) (map[string]int64, errors.Error) {
	d.store.muAgent.RLock()
	defer d.store.muAgent.RUnlock()
	out := make(map[string]int64, len(names))
	for _, n := range names {
		out[n] = 0
	}
	for _, a := range d.store.agents {
		if _, ok := out[a.ModelName]; ok {
			out[a.ModelName]++
		}
	}
	return out, nil
}

func (d *ModelDao) FindModelsByName(ctx context.Context, name string) ([]*model.LLMModel, errors.Error) {
	d.store.muModel.RLock()
	d.store.muProvider.RLock()
	defer d.store.muProvider.RUnlock()
	defer d.store.muModel.RUnlock()
	all := make([]*model.LLMModel, 0)
	for _, m := range d.store.models {
		if m.Name == name {
			mc := cloneModel(m)
			for _, p := range d.store.providers {
				if p.ID == m.ProviderID {
					mc.Provider = *cloneProvider(p)
					break
				}
			}
			all = append(all, mc)
		}
	}
	sort.SliceStable(all, func(i, j int) bool {
		if all[i].SortOrder != all[j].SortOrder {
			return all[i].SortOrder < all[j].SortOrder
		}
		return all[i].ID < all[j].ID
	})
	return all, nil
}

func (d *ModelDao) TakeDefaultEnabled(ctx context.Context) (*model.LLMModel, errors.Error) {
	d.store.muModel.RLock()
	d.store.muProvider.RLock()
	defer d.store.muProvider.RUnlock()
	defer d.store.muModel.RUnlock()
	for _, m := range d.store.models {
		if m.IsDefault && m.IsEnabled {
			for _, p := range d.store.providers {
				if p.ID == m.ProviderID && p.IsEnabled {
					out := cloneModel(m)
					out.Provider = *cloneProvider(p)
					return out, nil
				}
			}
		}
	}
	return nil, errors.ErrorRecordNotFound("dao.model.take_default_enabled")
}

func (d *ModelDao) TakeSynthesisEnabled(ctx context.Context) (*model.LLMModel, errors.Error) {
	d.store.muModel.RLock()
	d.store.muProvider.RLock()
	defer d.store.muProvider.RUnlock()
	defer d.store.muModel.RUnlock()
	for _, m := range d.store.models {
		if m.IsSynthesis && m.IsEnabled {
			for _, p := range d.store.providers {
				if p.ID == m.ProviderID && p.IsEnabled {
					out := cloneModel(m)
					out.Provider = *cloneProvider(p)
					return out, nil
				}
			}
		}
	}
	return nil, errors.ErrorRecordNotFound("dao.model.take_synthesis_enabled")
}

func (d *ModelDao) TakeFusionEnabled(ctx context.Context) (*model.LLMModel, errors.Error) {
	d.store.muModel.RLock()
	d.store.muProvider.RLock()
	defer d.store.muProvider.RUnlock()
	defer d.store.muModel.RUnlock()
	for _, m := range d.store.models {
		if m.IsFusion && m.IsEnabled {
			for _, p := range d.store.providers {
				if p.ID == m.ProviderID && p.IsEnabled {
					out := cloneModel(m)
					out.Provider = *cloneProvider(p)
					return out, nil
				}
			}
		}
	}
	return nil, errors.ErrorRecordNotFound("dao.model.take_fusion_enabled")
}

func (d *ModelDao) FindEnabledOptions(ctx context.Context) ([]model.LLMModelOption, errors.Error) {
	d.store.muModel.RLock()
	d.store.muProvider.RLock()
	defer d.store.muProvider.RUnlock()
	defer d.store.muModel.RUnlock()
	out := make([]model.LLMModelOption, 0)
	for _, m := range d.store.models {
		if !m.IsEnabled {
			continue
		}
		for _, p := range d.store.providers {
			if p.ID == m.ProviderID && p.IsEnabled {
				out = append(out, model.LLMModelOption{
					Name:                m.Name,
					DisplayName:         m.DisplayName,
					ProviderName:        p.Name,
					ProviderDisplayName: p.DisplayName,
					IsDefault:           m.IsDefault,
				})
				break
			}
		}
	}
	return out, nil
}

func (d *ModelDao) FindFirstEnabledModelByProvider(ctx context.Context, providerID uint) (*model.LLMModel, errors.Error) {
	d.store.muModel.RLock()
	d.store.muProvider.RLock()
	defer d.store.muProvider.RUnlock()
	defer d.store.muModel.RUnlock()
	var picked *model.LLMModel
	for _, m := range d.store.models {
		if m.ProviderID == providerID && m.IsEnabled {
			if picked == nil || m.SortOrder < picked.SortOrder || (m.SortOrder == picked.SortOrder && m.ID < picked.ID) {
				picked = m
			}
		}
	}
	if picked == nil {
		return nil, errors.ErrorRecordNotFound("dao.model.find_first_enabled_by_provider")
	}
	out := cloneModel(picked)
	for _, p := range d.store.providers {
		if p.ID == picked.ProviderID {
			out.Provider = *cloneProvider(p)
			break
		}
	}
	return out, nil
}

// ==================== helpers ====================

func clonePill(p *model.ElixirPill) *model.ElixirPill {
	if p == nil {
		return nil
	}
	c := *p
	return &c
}

func cloneAgent(a *model.DaoAgent) *model.DaoAgent {
	if a == nil {
		return nil
	}
	c := *a
	return &c
}

func cloneSession(s *model.ChatSession) *model.ChatSession {
	if s == nil {
		return nil
	}
	c := *s
	return &c
}

func cloneProvider(p *model.LLMProvider) *model.LLMProvider {
	if p == nil {
		return nil
	}
	c := *p
	return &c
}

func cloneModel(m *model.LLMModel) *model.LLMModel {
	if m == nil {
		return nil
	}
	c := *m
	return &c
}

// applyUpdates 把 map[string]any 写到结构体顶层字段(复刻 GORM Updates 的最小行为)
// key 既接受 PascalCase 也接受 snake_case;类型可转换则赋值
func applyUpdates(v any, updates map[string]any) {
	rv := reflect.ValueOf(v)
	if rv.Kind() == reflect.Ptr {
		rv = rv.Elem()
	}
	if rv.Kind() != reflect.Struct {
		return
	}
	for k, val := range updates {
		f := rv.FieldByName(k)
		if !f.IsValid() || !f.CanSet() {
			f = rv.FieldByName(snakeToPascal(k))
		}
		if !f.IsValid() || !f.CanSet() {
			continue
		}
		setField(f, val)
	}
}

// snakeToPascal "is_enabled" -> "IsEnabled";已是 PascalCase 则原样返回
func snakeToPascal(s string) string {
	if !strings.Contains(s, "_") {
		// 已是单词,首字母大写
		if len(s) > 0 {
			return strings.ToUpper(s[:1]) + s[1:]
		}
		return s
	}
	parts := strings.Split(s, "_")
	for i, p := range parts {
		if len(p) > 0 {
			parts[i] = strings.ToUpper(p[:1]) + p[1:]
		}
	}
	return strings.Join(parts, "")
}

// setField 把任意值赋到 reflect.Value(支持基础类型可转换赋值)
func setField(f reflect.Value, val any) {
	rv := reflect.ValueOf(val)
	if !rv.IsValid() {
		return
	}
	if rv.Type().ConvertibleTo(f.Type()) {
		f.Set(rv.Convert(f.Type()))
	}
}
