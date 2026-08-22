package chat

import (
	"github.com/alchemy-furnace/server/internal/context/contextutil"
	"github.com/alchemy-furnace/server/server/http/response"
	"github.com/gin-gonic/gin"
)

// ReadinessResponse 可对话就绪状态 DTO;只承载 ID 与派生布尔,不含任何凭证内容
type ReadinessResponse struct {
	ActiveAgentCount int      `json:"active_agent_count"`
	ReadyAgentIDs    []string `json:"ready_agent_ids"`
	CanCreateSingle  bool     `json:"can_create_single"`
	CanCreateGroup   bool     `json:"can_create_group"`
}

// GetReadiness GET /api/v1/chat/readiness
// 派生规则: ready>=1 可单聊, ready>=2 可群聊
func (h *Chat) GetReadiness(c *gin.Context) (response.Code, any, error) {
	readiness, err := h.chat.GetReadiness(contextutil.NewContextWithGin(c))
	if err != nil {
		return response.ServerInternalError, nil, err
	}

	readyIDs := make([]string, 0, len(readiness.ReadyAgentIDs))
	for _, id := range readiness.ReadyAgentIDs {
		readyIDs = append(readyIDs, id.String())
	}

	return response.Ok, ReadinessResponse{
		ActiveAgentCount: readiness.ActiveAgentCount,
		ReadyAgentIDs:    readyIDs,
		CanCreateSingle:  len(readiness.ReadyAgentIDs) >= 1,
		CanCreateGroup:   len(readiness.ReadyAgentIDs) >= 2,
	}, nil
}
