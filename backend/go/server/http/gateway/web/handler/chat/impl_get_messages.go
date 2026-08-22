package chat

import (
	"strconv"

	"github.com/alchemy-furnace/server/internal/context/contextutil"
	"github.com/gin-gonic/gin"
)

// GetMessages 获取会话消息历史(page=1 为最新一页,页内按时间正序)
// GET /api/v1/chat/sessions/:uuid/messages?page=1&page_size=20
func (cls *Chat) GetMessages(c *gin.Context) (int64, int, int, any, error) {
	sessionUID, err := parseUUID(c)
	if err != nil {
		return 0, 0, 0, nil, err
	}

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))

	total, messages, err := cls.chat.GetMessages(contextutil.NewContextWithGin(c), sessionUID, page, pageSize)
	if err != nil {
		return 0, 0, 0, nil, err
	}
	return total, page, pageSize, toMessageResponseList(messages), nil
}
