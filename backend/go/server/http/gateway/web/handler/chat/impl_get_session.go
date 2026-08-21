package chat

import (
	"github.com/alchemy-furnace/server/internal/context/contextutil"
	"github.com/alchemy-furnace/server/model"
	"github.com/alchemy-furnace/server/server/http/response"
	"github.com/gin-gonic/gin"
)

// GetSession 按 UUID 直接读取会话元数据。历史读取不执行 active/model 授权，
// 因此停用道人或含停用成员的旧会话仍可打开为只读。
// GET /api/v1/chat/sessions/:uuid
func (cls *Chat) GetSession(c *gin.Context) (response.Code, any, error) {
	sessionUID, err := parseUUID(c)
	if err != nil {
		return response.InvalidParams, nil, err
	}
	ctx := contextutil.NewContextWithGin(c)
	session, err := cls.chat.GetSessionAgentInfo(ctx, sessionUID)
	if err != nil {
		return 0, nil, err
	}
	if session.Type != model.SessionTypeGroup {
		return response.Ok, toSessionResponse(session), nil
	}
	members, err := cls.chat.ListMembers(ctx, sessionUID)
	if err != nil {
		return 0, nil, err
	}
	return response.Ok, toSessionResponseWithMembers(session, members), nil
}
