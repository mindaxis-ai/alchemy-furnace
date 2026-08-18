// Package distillation 封装女娲蒸馏引擎；业务层只依赖 Client 接口。
package distillation

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

type Client interface {
	Distill(ctx context.Context, subject, brief, locale string, creds *credential.ModelCredentials) (*Response, error)
}

type Source struct {
	Title     string `json:"title"`
	URL       string `json:"url"`
	Dimension string `json:"dimension"`
}

type Response struct {
	Name           string         `json:"name"`
	Description    string         `json:"description"`
	PersonaSummary string         `json:"persona_summary"`
	Tags           model.JSONList `json:"tags"`
	SkillSchema    model.JSONMap  `json:"skill_schema"`
	Sources        []Source       `json:"sources"`
	Model          string         `json:"model"`
}

type request struct {
	Subject string `json:"subject"`
	Brief   string `json:"brief"`
	Locale  string `json:"locale"`
	Model   string `json:"model,omitempty"`
	BaseURL string `json:"base_url,omitempty"`
	APIKey  string `json:"api_key,omitempty"`
}

type HTTPClient struct {
	baseURL func() string
	client  *http.Client
}

func NewDynamicClient(baseURL func() string) *HTTPClient {
	return &HTTPClient{baseURL: baseURL, client: &http.Client{Timeout: 180 * time.Second}}
}

func (c *HTTPClient) Distill(ctx context.Context, subject, brief, locale string, creds *credential.ModelCredentials) (*Response, error) {
	body := request{Subject: subject, Brief: brief, Locale: locale}
	if creds != nil {
		body.Model, body.BaseURL, body.APIKey = creds.Model, creds.BaseURL, creds.APIKey
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("序列化蒸馏请求失败: %w", err)
	}
	req, err := http.NewRequestWithContext(context.WithoutCancel(ctx), http.MethodPost,
		c.baseURL()+"/api/v1/distillation/nuwa", bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("构建蒸馏请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("连接女娲蒸馏引擎失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 16<<10))
		var detail struct {
			Detail string `json:"detail"`
		}
		_ = json.Unmarshal(raw, &detail)
		if detail.Detail == "" {
			detail.Detail = string(raw)
		}
		return nil, &RemoteError{Status: resp.StatusCode, Message: detail.Detail}
	}
	var result Response
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("解析蒸馏结果失败: %w", err)
	}
	return &result, nil
}

type RemoteError struct {
	Status  int
	Message string
}

func (e *RemoteError) Error() string { return e.Message }
