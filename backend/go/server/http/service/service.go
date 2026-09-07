// Package service wire 装配集(对齐 Luna-CY 模板 server/http/service)
// 每个域一个 wire.NewSet:dao 实现绑定接口 + service 实现绑定接口
package service

import (
	"time"

	"github.com/alchemy-furnace/server/internal/dao"
	"github.com/alchemy-furnace/server/internal/distillation"
	"github.com/alchemy-furnace/server/internal/engineendpoint"
	idao "github.com/alchemy-furnace/server/internal/interface/dao"
	iservice "github.com/alchemy-furnace/server/internal/interface/service"
	"github.com/alchemy-furnace/server/internal/service/agent_service"
	"github.com/alchemy-furnace/server/internal/service/chat_service"
	"github.com/alchemy-furnace/server/internal/service/credential"
	"github.com/alchemy-furnace/server/internal/service/distillation_service"
	"github.com/alchemy-furnace/server/internal/service/fusion_service"
	"github.com/alchemy-furnace/server/internal/service/language_pattern_service"
	"github.com/alchemy-furnace/server/internal/service/memory_service"
	"github.com/alchemy-furnace/server/internal/service/model_service"
	"github.com/alchemy-furnace/server/internal/service/pill_inventory_service"
	"github.com/alchemy-furnace/server/internal/service/pill_service"
	"github.com/alchemy-furnace/server/internal/service/provider_service"
	"github.com/alchemy-furnace/server/internal/service/trial_service"
	"github.com/alchemy-furnace/server/internal/synthesis"
	"github.com/google/wire"
	"gorm.io/gorm"
)

// ==================== DAO Provider ====================

// ProvidePillDao 金丹 DAO
func ProvidePillDao() idao.Pill {
	return dao.NewPillDao()
}

// ProvideAgentDao 道人 DAO
func ProvideAgentDao() idao.Agent {
	return dao.NewAgentDao()
}

// ProvideChatDao 对话域 DAO
func ProvideChatDao() idao.Chat {
	return dao.NewChatDao()
}

// ProvideProviderDao 供应商 DAO
func ProvideProviderDao() idao.Provider {
	return dao.NewProviderDao()
}

// ProvideModelDao 模型 DAO
func ProvideModelDao() idao.Model {
	return dao.NewModelDao()
}

// ProvideMemoryDao 记忆 DAO
func ProvideMemoryDao() idao.Memory {
	return dao.NewMemoryDao()
}

// ProvideMemoryService 记忆服务(检索/CRUD/蒸馏队列)
func ProvideMemoryService(memoryDAO idao.Memory, creds credential.Resolver, engineURL engineendpoint.Provider) iservice.Memory {
	return memory_service.NewMemoryService(memoryDAO, creds, engineURL)
}

// ==================== 域装配集 ====================

// PillService 金丹域装配集
var PillService = wire.NewSet(
	ProvidePillDao,
	pill_service.New, wire.Bind(new(iservice.Pill), new(*pill_service.Pill)),
)

// AgentService 道人域装配集(依赖库存服务、模型查询 DAO 与记忆服务;任务 3 起不再依赖金丹 DAO)
var AgentService = wire.NewSet(
	ProvideAgentDao,
	ProvideDB,
	ProvideInventory,
	wire.Bind(new(agent_service.PillConsumer), new(*pill_inventory_service.Inventory)),
	ProvideModelDao,
	ProvideMemoryDao,
	ProvideMemoryService,
	credential.NewResolver, wire.Bind(new(credential.Resolver), new(*credential.ModelResolver)),
	NewEngineBaseURL,
	agent_service.New, wire.Bind(new(iservice.Agent), new(*agent_service.Agent)),
)

// ProvideInventory 金丹库存服务 provider(依赖 DB 与真实时钟;服用/移除的事实来源)
// 返回具体类型而非 iservice.PillInventory: 任务 4 实现 ConfirmFusion 前 *Inventory
// 尚未满足完整接口;wire 按方法集推导注入 agent_service.PillConsumer 窄接口(本任务只需 Consume)
func ProvideInventory(db *gorm.DB) *pill_inventory_service.Inventory {
	return pill_inventory_service.New(db, time.Now)
}

