// Package synthesis 语言模式合成客户端(新架构;对话域与试丹域共用)
// 封装对 Python 语言模式合成引擎 /api/v1/synthesis/combine 的 HTTP 调用。
// 与旧 service/synthesis_client.go 行为对齐;pills[].id 由 uint 改为 UUID 字符串(契约 006)。
package synthesis

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/alchemy-furnace/server/internal/configuration"
	"github.com/alchemy-furnace/server/internal/service/credential"
	"github.com/alchemy-furnace/server/model"
)

// Client 合成引擎客户端接口(便于试丹/对话服务注入与单测 mock)
type Client interface {
	// Combine 将基础性格与金丹列表合成为系统提示词;creds 为解析后的合成模型凭证(可为 nil 回退环境变量)
	Combine(ctx context.Context, personality string, pills []PillInput, creds *credential.ModelCredentials) (*CombineResponse, error)
}

// PillInput 合成请求中的单颗金丹(id 为 UUID 字符串)
type PillInput struct {
	ID          string        `json:"id"`
	Name        string        `json:"name"`
	Weight      float64       `json:"weight"`
	SortOrder   int           `json:"sort_order"`
	SkillSchema model.JSONMap `json:"skill_schema"`
}

// combineRequest 合成请求体
// base_url/api_key 为按请求透传的模型凭证(可选),缺省时 Python 回退自身环境变量配置
type combineRequest struct {
	Personality string      `json:"personality"`
	Pills       []PillInput `json:"pills"`
	Model       string      `json:"model"`
	BaseURL     string      `json:"base_url,omitempty"`
	APIKey      string      `json:"api_key,omitempty"`
	Temperature float64     `json:"temperature"`
	MaxTokens   int         `json:"max_tokens"`
}

// InnerTension 内在冲突记录
type InnerTension struct {
	Dimension   string `json:"dimension"`
	Description string `json:"description"`
	Severity    string `json:"severity"` // low / medium / high
}

// CombineResponse 合成响应
type CombineResponse struct {
	SystemPrompt   string         `json:"system_prompt"`
	EmergenceRules model.JSONList `json:"emergence_rules"`
	InnerTensions  []InnerTension `json:"inner_tensions"`
	Fingerprint    string         `json:"fingerprint"`
	Model          string         `json:"model"`
	// Degraded 为 true 表示 Python 走了结构化合并兜底(LLM 不可用/失败),
	// 调用方不应落库,避免兜底提示词污染语言模式缓存
	Degraded bool `json:"degraded"`
}

// SynthesisClient Client 接口实现
type SynthesisClient struct {
	baseURL func() string
	client  *http.Client
}

// New 构造合成引擎客户端
func New(baseURL string) *SynthesisClient {
	return NewDynamic(func() string { return baseURL })
}

// NewDynamic 构造运行时读取最新地址的合成客户端（桌面随机端口场景）。
func NewDynamic(baseURL func() string) *SynthesisClient {
	return &SynthesisClient{
		baseURL: baseURL,
		client:  &http.Client{Timeout: 30 * time.Second},
	}
}

// Combine 调用 Python /api/v1/synthesis/combine,将基础性格与金丹列表合成为系统提示词
func (c *SynthesisClient) Combine(ctx context.Context, personality string, pills []PillInput, creds *credential.ModelCredentials) (*CombineResponse, error) {
	url := fmt.Sprintf("%s/api/v1/synthesis/combine", c.baseURL())

	reqBody := combineRequest{
		Personality: personality,
		Pills:       pills,
		Model:       configuration.Configuration.LLM.SynthesisModel,
		Temperature: 0.7,
		MaxTokens:   2048,
	}
	if creds != nil && creds.Model != "" {
		reqBody.Model = creds.Model
		reqBody.BaseURL = creds.BaseURL
		reqBody.APIKey = creds.APIKey
	}
	if reqBody.Model == "" {
		reqBody.Model = configuration.Configuration.LLM.DefaultModel
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("序列化合成请求失败: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewBuffer(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("构建合成请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("调用语言模式合成接口失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("合成接口返回错误: status=%d, body=%s", resp.StatusCode, string(body))
	}

	var result CombineResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("解析合成响应失败: %w", err)
	}
	return &result, nil
}
