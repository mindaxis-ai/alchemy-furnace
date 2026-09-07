// Package distillation 封装女娲蒸馏引擎；业务层只依赖 Client 接口。
package distillation

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/alchemy-furnace/server/internal/service/credential"
	"github.com/alchemy-furnace/server/model"
)

type Client interface {
	Distill(ctx context.Context, subject, brief, locale string, creds *credential.ModelCredentials) (*Response, error)
	SkillExport(ctx context.Context, skill *ExportableSkill, format string) (*ExportResult, error)
}

type Source struct {
	Title     string `json:"title"`
	URL       string `json:"url"`
	Dimension string `json:"dimension"`
}

// ResearchSummary 蒸馏研究摘要(证据等级与来源统计,不含正文)
type ResearchSummary struct {
	EvidenceLevel   string   `json:"evidence_level"`
	DocumentCount   int      `json:"document_count"`
	DomainCount     int      `json:"domain_count"`
	TotalCharacters int      `json:"total_characters"`
	Warnings        []string `json:"warnings"`
}

type Response struct {
	Name           string          `json:"name"`
	Description    string          `json:"description"`
	PersonaSummary string          `json:"persona_summary"`
	Avatar         string          `json:"avatar"`
	Tags           model.JSONList  `json:"tags"`
	SkillSchema    model.JSONMap   `json:"skill_schema"`
	Sources        []Source        `json:"sources"`
	Model          string          `json:"model"`
	Research       ResearchSummary `json:"research"`
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
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
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
		if remote, ok := decodeErrorDetail(raw); ok {
			remote.Status = resp.StatusCode
			return nil, remote
		}
		return nil, &RemoteError{Status: resp.StatusCode, Message: string(raw)}
	}
	var result Response
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("解析蒸馏结果失败: %w", err)
	}
	return &result, nil
}

// skillExportPayload Python 引擎 skill-export 请求体(snake_case,与 app/models/schemas.py 对齐)
type skillExportPayload struct {
	Name          string        `json:"name"`
	Description   string        `json:"description"`
	SkillSchema   model.JSONMap `json:"skill_schema"`
	Tags          []string      `json:"tags"`
	Sources       []Source      `json:"sources"`
	GeneratedAt   string        `json:"generated_at"`
	EvidenceLevel string        `json:"evidence_level"`
	Format        string        `json:"format"`
}

// SkillExport 请求 Python 引擎 skill_export 渲染并打包 ZIP(二进制响应)。
// 只传已保存金丹的规范化投影字段,绝不携带 pill_id/API Key/内部 ID。
func (c *HTTPClient) SkillExport(ctx context.Context, skill *ExportableSkill, format string) (*ExportResult, error) {
	payload := skillExportPayload{
		Name:          skill.Name,
		Description:   skill.Description,
		SkillSchema:   skill.SkillSchema,
		Tags:          skill.Tags,
		Sources:       skill.Sources,
		GeneratedAt:   skill.GeneratedAt,
		EvidenceLevel: skill.EvidenceLevel,
		Format:        format,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("序列化导出请求失败: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.baseURL()+"/api/v1/distillation/skill-export", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("构建导出请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("连接导出引擎失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 16<<10))
		if remote, ok := decodeErrorDetail(raw); ok {
			remote.Status = resp.StatusCode
			return nil, remote
		}
		return nil, &RemoteError{Status: resp.StatusCode, Message: string(raw)}
	}
	content, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return nil, fmt.Errorf("读取导出 ZIP 失败: %w", err)
	}
	return &ExportResult{
		Filename: exportFilename(resp.Header.Get("Content-Disposition")),
		Content:  content,
	}, nil
}

// exportFilename 从 Content-Disposition 解析纯 ASCII 下载名(§3.4 命名);
// 头部缺失或无法解析时回退通用名。
func exportFilename(header string) string {
	const fallback = "alchemy-skill-export.zip"
	if header == "" {
		return fallback
	}
	rest := header[strings.Index(header, "filename=")+len("filename="):]
	rest = strings.TrimSpace(rest)
	if strings.HasPrefix(rest, `"`) {
		rest = rest[1:]
		if end := strings.IndexByte(rest, '"'); end >= 0 {
			rest = rest[:end]
		}
	} else if end := strings.IndexAny(rest, "; \t"); end >= 0 {
		rest = rest[:end]
	}
	if rest == "" {
		return fallback
	}
	return rest
}

// decodeErrorDetail 解析 Python 错误体 detail 字段: 新协议为结构化对象,
// 旧协议为字符串。解析失败返回 ok=false,由调用方回退为原文 message。
func decodeErrorDetail(raw []byte) (*RemoteError, bool) {
	var envelope struct {
		Detail json.RawMessage `json:"detail"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil || len(envelope.Detail) == 0 {
		return nil, false
	}
	detail := envelope.Detail
	switch detail[0] {
	case '"': // 旧协议: {"detail":"旧版错误"}
		var message string
		if err := json.Unmarshal(detail, &message); err != nil {
			return nil, false
		}
		return &RemoteError{Message: message}, true
	case '{': // 新协议: {"detail":{"code":..,"stage":..,"message":..,"retryable":..,"details":..}}
		var structured struct {
			Code      string         `json:"code"`
			Stage     string         `json:"stage"`
			Message   string         `json:"message"`
			Retryable bool           `json:"retryable"`
			Details   map[string]any `json:"details"`
		}
		if err := json.Unmarshal(detail, &structured); err != nil {
			return nil, false
		}
		return &RemoteError{
			Code:      structured.Code,
			Stage:     structured.Stage,
			Message:   structured.Message,
			Retryable: structured.Retryable,
			Details:   structured.Details,
		}, true
	}
	return nil, false
}

type RemoteError struct {
	Status    int
	Code      string
	Stage     string
	Message   string
	Retryable bool
	Details   map[string]any
}

func (e *RemoteError) Error() string { return e.Message }
