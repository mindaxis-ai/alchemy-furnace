// Package model 定义「炼丹炉 · 金丹化性」的全部 GORM 数据模型
// 对应数据库表：金丹(elixir_pills)、道人(dao_agents)、服用记录(agent_pills)、
// 语言模式缓存(language_patterns)、对话会话(chat_sessions)、对话消息(chat_messages)
// 所有模型使用 GORM v2 标签，支持自动迁移
package model

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// ---------- 金丹（语言模式/人格特质技能包） ----------

// ElixirPill 金丹模型，对应 elixir_pills 表
// 金丹是一套可影响语言模式的结构化技能包，基于 nuwa-skill 的 SKILL.md 结构
// SkillSchema 存储于 PostgreSQL JSONB 中
type ElixirPill struct {
	ID          uint      `json:"id" gorm:"primaryKey;autoIncrement;comment:金丹唯一标识"`
	UUID        uuid.UUID `json:"-" gorm:"type:uuid;uniqueIndex;comment:对外标识"`
	Name        string    `json:"name" gorm:"size:100;not null;comment:金丹名称"`
	Description string    `json:"description" gorm:"type:text;comment:金丹简介（含触发语、反触发语）"`
	SkillSchema JSONMap   `json:"skill_schema" gorm:"type:jsonb;not null;comment:nuwa-skill 结构化内容"`
	Tags        JSONList  `json:"tags" gorm:"type:jsonb;comment:标签数组"`
	Author      string    `json:"author" gorm:"size:100;comment:作者"`
	Version     string    `json:"version" gorm:"size:20;default:1.0.0;comment:版本号"`
	IsBuiltin   bool      `json:"is_builtin" gorm:"default:false;index;comment:是否系统内置示例金丹"`
	CreatedAt   time.Time `json:"created_at" gorm:"autoCreateTime;comment:创建时间"`
	UpdatedAt   time.Time `json:"updated_at" gorm:"autoUpdateTime;comment:更新时间"`

	// 关联关系：一个金丹被多个道人服用
	AgentPills []AgentPill `json:"agent_pills,omitempty" gorm:"foreignKey:PillID;references:ID;constraint:OnDelete:CASCADE;"`
}

// TableName 指定表名
func (ElixirPill) TableName() string {
	return "elixir_pills"
}

// ---------- 道人（AI Agent） ----------

// DaoAgent 道人模型，对应 dao_agents 表
// 道人是 AI 对话代理，拥有基础性格，可服用多个金丹获得语言模式/人格特质
type DaoAgent struct {
	ID          uint      `json:"id" gorm:"primaryKey;autoIncrement;comment:道人唯一标识"`
	UUID        uuid.UUID `json:"-" gorm:"type:uuid;uniqueIndex;comment:对外标识"`
	Name        string    `json:"name" gorm:"size:100;not null;comment:道人名称"`
	Avatar      string    `json:"avatar" gorm:"size:255;comment:头像URL"`
	Personality string    `json:"personality" gorm:"type:text;comment:基础性格描述/系统提示词"`
	ModelName   string    `json:"model_name" gorm:"size:50;default:gpt-4o;comment:使用的LLM模型名称"`
	Status      string    `json:"status" gorm:"size:20;default:active;comment:状态: active(活跃)/inactive(停用)"`
	CreatedAt   time.Time `json:"created_at" gorm:"autoCreateTime;comment:创建时间"`

	// 关联关系：一个道人服用多个金丹
	AgentPills []AgentPill `json:"agent_pills,omitempty" gorm:"foreignKey:AgentID;references:ID;constraint:OnDelete:CASCADE;"`
	// 关联关系：一个道人参与多个会话
	Sessions []ChatSession `json:"sessions,omitempty" gorm:"foreignKey:AgentID;references:ID;constraint:OnDelete:CASCADE;"`
	// 关联关系：一个道人有一个语言模式缓存
	LanguagePattern *LanguagePattern `json:"language_pattern,omitempty" gorm:"foreignKey:AgentID;references:ID;constraint:OnDelete:CASCADE;"`
}

// TableName 指定表名
func (DaoAgent) TableName() string {
	return "dao_agents"
}

// ---------- 服用记录（Agent 绑定金丹） ----------

