// Package orchestration 客户端实现:见 types.go 包头文档。
package orchestration

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/alchemy-furnace/server/internal/engineendpoint"
)

// knownEvents 合法事件白名单(逐字对齐 Python events.py 的 EventName)。
var knownEvents = map[string]bool{
	"run_started":         true,
	"plan_created":        true,
	"prompt_debug":        true,
	"speaker_started":     true,
	"assistant_delta":     true,
	"assistant_final":     true,
	"memory_proposed":     true,
	"run_interrupted":     true,
	"permission_required": true,
	"run_completed":       true,
	"run_error":           true,
}

// terminalEvents 终态事件:收到即认为本轮生命周期已定。
var terminalEvents = map[string]bool{
	"run_completed":   true,
	"run_interrupted": true,
	"run_error":       true,
}

// Client Python 编排引擎内部 SSE 客户端。
type Client struct {
	baseURL engineendpoint.Provider
	http    *http.Client
}

// NewClient 构造编排客户端;baseURL 动态解析(桌面随机端口 Loopback)。
func NewClient(baseURL engineendpoint.Provider) *Client {
	return &Client{
		baseURL: baseURL,
		// SSE 长连接不设整体 Timeout,生命周期由 ctx 与响应体关闭管理
		http: &http.Client{},
	}
}

// streamError 把 Python 非 2xx 响应映射为稳定错误:只暴露状态码与 JSON 错误码,
// 不回显原始响应体(请求体凭据永不参与错误信息)。
func streamError(status int, body []byte) error {
	var parsed struct {
		Code string `json:"code"`
	}
	_ = json.Unmarshal(body, &parsed)
	if parsed.Code != "" {
		return fmt.Errorf("orchestration: HTTP %d: %s", status, parsed.Code)
	}
	trimmed := strings.TrimSpace(string(body))
	if len(trimmed) > 120 {
		trimmed = trimmed[:120]
	}
	if trimmed == "" {
		return fmt.Errorf("orchestration: HTTP %d", status)
	}
	return fmt.Errorf("orchestration: HTTP %d: %s", status, trimmed)
}

var errInterrupted = errors.New("orchestration: 流在终态事件前中断 (EOF without terminal event)")

// Stream 发起一次新编排 run 并消费其 SSE 事件流;
// 每个事件经 emit 同步回调,emit 返回错误立即中止。
func (c *Client) Stream(ctx context.Context, req Request, emit func(Event) error) error {
	return c.consume(ctx, fmt.Sprintf("%s/api/v1/orchestration/runs/stream", c.baseURL()), req, emit)
}

// Resume 续跑 interrupted run;未知/不可续跑 run 映射为含 run_not_found 的错误。
func (c *Client) Resume(ctx context.Context, runID string, emit func(Event) error) error {
	return c.consume(ctx, fmt.Sprintf("%s/api/v1/orchestration/runs/%s/resume", c.baseURL(), runID), nil, emit)
}

// Cancel 请求中断活跃 run;Python 侧对未知/已终态 run 幂等 no-op(照常 200)。
func (c *Client) Cancel(ctx context.Context, runID string) error {
	url := fmt.Sprintf("%s/api/v1/orchestration/runs/%s/cancel", c.baseURL(), runID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, nil)
	if err != nil {
		return err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return streamError(resp.StatusCode, body)
	}
	return nil
}

// consume 统一执行请求与 SSE 解析。data 行用 bufio.Reader.ReadBytes('\n') 读,
// 不设行宽上限(64KB+ 负载必须完整);EOF 无终态事件视为流中断错误。
func (c *Client) consume(ctx context.Context, url string, reqBody any, emit func(Event) error) error {
	var body io.Reader
	if reqBody != nil {
		raw, err := json.Marshal(reqBody)
		if err != nil {
			return err
		}
		body = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return streamError(resp.StatusCode, raw)
	}

	reader := bufio.NewReader(resp.Body)
	var eventName string
	var dataLines []string
	terminalSeen := false

	flushBlock := func() error {
		if eventName == "" && len(dataLines) == 0 {
			return nil
		}
		defer func() {
			eventName = ""
			dataLines = nil
		}()
		if eventName == "" {
			// 无 event: 行的孤儿 data 块:跳过注释/心跳,不算错误
			if strings.TrimSpace(strings.Join(dataLines, "\n")) != "" {
				return nil
			}
			return nil
		}
		if !knownEvents[eventName] {
			return fmt.Errorf("orchestration: 未知事件 %q", eventName)
		}
		payload := json.RawMessage(strings.Join(dataLines, "\n"))
		event := Event{Name: eventName, Payload: payload}
		if emit != nil {
			if err := emit(event); err != nil {
				return err
			}
		}
		if terminalEvents[eventName] {
			terminalSeen = true
		}
		return nil
	}

	for {
		line, err := reader.ReadBytes('\n')
		if len(line) > 0 {
			trimmed := strings.TrimRight(string(line), "\r\n")
			switch {
			case strings.HasPrefix(trimmed, "event:"):
				eventName = strings.TrimSpace(strings.TrimPrefix(trimmed, "event:"))
			case strings.HasPrefix(trimmed, "data:"):
				dataLines = append(dataLines, strings.TrimPrefix(strings.TrimSpace(trimmed), "data:"))
				// TrimSpace 后再 TrimPrefix 可能去掉前导空格:SSE 规范允许 "data: x" 与 "data:x"
			case trimmed == "":
				if err := flushBlock(); err != nil {
					return err
				}
			default:
				// 注释(: ping)或其他字段(id:/retry:)忽略
			}
		}
		if err != nil {
			if err == io.EOF {
				if !terminalSeen {
					return errInterrupted
				}
				return nil
			}
			return err
		}
	}
}
