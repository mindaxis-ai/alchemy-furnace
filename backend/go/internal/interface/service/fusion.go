package service

import (
	"context"
	"time"

	"github.com/alchemy-furnace/server/internal/errors"
	"github.com/alchemy-furnace/server/internal/synthesis"
	"github.com/alchemy-furnace/server/model"
	"github.com/google/uuid"
)

// FusionPreviewResult 融合预览结果（两阶段第一阶段；确认需携带 PreviewID）
type FusionPreviewResult struct {
	PreviewID   string // uuid 文本(实体的唯一标识即 uuid.UUID.String())
	ExpiresAt   time.Time
	Name        string
	Description string
	SkillSchema model.JSONMap
	Operator    synthesis.FuseOperator
	Model       string
	Degraded    bool
}

// Fusion 金丹融合业务逻辑接口
type Fusion interface {
	// Fuse 融合预览：不落库，直接返回融合结果（旧入口；任务 5 切换为 PreviewFusion）
	Fuse(ctx context.Context, pillUUIDs []uuid.UUID, excludeOperatorID string) (*synthesis.FuseResponse, errors.Error)
	// PreviewFusion 两阶段第一阶段：校验材料 → 模型生成（事务外）→ 持久化预览（15 分钟）
	PreviewFusion(ctx context.Context, req PreviewFusionRequest) (*FusionPreviewResult, errors.Error)
}
