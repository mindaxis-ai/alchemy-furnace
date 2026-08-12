// Package configuration 定义「炼丹炉」全局配置对象
// 配置来源优先级: 环境变量(AF_ 前缀或兼容裸变量) > config/main.toml > 结构体零值
// 由 internal/configuration/loader.LoadConfig 在进程启动时填充
package configuration

import (
	"fmt"
	"os"
	"strings"
)

// Configuration 全局配置实例,启动时加载后只读
var Configuration Config

// Config 全局配置总纲
type Config struct {
	// Debug 调试开关(预留,对齐模板)
	Debug bool `toml:"debug" mapstructure:"debug"`

	// Server HTTP 服务配置
	Server ServerConfig `toml:"server" mapstructure:"server"`

	// Database PostgreSQL 配置
	Database DatabaseConfig `toml:"database" mapstructure:"database"`

	// LLM 大语言模型默认配置(供应商/模型种子的来源)
	LLM LLMConfig `toml:"llm" mapstructure:"llm"`

	// PythonEngine Python 语言引擎地址
	PythonEngine PythonEngineConfig `toml:"python_engine" mapstructure:"python_engine"`

	// ModelKeySecret 模型 API Key 加密密钥(AES-GCM,经 SHA256 归一化为 32 字节)
	// 环境变量 MODEL_KEY_SECRET;为空时无法存储/解密模型 API Key
	ModelKeySecret string `toml:"model_key_secret" mapstructure:"model_key_secret"`
}

// ServerConfig HTTP 服务器配置
type ServerConfig struct {
	Port         string `toml:"port" mapstructure:"port"`                   // 监听端口
	Mode         string `toml:"mode" mapstructure:"mode"`                   // debug / release
	AllowOrigins string `toml:"allow_origins" mapstructure:"allow_origins"` // CORS 允许来源
}

// DatabaseConfig PostgreSQL 连接配置
type DatabaseConfig struct {
	Host     string `toml:"host" mapstructure:"host"`
	Port     int    `toml:"port" mapstructure:"port"`
	User     string `toml:"user" mapstructure:"user"`
	Password string `toml:"password" mapstructure:"password"`
	DBName   string `toml:"dbname" mapstructure:"dbname"`
	SSLMode  string `toml:"sslmode" mapstructure:"sslmode"`
}

// DSN 生成 PostgreSQL 连接字符串
func (d *DatabaseConfig) DSN() string {
	return fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		d.Host, d.Port, d.User, d.Password, d.DBName, d.SSLMode)
}

// LLMConfig 大语言模型默认配置(对话生成与语言模式合成的种子来源)
type LLMConfig struct {
	APIKey         string   `toml:"api_key" mapstructure:"api_key"`
	BaseURL        string   `toml:"base_url" mapstructure:"base_url"`
	DefaultModel   string   `toml:"default_model" mapstructure:"default_model"`
	SynthesisModel string   `toml:"synthesis_model" mapstructure:"synthesis_model"`
	FusionModel    string   `toml:"fusion_model" mapstructure:"fusion_model"`
	Models         []string `toml:"models" mapstructure:"models"`
}

// PythonEngineConfig Python 语言引擎配置
type PythonEngineConfig struct {
	BaseURL string `toml:"base_url" mapstructure:"base_url"`
}

// DemoConfig 演示模式配置(007-demo-mode)
// 由环境变量 DEMO_MODE 控制,接受 true/1/yes/demo(大小写不敏感)
type DemoConfig struct {
	// Enabled 演示模式开关(从 env DEMO_MODE 解析)
	Enabled bool
}

// demoConfig 全局只读,启动期由 LoadDemoConfig 写入
var demoConfig = DemoConfig{Enabled: false}

// IsDemo 报告当前是否处于演示模式
func IsDemo() bool { return demoConfig.Enabled }

// Mode 返回可读模式字符串("demo" / "real"),供日志与 HTTP 响应使用
func Mode() string {
	if IsDemo() {
		return "demo"
	}
	return "real"
}

// LoadDemoConfig 从环境变量解析演示模式开关(007-demo-mode)
// 接受 true/1/yes/demo(大小写不敏感),其他值视为 false
// 由 configuration/loader 在启动期调用一次
func LoadDemoConfig() {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("DEMO_MODE")))
	demoConfig.Enabled = v == "true" || v == "1" || v == "yes" || v == "demo"
}
