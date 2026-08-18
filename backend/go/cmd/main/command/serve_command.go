// serve 子命令: 启动 HTTP 服务
// 启动 HTTP 服务(根命令无参时亦走此入口),由 web.Register 集中注册所有路由
package command

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/alchemy-furnace/server/internal/configuration"
	"github.com/alchemy-furnace/server/internal/dao"
	"github.com/alchemy-furnace/server/internal/logger"
	"github.com/alchemy-furnace/server/server/http/gateway/web"
	"github.com/gin-gonic/gin"
	"github.com/spf13/cobra"
)

// NewServeCommand serve 子命令
func NewServeCommand() *cobra.Command {
	return &cobra.Command{
		Use:  "serve",
		Args: cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			runServe(cmd)
		},
	}
}

// runServe 启动 HTTP 服务(根命令无参时亦走此入口)
func runServe(cmd *cobra.Command) {
	cfg := &configuration.Configuration

	gin.SetMode(cfg.Server.Mode)

	if err := dao.InitDatabase(&cfg.Database); err != nil {
		log.Fatalf("[炼丹炉] 初始化数据库失败: %v", err)
	}
	defer dao.CloseDatabase()
	// 零配置首次启动:空库自动 AutoMigrate；已有表或显式关闭时跳过。
	if err := dao.MaybeAutoMigrate(); err != nil {
		log.Fatalf("[炼丹炉] 自动建表失败: %v", err)
	}
	defer logger.Sync()
	// 共用引擎装配(serve/desktop 都走 web.NewEngine;此处无 guards 保持固定端口行为)
	r, err := web.NewEngine(false)
	if err != nil {
		log.Fatalf("[炼丹炉] 引擎装配失败: %v", err)
	}

	port := cfg.Server.Port
	if port == "" {
		port = "8080"
	}
	addr := fmt.Sprintf(":%s", port)
	srv := &http.Server{Addr: addr, Handler: r}

	go func() {
		log.Printf("[炼丹炉] HTTP 服务已启动,监听地址: %s", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("[炼丹炉] HTTP 服务启动失败: %v", err)
		}
	}()

	// 优雅关闭
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("[炼丹炉] 正在关闭服务...")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("[炼丹炉] 服务强制关闭: %v", err)
	}
	log.Println("[炼丹炉] 服务已安全关闭,期待下次再会")
}
