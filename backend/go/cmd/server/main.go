// main.go - 「炼丹炉」API 网关入口
//
// 职责：
// 1. 加载配置（环境变量 / .env 文件）
// 2. 初始化日志系统（zap）
// 3. 初始化数据库连接（PostgreSQL + GORM）
// 4. 初始化 Gin 路由和中间件
// 5. 注册所有 API 路由
// 6. 启动 HTTP 服务
//
// 启动命令: go run cmd/server/main.go
package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/alchemy-furnace/server/dao"
	"github.com/alchemy-furnace/server/handler"
	"github.com/alchemy-furnace/server/middleware"
	"github.com/alchemy-furnace/server/pkg/config"
	"github.com/gin-gonic/gin"
)

func main() {
	log.Println("========================================")
	log.Println("  「炼丹炉」API 网关 启动中...")
	log.Println("========================================")

	// ---------- 1. 加载配置 ----------
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("[炼丹炉] 加载配置失败: %v", err)
	}

	// ---------- 2. 初始化日志 ----------
	_, err = middleware.InitLogger(cfg.Server.Mode)
	if err != nil {
		log.Printf("[炼丹炉] 初始化 zap 日志失败，使用默认日志: %v", err)
	} else {
		defer middleware.SyncLogger()
	}

	// ---------- 3. 初始化数据库 ----------
	if err := dao.InitDatabase(&cfg.Database); err != nil {
		log.Fatalf("[炼丹炉] 初始化数据库失败: %v", err)
	}
	defer dao.CloseDatabase()

	// 子命令: migrate / seed（InitDatabase 已包含自动迁移与内置金丹种子写入，均为幂等）
	if len(os.Args) > 1 && (os.Args[1] == "migrate" || os.Args[1] == "seed") {
		log.Printf("[炼丹炉] 子命令 %s 执行完成（迁移与种子数据均已就绪）", os.Args[1])
		return
	}

	// ---------- 4. 设置 Gin 模式 ----------
	gin.SetMode(cfg.Server.Mode)

	// ---------- 5. 创建 Gin 引擎 ----------
	r := gin.New()

	// ---------- 6. 注册全局中间件 ----------
	// 错误恢复（捕获 panic）
	r.Use(middleware.ErrorRecovery())
	// 日志记录
	r.Use(middleware.GinLogger())
	// 跨域支持
	r.Use(middleware.CORS())

	// ---------- 7. 初始化处理器 ----------
	pillHandler := handler.NewPillHandler()
	agentHandler := handler.NewAgentHandler()
	chatHandler := handler.NewChatHandler()
	systemHandler := handler.NewSystemHandler()
	trialHandler := handler.NewTrialHandler()

	// ---------- 8. 注册路由 ----------
	setupRoutes(r, pillHandler, agentHandler, chatHandler, systemHandler, trialHandler)

	// ---------- 9. 启动 HTTP 服务 ----------
	port := cfg.Server.Port
	if port == "" {
		port = "8080"
	}
	addr := fmt.Sprintf(":%s", port)

	srv := &http.Server{
		Addr:    addr,
		Handler: r,
	}

	// 在 goroutine 中启动服务，以便主线程可以监听关闭信号
	go func() {
		log.Printf("[炼丹炉] HTTP 服务已启动，监听地址: %s", addr)
		log.Printf("[炼丹炉] API 文档: http://localhost%s/api/v1/system/health", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("[炼丹炉] HTTP 服务启动失败: %v", err)
		}
	}()

	// ---------- 10. 优雅关闭 ----------
	// 监听系统信号（SIGINT, SIGTERM），实现优雅关闭
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("[炼丹炉] 正在关闭服务...")

	// 创建一个 30 秒超时的上下文用于关闭
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("[炼丹炉] 服务强制关闭: %v", err)
	}

	log.Println("[炼丹炉] 服务已安全关闭，期待下次再会")
}

// setupRoutes 注册所有 API 路由
// 按照模块分组，路径前缀统一为 /api/v1
func setupRoutes(
	r *gin.Engine,
	pill *handler.PillHandler,
	agent *handler.AgentHandler,
	chat *handler.ChatHandler,
	system *handler.SystemHandler,
	trial *handler.TrialHandler,
) {
	// API v1 根路径
	v1 := r.Group("/api/v1")

	// ---------- 金丹管理 ----------
	pills := v1.Group("/pills")
	{
		pills.GET("", pill.ListPills)         // 金丹列表
		pills.POST("", pill.CreatePill)       // 创建金丹
		pills.GET("/:id", pill.GetPill)       // 金丹详情
		pills.PUT("/:id", pill.UpdatePill)    // 更新金丹
		pills.DELETE("/:id", pill.DeletePill) // 删除金丹
	}

	// ---------- 道人管理 ----------
	agents := v1.Group("/agents")
	{
		agents.GET("", agent.ListAgents)                         // 道人列表
		agents.POST("", agent.CreateAgent)                       // 创建道人
		agents.GET("/:id", agent.GetAgent)                       // 道人详情
		agents.PUT("/:id", agent.UpdateAgent)                    // 更新道人
		agents.DELETE("/:id", agent.DeleteAgent)                 // 删除道人
		agents.POST("/:id/pills", agent.BindPill)                // 服用金丹
		agents.PUT("/:id/pills/:pill_id", agent.UpdateAgentPill) // 更新服用记录（权重/顺序）
		agents.DELETE("/:id/pills/:pill_id", agent.UnbindPill)   // 解除绑定
		agents.GET("/:id/pills", agent.ListAgentPills)           // 已服用金丹列表
	}

	// ---------- 对话管理 ----------
	chatGroup := v1.Group("/chat")
	{
		chatGroup.POST("/sessions", chat.CreateSession)           // 创建会话
		chatGroup.GET("/sessions", chat.ListSessions)             // 会话列表
		chatGroup.GET("/sessions/:id/messages", chat.GetMessages) // 消息历史
		chatGroup.GET("/ws/:session_id", chat.WebSocketChat)      // WebSocket 流式对话
	}

	// ---------- 试丹（临时组合预览） ----------
	trialGroup := v1.Group("/trial")
	{
		trialGroup.POST("/synthesis", trial.Synthesize) // 合成预览
		trialGroup.POST("/chat", trial.Chat)            // 临时对话（非流式）
	}

	// ---------- 系统接口 ----------
	sys := v1.Group("/system")
	{
		sys.GET("/health", system.HealthCheck) // 健康检查
		sys.GET("/config", system.GetConfig)   // 系统配置
	}

	// 404 和 405 处理
	r.NoRoute(middleware.NoRouteHandler())
	r.NoMethod(middleware.NoMethodHandler())

	log.Println("[炼丹炉] 所有路由已注册完毕")
}
