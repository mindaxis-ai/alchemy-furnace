// Package handler 系统管理 HTTP 处理器
// 提供健康检查和配置查询接口
// 对应 API: /api/v1/system/health, /api/v1/system/config
package handler

import (
	"net/http"
	"time"

	"github.com/alchemy-furnace/server/model"
	"github.com/alchemy-furnace/server/pkg/config"
	"github.com/alchemy-furnace/server/pkg/response"
	"github.com/gin-gonic/gin"
)

// SystemHandler 系统处理器
type SystemHandler struct {
	version string
}

// NewSystemHandler 创建系统处理器
func NewSystemHandler() *SystemHandler {
	return &SystemHandler{
		version: "2.0.0",
	}
}

// HealthCheck 健康检查
// GET /api/v1/system/health
// 返回各个组件的健康状态，用于监控和负载均衡
func (h *SystemHandler) HealthCheck(c *gin.Context) {
	pythonEngine := "unknown"
	if pingEngine(config.Get().PythonEngine.BaseURL) {
		pythonEngine = "ok"
	}

	status := "ok"
	if pythonEngine != "ok" {
		status = "degraded"
	}

	health := model.HealthCheckResponse{
		Status:       status,
		Version:      h.version,
		Timestamp:    time.Now().Unix(),
		DB:           "ok",
		PythonEngine: pythonEngine,
	}

	c.JSON(http.StatusOK, health)
}

// pingEngine 检测 Python 语言引擎连通性
func pingEngine(baseURL string) bool {
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get(baseURL + "/health")
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

// GetConfig 获取系统配置
// GET /api/v1/system/config
// 返回前端需要的配置信息，如可用模型列表
func (h *SystemHandler) GetConfig(c *gin.Context) {
	cfg := config.Get()

	data := gin.H{
		"version":         h.version,
		"models":          cfg.LLM.Models,
		"default_model":   cfg.LLM.DefaultModel,
		"synthesis_model": cfg.LLM.SynthesisModel,
	}

	response.Success(c, data)
}
