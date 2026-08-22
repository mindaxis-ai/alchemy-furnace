package pill

import (
	"github.com/alchemy-furnace/server/internal/context/contextutil"
	"github.com/alchemy-furnace/server/server/http/response"
	"github.com/gin-gonic/gin"
)

// Clone 深复制金丹为自定义副本
// POST /api/v1/pills/:uuid/clone
func (cls *Pill) Clone(c *gin.Context) (response.Code, any, error) {
	uid, err := parseUUID(c)
	if err != nil {
		return response.InvalidParams, nil, err
	}

	pill, serr := cls.pill.ClonePill(contextutil.NewContextWithGin(c), uid)
	if serr != nil {
		return 0, nil, serr
	}
	return response.CodeCreated, ToResponse(pill), nil
}