// NewSynthesisClient 合成引擎客户端 provider(从全局配置取 Python 引擎 BaseURL)
func NewSynthesisClient() synthesis.Client {
	return synthesis.NewDynamic(engineendpoint.Current)
}

// NewFusionClient 融合引擎客户端 provider(与 NewSynthesisClient 同一配置来源)
func NewFusionClient() synthesis.FusionClient {
	return synthesis.NewDynamicFusionClient(engineendpoint.Current)
}

// NewEngineBaseURL 语言引擎 BaseURL provider(对话流式接口直连 Python 引擎)
func NewEngineBaseURL() engineendpoint.Provider {
	return engineendpoint.Current
}

// ChatService 对话域装配集(依赖道人 DAO + 合成客户端 + 凭证解析器 + 语言模式服务 + 记忆服务)
var ChatService = wire.NewSet(
	ProvideChatDao,
	ProvideAgentDao,
	NewSynthesisClient,
	credential.NewResolver, wire.Bind(new(credential.Resolver), new(*credential.ModelResolver)),
	NewEngineBaseURL,
	language_pattern_service.New, wire.Bind(new(iservice.LanguagePatternProvider), new(*language_pattern_service.LanguagePatternService)),
	ProvideMemoryDao,
	ProvideMemoryService,
	chat_service.NewDynamic, wire.Bind(new(iservice.Chat), new(*chat_service.Chat)),
)

// TrialService 试丹域装配集(任务 5 起依赖丹方库存读接口,不再引用旧 ElixirPill DAO)
var TrialService = wire.NewSet(
	ProvideDB,
	ProvideInventory,
	wire.Bind(new(iservice.PillInventory), new(*pill_inventory_service.Inventory)),
	NewSynthesisClient,
	credential.NewResolver, wire.Bind(new(credential.Resolver), new(*credential.ModelResolver)),
	trial_service.New, wire.Bind(new(iservice.Trial), new(*trial_service.Trial)),
)

// FusionService 金丹融合域装配集(任务 4: 融合服务直连库存表,不再依赖旧金丹 DAO)
var FusionService = wire.NewSet(
	ProvideDB,
	NewFusionClient,
	credential.NewResolver, wire.Bind(new(credential.Resolver), new(*credential.ModelResolver)),
	fusion_service.New, wire.Bind(new(iservice.Fusion), new(*fusion_service.Fusion)),
)

// PillInventoryService 金丹消耗品库存域装配集(任务 5):
// 平铺库存/道人/融合三域的 provider（嵌套 set 会因重复 provider 报错，
// wire 只在同一 set 内按函数签名去重）。ProvideInventory 同时满足
// iservice.PillInventory 与 agent_service.PillConsumer 两个绑定。
var PillInventoryService = wire.NewSet(
	ProvideDB,
	ProvideInventory,
	wire.Bind(new(iservice.PillInventory), new(*pill_inventory_service.Inventory)),
	wire.Bind(new(agent_service.PillConsumer), new(*pill_inventory_service.Inventory)),
	ProvideAgentDao,
	ProvideModelDao,
	ProvideMemoryDao,
	ProvideMemoryService,
	credential.NewResolver, wire.Bind(new(credential.Resolver), new(*credential.ModelResolver)),
	NewEngineBaseURL,
	agent_service.New, wire.Bind(new(iservice.Agent), new(*agent_service.Agent)),
	NewFusionClient,
	fusion_service.New, wire.Bind(new(iservice.Fusion), new(*fusion_service.Fusion)),
)

func NewDistillationClient() distillation.Client {
	return distillation.NewDynamicClient(engineendpoint.Current)
}

// DistillationService 女娲炼制与 skill 导出装配集
// 任务 5 起导出依赖丹方库存读接口(GetRecipe/GetRecipeRevision),
// 不再引用旧 ElixirPill DAO;ProvideInventory 同时满足 iservice.PillInventory 绑定。
var DistillationService = wire.NewSet(
	ProvideDB,
	ProvideInventory,
	wire.Bind(new(iservice.PillInventory), new(*pill_inventory_service.Inventory)),
	NewDistillationClient,
	credential.NewResolver, wire.Bind(new(credential.Resolver), new(*credential.ModelResolver)),
	distillation_service.New, wire.Bind(new(iservice.Distillation), new(*distillation_service.Service)),
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
	return dao.GetDB()
}
