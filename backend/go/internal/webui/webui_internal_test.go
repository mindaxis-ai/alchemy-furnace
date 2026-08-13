// webui_internal_test.go - 验证 webui Handler 自身状态码
package webui

import (
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestHandler_IndexStatus(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/", nil)
	Handler().ServeHTTP(rec, req)
	require.Equal(t, 200, rec.Code, "根路径 index.html 应返 200")
}
