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

	// Database 多数据库配置(postgres / mysql / sqlite,详见 DatabaseConfig)
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

// 数据库驱动常量(多数据库支持:由 DatabaseConfig.Driver 选择)
const (
	DriverPostgres = "postgres" // 生产推荐:支持 JSONB/部分唯一索引/UUID 原生
	DriverMySQL    = "mysql"    // 兼容性广;需 MySQL 8.0.13+ 才能利用部分唯一索引
	DriverSQLite   = "sqlite"   // 零依赖,单文件;Demo / 试用 / 单机首选
)

// DatabaseConfig 多数据库连接配置
//   - Postgres: 填写 host/port/user/password/dbname/sslmode
//   - MySQL:    填写 host/port/user/password/dbname;SSLMode 字段忽略
//   - SQLite:   填写 sqlite_path(单文件路径);其他连接字段忽略
//
// Driver 为空时由 loader.ResolveDriver 智能补全:有 host → postgres;否则 sqlite
type DatabaseConfig struct {
	// Driver 驱动名(postgres / mysql / sqlite),空时由 loader 智能选择
	Driver string `toml:"driver" mapstructure:"driver"`

	// Postgres / MySQL 共用字段
	Host     string `toml:"host" mapstructure:"host"`
	Port     int    `toml:"port" mapstructure:"port"`
	User     string `toml:"user" mapstructure:"user"`
	Password string `toml:"password" mapstructure:"password"`
	DBName   string `toml:"dbname" mapstructure:"dbname"`
	SSLMode  string `toml:"sslmode" mapstructure:"sslmode"` // Postgres 专用(MySQL 忽略)

	// SQLite 专用:单文件路径(相对路径以程序 cwd 解析)
	// 默认 ./data/alchemy.db(loader 兜底);目录不存在由 InitDatabase 自动创建
	SQLitePath string `toml:"sqlite_path" mapstructure:"sqlite_path"`
}

// DSN 按 Driver 返回对应驱动可识别的连接字符串
//   - sqlite:    file:./data/alchemy.db?_loc=Local&_fk=1
//                (_fk=1 启用外键约束,让 OnDelete:CASCADE 真正生效)
//   - postgres:  host=... port=... user=... password=... dbname=... sslmode=...
//   - mysql:     user:pass@tcp(host:port)/dbname?charset=utf8mb4&parseTime=True&loc=Local
// 驱动未识别时降级返回空串,由调用方报错
func (d *DatabaseConfig) DSN() string {
	switch d.Driver {
	case DriverSQLite:
		path := d.SQLitePath
		if path == "" {
			path = "./data/alchemy.db"
		}
		// _fk=1: 启用 SQLite 外键约束(GORM 的 OnDelete:CASCADE 才生效)
		// _loc=Local: 时间字段按本地时区解析(配合 GORM autoCreateTime)
		return fmt.Sprintf("file:%s?_loc=Local&_fk=1", path)
	case DriverPostgres:
		return fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
			d.Host, d.Port, d.User, d.Password, d.DBName, d.SSLMode)
	case DriverMySQL:
		return fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=utf8mb4&parseTime=True&loc=Local",
			d.User, d.Password, d.Host, d.Port, d.DBName)
	default:
		return ""
	}
}

// IsSQLite 便捷判断(SQLite 走单连接 + 串行写,需要差异化处理)
func (d *DatabaseConfig) IsSQLite() bool { return d.Driver == DriverSQLite }

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
