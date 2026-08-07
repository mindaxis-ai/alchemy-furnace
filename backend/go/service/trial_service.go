// Package service 试丹（Trial）业务逻辑
// 提供无需创建道人即可临时组合「基础性格 + 金丹」快速预览效果的接口
// 对应 RESTful API: /api/v1/trial/synthesis, /api/v1/trial/chat
package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"time"

	"github.com/alchemy-furnace/server/dao"
	"github.com/alchemy-furnace/server/model"
	"github.com/alchemy-furnace/server/pkg/config"
)

// TrialService 试丹服务
type TrialService struct {
	synthesis *SynthesisClient
}

// NewTrialService 创建试丹服务
func NewTrialService() *TrialService {
	return &TrialService{
		synthesis: NewSynthesisClient(),
	}
}

// TrialPillInput 试丹请求中的单颗金丹引用
type TrialPillInput struct {
	PillID    uint    `json:"pill_id" binding:"required"` // 金丹ID
	Weight    float64 `json:"weight"`                     // 剂量/权重，默认 1.0
	SortOrder int     `json:"sort_order"`                 // 服用顺序
}

// TrialSynthesisRequest 试丹-合成预览请求
type TrialSynthesisRequest struct {
	Personality string           `json:"personality"` // 基础性格描述
	Pills       []TrialPillInput `json:"pills"`       // 临时组合的金丹列表
	ModelName   string           `json:"model_name"`  // 合成使用的模型（可选）
}

// TrialChatRequest 试丹-临时对话请求
type TrialChatRequest struct {
	Personality string              `json:"personality"`                 // 基础性格描述
	Pills       []TrialPillInput    `json:"pills"`                       // 临时组合的金丹列表
	Messages    []map[string]string `json:"messages" binding:"required"` // 对话消息（不含 system）
	Model       string              `json:"model"`                       // 对话模型（可选）
	Temperature float64             `json:"temperature"`                 // 温度（可选）
	MaxTokens   int                 `json:"max_tokens"`                  // 最大 token 数（可选）
}

// TrialChatResponse 试丹-临时对话响应
type TrialChatResponse struct {
	Content string                 `json:"content"` // 回复内容
	Model   string                 `json:"model"`   // 实际使用的模型
	Usage   map[string]interface{} `json:"usage"`   // token 用量
}

// loadTrialPills 按 ID 加载金丹并组装为合成输入，按 sort_order 排序
func (s *TrialService) loadTrialPills(inputs []TrialPillInput) ([]SynthesisPillInput, error) {
	if len(inputs) == 0 {
		return []SynthesisPillInput{}, nil
	}

	// 按 sort_order 排序，保证合成顺序稳定
	sorted := make([]TrialPillInput, len(inputs))
	copy(sorted, inputs)
	sort.SliceStable(sorted, func(i, j int) bool {
		return sorted[i].SortOrder < sorted[j].SortOrder
	})

	ids := make([]uint, 0, len(sorted))
	for _, in := range sorted {
		ids = append(ids, in.PillID)
	}

	var pills []model.ElixirPill
	if err := dao.GetDB().Where("id IN ?", ids).Find(&pills).Error; err != nil {
		return nil, fmt.Errorf("查询金丹失败: %w", err)
	}
	pillMap := make(map[uint]model.ElixirPill, len(pills))
	for _, p := range pills {
		pillMap[p.ID] = p
	}

	result := make([]SynthesisPillInput, 0, len(sorted))
	for _, in := range sorted {
		pill, ok := pillMap[in.PillID]
		if !ok {
			return nil, fmt.Errorf("金丹(id=%d)不存在", in.PillID)
		}
		weight := in.Weight
		if weight <= 0 {
			weight = 1.0
		}
		result = append(result, SynthesisPillInput{
			ID:          pill.ID,
			Name:        pill.Name,
			Weight:      weight,
			SortOrder:   in.SortOrder,
			SkillSchema: pill.SkillSchema,
		})
	}
	return result, nil
}

// Synthesize 试丹-合成预览：不写入缓存，直接返回合成结果
func (s *TrialService) Synthesize(req *TrialSynthesisRequest) (*CombineResponse, error) {
	pills, err := s.loadTrialPills(req.Pills)
	if err != nil {
		return nil, err
	}
	return s.synthesis.Combine(req.Personality, pills)
}

// Chat 试丹-临时对话：先合成系统提示词，再调用语言引擎非流式对话
func (s *TrialService) Chat(req *TrialChatRequest) (*TrialChatResponse, error) {
	pills, err := s.loadTrialPills(req.Pills)
	if err != nil {
		return nil, err
	}

	combined, err := s.synthesis.Combine(req.Personality, pills)
	if err != nil {
		return nil, err
	}

	// 组装消息：合成后的 system 提示词 + 用户提供的消息
	messages := make([]map[string]string, 0, len(req.Messages)+1)
	messages = append(messages, map[string]string{"role": "system", "content": combined.SystemPrompt})
	messages = append(messages, req.Messages...)

	modelName := req.Model
	if modelName == "" {
		modelName = config.Get().LLM.DefaultModel
	}
	temperature := req.Temperature
	if temperature <= 0 {
		temperature = 0.7
	}
	maxTokens := req.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 4096
	}

	reqBody := map[string]interface{}{
		"messages":    messages,
		"model":       modelName,
		"temperature": temperature,
		"max_tokens":  maxTokens,
	}
	jsonBody, _ := json.Marshal(reqBody)

	url := fmt.Sprintf("%s/api/v1/chat/completions", config.Get().PythonEngine.BaseURL)
	client := &http.Client{Timeout: 120 * time.Second}
	resp, err := client.Post(url, "application/json", bytes.NewBuffer(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("调用语言引擎对话接口失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("语言引擎对话接口返回错误: status=%d, body=%s", resp.StatusCode, string(body))
	}

	var result TrialChatResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("解析对话响应失败: %w", err)
	}
	return &result, nil
}
