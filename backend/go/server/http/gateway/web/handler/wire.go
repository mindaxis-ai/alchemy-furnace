//go:build wireinject
// +build wireinject

// Package handler 新网关 handler 装配入口(wire)
package handler

import (
	"github.com/alchemy-furnace/server/server/http/gateway/web/handler/agent"
	"github.com/alchemy-furnace/server/server/http/gateway/web/handler/chat"
	"github.com/alchemy-furnace/server/server/http/gateway/web/handler/fusion"
	"github.com/alchemy-furnace/server/server/http/gateway/web/handler/model"
	"github.com/alchemy-furnace/server/server/http/gateway/web/handler/pill"
	"github.com/alchemy-furnace/server/server/http/gateway/web/handler/trial"
	"github.com/alchemy-furnace/server/server/http/service"
	"github.com/google/wire"
)

// NewPill 金丹处理器装配
func NewPill() *pill.Pill {
	panic(wire.Build(
		service.PillService, pill.New,
	))
}

// NewAgent 道人处理器装配
func NewAgent() *agent.Agent {
	panic(wire.Build(
		service.AgentService, agent.New,
	))
}

// NewTrial 试丹处理器装配
func NewTrial() *trial.Trial {
	panic(wire.Build(
		service.TrialService, trial.New,
	))
}

// NewFusion 金丹融合处理器装配
func NewFusion() *fusion.Fusion {
	panic(wire.Build(
		service.FusionService, fusion.New,
	))
}

// NewModel 供应商与模型管理处理器装配
func NewModel() *model.Model {
	panic(wire.Build(
		service.ProviderService, service.ModelService, model.New,
	))
}

// NewChat 对话处理器装配
func NewChat() *chat.Chat {
	panic(wire.Build(
		service.ChatService, chat.New,
	))
}
