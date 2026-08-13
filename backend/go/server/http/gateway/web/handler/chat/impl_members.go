package chat

import (
	"github.com/alchemy-furnace/server/internal/context/contextutil"
	"github.com/alchemy-furnace/server/internal/errors"
	"github.com/alchemy-furnace/server/server/http/request"
	"github.com/alchemy-furnace/server/server/http/response"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// AddMembers 邀请入群(已在群静默跳过)
// POST /api/v1/chat/sessions/:uuid/members  body: {"agent_ids":[uuid…]}
func (cls *Chat) AddMembers(c *gin.Context) (response.Code, any, error) {
	uid, err := parseUUID(c)
	if err != nil {
		return response.InvalidParams, nil, err
	}
	var body struct {
		AgentIDs []string `json:"agent_ids" binding:"required"`
	}
	if berr := request.ShouldBindJSON(c, &body); berr != nil {
		return response.InvalidParams, nil, berr
	}
	uids := make([]uuid.UUID, 0, len(body.AgentIDs))
	for _, s := range body.AgentIDs {
		u, perr := uuid.Parse(s)
		if perr != nil {
			return response.InvalidParams, nil, errors.New(errors.ErrorTypeInvalidRequest, "handler.chat.invite_uuid", "道人ID格式不正确")
		}
		uids = append(uids, u)
	}
	ctx := contextutil.NewContextWithGin(c)
	if err := cls.chat.AddMembers(ctx, uid, uids); err != nil {
		return 0, nil, err
	}
	members, merr := cls.chat.ListMembers(ctx, uid)
	if merr != nil {
		return 0, nil, merr
	}
	return response.Ok, gin.H{"members": toMemberResponseList(members)}, nil
}

// RemoveMember 踢出群
// DELETE /api/v1/chat/sessions/:uuid/members/:agent_uuid
func (cls *Chat) RemoveMember(c *gin.Context) (response.Code, any, error) {
	uid, err := parseUUID(c)
	if err != nil {
		return response.InvalidParams, nil, err
	}
	agentUID, perr := uuid.Parse(c.Param("agent_uuid"))
	if perr != nil {
		return response.InvalidParams, nil, errors.New(errors.ErrorTypeInvalidRequest, "handler.chat.kick_uuid", "道人ID格式不正确")
	}
	ctx := contextutil.NewContextWithGin(c)
	if rerr := cls.chat.RemoveMember(ctx, uid, agentUID); rerr != nil {
		return 0, nil, rerr
	}
	members, merr := cls.chat.ListMembers(ctx, uid)
	if merr != nil {
		return 0, nil, merr
	}
	return response.Ok, gin.H{"members": toMemberResponseList(members)}, nil
}
