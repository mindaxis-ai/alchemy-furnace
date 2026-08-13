package agent

import (
	"github.com/alchemy-furnace/server/internal/context/contextutil"
	"github.com/alchemy-furnace/server/server/http/request"
	"github.com/alchemy-furnace/server/server/http/response"
	"github.com/gin-gonic/gin"
)

// CreateRequest 创建道人请求
type CreateRequest struct {
	Name        string `json:"name" binding:"required,max=100"`
	Avatar      string `json:"avatar"`
	Personality string `json:"personality"`
	ModelName   string `json:"model_name" binding:"max=50"`
	Proactivity *int   `json:"proactivity" binding:"omitempty,gte=0,lte=100"` // 主动性 0-100,缺省 50
}

// Create 创建道人
// POST /api/v1/agents
func (cls *Agent) Create(c *gin.Context) (response.Code, any, error) {
	var body CreateRequest
	if err := request.ShouldBindJSON(c, &body); err != nil {
		return response.InvalidParams, nil, err
	}

	// 错误路径返回码传 0:Wrapper 按错误类型映射(400 模型未启用/500 内部错误)
	agent, err := cls.agent.CreateAgent(contextutil.NewContextWithGin(c),
		body.Name, body.Avatar, body.Personality, body.ModelName, body.Proactivity)
	if err != nil {
		return 0, nil, err
	}
	return response.CodeCreated, toResponse(agent), nil
}