// AgentPill 服用记录模型，对应 agent_pills 表
// 记录道人与金丹的绑定关系，支持权重和服用顺序
// agent_id 和 pill_id 联合唯一
type AgentPill struct {
	ID        uint      `json:"id" gorm:"primaryKey;autoIncrement;comment:服用记录唯一标识"`
	AgentID   uint      `json:"agent_id" gorm:"not null;uniqueIndex:idx_agent_pill;index;comment:道人ID"`
	PillID    uint      `json:"pill_id" gorm:"not null;uniqueIndex:idx_agent_pill;index;comment:金丹ID"`
	Weight    float64   `json:"weight" gorm:"default:1.0;comment:剂量/权重(0-10)"`
	SortOrder int       `json:"sort_order" gorm:"default:0;comment:服用顺序"`
	CreatedAt time.Time `json:"created_at" gorm:"autoCreateTime;comment:服用时间"`

	// 关联关系
	Agent DaoAgent   `json:"agent,omitempty" gorm:"foreignKey:AgentID;references:ID"`
	Pill  ElixirPill `json:"pill,omitempty" gorm:"foreignKey:PillID;references:ID"`
}

// TableName 指定表名
func (AgentPill) TableName() string {
	return "agent_pills"
}

// ---------- 语言模式缓存 ----------

// LanguagePattern 语言模式缓存模型，对应 language_patterns 表
// 缓存每个道人合成后的系统提示词与涌现规则，避免每次对话重复合成
// 当道人性格、服用金丹或金丹内容变化时失效/重建
type LanguagePattern struct {
	ID                uint      `json:"id" gorm:"primaryKey;autoIncrement;comment:缓存唯一标识"`
	AgentID           uint      `json:"agent_id" gorm:"not null;uniqueIndex;comment:关联道人ID"`
	SystemPrompt      string    `json:"system_prompt" gorm:"type:text;not null;comment:合成后的系统提示词"`
	EmergenceRules    JSONList  `json:"emergence_rules" gorm:"type:jsonb;comment:涌现规则列表"`
	InnerTensions     JSONList  `json:"inner_tensions" gorm:"type:jsonb;comment:检测到的内在冲突"`
	SourceFingerprint string    `json:"source_fingerprint" gorm:"size:80;not null;comment:来源指纹(sha256: 前缀 + 64 位 hex = 71 字符)"`
	IsValid           bool      `json:"is_valid" gorm:"default:true;comment:是否有效"`
	CreatedAt         time.Time `json:"created_at" gorm:"autoCreateTime;comment:创建时间"`
	UpdatedAt         time.Time `json:"updated_at" gorm:"autoUpdateTime;comment:更新时间"`

	// 关联关系
	Agent DaoAgent `json:"agent,omitempty" gorm:"foreignKey:AgentID;references:ID"`
}

// TableName 指定表名
func (LanguagePattern) TableName() string {
	return "language_patterns"
}

// ---------- 对话会话 ----------

// ChatSession 对话会话模型，对应 chat_sessions 表
// 用户与某个道人之间的对话上下文，一个会话包含多条消息
type ChatSession struct {
	ID        uint      `json:"id" gorm:"primaryKey;autoIncrement;comment:会话唯一标识"`
	UUID        uuid.UUID `json:"-" gorm:"type:uuid;uniqueIndex;comment:对外标识"`
	AgentID   uint      `json:"agent_id" gorm:"not null;index;comment:关联的道人ID"`
	Title     string    `json:"title" gorm:"size:200;comment:会话标题"`
	CreatedAt time.Time `json:"created_at" gorm:"autoCreateTime;comment:创建时间"`
	UpdatedAt time.Time `json:"updated_at" gorm:"autoUpdateTime;comment:更新时间"`

	// 关联关系
	Agent    DaoAgent      `json:"agent,omitempty" gorm:"foreignKey:AgentID;references:ID"`
	Messages []ChatMessage `json:"messages,omitempty" gorm:"foreignKey:SessionID;references:ID;constraint:OnDelete:CASCADE;"`
}

// TableName 指定表名
func (ChatSession) TableName() string {
	return "chat_sessions"
}

// ---------- 对话消息 ----------

