package service

import (
	"context"

	"github.com/alchemy-furnace/server/internal/errors"
	"github.com/alchemy-furnace/server/internal/synthesis"
	"github.com/alchemy-furnace/server/model"
	"github.com/google/uuid"
)

// TrialPillInput 试丹请求中的单颗金丹引用(金丹消耗品重构后)
// 两种目标互斥: RecipeID(+RevisionID 指定版本) 引用丹方版本;
// Draft 未保存草稿内联内容(不落库)。试丹是模拟: 不消耗金丹、不写 AgentPillEffect。
type TrialPillInput struct {
	RecipeID   uuid.UUID       // 丹方 UUID(RevisionID 为空时取当前版本)
	RevisionID uuid.UUID       // 指定丹方版本(可选;空 = 当前版本)
	Draft      *TrialPillDraft // 未保存草稿(可选;不落库)
	Weight     float64         // 剂量/权重,默认 1.0
	SortOrder  int             // 服用顺序
}

// TrialPillDraft 未保存草稿的试丹内容(女娲编辑页未保存时试丹,不落库)
type TrialPillDraft struct {
	Name        string
	SkillSchema model.JSONMap
}

// TrialChatRequest 试丹-临时对话请求
type TrialChatRequest struct {
	Personality string              // 基础性格描述
	Pills       []TrialPillInput    // 临时组合的金丹列表
	Messages    []map[string]string // 对话消息(不含 system)
	Model       string              // 对话模型(可选)
	Temperature float64             // 温度(可选)
	MaxTokens   int                 // 最大 token 数(可选)
}

// TrialChatResponse 试丹-临时对话响应
type TrialChatResponse struct {
	Content string                 `json:"content"` // 回复内容
	Model   string                 `json:"model"`   // 实际使用的模型
	Usage   map[string]interface{} `json:"usage"`   // token 用量
}

// TrialSynthesisResult 试丹-合成预览结果
// 完整系统提示词由 Go 行为引擎确定性渲染(不再来自 Python combine),涌现层信息透传
type TrialSynthesisResult struct {
	SystemPrompt   string                   // 渲染后的完整系统提示词
	EmergenceRules model.JSONList           // 涌现规则
	InnerTensions  []synthesis.InnerTension // 内在冲突
	Fingerprint    string                   // 来源指纹(合成响应透传)
	Model          string                   // 合成模型
	Degraded       bool                     // 是否降级(涌现层不可用)
	DegradedReason string                   // 降级原因错误码
}

// Trial 试丹业务逻辑接口
// 提供无需创建道人即可临时组合「基础性格 + 金丹」快速预览效果的接口
type Trial interface {
	// Synthesize 试丹-合成预览:不写入缓存,返回行为引擎渲染结果
	Synthesize(ctx context.Context, personality string, pills []TrialPillInput, modelName string) (*TrialSynthesisResult, errors.Error)

	// Chat 试丹-临时对话:先合成系统提示词,再调用语言引擎非流式对话
	Chat(ctx context.Context, req *TrialChatRequest) (*TrialChatResponse, errors.Error)
}
