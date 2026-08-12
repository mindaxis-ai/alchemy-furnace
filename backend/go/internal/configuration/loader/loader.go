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
	"database.driver":        "DB_DRIVER",
	"database.host":          "DB_HOST",
	"database.port":          "DB_PORT",
	"database.user":          "DB_USER",
	"database.password":      "DB_PASSWORD",
	"database.dbname":        "DB_NAME",
	"database.sslmode":       "DB_SSLMODE",
	"database.sqlite_path":   "DB_SQLITE_PATH",
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

	// 多数据库支持: 智能补全 Driver(向后兼容 + 零配置降级)
	resolveDriver(&configuration.Configuration.Database)

	return nil
}

// resolveDriver 智能选择数据库驱动
// 优先级: 显式 driver > 有 host 配置 → postgres > 其他 → sqlite(零配置降级)
// SQLite 路径为空时默认 ./data/alchemy.db
// 同时校验显式 driver 取值的合法性,未知驱动直接报错
func resolveDriver(d *configuration.DatabaseConfig) {
	v := strings.ToLower(strings.TrimSpace(d.Driver))
	switch v {
	case "":
		// 未显式配置: 有 host 走 postgres(向后兼容旧部署),否则 sqlite(零门槛)
		if strings.TrimSpace(d.Host) != "" {
			d.Driver = configuration.DriverPostgres
		} else {
			d.Driver = configuration.DriverSQLite
		}
	case configuration.DriverPostgres, configuration.DriverMySQL, configuration.DriverSQLite:
		d.Driver = v
	default:
		// 显式给了非法值: 不静默,启动期直接报错暴露配置错误
		fmt.Printf("[炼丹炉] ❌ 未知的 database.driver=%q,合法值: postgres / mysql / sqlite\n", d.Driver)
		os.Exit(1)
	}

	if d.Driver == configuration.DriverSQLite && strings.TrimSpace(d.SQLitePath) == "" {
		d.SQLitePath = "./data/alchemy.db"
	}
}
