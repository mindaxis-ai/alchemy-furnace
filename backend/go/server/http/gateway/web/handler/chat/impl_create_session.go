package chat

import (
	"strings"

	"github.com/alchemy-furnace/server/internal/context/contextutil"
	"github.com/alchemy-furnace/server/internal/errors"
	"github.com/alchemy-furnace/server/model"
	"github.com/alchemy-furnace/server/server/http/request"
	"github.com/alchemy-furnace/server/server/http/response"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// CreateSessionRequest 创建会话请求
//   - single(默认):AgentID 必填
//   - group: Type="group" + MemberAgentIDs ≥2
//   - Title 字段忽略(自动命名)
type CreateSessionRequest struct {
	AgentID        string   `json:"agent_id"`
	Type           string   `json:"type"`
	MemberAgentIDs []string `json:"member_agent_ids"`
	Title          string   `json:"title"`
}

// CreateSession 创建对话会话(single 或 group)
// POST /api/v1/chat/sessions
func (cls *Chat) CreateSession(c *gin.Context) (response.Code, any, error) {
	var body CreateSessionRequest
	if err := request.ShouldBindJSON(c, &body); err != nil {
		return response.InvalidParams, nil, err
	}

	ctx := contextutil.NewContextWithGin(c)

	if strings.EqualFold(body.Type, model.SessionTypeGroup) {
		// 群聊分支
		uids := make([]uuid.UUID, 0, len(body.MemberAgentIDs))
		for _, s := range body.MemberAgentIDs {
			uid, err := uuid.Parse(s)
			if err != nil {
				return response.InvalidParams, nil, errors.New(errors.ErrorTypeInvalidRequest, "handler.chat.create_member_uuid", "道人ID格式不正确")
			}
			uids = append(uids, uid)
		}
		session, err := cls.chat.CreateGroupSession(ctx, uids)
		if err != nil {
			return 0, nil, err
		}
		// 成员随会话原子创建并由 service 带回,不再二次查询(消除第二个失败点)
		return response.CodeCreated, toSessionResponse(session), nil
	}

	// single 分支
	agentUID, err := uuid.Parse(body.AgentID)
	if err != nil {
		return response.InvalidParams, nil, errors.New(errors.ErrorTypeInvalidRequest, "handler.chat.create_agent_uuid", "道人ID格式不正确")
	}
	session, err := cls.chat.CreateSession(ctx, agentUID)
	if err != nil {
		return 0, nil, err
	}
	return response.CodeCreated, toSessionResponse(session), nil
}
