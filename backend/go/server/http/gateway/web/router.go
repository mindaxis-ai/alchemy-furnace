// Package web 新网关路由注册(对齐 Luna-CY 模板 server/http/gateway/web)
// 迁移期: 逐域在此注册并从 cmd/main/command/legacy_routes.go 注销旧路由
package web

import (
	"github.com/alchemy-furnace/server/internal/service/model_service"
	"github.com/alchemy-furnace/server/server/http/gateway/web/handler"
	"github.com/alchemy-furnace/server/server/http/gateway/web/handler/system"
	"github.com/alchemy-furnace/server/server/http/router"
	"github.com/alchemy-furnace/server/server/http/service"
	"github.com/gin-gonic/gin"
)

// Register 注册新网关全部路由(已迁移域: pill + agent + system)
func Register(r *gin.Engine, isDesktop bool, guards ...gin.HandlerFunc) error {
	v1 := r.Group("/api/v1")
	if len(guards) > 0 {
		v1.Use(guards...)
	}

	pillHandler := handler.NewPill()
	pillInventoryHandler := handler.NewPillInventory()
	agentHandler := handler.NewAgent()
	chatHandler := handler.NewChat()
	trialHandler := handler.NewTrial()
	fusionHandler := handler.NewFusion()
	distillationHandler := handler.NewDistillation()
	// 装配 system handler: 注入 model service 用于 GetConfig 返回实际配置的融合模型
	daoModel := service.ProvideModelDao()
	provider := service.ProvideProviderDao()
	modelService := model_service.New(daoModel, provider)
	systemHandler := system.New(modelService)
	modelHandler := handler.NewModel()
	userHandler := handler.NewUser()

	// 金丹管理(UUID 对外标识)
	// 旧写入/克隆路由任务 5 起恒 410 pill.legacy_api_removed(handler 方法体即 410,双保险);
	// 旧详情路由改道 LegacyMap 跳转(ResolveLegacyPill),不读取可用库存。
	pills := v1.Group("/pills")
	{
		pills.GET("", router.WrapperPage(pillHandler.List))
		pills.POST("", router.Wrapper(pillHandler.Create))
		pills.GET("/:uuid", router.Wrapper(pillInventoryHandler.ResolveLegacyPill))
		pills.PUT("/:uuid", router.Wrapper(pillHandler.Update))
		pills.DELETE("/:uuid", router.Wrapper(pillHandler.Delete))
		pills.POST("/:uuid/clone", router.Wrapper(pillHandler.Clone))
	}

	// 道人管理(UUID 对外标识;服用记录路径参数 :pill_uuid)
	agents := v1.Group("/agents")
	{
		agents.GET("", router.WrapperPage(agentHandler.List))
		agents.POST("", router.Wrapper(agentHandler.Create))
		agents.GET("/:uuid", router.Wrapper(agentHandler.Get))
		agents.PUT("/:uuid", router.Wrapper(agentHandler.Update))
		agents.DELETE("/:uuid", router.Wrapper(agentHandler.Delete))
		agents.POST("/:uuid/pills", router.Wrapper(agentHandler.BindPill))
		agents.PUT("/:uuid/pills", router.Wrapper(agentHandler.ReplacePills))
		agents.PUT("/:uuid/pills/:pill_uuid", router.Wrapper(agentHandler.UpdateAgentPill))
		agents.DELETE("/:uuid/pills/:pill_uuid", router.Wrapper(agentHandler.UnbindPill))
		agents.GET("/:uuid/pills", router.Wrapper(agentHandler.ListPills))

		// 道人本地记忆(UUID 对外标识;kind 筛选 + active 状态筛选)
		memories := agents.Group("/:uuid/memories")
		{
			memories.GET("", router.Wrapper(agentHandler.ListMemories))
			memories.POST("", router.Wrapper(agentHandler.CreateMemory))
			memories.PATCH("/:memory_uuid", router.Wrapper(agentHandler.UpdateMemory))
			memories.DELETE("/:memory_uuid", router.Wrapper(agentHandler.DeleteMemory))
			memories.DELETE("", router.Wrapper(agentHandler.ClearMemories))
		}
	}

	// 金丹消耗品库存(任务 5 新链路,§2.3 路由契约)
	// 所有写操作(预览与查询除外)要求 Idempotency-Key 头: 缺失/非法 → 400。
	inventoryGroup := v1.Group("")
	{
		// 丹方(永久保留;编辑生成新版本,不影响旧金丹/能力)
		inventoryGroup.GET("/recipes", router.Wrapper(pillInventoryHandler.ListRecipes))
		inventoryGroup.POST("/recipes", router.Wrapper(pillInventoryHandler.SaveRecipe))
		inventoryGroup.GET("/recipes/:id", router.Wrapper(pillInventoryHandler.GetRecipe))
		inventoryGroup.GET("/recipes/:id/revisions/:revision_id", router.Wrapper(pillInventoryHandler.GetRecipeRevision))
		inventoryGroup.POST("/recipes/:id/revisions", router.Wrapper(pillInventoryHandler.UpdateRecipe))
		inventoryGroup.POST("/recipes/:id/archive", router.Wrapper(pillInventoryHandler.ArchiveRecipe))
		inventoryGroup.POST("/recipes/:id/craft", router.Wrapper(pillInventoryHandler.CraftPill))
		// 金丹库存(实例级状态机 available→consumed_by_*/discarded)
		inventoryGroup.GET("/pill-items", router.Wrapper(pillInventoryHandler.ListPillItems))
		inventoryGroup.GET("/pill-items/:id", router.Wrapper(pillInventoryHandler.GetPillItem))
		inventoryGroup.POST("/pill-items/:id/discard", router.Wrapper(pillInventoryHandler.DiscardItem))
		// 服用与能力编排(服用消耗库存但保留能力;移除能力不返还)
		// 参数名 :uuid/:effect_uuid 为对齐既有 agents 路由树(Gin 同位置通配符名须一致),
		// 路径形状与 §2.3 契约一致
		inventoryGroup.POST("/agents/:uuid/consume", router.Wrapper(pillInventoryHandler.ConsumePill))
		inventoryGroup.GET("/agents/:uuid/effects", router.Wrapper(pillInventoryHandler.ListEffects))
		inventoryGroup.PUT("/agents/:uuid/effects", router.Wrapper(pillInventoryHandler.UpdateEffects))
		inventoryGroup.POST("/agents/:uuid/effects/:effect_id/remove", router.Wrapper(pillInventoryHandler.RemoveEffect))
		// 幂等操作查询(断线恢复)
		inventoryGroup.GET("/pill-operations/:id", router.Wrapper(pillInventoryHandler.GetOperation))
		// 迁移摘要只读(任务 8: 升级用户展示;无标记 migrated=false;不触发迁移)
		inventoryGroup.GET("/migration-summary", router.Wrapper(pillInventoryHandler.MigrationSummary))
	}

	// 系统接口(健康检查/配置;无 service 层,内联构造)
	sys := v1.Group("/system")
	{
		sys.GET("/health", router.Wrapper(systemHandler.HealthCheck))
		sys.GET("/config", router.Wrapper(systemHandler.GetConfig))
		if isDesktop {
			sys.GET("/diagnostics", router.Wrapper(systemHandler.Diagnostics))
		}
	}
	// 版本信息(全模式: serve + desktop 都暴露,前端关于区消费)
	v1.GET("/version", router.Wrapper(systemHandler.GetVersion))
	// 自动更新(仅 desktop 模式,无 isDesktop 时不挂载;guard 已挂到 v1 组,自动生效)
	if isDesktop {
		upd := v1.Group("/update")
		{
			upd.GET("/check", router.Wrapper(systemHandler.CheckUpdate))
			upd.POST("/apply", router.Wrapper(systemHandler.ApplyUpdate))
			upd.GET("/progress", router.Wrapper(systemHandler.ProgressUpdate))
		}
	}

	// 对话管理(会话 UUID 对外标识;SSE 流式对话为 RAW handler,不经 Wrapper)
	chatGroup := v1.Group("/chat")
	{
		chatGroup.GET("/readiness", router.Wrapper(chatHandler.GetReadiness))
		chatGroup.POST("/sessions", router.Wrapper(chatHandler.CreateSession))
		chatGroup.GET("/sessions", router.WrapperPage(chatHandler.ListSessions))
		chatGroup.GET("/sessions/:uuid", router.Wrapper(chatHandler.GetSession))
		chatGroup.GET("/sessions/:uuid/messages", router.WrapperPage(chatHandler.GetMessages))
		chatGroup.PUT("/sessions/:uuid", router.Wrapper(chatHandler.UpdateSession))
		chatGroup.POST("/sessions/:uuid/members", router.Wrapper(chatHandler.AddMembers))
		chatGroup.DELETE("/sessions/:uuid/members/:agent_uuid", router.Wrapper(chatHandler.RemoveMember))
		chatGroup.POST("/sse/:uuid", chatHandler.SSEChat) // RAW: 自行写出标准 SSE 事件(单/群分流)
		chatGroup.POST("/runs/:run_id/resume", chatHandler.ResumeRunSSE) // RAW: 续跑 interrupted run(Task 14)
	}

	// 试丹(临时组合「基础性格 + 金丹」预览,无需创建道人)
	trialGroup := v1.Group("/trial")
	{
		trialGroup.POST("/synthesis", router.Wrapper(trialHandler.Synthesize))
		trialGroup.POST("/chat", router.Wrapper(trialHandler.Chat))
	}

	// 金丹融合(两阶段: 预览不扣料/确认原子扣料;旧 /fuse 恒 410)
	fusionGroup := v1.Group("/fusion")
	{
		fusionGroup.POST("/fuse", router.Wrapper(fusionHandler.Fuse))
		fusionGroup.POST("/previews", router.Wrapper(pillInventoryHandler.PreviewFusion))
		fusionGroup.POST("/confirm", router.Wrapper(pillInventoryHandler.ConfirmFusion))
	}

	distillationGroup := v1.Group("/distillation")
	{
		distillationGroup.POST("/nuwa", router.Wrapper(distillationHandler.Nuwa))
		// RAW: 成功直接写 ZIP 二进制流(不经 Wrapper 的 JSON 信封)
		distillationGroup.POST("/skill-export", distillationHandler.SkillExport)
	}

	// 供应商与模型管理(UUID 对外标识;/templates 与 /options 静态路由先于 :uuid 注册)
	providers := v1.Group("/providers")
	{
		providers.GET("/templates", router.Wrapper(modelHandler.Templates))
		providers.GET("", router.WrapperPage(modelHandler.ListProviders))
		providers.POST("", router.Wrapper(modelHandler.CreateProvider))
		providers.GET("/:uuid", router.Wrapper(modelHandler.GetProvider))
		providers.PUT("/:uuid", router.Wrapper(modelHandler.UpdateProvider))
		providers.DELETE("/:uuid", router.Wrapper(modelHandler.DeleteProvider))
		providers.POST("/:uuid/test-connection", router.Wrapper(modelHandler.TestConnection))
		providers.GET("/:uuid/models", router.Wrapper(modelHandler.ListModels))
		providers.POST("/:uuid/models", router.Wrapper(modelHandler.CreateModel))
	}

	models := v1.Group("/models")
	{
		models.GET("/options", router.Wrapper(modelHandler.Options))
		models.GET("/:uuid", router.Wrapper(modelHandler.GetModel))
		models.PUT("/:uuid", router.Wrapper(modelHandler.UpdateModel))
		models.DELETE("/:uuid", router.Wrapper(modelHandler.DeleteModel))
	}

	// 用户档案(本地/单用户部署,整库固定 id=1)
	userGroup := v1.Group("/user")
	{
		userGroup.GET("/profile", router.Wrapper(userHandler.Get))
		userGroup.PUT("/profile", router.Wrapper(userHandler.Update))
	}

	return nil
}
