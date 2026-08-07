// Package handler 试丹（Trial）HTTP 处理器
// 提供无需创建道人即可临时组合「基础性格 + 金丹」快速预览效果的接口
// 对应 RESTful API: /api/v1/trial/synthesis, /api/v1/trial/chat
package handler

import (
	"github.com/alchemy-furnace/server/pkg/response"
	"github.com/alchemy-furnace/server/service"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// TrialHandler 试丹 HTTP 处理器
type TrialHandler struct {
	service *service.TrialService
}

// NewTrialHandler 创建试丹处理器
func NewTrialHandler() *TrialHandler {
	return &TrialHandler{
		service: service.NewTrialService(),
	}
}

// Synthesize 试丹-合成预览
// POST /api/v1/trial/synthesis
func (h *TrialHandler) Synthesize(c *gin.Context) {
	var req service.TrialSynthesisRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "请求参数格式不正确: "+err.Error())
		return
	}

	result, err := h.service.Synthesize(&req)
	if err != nil {
		zap.L().Error("[炼丹炉] 试丹合成失败", zap.Error(err))
		response.InternalError(c, err.Error())
		return
	}

	response.Success(c, gin.H{
		"system_prompt":   result.SystemPrompt,
		"emergence_rules": result.EmergenceRules,
		"inner_tensions":  result.InnerTensions,
		"fingerprint":     result.Fingerprint,
		"model":           result.Model,
	})
}

// Chat 试丹-临时对话（非流式）
// POST /api/v1/trial/chat
func (h *TrialHandler) Chat(c *gin.Context) {
	var req service.TrialChatRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "请求参数格式不正确: "+err.Error())
		return
	}

	result, err := h.service.Chat(&req)
	if err != nil {
		zap.L().Error("[炼丹炉] 试丹对话失败", zap.Error(err))
		response.InternalError(c, err.Error())
		return
	}

	response.Success(c, result)
}
