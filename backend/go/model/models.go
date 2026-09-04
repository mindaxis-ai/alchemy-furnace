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
	SkillSchema JSONMap   `json:"skill_schema" gorm:"not null;serializer:json;comment:nuwa-skill 结构化内容"`
	Tags        JSONList  `json:"tags" gorm:"serializer:json;comment:标签数组"`
	Author      string    `json:"author" gorm:"size:100;comment:作者"`
	Version     string    `json:"version" gorm:"size:20;default:1.0.0;comment:版本号"`
	IsBuiltin   bool      `json:"is_builtin" gorm:"default:false;index;comment:是否系统内置示例金丹"`
	CreatedAt   time.Time `json:"created_at" gorm:"autoCreateTime;comment:创建时间"`
	UpdatedAt   time.Time `json:"updated_at" gorm:"autoUpdateTime;comment:更新时间"`

	// 关联关系：一个金丹被多个道人服用
	AgentPills []AgentPill `json:"agent_pills,omitempty" gorm:"foreignKey:PillID;references:UUID;constraint:OnDelete:CASCADE;"`
}

// TableName 指定表名
func (ElixirPill) TableName() string {
	return "elixir_pills"
}

// ---------- 道人（AI Agent） ----------

// DaoAgent 道人模型，对应 dao_agents 表
// 道人是 AI 对话代理，拥有基础性格，可服用多个金丹获得语言模式/人格特质
type DaoAgent struct {
	ID            uint      `json:"id" gorm:"primaryKey;autoIncrement;comment:道人唯一标识"`
	UUID          uuid.UUID `json:"-" gorm:"type:uuid;uniqueIndex;comment:对外标识"`
	Name          string    `json:"name" gorm:"size:100;not null;comment:道人名称"`
	Avatar        string    `json:"avatar" gorm:"type:text;comment:头像 URL 或 data:image 数据 URI(≤1.5M 字符)"`
	Personality   string    `json:"personality" gorm:"type:text;comment:基础性格描述/系统提示词"`
	ModelName     string    `json:"model_name" gorm:"size:50;default:gpt-4o;comment:使用的LLM模型名称"`
	Status        string    `json:"status" gorm:"size:20;default:active;comment:状态: active(活跃)/inactive(停用)"`
	Proactivity   int       `json:"proactivity" gorm:"default:50;comment:主动性/表达欲(0-100,群聊发言欲)"`
	MemoryEnabled bool      `json:"memory_enabled" gorm:"not null;default:true;comment:是否启用本地记忆(检索/蒸馏)"`
	// EffectsRevision 能力编排版本：服用/移除/调权重顺序时同事务加一，用于缓存并发保护
	EffectsRevision int       `json:"-" gorm:"not null;default:0;comment:能力编排版本(单调递增)"`
	CreatedAt       time.Time `json:"created_at" gorm:"autoCreateTime;comment:创建时间"`

	// 关联关系：一个道人服用多个金丹
	AgentPills []AgentPill `json:"agent_pills,omitempty" gorm:"foreignKey:AgentID;references:UUID;constraint:OnDelete:CASCADE;"`
	// 关联关系：一个道人拥有多个已吸收能力（任务 3；语言模式编译输入的事实来源）
	AgentPillEffects []AgentPillEffect `json:"agent_pill_effects,omitempty" gorm:"foreignKey:AgentID;references:UUID;constraint:OnDelete:CASCADE;"`
	// 关联关系：一个道人参与多个会话
	Sessions []ChatSession `json:"sessions,omitempty" gorm:"foreignKey:AgentID;references:UUID;constraint:OnDelete:CASCADE;"`
	// 关联关系：一个道人有一个语言模式缓存
	LanguagePattern *LanguagePattern `json:"language_pattern,omitempty" gorm:"foreignKey:AgentID;references:UUID;constraint:OnDelete:CASCADE;"`
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
	AgentID   string    `json:"agent_id" gorm:"type:uuid;not null;uniqueIndex:idx_agent_pill;index;comment:道人UUID文本"`
	PillID    string    `json:"pill_id" gorm:"type:uuid;not null;uniqueIndex:idx_agent_pill;index;comment:金丹UUID文本"`
	Weight    float64   `json:"weight" gorm:"default:1.0;comment:剂量/权重(0-10)"`
	SortOrder int       `json:"sort_order" gorm:"default:0;comment:服用顺序"`
	CreatedAt time.Time `json:"created_at" gorm:"autoCreateTime;comment:服用时间"`

	// 关联关系
	Agent DaoAgent   `json:"agent,omitempty" gorm:"foreignKey:AgentID;references:UUID;constraint:OnDelete:CASCADE;"`
	Pill  ElixirPill `json:"pill,omitempty" gorm:"foreignKey:PillID;references:UUID;constraint:OnDelete:CASCADE;"`
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
	ID             uint     `json:"id" gorm:"primaryKey;autoIncrement;comment:缓存唯一标识"`
	AgentID        string   `json:"agent_id" gorm:"type:uuid;not null;uniqueIndex;comment:关联道人UUID文本"`
	SystemPrompt   string   `json:"system_prompt" gorm:"type:text;not null;comment:合成后的系统提示词"`
	EmergenceRules JSONList `json:"emergence_rules" gorm:"serializer:json;comment:涌现规则列表"`
	InnerTensions  JSONList `json:"inner_tensions" gorm:"serializer:json;comment:检测到的内在冲突"`
	// BehaviorProfile 完整结构化行为档案(P1 起每次合成必写;老库为 NULL 视为失效缓存自动重建。
	// 刻意偏离 spec §6.3 的 NOT NULL:SQLite ADD COLUMN NOT NULL(无默认值)在非空表上会失败)
	BehaviorProfile JSONMap `json:"behavior_profile,omitempty" gorm:"serializer:json;comment:完整结构化行为档案"`
	// ProfileVersion 行为档案版本(behavior.ProfileVersion);不一致视为失效重建
	ProfileVersion    int       `json:"profile_version" gorm:"not null;default:1;comment:行为档案版本"`
	SourceFingerprint string    `json:"source_fingerprint" gorm:"size:80;not null;comment:来源指纹(sha256: 前缀 + 64 位 hex = 71 字符)"`
	IsValid           bool      `json:"is_valid" gorm:"default:true;comment:是否有效"`
	CreatedAt         time.Time `json:"created_at" gorm:"autoCreateTime;comment:创建时间"`
	UpdatedAt         time.Time `json:"updated_at" gorm:"autoUpdateTime;comment:更新时间"`

	// 关联关系
	Agent DaoAgent `json:"agent,omitempty" gorm:"foreignKey:AgentID;references:UUID;constraint:OnDelete:CASCADE;"`
}

