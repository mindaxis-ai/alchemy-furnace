package service

import (
	"context"

	"github.com/alchemy-furnace/server/internal/distillation"
	"github.com/alchemy-furnace/server/internal/errors"
)

type Distillation interface {
	Distill(ctx context.Context, subject, brief, locale string) (*distillation.Response, errors.Error)
}
