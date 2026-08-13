// Package agent 道人管理 HTTP 处理器(新网关;对齐 Luna-CY 模板 handler 分包风格)
// 路由: /api/v1/agents;路径参数 :uuid / :pill_uuid 为对外唯一标识
package agent

import (
	"time"

	"github.com/alchemy-furnace/server/internal/errors"
	"github.com/alchemy-furnace/server/internal/interface/service"
	"github.com/alchemy-furnace/server/model"
	"github.com/alchemy-furnace/server/server/http/gateway/web/handler/pill"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// Agent 道人处理器
type Agent struct {
	agent service.Agent
}

// New 构造道人处理器
func New(agent service.Agent) *Agent {
	return &Agent{agent: agent}
}

// ---------- 响应 DTO ----------

// Response 道人响应 DTO:id 输出 UUID 字符串
type Response struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Avatar      string    `json:"avatar"`
	Personality string    `json:"personality"`
	ModelName   string    `json:"model_name"`
	Status      string    `json:"status"`
	Proactivity int       `json:"proactivity"`
	CreatedAt   time.Time `json:"created_at"`
}

// toResponse 内部模型 → 对外 DTO
func toResponse(a *model.DaoAgent) *Response {
	return &Response{
		ID:          a.UUID.String(),
		Name:        a.Name,
		Avatar:      a.Avatar,
		Personality: a.Personality,
		ModelName:   a.ModelName,
		Status:      a.Status,
		Proactivity: a.Proactivity,
		CreatedAt:   a.CreatedAt,
	}
}

// toResponseList 批量转换
func toResponseList(agents []*model.DaoAgent) []*Response {
	list := make([]*Response, 0, len(agents))
	for _, a := range agents {
		list = append(list, toResponse(a))
	}
	return list
}

// AgentPillResponse 服用记录响应 DTO:agent_pills 无 uuid,pill_id 输出金丹 UUID
type AgentPillResponse struct {
	PillID    string         `json:"pill_id"`
	Weight    float64        `json:"weight"`
	SortOrder int            `json:"sort_order"`
	CreatedAt time.Time      `json:"created_at"`
	Pill      *pill.Response `json:"pill,omitempty"`
}

// LanguagePatternResponse 语言模式缓存响应 DTO(纯内部缓存,无 id 输出)
type LanguagePatternResponse struct {
	SystemPrompt   string         `json:"system_prompt"`
	EmergenceRules model.JSONList `json:"emergence_rules"`
	InnerTensions  model.JSONList `json:"inner_tensions"`
	IsValid        bool           `json:"is_valid"`
}

// DetailResponse 道人详情 DTO:道人 + 服用记录 + 语言模式缓存
type DetailResponse struct {
	*Response
	AgentPills      []*AgentPillResponse     `json:"agent_pills,omitempty"`
	LanguagePattern *LanguagePatternResponse `json:"language_pattern,omitempty"`
}

// toDetailResponse 道人详情模型 → 对外 DTO
func toDetailResponse(a *model.DaoAgent) *DetailResponse {
	detail := &DetailResponse{Response: toResponse(a)}

	if a.AgentPills != nil {
		detail.AgentPills = make([]*AgentPillResponse, 0, len(a.AgentPills))
		for _, ap := range a.AgentPills {
			item := &AgentPillResponse{
				PillID:    ap.Pill.UUID.String(),
				Weight:    ap.Weight,
				SortOrder: ap.SortOrder,
				CreatedAt: ap.CreatedAt,
			}
			if ap.Pill.ID != 0 {
				item.Pill = pill.ToResponse(&ap.Pill)
			}
			detail.AgentPills = append(detail.AgentPills, item)
		}
	}

	if a.LanguagePattern != nil {
		detail.LanguagePattern = &LanguagePatternResponse{
			SystemPrompt:   a.LanguagePattern.SystemPrompt,
			EmergenceRules: a.LanguagePattern.EmergenceRules,
			InnerTensions:  a.LanguagePattern.InnerTensions,
			IsValid:        a.LanguagePattern.IsValid,
		}
	}

	return detail
}

// ---------- 路径参数解析 ----------

// parseUUID 解析 :uuid 路径参数(道人);非法形态返回 400
func parseUUID(c *gin.Context) (uuid.UUID, errors.Error) {
	uid, err := uuid.Parse(c.Param("uuid"))
	if err != nil {
		return uuid.Nil, errors.New(errors.ErrorTypeInvalidRequest, "handler.agent.uuid_parse", "道人ID格式不正确")
	}
	return uid, nil
}

// parsePillUUID 解析 :pill_uuid 路径参数(金丹);非法形态返回 400
func parsePillUUID(c *gin.Context) (uuid.UUID, errors.Error) {
	uid, err := uuid.Parse(c.Param("pill_uuid"))
	if err != nil {
		return uuid.Nil, errors.New(errors.ErrorTypeInvalidRequest, "handler.agent.pill_uuid_parse", "金丹ID格式不正确")
	}
	return uid, nil
}