// TableName 指定表名
func (LanguagePattern) TableName() string {
	return "language_patterns"
}

// ---------- 对话会话 ----------

// 会话类型常量
const (
	SessionTypeSingle = "single" // 单聊(1v1)
	SessionTypeGroup  = "group"  // 群聊(多道人)
)

// 编排 run 状态机（设计 §10）：pending 仅出 running；interrupted 可回 running（续跑）；
// completed/failed/cancelled 为终态无出路；同状态重复更新=幂等 no-op
const (
	ChatRunStatusPending     = "pending"
	ChatRunStatusRunning     = "running"
	ChatRunStatusCompleted   = "completed"
	ChatRunStatusFailed      = "failed"
	ChatRunStatusInterrupted = "interrupted"
	ChatRunStatusCancelled   = "cancelled"
)

// ChatSession 对话会话模型，对应 chat_sessions 表
// single: 用户与某个道人;group: 用户与多个道人(成员见 session_members,AgentID 为 NULL)
type ChatSession struct {
	ID        uint      `json:"id" gorm:"primaryKey;autoIncrement;comment:会话唯一标识"`
	UUID      uuid.UUID `json:"-" gorm:"type:uuid;uniqueIndex;comment:对外标识"`
	Type      string    `json:"type" gorm:"size:10;default:single;index;comment:会话类型: single/group"`
	AgentID   *string   `json:"agent_id" gorm:"type:uuid;index;comment:单聊所属道人UUID文本;群聊为NULL"`
	Title     string    `json:"title" gorm:"size:200;comment:会话标题(空=待自动命名)"`
	CreatedAt time.Time `json:"created_at" gorm:"autoCreateTime;comment:创建时间"`
	UpdatedAt time.Time `json:"updated_at" gorm:"autoUpdateTime;comment:更新时间"`

	// 关联关系
	Agent    DaoAgent        `json:"agent,omitempty" gorm:"foreignKey:AgentID;references:UUID;constraint:OnDelete:CASCADE;"`
	Messages []ChatMessage   `json:"messages,omitempty" gorm:"foreignKey:SessionID;references:UUID;constraint:OnDelete:CASCADE;"`
	Members  []SessionMember `json:"members,omitempty" gorm:"foreignKey:SessionID;references:UUID;constraint:OnDelete:CASCADE;"`
}

