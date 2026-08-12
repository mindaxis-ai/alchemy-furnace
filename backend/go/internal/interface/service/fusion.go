package service

import (
	"context"

	"github.com/alchemy-furnace/server/internal/errors"
	"github.com/alchemy-furnace/server/internal/synthesis"
	"github.com/google/uuid"
)

// Fusion 金丹融合业务逻辑接口
type Fusion interface {
	// Fuse 融合预览:不落库,直接返回融合结果
	Fuse(ctx context.Context, pillUUIDs []uuid.UUID, excludeOperatorID string) (*synthesis.FuseResponse, errors.Error)
}
