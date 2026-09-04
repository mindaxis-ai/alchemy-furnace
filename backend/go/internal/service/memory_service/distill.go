// 蒸馏:单 worker 队列(容量 32,非阻塞)+ LLM 结构化提取(§10.3)
// 系统提示词静态写入,不入库不日志;日志纪律只记 count/reason/queue full(spec §12)
package memory_service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/alchemy-furnace/server/internal/interface/service"
	"github.com/alchemy-furnace/server/internal/service/credential"
)

// distillJob 队列任务(§10.3:单 worker、容量 32、非阻塞)
type distillJob struct {
	spec service.DistillationSpec
}

// distillSystemPrompt 静态蒸馏指令(不落库、不日志)
const distillSystemPrompt = `你是炼丹炉的本地记忆提取器。从对话中提取值得长期记住的内容,输出 JSON 数组,每个元素:{"kind":"user_fact|user_preference|relationship|open_loop|episode","content":"不超过500字的陈述句","keywords":["不超过12个"],"importance":1-5,"confidence":0-1}。只输出 JSON,不要解释;没有值得记忆的内容时输出 []。`

// distillCandidate LLM 返回的单条候选
type distillCandidate struct {
	Kind       string   `json:"kind"`
	Content    string   `json:"content"`
	Keywords   []string `json:"keywords"`
	Importance *int     `json:"importance"`
	Confidence *float64 `json:"confidence"`
}

// EnqueueDistillation 非阻塞入队(§10.3);队列满返回 false
func (s *MemoryService) EnqueueDistillation(ctx context.Context, spec service.DistillationSpec) bool {
	if len(spec.Targets) == 0 || spec.Model == "" {
		return false
	}
	s.startWorker()
	select {
	case s.queue <- &distillJob{spec: spec}:
		return true
	default:
		logWarn("memory.distill_queue_full", "queue_capacity", fmt.Sprintf("%d", queueCapacity))
		return false
	}
}

// startWorker 惰性启动单 worker(首次入队时)
func (s *MemoryService) startWorker() {
	s.startOnce.Do(func() {
		s.started.Store(true)
		go func() {
			defer close(s.doneCh)
			for {
				select {
				case job := <-s.queue:
					s.process(job)
				case <-s.stopCh:
					return
				}
			}
		}()
	})
}

// Close 有限关闭:停收新任务,处理完当前任务后退出(§10.3);worker 从未启动时直接返回
func (s *MemoryService) Close() {
	s.closeOnce.Do(func() {
		close(s.stopCh)
		if s.started.Load() {
			<-s.doneCh
		}
	})
}

// process 按 target 逐个蒸馏(§10.3:群聊一轮 N 个发言道人 = N 次调用)
func (s *MemoryService) process(job *distillJob) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	for _, target := range job.spec.Targets {
		if err := s.distillTarget(ctx, job.spec, target); err != nil {
			logWarn("memory.distill_skip", "reason", err.Error())
		}
	}
}

// distillTarget 单目标蒸馏:构造消息 → LLM 提取 → 逐条校验持久化
func (s *MemoryService) distillTarget(ctx context.Context, spec service.DistillationSpec, target service.DistillTarget) error {
	messages := []map[string]string{
		{"role": "system", "content": distillSystemPrompt},
		{"role": "user", "content": fmt.Sprintf("用户消息:%s\n\n道人的回应:%s",
			spec.UserMessage, joinDistillMessages(target.Messages))},
	}
	raw, err := s.llmJSON(ctx, s.engineBaseURLString(), s.resolveDistillCredentials(ctx, spec.Model), spec.Model, messages)
	if err != nil {
		return fmt.Errorf("llm: %w", err)
	}
	var candidates []distillCandidate
	if err := json.Unmarshal([]byte(raw), &candidates); err != nil {
		return fmt.Errorf("parse: %w", err)
	}
	for _, c := range candidates {
		if err := s.persistCandidate(ctx, target.AgentID, c); err != nil {
			logWarn("memory.distill_candidate_skip", "reason", err.Error())
		}
	}
	return nil
}

// persistCandidate 校验 + 哈希去重 + 冲突置替(§10.2);agentUID 为道人 UUID 文本
func (s *MemoryService) persistCandidate(ctx context.Context, agentUID string, c distillCandidate) error {
	in := service.MemoryInput{
		Kind:       c.Kind,
		Content:    c.Content,
		Keywords:   c.Keywords,
		Importance: c.Importance,
		Confidence: c.Confidence,
	}
	if err := validateInput(in); err != nil {
		return err
	}
	_, err := s.CreateMemory(ctx, agentUID, in)
	return err
}

func joinDistillMessages(msgs []service.DistillMessage) string {
	var b strings.Builder
	for _, m := range msgs {
		b.WriteString(m.Role + ":" + m.Content + "\n")
	}
	return strings.TrimSpace(b.String())
}

func (s *MemoryService) engineBaseURLString() string {
	if s.engineBaseURL == nil {
		return ""
	}
	return s.engineBaseURL()
}

// resolveDistillCredentials 按目标模型解析凭证;解析失败降级 nil(Python 回退环境变量),不阻塞蒸馏
// (对齐 trial_service.resolveTrialCredentials 的降级惯例)
func (s *MemoryService) resolveDistillCredentials(ctx context.Context, model string) *credential.ModelCredentials {
	if s.creds == nil {
		return nil
	}
	creds, err := s.creds.ResolveCredentials(ctx, model)
	if err != nil {
		zap.L().Warn("[炼丹炉] 蒸馏模型凭证解析失败，回退环境变量配置",
			zap.String("model", model), zap.Error(err))
		return nil
	}
	return creds
}

// defaultLLMJSON 默认 LLM 调用:镜像 chat_service.callChatCompletion 的
// POST {base}/api/v1/chat/completions 构造与响应解析(非流式,content 字段)
func defaultLLMJSON(ctx context.Context, baseURL string, creds *credential.ModelCredentials, model string, messages []map[string]string) (string, error) {
	url := fmt.Sprintf("%s/api/v1/chat/completions", baseURL)
	modelName := model
	if modelName == "" && creds != nil && creds.Model != "" {
		modelName = creds.Model
	}
	reqBody := map[string]interface{}{
		"messages":    messages,
		"model":       modelName,
		"temperature": 0,
		"max_tokens":  512,
	}
	if creds != nil {
		if creds.BaseURL != "" {
			reqBody["base_url"] = creds.BaseURL
		}
		if creds.APIKey != "" {
			reqBody["api_key"] = creds.APIKey
		}
	}
	jsonBody, _ := json.Marshal(reqBody)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewBuffer(jsonBody))
	if err != nil {
		return "", fmt.Errorf("构建蒸馏请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		return "", fmt.Errorf("蒸馏请求失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("蒸馏接口返回 %d: %s", resp.StatusCode, string(body))
	}
	// Python 端走统一 BaseResponse 包络:{code, message, data:{content, model, usage}}
	var wrapper struct {
		Data struct {
			Content string `json:"content"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&wrapper); err != nil {
		return "", fmt.Errorf("解析蒸馏响应失败: %w", err)
	}
	return wrapper.Data.Content, nil
}

// logWarn 蒸馏日志:只记 count/reason/queue full,不记记忆内容与蒸馏 prompt(spec §12)
func logWarn(msg string, kv ...string) {
	fields := make([]zap.Field, 0, len(kv)/2)
	for i := 0; i+1 < len(kv); i += 2 {
		fields = append(fields, zap.String(kv[i], kv[i+1]))
	}
	zap.L().Warn("[炼丹炉] "+msg, fields...)
}