// TableName 指定表名
func (ChatSession) TableName() string {
	return "chat_sessions"
}

// ---------- 对话消息 ----------

// ChatMessage 对话消息模型，对应 chat_messages 表
// 存储用户与道人的对话内容
// role: user(用户提问) / assistant(道人回答) / system(系统提示)
type ChatMessage struct {
	ID        uint      `json:"id" gorm:"primaryKey;autoIncrement;comment:消息唯一标识"`
	UUID      uuid.UUID `json:"-" gorm:"type:uuid;uniqueIndex;comment:对外标识"`
	SessionID string    `json:"session_id" gorm:"type:uuid;not null;index;comment:所属会话UUID文本"`
	Role      string    `json:"role" gorm:"size:20;not null;comment:角色: user/assistant/system"`
	Content   string    `json:"content" gorm:"type:text;not null;comment:消息内容"`
	AgentID   *string   `json:"agent_id" gorm:"type:uuid;index;comment:发言道人UUID文本(群聊);NULL=用户或系统通知"`
	Mentions  JSONMap   `json:"mentions,omitempty" gorm:"serializer:json;comment:@提及:{\"agents\":[agent_uuid…],\"user\":bool}"`
	RunID     *string   `json:"run_id,omitempty" gorm:"type:uuid;index:idx_chat_run_reply,unique;comment:编排run标识文本(仅编排产物;NULL=普通消息不受唯一约束)"`
	ReplyID   *string   `json:"reply_id,omitempty" gorm:"size:64;index:idx_chat_run_reply,unique;comment:最终回复幂等键(与run_id组成复合唯一)"`
	CreatedAt time.Time `json:"created_at" gorm:"autoCreateTime;comment:创建时间"`

	// 关联关系
	Session ChatSession `json:"session,omitempty" gorm:"foreignKey:SessionID;references:UUID;constraint:OnDelete:CASCADE;"`
	Agent   *DaoAgent   `json:"agent,omitempty" gorm:"foreignKey:AgentID;references:UUID;constraint:OnDelete:SET NULL;"`
}

// TableName 指定表名
func (ChatMessage) TableName() string {
	return "chat_messages"
}

// ---------- 编排 run ----------

// ChatRun 编排运行记录模型，对应 chat_runs 表
// 记录一次 LangGraph 编排 run 的持久化身份（Task 9）：
// Go 侧以 (run UUID, reply ID) 对最终回复消息做幂等落库，重试投递不产生重复行
type ChatRun struct {
	ID            uint      `json:"id" gorm:"primaryKey;autoIncrement;comment:运行记录唯一标识"`
	UUID          uuid.UUID `json:"-" gorm:"type:uuid;uniqueIndex;comment:对外 run 标识"`
	SessionID     string    `json:"session_id" gorm:"type:uuid;not null;index;comment:所属会话UUID文本"`
	UserMessageID *string   `json:"user_message_id" gorm:"type:uuid;index;comment:触发本轮的用户消息UUID文本(NULL=尚未落库)"`
	Status        string    `json:"status" gorm:"size:20;not null;default:pending;index;comment:状态: pending/running/completed/failed/interrupted/cancelled"`
	Engine        string    `json:"engine" gorm:"size:20;not null;default:langgraph;comment:编排引擎标识"`
	CreatedAt     time.Time `json:"created_at" gorm:"autoCreateTime;comment:创建时间"`
	UpdatedAt     time.Time `json:"updated_at" gorm:"autoUpdateTime;comment:更新时间"`

	// 关联关系
	Session     ChatSession `json:"session,omitempty" gorm:"foreignKey:SessionID;references:UUID;constraint:OnDelete:CASCADE;"`
	UserMessage ChatMessage `json:"-" gorm:"foreignKey:UserMessageID;references:UUID;constraint:OnDelete:SET NULL;"`
}

