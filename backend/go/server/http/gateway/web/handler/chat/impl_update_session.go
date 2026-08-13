package chat

import (
	"github.com/alchemy-furnace/server/internal/context/contextutil"
	"github.com/alchemy-furnace/server/server/http/request"
	"github.com/alchemy-furnace/server/server/http/response"
	"github.com/gin-gonic/gin"
)

// UpdateSession 重命名会话
// PUT /api/v1/chat/sessions/:uuid  body: {"title": "..."}
func (cls *Chat) UpdateSession(c *gin.Context) (response.Code, any, error) {
	uid, err := parseUUID(c)
	if err != nil {
		return response.InvalidParams, nil, err
	}
	var body struct {
		Title string `json:"title" binding:"required"`
	}
	if berr := request.ShouldBindJSON(c, &body); berr != nil {
		return response.InvalidParams, nil, berr
	}
	ctx := contextutil.NewContextWithGin(c)
	if err := cls.chat.UpdateSessionTitle(ctx, uid, body.Title); err != nil {
		return 0, nil, err
	}
	session, err := cls.chat.GetSessionAgentInfo(ctx, uid)
	if err != nil {
		return 0, nil, err
	}
	// 群聊附带 members;单聊直接返回
	members, merr := cls.chat.ListMembers(ctx, uid)
	if merr == nil && len(members) > 0 {
		return response.Ok, toSessionResponseWithMembers(session, members), nil
	}
	return response.Ok, toSessionResponse(session), nil
}
