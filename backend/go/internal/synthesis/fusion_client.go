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
	return &FusionHTTPClient{
		baseURL: baseURL,
		client:  &http.Client{Timeout: 120 * time.Second}, // 融合 prompt 长,超时对齐 trial
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

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewBuffer(jsonBody))
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