// TableName 指定表名
func (ChatRun) TableName() string {
	return "chat_runs"
}

// ---------- 群聊成员 ----------

// SessionMember 群聊成员模型，对应 session_members 表
// 仅 group 会话使用;(session_id, agent_id) 联合唯一;被踢后重新邀请=删旧行插新行
type SessionMember struct {
	ID        uint      `json:"id" gorm:"primaryKey;autoIncrement;comment:成员记录唯一标识"`
	SessionID string    `json:"session_id" gorm:"type:uuid;not null;uniqueIndex:idx_session_agent;index;comment:所属会话UUID文本"`
	AgentID   string    `json:"agent_id" gorm:"type:uuid;not null;uniqueIndex:idx_session_agent;comment:道人UUID文本"`
	SortOrder int       `json:"sort_order" gorm:"default:0;comment:发言顺序(拉人顺序)"`
	JoinedAt  time.Time `json:"joined_at" gorm:"autoCreateTime;comment:入群时间"`

	// 关联关系
	Agent DaoAgent `json:"agent,omitempty" gorm:"foreignKey:AgentID;references:UUID;constraint:OnDelete:CASCADE;"`
}

// TableName 指定表名
func (SessionMember) TableName() string {
	return "session_members"
}

// ---------- LLM 供应商配置 ----------

