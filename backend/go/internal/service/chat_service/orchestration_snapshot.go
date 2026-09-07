package chat_service

// BuildOrchestrationRequest 组装一次编排用户轮的完整执行快照(LangGraph 权威执行体,Task 11)。
//
// 职责边界:快照携带成员序、语言模式服务合成的 system prompt、模型引用、
// 按请求凭据与记忆原文;Python 图消费该快照,不重复拼装人设与金丹效果。
// 凭据键约定(对齐 Python supervisor.py):道人=agent_id(UUID 字符串),
// Supervisor=默认模型名,两键并存于同一 map。
//
// 安全:返回值含运行期凭据;本函数与调用方不得 log/format 返回值本体,
// 调试/日志场景一律经 Request.StateProjection()(剔除凭据的投影)。

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/alchemy-furnace/server/internal/service/orchestration"
	"github.com/alchemy-furnace/server/model"
	"github.com/google/uuid"
)

// mentionsFromSnapshot 从用户消息 Mentions JSONMap 读取本轮被 @ 的道人 UUID 列表。
// agents 值运行期为 []string,经 DB 往返后为 []any:统一 re-marshal 后解码兼容两种形态;
// 解析失败按无 @ 处理(不阻断快照)。
func mentionsFromSnapshot(m model.JSONMap) []string {
	if m == nil {
		return nil
	}
	raw, ok := m["agents"]
	if !ok || raw == nil {
		return nil
	}
	encoded, err := json.Marshal(raw)
	if err != nil {
		return nil
	}
	var out []string
	if json.Unmarshal(encoded, &out) != nil {
		return nil
	}
	return out
}

// BuildOrchestrationRequest 组装一次编排用户轮的完整执行快照。
// 参与者校验/模型停用等失败即整体失败:返回值不可作为可执行请求使用。
func (s *Chat) BuildOrchestrationRequest(ctx context.Context, session *model.ChatSession, userMessage *model.ChatMessage, run *model.ChatRun, modelOverride string) (orchestration.Request, error) {
	req := orchestration.Request{
		RunID:       run.ChatRunID,
		SessionID:   session.ChatSessionID,
		SessionType: session.Type,
		UserTurn: orchestration.UserTurn{
			MessageID: userMessage.ChatMessageID,
			Text:      userMessage.Content,
			Mentions:  mentionsFromSnapshot(userMessage.Mentions),
		},
		// 空快照必须初始化为空 slice/map:Python 契约对必填 list 拒收 null,
		// nil 切片会序列化为 null 导致全新会话首条消息 422。
		History:      []orchestration.Message{},
		Agents:       []orchestration.Agent{},
		Memories:     []orchestration.Memory{},
		Credentials:  map[string]orchestration.Credential{},
		DebugEnabled: promptDebugEnabled(ctx),
	}

	// 参与者:群聊按 FindMembers 返回序(建群序);单聊=会话归属道人(TakeSessionByUUID 预加载)。
	var participants []model.DaoAgent
	if session.Type == model.SessionTypeGroup {
		members, mErr := s.chat.FindMembers(ctx, session.ChatSessionID)
		if mErr != nil {
			return req, fmt.Errorf("编排快照查询成员失败: %w", mErr)
		}
		for _, m := range members {
			participants = append(participants, m.Agent)
		}
	} else {
		participants = append(participants, session.Agent)
	}

	for _, agent := range participants {
		agentUID, perr := uuid.Parse(agent.DaoAgentID)
		if perr != nil {
			return req, fmt.Errorf("编排快照道人标识无效: %w", perr)
		}
		got, creds, verr := s.validateChatAgentAccess(ctx, agentUID, modelOverride)
		if verr != nil {
			return req, verr
		}
		pattern, patternErr := s.pattern.GetOrBuildPattern(ctx, got.DaoAgentID)
		if patternErr != nil {
			return req, fmt.Errorf("编排快照构建道人语言模式失败: %w", patternErr)
		}
		systemPrompt := ""
		if pattern != nil {
			systemPrompt = pattern.SystemPrompt
		}
		agentID := got.DaoAgentID
		req.Agents = append(req.Agents, orchestration.Agent{
			AgentID:      agentID,
			Name:         got.Name,
			SystemPrompt: systemPrompt,
			ModelRef: orchestration.ModelRef{
				ProviderType: creds.ProviderType,
				Name:         creds.Model,
			},
		})
		req.Credentials[agentID] = orchestration.Credential{APIKey: creds.APIKey, BaseURL: creds.BaseURL}
		if got.MemoryEnabled {
			for i, snip := range s.RetrieveMemories(ctx, got.DaoAgentID, userMessage.Content) {
				req.Memories = append(req.Memories, orchestration.Memory{
					MemoryID: fmt.Sprintf("%s#%d", agentID, i+1),
					AgentID:  agentID,
					Text:     snip.Content,
				})
			}
		}
	}

	// 历史:最近 20 条;剔除 system 通知与本轮用户消息(重试场景下本轮已在库)。
	_, msgs, hErr := s.chat.FindMessages(ctx, session.ChatSessionID, 1, 20)
	if hErr != nil {
		return req, fmt.Errorf("编排快照查询历史失败: %w", hErr)
	}
	for _, m := range msgs {
		if m.Role == "system" || m.ChatMessageID == userMessage.ChatMessageID {
			continue
		}
		entry := orchestration.Message{
			MessageID: m.ChatMessageID,
			Role:      m.Role,
			Text:      m.Content,
		}
		if m.AgentID != nil && m.Agent != nil {
			agentUUID := m.Agent.DaoAgentID
			entry.AgentID = &agentUUID
		}
		req.History = append(req.History, entry)
	}

	// Supervisor=当前默认模型(非人格,设计 §Supervisor)。解析失败不阻断本轮:
	// DefaultModelRef 缺失时 Python 图走确定性回退(主成员发言)。
	if s.creds != nil {
		if def, defErr := s.creds.ResolveCredentials(ctx, ""); defErr == nil && def != nil && def.Model != "" {
			ref := orchestration.ModelRef{ProviderType: def.ProviderType, Name: def.Model}
			req.DefaultModelRef = &ref
			req.Credentials[def.Model] = orchestration.Credential{APIKey: def.APIKey, BaseURL: def.BaseURL}
		}
	}
	return req, nil
}
