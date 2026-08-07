// Package config 负责管理「炼丹炉 · 金丹化性」的全局配置
// 使用 Viper 从环境变量读取配置，支持默认值，确保各层组件能正确初始化
package config

import (
	"fmt"
	"log"
	"strings"

	"github.com/spf13/viper"
)

// Config 是「炼丹炉」的全局配置总纲，包含数据库、LLM、服务器等全部配置项
type Config struct {
	// Database 数据库配置（PostgreSQL）
	Database DatabaseConfig `mapstructure:"database"`

	// LLM 大语言模型配置
	LLM LLMConfig `mapstructure:"llm"`

	// Server HTTP 服务配置
	Server ServerConfig `mapstructure:"server"`

	// PythonEngine Python 语言引擎服务地址
	PythonEngine PythonEngineConfig `mapstructure:"python_engine"`
}

// DatabaseConfig 数据库配置，用于连接 PostgreSQL 存储金丹、道人、语言模式等业务数据
type DatabaseConfig struct {
	Host     string `mapstructure:"host"`
	Port     int    `mapstructure:"port"`
	User     string `mapstructure:"user"`
	Password string `mapstructure:"password"`
	DBName   string `mapstructure:"dbname"`
	SSLMode  string `mapstructure:"sslmode"`
}

// LLMConfig 大语言模型配置，用于对话生成与语言模式合成
type LLMConfig struct {
	APIKey         string   `mapstructure:"api_key"`
	BaseURL        string   `mapstructure:"base_url"`
	DefaultModel   string   `mapstructure:"default_model"`
	SynthesisModel string   `mapstructure:"synthesis_model"`
	Models         []string `mapstructure:"models"` // 可用的模型列表
}

// ServerConfig HTTP 服务器配置
type ServerConfig struct {
	Port         string `mapstructure:"port"`
	Mode         string `mapstructure:"mode"` // debug / release
	AllowOrigins string `mapstructure:"allow_origins"`
}

// PythonEngineConfig Python 语言引擎配置，用于与 Python 服务通信
type PythonEngineConfig struct {
	BaseURL string `mapstructure:"base_url"`
}

// globalConfig 全局配置实例，初始化后只读
var globalConfig *Config

// Load 从环境变量加载配置，支持 .env 文件
// 初始化 Viper，设置前缀和默认值，然后解析到 Config 结构体
func Load() (*Config, error) {
	v := viper.New()

	// 设置环境变量前缀为 AF（Alchemy Furnace 的缩写）
	v.SetEnvPrefix("AF")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	// 读取 .env 文件（如果存在）
	v.SetConfigName(".env")
	v.SetConfigType("env")
	v.AddConfigPath(".")
	v.AddConfigPath("/app")
	// 忽略配置文件不存在的错误，环境变量优先
	_ = v.ReadInConfig()

	// 数据库默认配置
	v.SetDefault("database.host", getEnv(v, "DB_HOST", "localhost"))
	v.SetDefault("database.port", getEnvInt(v, "DB_PORT", 5432))
	v.SetDefault("database.user", getEnv(v, "DB_USER", "alchemy"))
	v.SetDefault("database.password", getEnv(v, "DB_PASSWORD", "alchemy123"))
	v.SetDefault("database.dbname", getEnv(v, "DB_NAME", "alchemy_db"))
	v.SetDefault("database.sslmode", getEnv(v, "DB_SSLMODE", "disable"))

	// LLM 默认配置
	v.SetDefault("llm.api_key", getEnv(v, "OPENAI_API_KEY", ""))
	v.SetDefault("llm.base_url", getEnv(v, "OPENAI_BASE_URL", "https://api.openai.com/v1"))
	v.SetDefault("llm.default_model", getEnv(v, "DEFAULT_MODEL", "gpt-4o"))
	v.SetDefault("llm.synthesis_model", getEnv(v, "SYNTHESIS_MODEL", "gpt-4o-mini"))
	v.SetDefault("llm.models", []string{"gpt-4o", "gpt-4o-mini", "deepseek-chat", "qwen-max"})

	// 服务器默认配置
	v.SetDefault("server.port", getEnv(v, "GO_PORT", "8080"))
	v.SetDefault("server.mode", getEnv(v, "GIN_MODE", "debug"))
	v.SetDefault("server.allow_origins", getEnv(v, "ALLOW_ORIGINS", "*"))

	// Python 语言引擎默认配置
	v.SetDefault("python_engine.base_url", getEnv(v, "PYTHON_ENGINE_BASE_URL", "http://localhost:8000"))

	// 解析配置到结构体
	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("解析配置失败: %w", err)
	}

	globalConfig = &cfg
	log.Printf("[炼丹炉] 配置加载成功，服务端口: %s, 数据库: %s:%d", cfg.Server.Port, cfg.Database.Host, cfg.Database.Port)
	return &cfg, nil
}

// Get 获取全局配置实例
func Get() *Config {
	if globalConfig == nil {
		log.Println("[炼丹炉] 警告: 配置未加载，尝试重新加载")
		cfg, err := Load()
		if err != nil {
			log.Fatalf("[炼丹炉] 配置加载失败: %v", err)
		}
		return cfg
	}
	return globalConfig
}

// DSN 生成 PostgreSQL 连接字符串（Data Source Name）
func (d *DatabaseConfig) DSN() string {
	return fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		d.Host, d.Port, d.User, d.Password, d.DBName, d.SSLMode)
}

// getEnv 从环境变量获取字符串值，若不存在则返回默认值
func getEnv(v *viper.Viper, key, defaultVal string) string {
	val := v.GetString(key)
	if val == "" {
		return defaultVal
	}
	return val
}

// getEnvInt 从环境变量获取整数值，若不存在或无效则返回默认值
func getEnvInt(v *viper.Viper, key string, defaultVal int) int {
	val := v.GetInt(key)
	if val == 0 {
		return defaultVal
	}
	return val
}