// LLMProvider 供应商配置，对应 llm_providers 表
// 供应商是协议 + Base URL + 加密 API Key 的唯一持有者；api_key 以 AES-GCM 加密存储
// 停用供应商后其下全部模型在凭证解析链中不可用
type LLMProvider struct {
	ID              uint      `json:"id" gorm:"primaryKey;autoIncrement;comment:供应商唯一标识"`
	UUID            uuid.UUID `json:"-" gorm:"type:uuid;uniqueIndex;comment:对外标识"`
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
	Models []LLMModel `json:"models,omitempty" gorm:"foreignKey:ProviderID;references:UUID"`
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
	ProviderID  string    `json:"provider_id" gorm:"type:uuid;not null;index:idx_llm_models_provider_id;uniqueIndex:idx_llm_models_provider_name;comment:所属供应商UUID文本"`
	Name        string    `json:"name" gorm:"size:100;not null;uniqueIndex:idx_llm_models_provider_name;comment:模型名（API 调用用，如 gpt-4o）"`
	DisplayName string    `json:"display_name" gorm:"size:100;not null;comment:显示名"`
	Temperature float64   `json:"temperature" gorm:"default:0.7;comment:默认温度(0-2)"`
	MaxTokens   int       `json:"max_tokens" gorm:"default:4096;comment:默认最大 token"`
	IsEnabled   bool      `json:"is_enabled" gorm:"default:true;index;comment:是否启用"`
	IsDefault   bool      `json:"is_default" gorm:"default:false;uniqueIndex:idx_llm_models_default,where:is_default = 1;comment:是否默认模型（全表最多一个,部分唯一索引:PG/SQLite 生效,MySQL 靠 service 层校验）"`
	IsSynthesis bool      `json:"is_synthesis" gorm:"default:false;uniqueIndex:idx_llm_models_synthesis,where:is_synthesis = 1;comment:是否语言模式合成专用模型（全表最多一个,部分唯一索引:PG/SQLite 生效,MySQL 靠 service 层校验）"`
	IsFusion    bool      `json:"is_fusion" gorm:"default:false;uniqueIndex:idx_llm_models_fusion,where:is_fusion = 1;comment:是否金丹融合专用模型（全表最多一个,部分唯一索引:PG/SQLite 生效,MySQL 靠 service 层校验）"`
	SortOrder   int       `json:"sort_order" gorm:"default:0;comment:展示顺序"`
	CreatedAt   time.Time `json:"created_at" gorm:"autoCreateTime;comment:创建时间"`
	UpdatedAt   time.Time `json:"updated_at" gorm:"autoUpdateTime;comment:更新时间"`

	// 关联关系
	Provider LLMProvider `json:"provider,omitempty" gorm:"foreignKey:ProviderID;references:UUID"`
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

func (m *ChatRun) BeforeCreate(tx *gorm.DB) error {
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

// ---------- 用户简介(单行表;本地/单用户部署,无注册登录) ----------

// UserProfile 用户档案表
// 整库固定 1 行(id=1):本地或单用户部署,无注册登录
// 字段:
//   - DisplayName: 聊天消息/选人列表展示的"我"的名字
//   - Bio: 点击用户头像的 popover 简介(支持多行)
//   - Avatar: 自定义头像(URL 或 data:image/...);为空时由前端首字渐变
type UserProfile struct {
	ID          uint      `json:"-" gorm:"primaryKey;autoIncrement;comment:固定为 1"`
	DisplayName string    `json:"display_name" gorm:"size:64;not null;default:'用户';comment:显示名"`
	Bio         string    `json:"bio" gorm:"type:text;comment:简介(支持多行,最多 500 字)"`
	Avatar      string    `json:"avatar" gorm:"type:text;default:'';comment:头像 URL 或 data URI"`
	UpdatedAt   time.Time `json:"updated_at" gorm:"autoUpdateTime;comment:最近更新时间"`
}

// TableName 指定表名
func (UserProfile) TableName() string {
	return "user_profile"
}

// BeforeCreate 强制单行(id=1),避免误增
func (m *UserProfile) BeforeCreate(tx *gorm.DB) error {
	m.ID = 1
	return nil
}

// ---------- 本地记忆(Agent Memory) ----------

// AgentMemory 道人本地记忆(spec §10.1)
// 内容规则:Content ≤500 Unicode 字符;Keywords ≤12;Importance 1-5;Confidence 0-1;
// ContentHash=SHA256(kind|normalized_content);同哈希 active 只更新 confidence/importance;
// 冲突(同 kind + bigram ≥0.85)→ 新 active + 旧 superseded;pinned 永不自动置替;
// 用户删除/清空 = 物理删除
type AgentMemory struct {
	ID              uint       `json:"id" gorm:"primaryKey;autoIncrement;comment:记忆唯一标识"`
	UUID            uuid.UUID  `json:"-" gorm:"type:uuid;uniqueIndex;comment:对外标识"`
	AgentID         string     `json:"-" gorm:"type:uuid;index;not null;comment:所属道人UUID文本"`
	Kind            string     `json:"kind" gorm:"size:32;index;not null;comment:类型: user_fact/user_preference/relationship/open_loop/episode"`
	Content         string     `json:"content" gorm:"type:text;not null;comment:记忆内容(≤500字)"`
	Keywords        JSONList   `json:"keywords" gorm:"serializer:json;comment:关键词数组(≤12)"`
	Importance      int        `json:"importance" gorm:"default:3;comment:重要性1-5"`
	Confidence      float64    `json:"confidence" gorm:"default:0.8;comment:置信度0-1"`
	Pinned          bool       `json:"pinned" gorm:"default:false;comment:置顶(永不自动置替)"`
	Status          string     `json:"status" gorm:"size:16;index;not null;default:active;comment:active/superseded/archived"`
	SourceSessionID string     `json:"source_session_id" gorm:"size:36;comment:来源会话UUID"`
	SourceMessageID string     `json:"source_message_id" gorm:"size:36;comment:来源消息UUID"`
	ContentHash     string     `json:"-" gorm:"char:64;index;not null;comment:内容哈希"`
	LastAccessedAt  *time.Time `json:"last_accessed_at" gorm:"comment:最近检索时间"`
	CreatedAt       time.Time  `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt       time.Time  `json:"updated_at" gorm:"autoUpdateTime"`
}

// TableName 指定表名
func (AgentMemory) TableName() string {
	return "agent_memories"
}

// BeforeCreate 默认对外 UUID
func (m *AgentMemory) BeforeCreate(tx *gorm.DB) error {
	if m.UUID == uuid.Nil {
		m.UUID = uuid.New()
	}
	return nil
}
