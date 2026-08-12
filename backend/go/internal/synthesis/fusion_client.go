// Package synthesis 金丹融合引擎客户端(新架构)
// 封装对 Python 融合引擎 /api/v1/fusion/fuse 的 HTTP 调用。
package synthesis

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/alchemy-furnace/server/internal/service/credential"
	"github.com/alchemy-furnace/server/model"
)

// FusionClient 金丹融合引擎客户端接口(便于 service 注入与单测 mock)
type FusionClient interface {
	// Fuse 调用 Python /api/v1/fusion/fuse;creds 可为 nil 回退环境变量
	Fuse(ctx context.Context, pills []PillInput, excludeOperatorID string, creds *credential.ModelCredentials) (*FuseResponse, error)
}

// FuseOperator 本次融合抽中的算子
type FuseOperator struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// FuseResponse 融合响应(与 Python FuseResponse 对齐;直返无信封,同 /synthesis/combine)
type FuseResponse struct {
	Name        string        `json:"name"`
	Description string        `json:"description"`
	SkillSchema model.JSONMap `json:"skill_schema"`
	Operator    FuseOperator  `json:"operator"`
	Model       string        `json:"model"`
	Degraded    bool          `json:"degraded"`
}

// fuseRequest 融合请求体(base_url/api_key 按请求透传)
type fuseRequest struct {
	Pills             []PillInput `json:"pills"`
	Model             string      `json:"model"`
	BaseURL           string      `json:"base_url,omitempty"`
	APIKey            string      `json:"api_key,omitempty"`
	ExcludeOperatorID string      `json:"exclude_operator_id,omitempty"`
}

// FusionHTTPClient FusionClient 实现
type FusionHTTPClient struct {
	baseURL string
	client  *http.Client
}

// NewFusionClient 构造融合引擎客户端
func NewFusionClient(baseURL string) *FusionHTTPClient {
	// 融合 prompt 长 + LLM 重试可达 60s+;前端走 Next.js dev proxy 转发(默认 30s 超时),
	// 实测需要 ≥180s 才能覆盖 deepseek 慢响应 + Python 内部 JSON 解析重试一轮的场景。
	// 真正的根因修复在 next.config.mjs 侧加 proxyTimeout;此处给 Go 客户端也留足余量。
	return &FusionHTTPClient{
		baseURL: baseURL,
		client:  &http.Client{Timeout: 180 * time.Second},
	}
}

// Fuse 调用 Python /api/v1/fusion/fuse,将 N 枚金丹融合为新金丹预览
// creds 为 nil 或 Model 为空时 model 字段留空,Python 回退自身默认配置
func (c *FusionHTTPClient) Fuse(ctx context.Context, pills []PillInput, excludeOperatorID string, creds *credential.ModelCredentials) (*FuseResponse, error) {
	url := fmt.Sprintf("%s/api/v1/fusion/fuse", c.baseURL)

	reqBody := fuseRequest{
		Pills:             pills,
		ExcludeOperatorID: excludeOperatorID,
	}
	if creds != nil && creds.Model != "" {
		reqBody.Model = creds.Model
		reqBody.BaseURL = creds.BaseURL
		reqBody.APIKey = creds.APIKey
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("序列化融合请求失败: %w", err)
	}

	// 解耦客户端断开取消:Next.js dev proxy 默认 30s 超时,断开时 c.Request.Context() 会被取消,
	// 进而把 Python 的长调用一刀切掉。用 context.WithoutCancel 保留 request_id 等 values,
	// 但切断取消传播,让 c.client.Timeout(180s) 真正生效。
	req, err := http.NewRequestWithContext(context.WithoutCancel(ctx), http.MethodPost, url, bytes.NewBuffer(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("构建融合请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("调用金丹融合接口失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("融合接口返回错误: status=%d, body=%s", resp.StatusCode, string(body))
	}

	var result FuseResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("解析融合响应失败: %w", err)
	}
	return &result, nil
}
