package errors

import (
	"net/http"
	"testing"
)

func TestHTTPStatusMapsServiceUnavailableTo503(t *testing.T) {
	err := New(ErrorTypeServiceUnavailable, "service.distillation.remote", "公开资料源当前不可用，请稍后重试")
	if got := HTTPStatus(err); got != http.StatusServiceUnavailable {
		t.Fatalf("HTTPStatus = %d, want %d", got, http.StatusServiceUnavailable)
	}
}
