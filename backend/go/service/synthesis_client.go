// Package service 语言模式合成客户端
// 封装对 Python 语言模式合成引擎（Language Synthesis Engine）的 HTTP 调用
package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/alchemy-furnace/server/model"
	"github.com/alchemy-furnace/server/pkg/config"
)

// SynthesisClient 调用 Python 合成引擎的 HTTP 客户端
type SynthesisClient struct {
	baseURL string
	client  *http.Client
}

// NewSynthesisClient 创建合成引擎客户端
func NewSynthesisClient() *SynthesisClient {
	return &SynthesisClient{
		baseURL: config.Get().PythonEngine.BaseURL,
		client:  &http.Client{Timeout: 30 * time.Second},
	}
}

// SynthesisPillInput 合成请求中的单颗金丹
type SynthesisPillInput struct {
	ID          uint          `json:"id"`
	Name        string        `json:"name"`
	Weight      float64       `json:"weight"`
	SortOrder   int           `json:"sort_order"`
	SkillSchema model.JSONMap `json:"skill_schema"`
}

// CombineRequest 合成请求
type CombineRequest struct {
	Personality string               `json:"personality"`
	Pills       []SynthesisPillInput `json:"pills"`
	Model       string               `json:"model"`
	Temperature float64              `json:"temperature"`
	MaxTokens   int                  `json:"max_tokens"`
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
}

// Combine 调用 Python /api/v1/synthesis/combine，将基础性格与金丹列表合成为系统提示词
func (c *SynthesisClient) Combine(personality string, pills []SynthesisPillInput) (*CombineResponse, error) {
	url := fmt.Sprintf("%s/api/v1/synthesis/combine", c.baseURL)

	reqBody := CombineRequest{
		Personality: personality,
		Pills:       pills,
		Model:       config.Get().LLM.SynthesisModel,
		Temperature: 0.7,
		MaxTokens:   2048,
	}
	if reqBody.Model == "" {
		reqBody.Model = config.Get().LLM.DefaultModel
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("序列化合成请求失败: %w", err)
	}

	resp, err := c.client.Post(url, "application/json", bytes.NewBuffer(jsonBody))
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
