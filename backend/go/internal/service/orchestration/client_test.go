package orchestration

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/alchemy-furnace/server/internal/engineendpoint"
)

// newSSEServer 起一个把给定事件名逐个按 SSE 块写出的测试服务器(空负载)。
func newSSEServer(events ...string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming unsupported", http.StatusInternalServerError)
			return
		}
		for _, name := range events {
			fmt.Fprintf(w, "event: %s\ndata: {}\n\n", name)
			flusher.Flush()
		}
	}))
}

// validRequest 构造最小合法编排请求(无凭据字段参与断言)。
func validRequest() Request {
	return Request{
		RunID:       "run-1",
		SessionID:   "session-1",
		SessionType: "group",
		UserTurn: UserTurn{
			MessageID: "msg-1",
			Text:      "报数",
			Mentions:  []string{"zhang"},
		},
		Credentials:  map[string]Credential{},
		DebugEnabled: false,
	}
}

func TestClientStreamsTypedEvents(t *testing.T) {
	server := newSSEServer("run_started", "assistant_delta", "assistant_final", "run_completed")
	defer server.Close()
	client := NewClient(engineendpoint.Static(server.URL))

	var names []string
	err := client.Stream(context.Background(), validRequest(), func(e Event) error {
		names = append(names, e.Name)
		return nil
	})
	if err != nil {
		t.Fatalf("Stream error = %v", err)
	}
	want := []string{"run_started", "assistant_delta", "assistant_final", "run_completed"}
	if strings.Join(names, ",") != strings.Join(want, ",") {
		t.Fatalf("events = %v, want %v", names, want)
	}
}

func TestClientStreamRejectsUnknownEvent(t *testing.T) {
	server := newSSEServer("run_started", "h4x0r_event", "run_completed")
	defer server.Close()
	client := NewClient(engineendpoint.Static(server.URL))

	err := client.Stream(context.Background(), validRequest(), func(e Event) error {
		if e.Name == "h4x0r_event" {
			t.Errorf("unknown event should not be delivered, got %q", e.Name)
		}
		return nil
	})
	if err == nil {
		t.Fatal("Stream error = nil, want unknown-event rejection")
	}
	if !strings.Contains(err.Error(), "h4x0r_event") {
		t.Fatalf("error = %v, want it to name the unknown event", err)
	}
}

func TestClientStreamDetectsInterruptedStream(t *testing.T) {
	// 流被中途掐断:先发 run_started 随即断开,无终态事件 → 必须报错而非静默成功
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "event: run_started\ndata: {}\n\n")
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
	}))
	defer server.Close()
	client := NewClient(engineendpoint.Static(server.URL))

	err := client.Stream(context.Background(), validRequest(), func(e Event) error {
		return nil
	})
	if err == nil {
		t.Fatal("Stream error = nil, want interrupted-stream error (EOF without terminal event)")
	}
	if !strings.Contains(err.Error(), "terminal") && !strings.Contains(err.Error(), "终态") {
		t.Fatalf("error = %v, want terminal-event-missing reason", err)
	}
}

func TestClientStreamRespectsContextCancellation(t *testing.T) {
	// 服务器挂流不终态;客户端收到首个事件后取消 ctx → Stream 返回 context.Canceled
	firstEvent := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher := w.(http.Flusher)
		fmt.Fprint(w, "event: run_started\ndata: {}\n\n")
		flusher.Flush()
		close(firstEvent)
		for {
			if _, err := fmt.Fprint(w, ": ping\n\n"); err != nil {
				return
			}
			flusher.Flush()
			time.Sleep(10 * time.Millisecond)
		}
	}))
	defer server.Close()
	client := NewClient(engineendpoint.Static(server.URL))

	ctx, cancel := context.WithCancel(context.Background())
	got := make(chan error, 1)
	go func() {
		emit := func(e Event) error {
			<-firstEvent
			cancel()
			return nil
		}
		got <- client.Stream(ctx, validRequest(), emit)
	}()

	select {
	case err := <-got:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Stream error = %v, want context.Canceled", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Stream did not return within 5s after ctx cancel")
	}
}

func TestClientStreamHandlesLargeLines(t *testing.T) {
	// 单行 data 超过 64KB(bufio.Scanner 默认上限):ReadBytes 必须完整解析
	big := strings.Repeat("甲", 70_000)
	payload := fmt.Sprintf(`{"text":%q}`, big)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprintf(w, "event: assistant_delta\ndata: %s\n\n", payload)
		fmt.Fprint(w, "event: run_completed\ndata: {}\n\n")
	}))
	defer server.Close()
	client := NewClient(engineendpoint.Static(server.URL))

	var got string
	err := client.Stream(context.Background(), validRequest(), func(e Event) error {
		if e.Name == "assistant_delta" {
			var p struct {
				Text string `json:"text"`
			}
			if err := json.Unmarshal(e.Payload, &p); err != nil {
				t.Fatalf("unmarshal big payload: %v", err)
			}
			got = p.Text
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Stream error = %v", err)
	}
	if got != big {
		t.Fatalf("large payload corrupted: got %d bytes, want %d", len(got), len(big))
	}
}

func TestClientResumeMapsNotFoundSafely(t *testing.T) {
	// resume 未知 run:Python 404 + 稳定码;错误暴露稳定码,不回显原始响应体
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/resume") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, `{"code":"run_not_found","message":"run 不存在或没有可续跑的 interrupted 检查点"}`)
	}))
	defer server.Close()
	client := NewClient(engineendpoint.Static(server.URL))

	err := client.Resume(context.Background(), "run-404", func(e Event) error {
		t.Errorf("no events expected on 404, got %q", e.Name)
		return nil
	})
	if err == nil {
		t.Fatal("Resume error = nil, want 404 mapping")
	}
	if !strings.Contains(err.Error(), "run_not_found") {
		t.Fatalf("error = %v, want stable code run_not_found", err)
	}
}

func TestClientCancelPropagatesFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/cancel") {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()
	client := NewClient(engineendpoint.Static(server.URL))

	if err := client.Cancel(context.Background(), "run-1"); err == nil {
		t.Fatal("Cancel error = nil, want HTTP 500 mapping")
	}
}

func TestClientCancelSucceedsOnUnknownRun(t *testing.T) {
	// Python cancel 对未知 run 幂等 no-op(Task 8 契约) → Go 侧 nil
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/cancel") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"code":0,"message":"已请求中断（interrupted，可续跑）"}`)
	}))
	defer server.Close()
	client := NewClient(engineendpoint.Static(server.URL))

	if err := client.Cancel(context.Background(), "run-unknown"); err != nil {
		t.Fatalf("Cancel error = %v, want nil on idempotent no-op", err)
	}
}
