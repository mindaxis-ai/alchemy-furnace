package chat_service

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

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
