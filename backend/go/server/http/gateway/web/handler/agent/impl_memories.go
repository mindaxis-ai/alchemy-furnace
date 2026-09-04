package agent

import (
	"time"

	"github.com/alchemy-furnace/server/internal/context/contextutil"
	"github.com/alchemy-furnace/server/internal/errors"
	"github.com/alchemy-furnace/server/internal/interface/service"
	"github.com/alchemy-furnace/server/model"
	"github.com/alchemy-furnace/server/server/http/request"
	"github.com/alchemy-furnace/server/server/http/response"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// ---------- 记忆 DTO ----------

// MemoryResponse 记忆响应 DTO:对外输出 UUID
type MemoryResponse struct {
	ID       string   `json:"id"`
	Kind     string   `json:"kind"`
	Content  string   `json:"content"`
	Keywords []string `json:"keywords"`

	Importance int     `json:"importance"`
	Confidence float64 `json:"confidence"`
	Pinned     bool    `json:"pinned"`
	Status     string  `json:"status"`
	// 来源关系为 UUID 文本;无来源(手工录入)时缺省(011 契约:*_id 键要么是 UUID 要么不出现,禁止空串)
	SourceSessionID *string   `json:"source_session_id,omitempty"`
	SourceMessageID *string   `json:"source_message_id,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// toMemoryResponse 内部模型 → 对外 DTO(Keywords JSONList → []string)
func toMemoryResponse(m *model.AgentMemory) *MemoryResponse {
	keywords := make([]string, 0, len(m.Keywords))
	for _, kw := range m.Keywords {
		if s, ok := kw.(string); ok {
			keywords = append(keywords, s)
		}
	}
	return &MemoryResponse{
		ID:              m.UUID.String(),
		Kind:            m.Kind,
		Content:         m.Content,
		Keywords:        keywords,
		Importance:      m.Importance,
		Confidence:      m.Confidence,
		Pinned:          m.Pinned,
		Status:          m.Status,
		SourceSessionID: uuidTextPtr(m.SourceSessionID),
		SourceMessageID: uuidTextPtr(m.SourceMessageID),
		CreatedAt:       m.CreatedAt,
		UpdatedAt:       m.UpdatedAt,
	}
}

// uuidTextPtr 空串 → nil(响应缺省该 *_id 键);非空原样返回指针
func uuidTextPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// toMemoryResponseList 批量转换
func toMemoryResponseList(list []*model.AgentMemory) []*MemoryResponse {
	out := make([]*MemoryResponse, 0, len(list))
	for _, m := range list {
		out = append(out, toMemoryResponse(m))
	}
	return out
}

// MemoryUpsertRequest 记忆创建/更新请求
// kind/content 创建时必填(service 层校验);PATCH 部分更新时缺省=不更新
type MemoryUpsertRequest struct {
	Kind       string   `json:"kind" binding:"omitempty,oneof=user_fact user_preference relationship open_loop episode"`
	Content    string   `json:"content" binding:"omitempty,max=500"`
	Keywords   []string `json:"keywords"`
	Importance *int     `json:"importance" binding:"omitempty,gte=1,lte=5"`
	Confidence *float64 `json:"confidence" binding:"omitempty,gte=0,lte=1"`
	Pinned     *bool    `json:"pinned"`
}

// toMemoryInput 请求 → service 层输入
func (b *MemoryUpsertRequest) toMemoryInput() service.MemoryInput {
	return service.MemoryInput{
		Kind:       b.Kind,
		Content:    b.Content,
		Keywords:   b.Keywords,
		Importance: b.Importance,
		Confidence: b.Confidence,
		Pinned:     b.Pinned,
	}
}

// ---------- 路径参数 ----------

// parseMemoryUUID 解析 :memory_uuid 路径参数(记忆);非法形态返回 400
func parseMemoryUUID(c *gin.Context) (uuid.UUID, errors.Error) {
	uid, err := uuid.Parse(c.Param("memory_uuid"))
	if err != nil {
		return uuid.Nil, errors.New(errors.ErrorTypeInvalidRequest, "handler.agent.memory_uuid_parse", "记忆ID格式不正确")
	}
	return uid, nil
}

// takeMemoryAgent 解析 :uuid 并取出道人(不存在 → 404,记忆接口按内部 ID 操作)
func (cls *Agent) takeMemoryAgent(c *gin.Context, uid uuid.UUID) (*model.DaoAgent, errors.Error) {
	agent, serr := cls.agent.GetAgentDetailByUUID(contextutil.NewContextWithGin(c), uid)
	if serr != nil {
		return nil, serr
	}
	return agent, nil
}

// ---------- 端点 ----------

// ListMemories 列出道人记忆
// GET /api/v1/agents/:uuid/memories?kind=&active=
func (cls *Agent) ListMemories(c *gin.Context) (response.Code, any, error) {
	uid, err := parseUUID(c)
	if err != nil {
		return response.InvalidParams, nil, err
	}
	agent, serr := cls.takeMemoryAgent(c, uid)
	if serr != nil {
		return 0, nil, serr
	}
	kind := c.Query("kind")
	onlyActive := true
	if v, ok := c.GetQuery("active"); ok {
		onlyActive = v != "false"
	}
	list, merr := cls.memory.ListMemories(contextutil.NewContextWithGin(c), agent.UUID.String(), kind, onlyActive)
	if merr != nil {
		return 0, nil, merr
	}
	return response.Ok, toMemoryResponseList(list), nil
}

// CreateMemory 创建道人记忆(校验/哈希去重/冲突置替在 service 层)
// POST /api/v1/agents/:uuid/memories
func (cls *Agent) CreateMemory(c *gin.Context) (response.Code, any, error) {
	uid, err := parseUUID(c)
	if err != nil {
		return response.InvalidParams, nil, err
	}
	agent, serr := cls.takeMemoryAgent(c, uid)
	if serr != nil {
		return 0, nil, serr
	}
	var body MemoryUpsertRequest
	if berr := request.ShouldBindJSON(c, &body); berr != nil {
		return response.InvalidParams, nil, berr
	}
	m, merr := cls.memory.CreateMemory(contextutil.NewContextWithGin(c), agent.UUID.String(), body.toMemoryInput())
	if merr != nil {
		return 0, nil, merr
	}
	return response.CodeCreated, toMemoryResponse(m), nil
}

// UpdateMemory 部分更新道人记忆(nil 字段不更新;不属于该道人 → 400)
// PATCH /api/v1/agents/:uuid/memories/:memory_uuid
func (cls *Agent) UpdateMemory(c *gin.Context) (response.Code, any, error) {
	uid, err := parseUUID(c)
	if err != nil {
		return response.InvalidParams, nil, err
	}
	memUID, err := parseMemoryUUID(c)
	if err != nil {
		return response.InvalidParams, nil, err
	}
	agent, serr := cls.takeMemoryAgent(c, uid)
	if serr != nil {
		return 0, nil, serr
	}
	var body MemoryUpsertRequest
	if berr := request.ShouldBindJSON(c, &body); berr != nil {
		return response.InvalidParams, nil, berr
	}
	m, merr := cls.memory.UpdateMemory(contextutil.NewContextWithGin(c), agent.UUID.String(), memUID, body.toMemoryInput())
	if merr != nil {
		return 0, nil, merr
	}
	return response.Ok, toMemoryResponse(m), nil
}

// DeleteMemory 物理删除单条记忆
// DELETE /api/v1/agents/:uuid/memories/:memory_uuid
func (cls *Agent) DeleteMemory(c *gin.Context) (response.Code, any, error) {
	uid, err := parseUUID(c)
	if err != nil {
		return response.InvalidParams, nil, err
	}
	memUID, err := parseMemoryUUID(c)
	if err != nil {
		return response.InvalidParams, nil, err
	}
	agent, serr := cls.takeMemoryAgent(c, uid)
	if serr != nil {
		return 0, nil, serr
	}
	if merr := cls.memory.DeleteMemory(contextutil.NewContextWithGin(c), agent.UUID.String(), memUID); merr != nil {
		return 0, nil, merr
	}
	return response.Ok, gin.H{"deleted": true}, nil
}

// ClearMemories 物理清空道人全部记忆
// DELETE /api/v1/agents/:uuid/memories
func (cls *Agent) ClearMemories(c *gin.Context) (response.Code, any, error) {
	uid, err := parseUUID(c)
	if err != nil {
		return response.InvalidParams, nil, err
	}
	agent, serr := cls.takeMemoryAgent(c, uid)
	if serr != nil {
		return 0, nil, serr
	}
	n, merr := cls.memory.ClearMemories(contextutil.NewContextWithGin(c), agent.UUID.String())
	if merr != nil {
		return 0, nil, merr
	}
	return response.Ok, gin.H{"deleted_count": n}, nil
}
