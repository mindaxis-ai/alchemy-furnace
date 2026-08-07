// Package handler 道人管理 HTTP 处理器
// 处理道人的增删改查，以及道人与金丹的绑定（服用/解除）
// 对应 RESTful API: /api/v1/agents, /api/v1/agents/:id/pills
package handler

import (
	"strconv"

	"github.com/alchemy-furnace/server/model"
	"github.com/alchemy-furnace/server/pkg/response"
	"github.com/alchemy-furnace/server/service"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// AgentHandler 道人 HTTP 处理器
type AgentHandler struct {
	service *service.AgentService
}

// NewAgentHandler 创建道人处理器
func NewAgentHandler() *AgentHandler {
	return &AgentHandler{
		service: service.NewAgentService(),
	}
}

// ListAgents 道人列表
// GET /api/v1/agents?page=1&page_size=10&status=active
func (h *AgentHandler) ListAgents(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "10"))
	status := c.Query("status")

	agents, total, err := h.service.ListAgents(page, pageSize, status)
	if err != nil {
		zap.L().Error("[炼丹炉] 查询道人列表失败", zap.Error(err))
		response.InternalError(c, "查询道人列表失败")
		return
	}

	response.SuccessWithPage(c, total, page, pageSize, agents)
}

// GetAgent 道人详情
// GET /api/v1/agents/:id
func (h *AgentHandler) GetAgent(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, "道人ID格式不正确")
		return
	}

	agent, err := h.service.GetAgent(uint(id))
	if err != nil {
		zap.L().Error("[炼丹炉] 查询道人详情失败", zap.Uint64("id", id), zap.Error(err))
		response.NotFound(c, "道人不存在")
		return
	}

	response.Success(c, agent)
}

// CreateAgent 创建道人
// POST /api/v1/agents
// Body: { "name": "太上老君", "avatar": "...", "personality": "...", "model_name": "gpt-4o" }
func (h *AgentHandler) CreateAgent(c *gin.Context) {
	var req model.CreateAgentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "请求参数错误: "+err.Error())
		return
	}

	agent, err := h.service.CreateAgent(&req)
	if err != nil {
		zap.L().Error("[炼丹炉] 创建道人失败", zap.Error(err))
		response.InternalError(c, "创建道人失败")
		return
	}

	response.Created(c, agent)
}

// UpdateAgent 更新道人
// PUT /api/v1/agents/:id
func (h *AgentHandler) UpdateAgent(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, "道人ID格式不正确")
		return
	}

	var req model.UpdateAgentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "请求参数错误: "+err.Error())
		return
	}

	agent, err := h.service.UpdateAgent(uint(id), &req)
	if err != nil {
		zap.L().Error("[炼丹炉] 更新道人失败", zap.Uint64("id", id), zap.Error(err))
		response.InternalError(c, "更新道人失败")
		return
	}

	response.Success(c, agent)
}

// DeleteAgent 删除道人
// DELETE /api/v1/agents/:id
func (h *AgentHandler) DeleteAgent(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, "道人ID格式不正确")
		return
	}

	if err := h.service.DeleteAgent(uint(id)); err != nil {
		zap.L().Error("[炼丹炉] 删除道人失败", zap.Uint64("id", id), zap.Error(err))
		response.InternalError(c, "删除道人失败")
		return
	}

	response.Success(c, gin.H{"deleted": true})
}

// ---------- 服用金丹 ----------

// BindPill 道人服用金丹
// POST /api/v1/agents/:id/pills
// Body: { "pill_id": 1, "weight": 1.0, "sort_order": 0 }
func (h *AgentHandler) BindPill(c *gin.Context) {
	agentID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, "道人ID格式不正确")
		return
	}

	var req model.BindPillRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "请求参数错误: "+err.Error())
		return
	}

	if err := h.service.BindPill(uint(agentID), req.PillID, req.Weight, req.SortOrder); err != nil {
		zap.L().Error("[炼丹炉] 服用金丹失败",
			zap.Uint64("agent_id", agentID),
			zap.Uint("pill_id", req.PillID),
			zap.Error(err))
		response.Error(c, 4001, err.Error())
		return
	}

	response.Success(c, gin.H{"bound": true})
}

// UpdateAgentPill 更新服用记录（权重/顺序）
// PUT /api/v1/agents/:id/pills/:pill_id
// Body: { "weight": 1.5, "sort_order": 2 }
func (h *AgentHandler) UpdateAgentPill(c *gin.Context) {
	agentID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, "道人ID格式不正确")
		return
	}
	pillID, err := strconv.ParseUint(c.Param("pill_id"), 10, 32)
	if err != nil {
		response.BadRequest(c, "金丹ID格式不正确")
		return
	}

	var req model.BindPillRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "请求参数错误: "+err.Error())
		return
	}

	if err := h.service.UpdateAgentPill(uint(agentID), uint(pillID), req.Weight, req.SortOrder); err != nil {
		zap.L().Error("[炼丹炉] 更新服用记录失败",
			zap.Uint64("agent_id", agentID),
			zap.Uint64("pill_id", pillID),
			zap.Error(err))
		response.Error(c, 4003, err.Error())
		return
	}

	response.Success(c, gin.H{"updated": true})
}

// UnbindPill 道人解除金丹绑定
// DELETE /api/v1/agents/:id/pills/:pill_id
func (h *AgentHandler) UnbindPill(c *gin.Context) {
	agentID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, "道人ID格式不正确")
		return
	}

	pillID, err := strconv.ParseUint(c.Param("pill_id"), 10, 32)
	if err != nil {
		response.BadRequest(c, "金丹ID格式不正确")
		return
	}

	if err := h.service.UnbindPill(uint(agentID), uint(pillID)); err != nil {
		zap.L().Error("[炼丹炉] 解除金丹绑定失败",
			zap.Uint64("agent_id", agentID),
			zap.Uint64("pill_id", pillID),
			zap.Error(err))
		response.Error(c, 4002, err.Error())
		return
	}

	response.Success(c, gin.H{"unbound": true})
}

// ListAgentPills 获取道人已服用的金丹列表
// GET /api/v1/agents/:id/pills
func (h *AgentHandler) ListAgentPills(c *gin.Context) {
	agentID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, "道人ID格式不正确")
		return
	}

	pills, err := h.service.ListAgentPills(uint(agentID))
	if err != nil {
		zap.L().Error("[炼丹炉] 查询道人金丹列表失败", zap.Uint64("agent_id", agentID), zap.Error(err))
		response.InternalError(c, "查询失败")
		return
	}

	response.Success(c, pills)
}
