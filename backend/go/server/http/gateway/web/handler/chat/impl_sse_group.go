package chat

import (
	"net/http"
	"sync"
	"time"

	"github.com/alchemy-furnace/server/internal/context/contextutil"
	"github.com/alchemy-furnace/server/server/http/response"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// sseWriter 带锁的 SSE 写出器:编排器 emit(单 goroutine)与心跳 goroutine 并发写保护
type sseWriter struct {
	w       http.ResponseWriter
	flusher http.Flusher
	mu      sync.Mutex
}

func (s *sseWriter) event(event string, payload any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sseWriteEvent(s.w, s.flusher, event, payload)
}

func (s *sseWriter) ping() {
	s.mu.Lock()
	defer s.mu.Unlock()
	sseWriteComment(s.w, s.flusher, "ping")
}

// runGroupSSE 群聊 SSE 通道:编排器驱动事件流,心跳 goroutine 保活至回合结束
func (cls *Chat) runGroupSSE(c *gin.Context, sessionUID uuid.UUID, content string, retry bool) {
	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		response.InternalError(c, "当前服务不支持流式响应")
		return
	}
	setSSEHeaders(c)
	ctx := contextutil.NewContextWithGin(c)

	sw := &sseWriter{w: c.Writer, flusher: flusher}
	done := make(chan struct{})
	defer close(done)
	go func() { // 心跳:长回合多轮串行,防代理空闲超时
		ticker := time.NewTicker(sseHeartbeatInterval)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ctx.Done():
				return
			case <-ticker.C:
				sw.ping()
			}
		}
	}()

	if retry {
		cls.chat.RetryGroupTurn(ctx, sessionUID, content, sw.event)
		return
	}
	cls.chat.RunGroupTurn(ctx, sessionUID, content, sw.event)
}
