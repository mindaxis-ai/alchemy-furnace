package chat_service

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/alchemy-furnace/server/internal/interface/service"
	"github.com/alchemy-furnace/server/internal/service/credential"
)

type codedStreamError interface {
	StreamErrorCode() string
}

func TestStreamChatPrematureEOFReturnsSafeTypedInterruption(t *testing.T) {
	engine := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"content\":\"partial answer\"}\n\n")
		// Deliberately close without the protocol's terminal [DONE] marker.
	}))
	t.Cleanup(engine.Close)

	svc := New(nil, nil, nil, nil, engine.URL)
	var chunks []string
	full, canceled, err := svc.StreamChat(
		context.Background(),
		[]map[string]string{{"role": "user", "content": "question"}},
		&credential.ModelCredentials{Model: "test-model", APIKey: "secret"},
		service.GenerationOptions{MaxTokens: 384},
		func(chunk string) { chunks = append(chunks, chunk) },
	)

	if full != "partial answer" || strings.Join(chunks, "") != full {
		t.Fatalf("full = %q, chunks = %q, want retained partial answer", full, chunks)
	}
	if canceled {
		t.Fatal("premature EOF must be an interruption, not a local cancellation")
	}
	if err == nil {
		t.Fatal("premature EOF returned nil error, want typed interruption")
	}
	coded, ok := err.(codedStreamError)
	if !ok || coded.StreamErrorCode() != "service.chat.stream_interrupted" {
		t.Fatalf("error = %#v, want safe typed service.chat.stream_interrupted", err)
	}
	if strings.Contains(err.Error(), "EOF") || strings.Contains(err.Error(), "secret") {
		t.Fatalf("error leaked transport or credentials: %q", err.Error())
	}
}

// Task 7/8:GenerationOptions.MaxTokens 必须出现在引擎请求体(spec §7.2)
func TestStreamChatPassesMaxTokensToEngine(t *testing.T) {
	var body string
	engine := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/chat/completions/stream" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		raw, _ := io.ReadAll(r.Body)
		body = string(raw)
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"content\":\"ok\"}\n\ndata: [DONE]\n\n")
	}))
	t.Cleanup(engine.Close)

	svc := New(nil, nil, nil, nil, engine.URL)
	full, _, err := svc.StreamChat(
		context.Background(),
		[]map[string]string{{"role": "user", "content": "question"}},
		&credential.ModelCredentials{Model: "test-model", APIKey: "secret"},
		service.GenerationOptions{MaxTokens: 384},
		nil,
	)
	if err != nil || full != "ok" {
		t.Fatalf("StreamChat() = %q, %v", full, err)
	}
	if !strings.Contains(body, `"max_tokens":384`) {
		t.Fatalf("引擎请求体缺 max_tokens: %s", body)
	}
	// 0 表示不限制:不得出现在请求体(回退 Python 默认)
	svc.StreamChat(context.Background(), []map[string]string{{"role": "user", "content": "q"}}, nil, service.GenerationOptions{}, nil)
	if strings.Contains(body, "max_tokens") {
		t.Fatalf("MaxTokens=0 不应携带 max_tokens: %s", body)
	}
}

// Task 10:句数硬限制——SSE 连续输出三句时只发前两句并正常返回
// (导演策略完成:非取消、非错误)
func TestStreamChatStopsAtSentenceBudget(t *testing.T) {
	engine := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"content\":\"我懂。\"}\n\n")
		fmt.Fprint(w, "data: {\"content\":\"先歇会。还有第三句。\"}\n\n")
		fmt.Fprint(w, "data: {\"content\":\"第四句。\"}\n\n")
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	t.Cleanup(engine.Close)

	svc := New(nil, nil, nil, nil, engine.URL)
	var chunks []string
	full, canceled, err := svc.StreamChat(
		context.Background(),
		[]map[string]string{{"role": "user", "content": "question"}},
		&credential.ModelCredentials{Model: "test-model", APIKey: "secret"},
		service.GenerationOptions{MaxTokens: 128, MaxSentences: 2},
		func(chunk string) { chunks = append(chunks, chunk) },
	)

	if full != "我懂。先歇会。" {
		t.Fatalf("full = %q, want 只有前两句", full)
	}
	if joined := strings.Join(chunks, ""); joined != full {
		t.Fatalf("chunks = %q, want 与全文一致 %q", joined, full)
	}
	if canceled {
		t.Fatal("句数预算完成不得标记为用户取消")
	}
	if err != nil {
		t.Fatalf("句数预算完成不得报错: %v", err)
	}
}

// Task 12 回归:流结束(正常)时限制器缓冲的未完成内容必须发出——
// 无句末标点的回复(「好的」等)在导演预算下不得丢失
func TestStreamChatFlushesTrailingBufferAtStreamEnd(t *testing.T) {
	engine := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"content\":\"好的\"}\n\n")
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	t.Cleanup(engine.Close)

	svc := New(nil, nil, nil, nil, engine.URL)
	var chunks []string
	full, canceled, err := svc.StreamChat(
		context.Background(),
		[]map[string]string{{"role": "user", "content": "question"}},
		&credential.ModelCredentials{Model: "test-model", APIKey: "secret"},
		service.GenerationOptions{MaxTokens: 128, MaxSentences: 2},
		func(chunk string) { chunks = append(chunks, chunk) },
	)

	if full != "好的" || strings.Join(chunks, "") != full {
		t.Fatalf("full = %q, chunks = %q, want 无句末内容在流结束时完整返回", full, chunks)
	}
	if canceled || err != nil {
		t.Fatalf("canceled=%v err=%v, want 正常结束", canceled, err)
	}
}