// ChatMessage 对话消息模型，对应 chat_messages 表
// 存储用户与道人的对话内容
// role: user(用户提问) / assistant(道人回答) / system(系统提示)
// sources 字段已废弃，保留 JSONB 列以兼容历史数据，不再写入新数据
type ChatMessage struct {
	ID        uint      `json:"id" gorm:"primaryKey;autoIncrement;comment:消息唯一标识"`
	UUID        uuid.UUID `json:"-" gorm:"type:uuid;uniqueIndex;comment:对外标识"`
	SessionID uint      `json:"session_id" gorm:"not null;index;comment:所属会话ID"`
	Role      string    `json:"role" gorm:"size:20;not null;comment:角色: user/assistant/system"`
	Content   string    `json:"content" gorm:"type:text;not null;comment:消息内容"`
	Sources   JSONMap   `json:"sources,omitempty" gorm:"type:jsonb;comment:废弃: 原RAG引用来源(JSONB格式)"`
	CreatedAt time.Time `json:"created_at" gorm:"autoCreateTime;comment:创建时间"`

	// 关联关系
	Session ChatSession `json:"session,omitempty" gorm:"foreignKey:SessionID;references:ID"`
}

// TableName 指定表名
func (ChatMessage) TableName() string {
	return "chat_messages"
}

// ---------- LLM 供应商配置 ----------

// LLMProvider 供应商配置，对应 llm_providers 表
// 供应商是协议 + Base URL + 加密 API Key 的唯一持有者；api_key 以 AES-GCM 加密存储
// 停用供应商后其下全部模型在凭证解析链中不可用
type LLMProvider struct {
	ID              uint      `json:"id" gorm:"primaryKey;autoIncrement;comment:供应商唯一标识"`
	UUID              uuid.UUID `json:"-" gorm:"type:uuid;uniqueIndex;comment:对外标识"`
	Name            string    `json:"name" gorm:"size:50;not null;uniqueIndex;comment:供应商标识（如 openai/deepseek/dashscope）"`
	DisplayName     string    `json:"display_name" gorm:"size:100;not null;comment:显示名（如 OpenAI/通义千问）"`
	Protocol        string    `json:"protocol" gorm:"size:50;not null;default:openai-compatible;comment:协议类型（预留扩展）"`
	BaseURL         string    `json:"base_url" gorm:"size:255;not null;comment:OpenAI 兼容接口地址"`
	APIKeyEncrypted string    `json:"-" gorm:"type:text;comment:AES-GCM 加密后的 api_key（空=免密钥本地服务）"`
	IsEnabled       bool      `json:"is_enabled" gorm:"default:true;index;comment:是否启用"`
	SortOrder       int       `json:"sort_order" gorm:"default:0;comment:展示顺序"`
	Remark          string    `json:"remark" gorm:"size:255;default:'';comment:备注"`
	CreatedAt       time.Time `json:"created_at" gorm:"autoCreateTime;comment:创建时间"`
	UpdatedAt       time.Time `json:"updated_at" gorm:"autoUpdateTime;comment:更新时间"`

	// 关联关系：一个供应商下有多个模型
	Models []LLMModel `json:"models,omitempty" gorm:"foreignKey:ProviderID;references:ID"`
}

// TableName 指定表名
func (LLMProvider) TableName() string {
	return "llm_providers"
}

// ---------- LLM 模型配置 ----------

// LLMModel 模型配置，对应 llm_models 表
// 模型归属供应商（provider_id 外键），仅声明模型名与生成参数，凭证由供应商持有
// (provider_id, name) 联合唯一；is_default / is_synthesis 全表最多一个（由部分唯一索引保证）
type LLMModel struct {
	ID          uint      `json:"id" gorm:"primaryKey;autoIncrement;comment:模型配置唯一标识"`
	UUID        uuid.UUID `json:"-" gorm:"type:uuid;uniqueIndex;comment:对外标识"`
	ProviderID  uint      `json:"provider_id" gorm:"not null;index:idx_llm_models_provider_id;uniqueIndex:idx_llm_models_provider_name;comment:所属供应商ID"`
	Name        string    `json:"name" gorm:"size:100;not null;uniqueIndex:idx_llm_models_provider_name;comment:模型名（API 调用用，如 gpt-4o）"`
	DisplayName string    `json:"display_name" gorm:"size:100;not null;comment:显示名"`
	Temperature float64   `json:"temperature" gorm:"default:0.7;comment:默认温度(0-2)"`
	MaxTokens   int       `json:"max_tokens" gorm:"default:4096;comment:默认最大 token"`
	IsEnabled   bool      `json:"is_enabled" gorm:"default:true;index;comment:是否启用"`
	IsDefault   bool      `json:"is_default" gorm:"default:false;comment:是否默认模型（全表最多一个）"`
	IsSynthesis bool      `json:"is_synthesis" gorm:"default:false;comment:是否语言模式合成专用模型（全表最多一个）"`
	IsFusion    bool      `json:"is_fusion" gorm:"default:false;comment:是否金丹融合专用模型（全表最多一个）"`
	SortOrder   int       `json:"sort_order" gorm:"default:0;comment:展示顺序"`
	CreatedAt   time.Time `json:"created_at" gorm:"autoCreateTime;comment:创建时间"`
	UpdatedAt   time.Time `json:"updated_at" gorm:"autoUpdateTime;comment:更新时间"`

	// 关联关系
	Provider LLMProvider `json:"provider,omitempty" gorm:"foreignKey:ProviderID;references:ID"`
}

