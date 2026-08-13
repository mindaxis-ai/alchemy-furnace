package agent

import (
	"github.com/alchemy-furnace/server/internal/context/contextutil"
	"github.com/alchemy-furnace/server/server/http/request"
	"github.com/alchemy-furnace/server/server/http/response"
	"github.com/gin-gonic/gin"
)

// UpdateRequest 更新道人请求(指针字段区分「未传」与「置空」)
type UpdateRequest struct {
	Name        *string `json:"name" binding:"omitempty,max=100"`
	Avatar      *string `json:"avatar"`
	Personality *string `json:"personality"`
	ModelName   *string `json:"model_name" binding:"omitempty,max=50"`
	Status      *string `json:"status" binding:"omitempty,oneof=active inactive"`
	Proactivity *int    `json:"proactivity" binding:"omitempty,gte=0,lte=100"`
}

// Update 更新道人
// PUT /api/v1/agents/:uuid
func (cls *Agent) Update(c *gin.Context) (response.Code, any, error) {
	uid, err := parseUUID(c)
	if err != nil {
		return response.InvalidParams, nil, err
	}

	var body UpdateRequest
	if berr := request.ShouldBindJSON(c, &body); berr != nil {
		return response.InvalidParams, nil, berr
	}

	agent, serr := cls.agent.UpdateAgent(contextutil.NewContextWithGin(c), uid,
		body.Name, body.Avatar, body.Personality, body.ModelName, body.Status, body.Proactivity)
	if serr != nil {
		return 0, nil, serr
	}
	return response.Ok, toResponse(agent), nil
}
