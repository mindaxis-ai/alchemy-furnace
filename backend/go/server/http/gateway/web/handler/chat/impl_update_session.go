package chat

import (
	"github.com/alchemy-furnace/server/internal/context/contextutil"
	ierr "github.com/alchemy-furnace/server/internal/errors"
	"github.com/alchemy-furnace/server/server/http/request"
	"github.com/alchemy-furnace/server/server/http/response"
	"github.com/gin-gonic/gin"
)

// UpdateSession 更新会话标题或群聊头像
// PUT /api/v1/chat/sessions/:uuid  body: {"title": "..."} | {"avatar": "..."}
func (cls *Chat) UpdateSession(c *gin.Context) (response.Code, any, error) {
	uid, err := parseUUID(c)
	if err != nil {
		return response.InvalidParams, nil, err
	}
	var body struct {
		Title  *string `json:"title"`
		Avatar *string `json:"avatar"`
	}
	if berr := request.ShouldBindJSON(c, &body); berr != nil {
		return response.InvalidParams, nil, berr
	}
	if (body.Title == nil) == (body.Avatar == nil) {
		return response.InvalidParams, nil, ierr.New(ierr.ErrorTypeInvalidRequest, "handler.chat.update_fields", "必须且只能提供 title 或 avatar")
	}
	ctx := contextutil.NewContextWithGin(c)
	if body.Title != nil {
		if err := cls.chat.UpdateSessionTitle(ctx, uid, *body.Title); err != nil {
			return 0, nil, err
		}
	}
	if body.Avatar != nil {
		if err := cls.chat.UpdateGroupAvatar(ctx, uid, *body.Avatar); err != nil {
			return 0, nil, err
		}
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
