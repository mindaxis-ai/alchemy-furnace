package chat

import (
	"github.com/alchemy-furnace/server/internal/context/contextutil"
	"github.com/alchemy-furnace/server/server/http/response"
	"github.com/gin-gonic/gin"
)

// DeleteSession 永久删除会话及其关联消息。
// DELETE /api/v1/chat/sessions/:uuid
func (cls *Chat) DeleteSession(c *gin.Context) (response.Code, any, error) {
	uid, err := parseUUID(c)
	if err != nil {
		return response.InvalidParams, nil, err
	}
	if err := cls.chat.DeleteSession(contextutil.NewContextWithGin(c), uid); err != nil {
		return 0, nil, err
	}
	return response.Ok, nil, nil
}
