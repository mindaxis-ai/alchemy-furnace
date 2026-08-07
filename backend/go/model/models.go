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
)

// ---------- 金丹（语言模式/人格特质技能包） ----------

// ElixirPill 金丹模型，对应 elixir_pills 表
// 金丹是一套可影响语言模式的结构化技能包，基于 nuwa-skill 的 SKILL.md 结构
// SkillSchema 存储于 PostgreSQL JSONB 中
type ElixirPill struct {
	ID          uint      `json:"id" gorm:"primaryKey;autoIncrement;comment:金丹唯一标识"`
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
	SourceFingerprint string    `json:"source_fingerprint" gorm:"size:64;not null;comment:来源指纹(SHA256)"`
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
