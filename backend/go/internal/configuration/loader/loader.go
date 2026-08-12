// Package loader 负责加载全局配置: config/main.toml + 环境变量覆盖
// 环境变量优先级最高,支持 AF_ 前缀(viper AutomaticEnv)与历史裸变量名(DB_HOST 等)
package loader

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/alchemy-furnace/server/internal/configuration"
	"github.com/spf13/viper"
)

// legacyEnv 历史裸环境变量名 → 配置键 的兼容映射(保证既有 .env / docker-compose 零修改可用)
var legacyEnv = map[string]string{
	"database.host":          "DB_HOST",
	"database.port":          "DB_PORT",
	"database.user":          "DB_USER",
	"database.password":      "DB_PASSWORD",
	"database.dbname":        "DB_NAME",
	"database.sslmode":       "DB_SSLMODE",
	"server.port":            "GO_PORT",
	"server.mode":            "GIN_MODE",
	"server.allow_origins":   "ALLOW_ORIGINS",
	"llm.api_key":            "OPENAI_API_KEY",
	"llm.base_url":           "OPENAI_BASE_URL",
	"llm.default_model":      "DEFAULT_MODEL",
	"llm.synthesis_model":    "SYNTHESIS_MODEL",
	"llm.fusion_model":       "FUSION_MODEL",
	"python_engine.base_url": "PYTHON_ENGINE_BASE_URL",
	"model_key_secret":       "MODEL_KEY_SECRET",
}

// LoadConfig 从 dir 目录读取 main.toml 并应用环境变量覆盖,填充 configuration.Configuration
// dir 缺省依次尝试: ./config、/app/config(Docker 镜像 WORKDIR)
func LoadConfig(dir string) error {
	// 演示模式开关(007-demo-mode): 先解析,后续服务启动时据此选 memory/gorm
	configuration.LoadDemoConfig()
	if configuration.IsDemo() {
		fmt.Println("[炼丹炉] 演示模式已开启 — 内存 mock,无 PostgreSQL")
	}

	v := viper.New()
	v.SetConfigName("main")
	v.SetConfigType("toml")
	if dir != "" {
		v.AddConfigPath(dir)
	} else {
		v.AddConfigPath("config")
		v.AddConfigPath("/app/config")
	}

	if err := v.ReadInConfig(); err != nil {
		var notFound viper.ConfigFileNotFoundError
		if !errors.As(err, &notFound) {
			return fmt.Errorf("读取配置文件失败: %w", err)
		}
		// 配置文件缺失时允许纯环境变量运行
		fmt.Println("[炼丹炉] 警告: 未找到 config/main.toml,使用纯环境变量/默认值配置")
	}

	// AF_ 前缀环境变量(AF_DATABASE_HOST 等)
	v.SetEnvPrefix("AF")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	// 历史裸变量名兼容层(仅当对应变量存在时覆盖)
	for key, envName := range legacyEnv {
		if val, ok := os.LookupEnv(envName); ok {
			v.Set(key, val)
		}
	}

	if err := v.Unmarshal(&configuration.Configuration); err != nil {
		return fmt.Errorf("解析配置失败: %w", err)
	}

	return nil
}
