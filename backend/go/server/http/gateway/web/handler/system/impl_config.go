package system

import (
	"github.com/alchemy-furnace/server/internal/configuration"
	"github.com/alchemy-furnace/server/internal/context/contextutil"
	"github.com/alchemy-furnace/server/internal/interface/service"
	"github.com/alchemy-furnace/server/server/http/response"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// GetConfig 获取系统配置
// GET /api/v1/system/config
// 返回前端需要的配置信息(可用模型清单 + 实际配置的融合模型)
func (cls *System) GetConfig(c *gin.Context) (response.Code, any, error) {
	cfg := configuration.Configuration

	// 实际配置的融合模型(供 /fusion banner 展示):失败不阻塞主流程
	var fusionInfo *service.FusionModelConfig
	if cls.model != nil {
		if fi, err := cls.model.GetFusionModelConfig(contextutil.NewContextWithGin(c)); err != nil {
			zap.L().Warn("[炼丹炉] 获取融合模型配置失败,降级返回空", zap.Error(err))
		} else {
			fusionInfo = fi
		}
	}

	return response.Ok, &ConfigResponse{
		Version:         cls.version,
		Models:          cfg.LLM.Models,
		DefaultModel:    cfg.LLM.DefaultModel,
		SynthesisModel:  cfg.LLM.SynthesisModel,
		FusionModel:     cfg.LLM.FusionModel,
		FusionModelInfo: fusionInfo,
	}, nil
}