// TableName 指定表名
func (LLMModel) TableName() string {
	return "llm_models"
}

// ---------- JSONB 支持 ----------

// JSONMap 是 map[string]interface{} 的包装类型，用于支持 PostgreSQL 的 JSONB 字段
type JSONMap map[string]interface{}

// Value 实现 driver.Valuer 接口，将 JSONMap 转换为 JSON 字符串存入数据库
func (j JSONMap) Value() (driver.Value, error) {
	if j == nil {
		return "{}", nil
	}
	bytes, err := json.Marshal(j)
	if err != nil {
		return nil, err
	}
	return string(bytes), nil
}

// Scan 实现 sql.Scanner 接口，从数据库 JSON 字符串扫描为 JSONMap
func (j *JSONMap) Scan(value interface{}) error {
	if value == nil {
		*j = JSONMap{}
		return nil
	}
	var bytes []byte
	switch v := value.(type) {
	case []byte:
		bytes = v
	case string:
		bytes = []byte(v)
	default:
		return errors.New("不支持的 JSONB 扫描类型")
	}
	if len(bytes) == 0 {
		*j = JSONMap{}
		return nil
	}
	return json.Unmarshal(bytes, j)
}

// JSONList 是 []interface{} 的包装类型，用于支持 PostgreSQL 的 JSONB 数组字段
type JSONList []interface{}

// Value 实现 driver.Valuer 接口
func (j JSONList) Value() (driver.Value, error) {
	if j == nil {
		return "[]", nil
	}
	bytes, err := json.Marshal(j)
	if err != nil {
		return nil, err
	}
	return string(bytes), nil
}

// Scan 实现 sql.Scanner 接口
func (j *JSONList) Scan(value interface{}) error {
	if value == nil {
		*j = JSONList{}
		return nil
	}
	var bytes []byte
	switch v := value.(type) {
	case []byte:
		bytes = v
	case string:
		bytes = []byte(v)
	default:
		return errors.New("不支持的 JSONB 扫描类型")
	}
	if len(bytes) == 0 {
		*j = JSONList{}
		return nil
	}
	return json.Unmarshal(bytes, j)
}

// ---------- 请求/响应 DTO ----------

// CreatePillRequest 创建金丹请求
type CreatePillRequest struct {
	Name        string   `json:"name" binding:"required,max=100"` // 金丹名称
	Description string   `json:"description"`                     // 金丹简介（含触发语、反触发语）
	SkillSchema JSONMap  `json:"skill_schema" binding:"required"` // nuwa-skill 结构化内容
	Tags        JSONList `json:"tags"`                            // 标签数组
	Author      string   `json:"author" binding:"max=100"`        // 作者
	Version     string   `json:"version" binding:"max=20"`        // 版本号
}

// UpdatePillRequest 更新金丹请求
type UpdatePillRequest struct {
	Name        string   `json:"name" binding:"max=100"`   // 金丹名称
	Description string   `json:"description"`              // 金丹简介
	SkillSchema JSONMap  `json:"skill_schema"`             // nuwa-skill 结构化内容
	Tags        JSONList `json:"tags"`                     // 标签数组
	Author      string   `json:"author" binding:"max=100"` // 作者
	Version     string   `json:"version" binding:"max=20"` // 版本号
}

// CreateAgentRequest 创建道人请求
type CreateAgentRequest struct {
	Name        string `json:"name" binding:"required,max=100"` // 道人名称
	Avatar      string `json:"avatar"`                          // 头像URL
	Personality string `json:"personality"`                     // 基础性格描述/系统提示词
	ModelName   string `json:"model_name" binding:"max=50"`     // 使用的LLM模型
}

