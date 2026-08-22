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
	pills := v1.Group("/pills")
	{
		pills.GET("", router.WrapperPage(pillHandler.List))
		pills.POST("", router.Wrapper(pillHandler.Create))
		pills.GET("/:uuid", router.Wrapper(pillHandler.Get))
		pills.PUT("/:uuid", router.Wrapper(pillHandler.Update))
		pills.DELETE("/:uuid", router.Wrapper(pillHandler.Delete))
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
		agents.PUT("/:uuid/pills/:pill_uuid", router.Wrapper(agentHandler.UpdateAgentPill))
		agents.DELETE("/:uuid/pills/:pill_uuid", router.Wrapper(agentHandler.UnbindPill))
		agents.GET("/:uuid/pills", router.Wrapper(agentHandler.ListPills))
	}

	// 系统接口(健康检查/配置;无 service 层,内联构造)
	sys := v1.Group("/system")
	{
		sys.GET("/health", router.Wrapper(systemHandler.HealthCheck))
		sys.GET("/config", router.Wrapper(systemHandler.GetConfig))
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
	}

	// 试丹(临时组合「基础性格 + 金丹」预览,无需创建道人)
	trialGroup := v1.Group("/trial")
	{
		trialGroup.POST("/synthesis", router.Wrapper(trialHandler.Synthesize))
		trialGroup.POST("/chat", router.Wrapper(trialHandler.Chat))
	}

	// 金丹融合(N 枚金丹随机融合为新丹预览,不落库)
	fusionGroup := v1.Group("/fusion")
	{
		fusionGroup.POST("/fuse", router.Wrapper(fusionHandler.Fuse))
	}

	distillationGroup := v1.Group("/distillation")
	{
		distillationGroup.POST("/nuwa", router.Wrapper(distillationHandler.Nuwa))
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
