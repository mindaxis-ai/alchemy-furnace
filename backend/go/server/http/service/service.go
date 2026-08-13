// Package service wire 装配集(对齐 Luna-CY 模板 server/http/service)
// 每个域一个 wire.NewSet:dao 实现绑定接口 + service 实现绑定接口
//
// 007-demo-mode: DAO provider 函数根据 configuration.IsDemo() 在
// GORM(internal/dao)与内存(internal/dao/memory)实现间二选一,
// service 层与 handler 层无感知。
package service

import (
	"github.com/alchemy-furnace/server/internal/configuration"
	"github.com/alchemy-furnace/server/internal/dao"
	"github.com/alchemy-furnace/server/internal/dao/memory"
	idao "github.com/alchemy-furnace/server/internal/interface/dao"
	iservice "github.com/alchemy-furnace/server/internal/interface/service"
	"github.com/alchemy-furnace/server/internal/service/agent_service"
	"github.com/alchemy-furnace/server/internal/service/chat_service"
	"github.com/alchemy-furnace/server/internal/service/credential"
	"github.com/alchemy-furnace/server/internal/service/fusion_service"
	"github.com/alchemy-furnace/server/internal/service/language_pattern_service"
	"github.com/alchemy-furnace/server/internal/service/model_service"
	"github.com/alchemy-furnace/server/internal/service/pill_service"
	"github.com/alchemy-furnace/server/internal/service/provider_service"
	"github.com/alchemy-furnace/server/internal/service/trial_service"
	"github.com/alchemy-furnace/server/internal/synthesis"
	"github.com/google/wire"
	"gorm.io/gorm"
)

// ==================== DAO Provider(真实/演示二选一) ====================

// ProvidePillDao 金丹 DAO:演示模式用内存实现,否则用 GORM 实现
func ProvidePillDao() idao.Pill {
	if configuration.IsDemo() {
		return memory.NewPillDao()
	}
	return dao.NewPillDao()
}

// ProvideAgentDao 道人 DAO
func ProvideAgentDao() idao.Agent {
	if configuration.IsDemo() {
		return memory.NewAgentDao()
	}
	return dao.NewAgentDao()
}

// ProvideChatDao 对话域 DAO
func ProvideChatDao() idao.Chat {
	if configuration.IsDemo() {
		return memory.NewChatDao()
	}
	return dao.NewChatDao()
}

// ProvideProviderDao 供应商 DAO
func ProvideProviderDao() idao.Provider {
	if configuration.IsDemo() {
		return memory.NewProviderDao()
	}
	return dao.NewProviderDao()
}

// ProvideModelDao 模型 DAO
func ProvideModelDao() idao.Model {
	if configuration.IsDemo() {
		return memory.NewModelDao()
	}
	return dao.NewModelDao()
}

// ==================== 域装配集 ====================

// PillService 金丹域装配集
var PillService = wire.NewSet(
	ProvidePillDao,
	pill_service.New, wire.Bind(new(iservice.Pill), new(*pill_service.Pill)),
)

// AgentService 道人域装配集(依赖金丹 DAO 与模型查询 DAO)
var AgentService = wire.NewSet(
	ProvideAgentDao,
	ProvidePillDao,
	ProvideModelDao,
	agent_service.New, wire.Bind(new(iservice.Agent), new(*agent_service.Agent)),
)

// NewSynthesisClient 合成引擎客户端 provider(从全局配置取 Python 引擎 BaseURL)
func NewSynthesisClient() synthesis.Client {
	return synthesis.New(configuration.Configuration.PythonEngine.BaseURL)
}

// NewFusionClient 融合引擎客户端 provider(与 NewSynthesisClient 同一配置来源)
func NewFusionClient() synthesis.FusionClient {
	return synthesis.NewFusionClient(configuration.Configuration.PythonEngine.BaseURL)
}

// NewEngineBaseURL 语言引擎 BaseURL provider(对话流式接口直连 Python 引擎)
func NewEngineBaseURL() string {
	return configuration.Configuration.PythonEngine.BaseURL
}

// ChatService 对话域装配集(依赖道人 DAO + 合成客户端 + 凭证解析器 + 语言模式服务)
var ChatService = wire.NewSet(
	ProvideChatDao,
	ProvideAgentDao,
	NewSynthesisClient,
	credential.NewResolver, wire.Bind(new(credential.Resolver), new(*credential.ModelResolver)),
	NewEngineBaseURL,
	language_pattern_service.New, wire.Bind(new(iservice.LanguagePatternProvider), new(*language_pattern_service.LanguagePatternService)),
	chat_service.New, wire.Bind(new(iservice.Chat), new(*chat_service.Chat)),
)

// TrialService 试丹域装配集(依赖金丹 DAO + 合成客户端 + 凭证解析器)
var TrialService = wire.NewSet(
	ProvidePillDao,
	NewSynthesisClient,
	credential.NewResolver, wire.Bind(new(credential.Resolver), new(*credential.ModelResolver)),
	trial_service.New, wire.Bind(new(iservice.Trial), new(*trial_service.Trial)),
)

// FusionService 金丹融合域装配集(依赖金丹 DAO + 融合客户端 + 凭证解析器)
var FusionService = wire.NewSet(
	ProvidePillDao,
	NewFusionClient,
	credential.NewResolver, wire.Bind(new(credential.Resolver), new(*credential.ModelResolver)),
	fusion_service.New, wire.Bind(new(iservice.Fusion), new(*fusion_service.Fusion)),
)

// ProviderService 供应商域装配集(连接测试回退取首个启用模型,故依赖模型 DAO)
var ProviderService = wire.NewSet(
	ProvideProviderDao,
	ProvideModelDao,
	provider_service.New, wire.Bind(new(iservice.Provider), new(*provider_service.ProviderService)),
)

// ModelService 模型域装配集(DAO 由 ProviderService 提供,NewModel 总与 ProviderService 同用)
var ModelService = wire.NewSet(
	model_service.New, wire.Bind(new(iservice.Model), new(*model_service.ModelService)),
)

// ProvideDB 暴露 GORM *DB 给需要直接 query 的 handler(目前只有 user 包用)
func ProvideDB() *gorm.DB {
	if configuration.IsDemo() {
		// 演示模式无真实 DB;用户档案 handler 应走内存路径(本次实现未支持,先 nil 触底保护)
		return nil
	}
	return dao.GetDB()
}