// UpdateAgentRequest 更新道人请求
type UpdateAgentRequest struct {
	Name        string `json:"name" binding:"max=100"`                           // 道人名称
	Avatar      string `json:"avatar"`                                           // 头像URL
	Personality string `json:"personality"`                                      // 基础性格描述/系统提示词
	ModelName   string `json:"model_name" binding:"max=50"`                      // 使用的LLM模型
	Status      string `json:"status" binding:"omitempty,oneof=active inactive"` // 状态
}

// BindPillRequest 服用金丹请求
type BindPillRequest struct {
	PillID    uint    `json:"pill_id" binding:"required"`    // 金丹ID
	Weight    float64 `json:"weight" binding:"gte=0,lte=10"` // 剂量/权重
	SortOrder int     `json:"sort_order" binding:"gte=0"`    // 服用顺序
}

// CreateSessionRequest 创建会话请求
type CreateSessionRequest struct {
	AgentID uint   `json:"agent_id" binding:"required"` // 道人ID
	Title   string `json:"title" binding:"max=200"`     // 会话标题
}

// ChatMessageRequest 聊天消息请求结构
type ChatMessageRequest struct {
	Content string `json:"content" binding:"required"` // 消息内容
}

// HealthCheckResponse 健康检查响应
type HealthCheckResponse struct {
	Status       string `json:"status"`        // 状态: ok/degraded/down
	Version      string `json:"version"`       // 版本号
	Timestamp    int64  `json:"timestamp"`     // 时间戳
	DB           string `json:"db"`            // 数据库状态
	PythonEngine string `json:"python_engine"` // Python 语言引擎状态
}

// ---------- 供应商管理 DTO ----------

