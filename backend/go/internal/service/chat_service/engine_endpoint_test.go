package chat_service

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/alchemy-furnace/server/internal/engineendpoint"
)

func TestChatReadsEngineEndpointAtRequestTime(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/chat/completions" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"code":0,"message":"ok","data":{"content":"ready"}}`)
	}))
	defer server.Close()

	baseURL := "http://127.0.0.1:1"
	chat := NewDynamic(nil, nil, nil, nil, engineendpoint.Provider(func() string { return baseURL }))
	baseURL = server.URL // 模拟桌面路由装配后，内嵌引擎才完成健康检查并发布随机端口。

	content, err := chat.callChatCompletion(context.Background(), []map[string]string{{"role": "user", "content": "ping"}}, nil)
	if err != nil {
		t.Fatalf("callChatCompletion() error = %v", err)
	}
	if content != "ready" {
		t.Fatalf("content = %q, want ready", content)
	}
}
