package distillation

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHTTPClientDecodesStructuredRemoteError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"detail":{"code":"research_search_blocked","stage":"research","message":"搜索受限","retryable":true,"details":{"documents":0}}}`))
	}))
	defer server.Close()
	client := NewDynamicClient(func() string { return server.URL })
	_, err := client.Distill(context.Background(), "人物", "目标描述", "zh-CN", nil)
	var remote *RemoteError
	require.ErrorAs(t, err, &remote)
	assert.Equal(t, "research_search_blocked", remote.Code)
	assert.Equal(t, "research", remote.Stage)
	assert.Equal(t, "搜索受限", remote.Message)
	assert.True(t, remote.Retryable)
	assert.Equal(t, float64(0), remote.Details["documents"])
}

func TestHTTPClientStillDecodesLegacyStringDetail(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"detail":"旧版错误"}`))
	}))
	defer server.Close()
	client := NewDynamicClient(func() string { return server.URL })
	_, err := client.Distill(context.Background(), "人物", "目标描述", "zh-CN", nil)
	var remote *RemoteError
	require.ErrorAs(t, err, &remote)
	assert.Equal(t, "旧版错误", remote.Message)
	assert.Equal(t, http.StatusBadRequest, remote.Status)
	assert.Empty(t, remote.Code)
}

func TestHTTPClientHonorsCancelledContext(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	client := NewDynamicClient(func() string { return server.URL })
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := client.Distill(ctx, "人物", "目标描述", "zh-CN", nil)
	require.Error(t, err)
	assert.True(t, errors.Is(err, context.Canceled), "err = %v", err)
}
