package chat

import (
	"github.com/alchemy-furnace/server/internal/context/contextutil"
	"github.com/alchemy-furnace/server/internal/errors"
	"github.com/alchemy-furnace/server/server/http/request"
	"github.com/alchemy-furnace/server/server/http/response"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// CreateSessionRequest 创建会话请求(agent_id 为道人 UUID 字符串)
type CreateSessionRequest struct {
	AgentID string `json:"agent_id" binding:"required"` // 道人 UUID
	Title   string `json:"title" binding:"max=200"`     // 会话标题(可空,服务端按道人名生成默认值)
}

// CreateSession 创建对话会话
// POST /api/v1/chat/sessions
func (cls *Chat) CreateSession(c *gin.Context) (response.Code, any, error) {
	var body CreateSessionRequest
	if err := request.ShouldBindJSON(c, &body); err != nil {
		return response.InvalidParams, nil, err
	}

	agentUID, err := uuid.Parse(body.AgentID)
	if err != nil {
		return response.InvalidParams, nil, errors.New(errors.ErrorTypeInvalidRequest, "handler.chat.create_agent_uuid", "道人ID格式不正确")
	}

	// Title 字段本任务保留(忽略),Task 8 才扩展群创建入参
	session, err := cls.chat.CreateSession(contextutil.NewContextWithGin(c), agentUID)
	if err != nil {
		return 0, nil, err
	}
	return response.CodeCreated, toSessionResponse(session), nil
}