// ProviderResponse 供应商配置响应（api_key 永不明文返回，仅返回掩码）
type ProviderResponse struct {
	ID           uint      `json:"id"`
	Name         string    `json:"name"`
	DisplayName  string    `json:"display_name"`
	Protocol     string    `json:"protocol"`
	BaseURL      string    `json:"base_url"`
	APIKeyMasked string    `json:"api_key_masked"` // 掩码形式，如 sk-****wxyz
	HasAPIKey    bool      `json:"has_api_key"`    // 是否已配置 api_key
	IsEnabled    bool      `json:"is_enabled"`
	SortOrder    int       `json:"sort_order"`
	Remark       string    `json:"remark"`
	ModelCount   int64     `json:"model_count"` // 该供应商下的模型数量
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// CreateProviderRequest 创建供应商请求
type CreateProviderRequest struct {
	Name        string `json:"name" binding:"required,max=50"`          // 供应商标识（唯一）
	DisplayName string `json:"display_name" binding:"required,max=100"` // 显示名
	Protocol    string `json:"protocol" binding:"omitempty,max=50"`     // 协议类型，缺省 openai-compatible
	BaseURL     string `json:"base_url" binding:"required,max=255"`     // OpenAI 兼容接口地址
	APIKey      string `json:"api_key"`                                 // 明文 api_key（仅写入时传输）
	IsEnabled   *bool  `json:"is_enabled"`                              // 是否启用，缺省 true
	SortOrder   int    `json:"sort_order"`                              // 展示顺序
	Remark      string `json:"remark" binding:"max=255"`                // 备注
}

// UpdateProviderRequest 更新供应商请求（指针字段区分「未传」与「置空/置零」）
// api_key: 不传(nil)=不修改，传空字符串=清除密钥，传值=重新加密存储
type UpdateProviderRequest struct {
	Name        *string `json:"name" binding:"omitempty,max=50"`
	DisplayName *string `json:"display_name" binding:"omitempty,max=100"`
	Protocol    *string `json:"protocol" binding:"omitempty,max=50"`
	BaseURL     *string `json:"base_url" binding:"omitempty,max=255"`
	APIKey      *string `json:"api_key"`
	IsEnabled   *bool   `json:"is_enabled"`
	SortOrder   *int    `json:"sort_order"`
	Remark      *string `json:"remark" binding:"omitempty,max=255"`
}

// TestConnectionRequest 供应商连接测试请求（model 可选，缺省用该供应商下第一个启用模型）
type TestConnectionRequest struct {
	Model string `json:"model"`
}

// ---------- 模型管理 DTO ----------

// LLMModelResponse 模型配置响应（凭证在供应商上，模型仅含供应商引用信息）
type LLMModelResponse struct {
	ID                  uint      `json:"id"`
	ProviderID          uint      `json:"provider_id"`
	Name                string    `json:"name"`
	DisplayName         string    `json:"display_name"`
	ProviderName        string    `json:"provider_name"`         // 所属供应商标识
	ProviderDisplayName string    `json:"provider_display_name"` // 所属供应商显示名
	Temperature         float64   `json:"temperature"`
	MaxTokens           int       `json:"max_tokens"`
	IsEnabled           bool      `json:"is_enabled"`
	IsDefault           bool      `json:"is_default"`
	IsSynthesis         bool      `json:"is_synthesis"`
	IsFusion            bool      `json:"is_fusion"`
	SortOrder           int       `json:"sort_order"`
	ReferencedBy        int64     `json:"referenced_by"` // 引用该模型的道人数量
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`
}

// LLMModelOption 道人表单下拉用的精简模型项
type LLMModelOption struct {
	Name                string `json:"name"`
	DisplayName         string `json:"display_name"`
	ProviderName        string `json:"provider_name"`         // 所属供应商标识
	ProviderDisplayName string `json:"provider_display_name"` // 所属供应商显示名
	IsDefault           bool   `json:"is_default"`
}

// CreateLLMModelRequest 创建模型请求（供应商由嵌套路径 :id 提供）
type CreateLLMModelRequest struct {
	Name        string  `json:"name" binding:"required,max=100"`         // 模型名（同供应商下唯一）
	DisplayName string  `json:"display_name" binding:"required,max=100"` // 显示名
	Temperature float64 `json:"temperature"`                             // 默认温度(0-2)，0 值视为默认 0.7
	MaxTokens   int     `json:"max_tokens"`                              // 默认最大 token，0 值视为默认 4096
	IsEnabled   *bool   `json:"is_enabled"`                              // 是否启用，缺省 true
	IsDefault   bool    `json:"is_default"`                              // 是否默认模型
	IsSynthesis bool    `json:"is_synthesis"`                            // 是否合成专用模型
	IsFusion    bool    `json:"is_fusion"`                               // 是否金丹融合专用模型
	SortOrder   int     `json:"sort_order"`                              // 展示顺序
}

// UpdateLLMModelRequest 更新模型请求（指针字段区分「未传」与「置空/置零」）
type UpdateLLMModelRequest struct {
	Name        *string  `json:"name" binding:"omitempty,max=100"`
	DisplayName *string  `json:"display_name" binding:"omitempty,max=100"`
	Temperature *float64 `json:"temperature"`
	MaxTokens   *int     `json:"max_tokens"`
	IsEnabled   *bool    `json:"is_enabled"`
	IsDefault   *bool    `json:"is_default"`
	IsSynthesis *bool    `json:"is_synthesis"`
	IsFusion    *bool    `json:"is_fusion"`
	SortOrder   *int     `json:"sort_order"`
}

// TestConnectionResult 模型连接测试结果
type TestConnectionResult struct {
	Success   bool   `json:"success"`    // 是否连通
	LatencyMs int64  `json:"latency_ms"` // 耗时（毫秒）
	Error     string `json:"error"`      // 失败时的可读中文描述
}

// ---------- UUID 兜底钩子 ----------
// PG schema 由 migration SQL 负责(列默认 gen_random_uuid());GORM 标签不带 default,
// 以便 sqlite 测试库可 AutoMigrate;应用层 BeforeCreate 统一兜底生成

func (m *ElixirPill) BeforeCreate(tx *gorm.DB) error {
	if m.UUID == uuid.Nil {
		m.UUID = uuid.New()
	}
	return nil
}

func (m *DaoAgent) BeforeCreate(tx *gorm.DB) error {
	if m.UUID == uuid.Nil {
		m.UUID = uuid.New()
	}
	return nil
}

func (m *ChatSession) BeforeCreate(tx *gorm.DB) error {
	if m.UUID == uuid.Nil {
		m.UUID = uuid.New()
	}
	return nil
}

func (m *ChatMessage) BeforeCreate(tx *gorm.DB) error {
	if m.UUID == uuid.Nil {
		m.UUID = uuid.New()
	}
	return nil
}

func (m *LLMProvider) BeforeCreate(tx *gorm.DB) error {
	if m.UUID == uuid.Nil {
		m.UUID = uuid.New()
	}
	return nil
}

func (m *LLMModel) BeforeCreate(tx *gorm.DB) error {
	if m.UUID == uuid.Nil {
		m.UUID = uuid.New()
	}
	return nil
}
