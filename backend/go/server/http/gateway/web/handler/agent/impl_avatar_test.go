// 头像字段契约回归测试(计划 Task 4 Go 部分)
// 契约: 空值合法;http/https 完整 URL(≤2048 字符,拒绝内嵌凭据);
//
//	data:image/(png|jpeg|webp|gif);base64,(总长 ≤1.5M 字符,payload 仅 base64 字符);
//	其余(相对路径/javascript:/vbscript:/blob:/其他 MIME/超长)→ 400 字段错误
//
// 真实 sqlite 内存库 + 真实 service + 真实 handler,不引入 mock
package agent

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/alchemy-furnace/server/internal/dao"
	"github.com/alchemy-furnace/server/internal/service/agent_service"
	"github.com/alchemy-furnace/server/internal/service/pill_inventory_service"
	"github.com/alchemy-furnace/server/model"
	"github.com/alchemy-furnace/server/server/http/middleware"
	"github.com/alchemy-furnace/server/server/http/router"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// avatarPNGBase64 1x1 红色 PNG 的 base64(仅含合法 base64 字符)
const avatarPNGBase64 = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg=="

// setupAvatarDB 在共享 setupTestDB 基础上补齐 llm_models/llm_providers 并造默认模型
// (UpdateAgent 对最终 active 状态会校验模型可用,需要真实模型行)
func setupAvatarDB(t *testing.T) *gorm.DB {
	t.Helper()
	db := setupTestDB(t)
	if err := db.AutoMigrate(&model.LLMProvider{}, &model.LLMModel{}); err != nil {
		t.Fatalf("迁移模型表失败: %v", err)
	}
	provider := model.LLMProvider{Name: "test", DisplayName: "Test", BaseURL: "http://localhost", IsEnabled: true}
	if err := db.Create(&provider).Error; err != nil {
		t.Fatalf("创建测试供应商失败: %v", err)
	}
	m := model.LLMModel{ProviderID: provider.UUID.String(), Name: "gpt-4o", DisplayName: "GPT-4o", IsEnabled: true}
	if err := db.Create(&m).Error; err != nil {
		t.Fatalf("创建默认模型失败: %v", err)
	}
	return db
}

// setupAvatarRouter 装配创建/更新道人路由(真实 service + handler)
// memory 参数本测试未涉及,传 nil
func setupAvatarRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(middleware.RequestID())
	h := New(agent_service.New(dao.NewAgentDao(), dao.NewModelDao(),
		pill_inventory_service.New(dao.GetDB(), time.Now)), nil)
	r.POST("/api/v1/agents", router.Wrapper(h.Create))
	r.PUT("/api/v1/agents/:uuid", router.Wrapper(h.Update))
	return r
}

// postJSON 发送 POST JSON 请求并解析响应包络
func postJSON(t *testing.T, r *gin.Engine, path string, body string) (int, map[string]interface{}) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	var envelope map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("解析响应包络失败: %v, body: %s", err, w.Body.String())
	}
	return w.Code, envelope
}

// createAvatarAgent 通过真实接口创建道人并返回其 UUID
func createAvatarAgent(t *testing.T, r *gin.Engine, avatar string) string {
	t.Helper()
	body := fmt.Sprintf(`{"name":"测试道人","avatar":%q}`, avatar)
	status, envelope := postJSON(t, r, "/api/v1/agents", body)
	if status != http.StatusCreated {
		t.Fatalf("预置道人失败: %d %v", status, envelope)
	}
	data, ok := envelope["data"].(map[string]interface{})
	if !ok {
		t.Fatalf("创建响应缺 data: %v", envelope)
	}
	uid, _ := data["id"].(string)
	if uid == "" {
		t.Fatalf("创建响应缺 id: %v", envelope)
	}
	return uid
}

// assertAvatar400 断言 400 字段错误(HTTP 状态 + 业务码),且消息不泄露 data URI 内容
func assertAvatar400(t *testing.T, status int, envelope map[string]interface{}) {
	t.Helper()
	if status != http.StatusBadRequest {
		t.Fatalf("期望 HTTP 400, 实际 %d, body: %v", status, envelope)
	}
	if code, ok := envelope["code"].(float64); !ok || int(code) != http.StatusBadRequest {
		t.Fatalf("期望业务码 400, 实际 %v", envelope["code"])
	}
	if msg, _ := envelope["message"].(string); strings.Contains(msg, "iVBOR") {
		t.Fatalf("错误消息泄露了 data URI 内容: %s", msg)
	}
}

// ---------- 创建 ----------

func TestCreateAgent_AvatarValidURLs(t *testing.T) {
	setupAvatarDB(t)
	r := setupAvatarRouter()
	for _, avatar := range []string{
		"http://cdn.example.com/avatar.png",
		"https://cdn.example.com/a/avatar.png?size=64",
		"HTTP://cdn.example.com/avatar.png", // 协议大小写不敏感
	} {
		status, envelope := postJSON(t, r, "/api/v1/agents",
			fmt.Sprintf(`{"name":"测试道人","avatar":%q}`, avatar))
		if status != http.StatusCreated {
			t.Fatalf("avatar=%q 期望 201, 实际 %d, body: %v", avatar, status, envelope)
		}
		got := envelope["data"].(map[string]interface{})["avatar"].(string)
		if got != avatar {
			t.Fatalf("avatar=%q 创建后返回 %q", avatar, got)
		}
	}
}

func TestCreateAgent_AvatarEmptyValid(t *testing.T) {
	setupAvatarDB(t)
	r := setupAvatarRouter()
	uid := createAvatarAgent(t, r, "")
	var agent model.DaoAgent
	if err := dao.DB.Where("uuid = ?", uid).First(&agent).Error; err != nil {
		t.Fatalf("查询道人失败: %v", err)
	}
	if agent.Avatar != "" {
		t.Fatalf("空头像应落库为空, 实际 %q", agent.Avatar)
	}
}

func TestCreateAgent_AvatarDataURIAccepted(t *testing.T) {
	setupAvatarDB(t)
	r := setupAvatarRouter()
	for _, mime := range []string{"png", "jpeg", "webp", "gif"} {
		avatar := fmt.Sprintf("data:image/%s;base64,%s", mime, avatarPNGBase64)
		status, envelope := postJSON(t, r, "/api/v1/agents",
			fmt.Sprintf(`{"name":"测试道人","avatar":%q}`, avatar))
		if status != http.StatusCreated {
			t.Fatalf("mime=%s 期望 201, 实际 %d, body: %v", mime, status, envelope)
		}
		got := envelope["data"].(map[string]interface{})["avatar"].(string)
		if got != avatar {
			t.Fatalf("mime=%s 创建后返回不一致", mime)
		}
	}
}

func TestCreateAgent_AvatarDataURIOver255CharsPersists(t *testing.T) {
	// 契约: TEXT 列必须能存下超过 255 字符的 data URI(原 VARCHAR(255) 会保存失败/截断)
	setupAvatarDB(t)
	r := setupAvatarRouter()
	avatar := "data:image/png;base64," + strings.Repeat("A", 1024)
	status, envelope := postJSON(t, r, "/api/v1/agents",
		fmt.Sprintf(`{"name":"测试道人","avatar":%q}`, avatar))
	if status != http.StatusCreated {
		t.Fatalf("期望 201, 实际 %d, body: %v", status, envelope)
	}
	got := envelope["data"].(map[string]interface{})["avatar"].(string)
	if got != avatar {
		t.Fatalf("长 data URI 未原样返回(列可能被截断)")
	}
}

func TestCreateAgent_AvatarRelativePathRejected(t *testing.T) {
	setupAvatarDB(t)
	r := setupAvatarRouter()
	status, envelope := postJSON(t, r, "/api/v1/agents", `{"name":"测试道人","avatar":"/avatar.png"}`)
	assertAvatar400(t, status, envelope)
}

func TestCreateAgent_AvatarExecutableSchemesRejected(t *testing.T) {
	setupAvatarDB(t)
	r := setupAvatarRouter()
	for _, avatar := range []string{
		"javascript:alert(1)",
		"vbscript:msgbox('x')",
		"blob:https://example.com/abc",
		"file:///etc/passwd",
	} {
		status, envelope := postJSON(t, r, "/api/v1/agents",
			fmt.Sprintf(`{"name":"测试道人","avatar":%q}`, avatar))
		assertAvatar400(t, status, envelope)
	}
}

func TestCreateAgent_AvatarURLCredentialsRejected(t *testing.T) {
	setupAvatarDB(t)
	r := setupAvatarRouter()
	for _, avatar := range []string{
		"http://user:pass@cdn.example.com/avatar.png",
		"https://user@cdn.example.com/avatar.png",
	} {
		status, envelope := postJSON(t, r, "/api/v1/agents",
			fmt.Sprintf(`{"name":"测试道人","avatar":%q}`, avatar))
		assertAvatar400(t, status, envelope)
	}
}

func TestCreateAgent_AvatarURLTooLongRejected(t *testing.T) {
	setupAvatarDB(t)
	r := setupAvatarRouter()
	avatar := "https://cdn.example.com/" + strings.Repeat("a", 2049)
	status, envelope := postJSON(t, r, "/api/v1/agents",
		fmt.Sprintf(`{"name":"测试道人","avatar":%q}`, avatar))
	assertAvatar400(t, status, envelope)
}

func TestCreateAgent_AvatarDataURIWrongMIMERejected(t *testing.T) {
	setupAvatarDB(t)
	r := setupAvatarRouter()
	for _, avatar := range []string{
		"data:image/svg+xml;base64,AAAA",
		"data:image/png,rawdata",
		"data:text/plain;base64,AAAA",
	} {
		status, envelope := postJSON(t, r, "/api/v1/agents",
			fmt.Sprintf(`{"name":"测试道人","avatar":%q}`, avatar))
		assertAvatar400(t, status, envelope)
	}
}

func TestCreateAgent_AvatarDataURIOverlongRejected(t *testing.T) {
	setupAvatarDB(t)
	r := setupAvatarRouter()
	avatar := "data:image/png;base64," + strings.Repeat("A", 1_500_001)
	status, envelope := postJSON(t, r, "/api/v1/agents",
		fmt.Sprintf(`{"name":"测试道人","avatar":%q}`, avatar))
	assertAvatar400(t, status, envelope)
}

func TestCreateAgent_AvatarDataURIInvalidCharsRejected(t *testing.T) {
	setupAvatarDB(t)
	r := setupAvatarRouter()
	for _, payload := range []string{"@@@", "A A", avatarPNGBase64 + "!"} {
		avatar := "data:image/png;base64," + payload
		status, envelope := postJSON(t, r, "/api/v1/agents",
			fmt.Sprintf(`{"name":"测试道人","avatar":%q}`, avatar))
		assertAvatar400(t, status, envelope)
	}
}

// ---------- 更新 ----------

func TestUpdateAgent_AvatarSet(t *testing.T) {
	setupAvatarDB(t)
	r := setupAvatarRouter()
	uid := createAvatarAgent(t, r, "")
	avatar := "https://cdn.example.com/avatar.png"
	status, envelope := putJSON(t, r, "/api/v1/agents/"+uid,
		fmt.Sprintf(`{"avatar":%q}`, avatar))
	if status != http.StatusOK {
		t.Fatalf("期望 200, 实际 %d, body: %v", status, envelope)
	}
	got := envelope["data"].(map[string]interface{})["avatar"].(string)
	if got != avatar {
		t.Fatalf("更新后 avatar=%q, 期望 %q", got, avatar)
	}
}

func TestUpdateAgent_AvatarClear(t *testing.T) {
	setupAvatarDB(t)
	r := setupAvatarRouter()
	uid := createAvatarAgent(t, r, "https://cdn.example.com/old.png")
	status, envelope := putJSON(t, r, "/api/v1/agents/"+uid, `{"avatar":""}`)
	if status != http.StatusOK {
		t.Fatalf("期望 200, 实际 %d, body: %v", status, envelope)
	}
	got := envelope["data"].(map[string]interface{})["avatar"].(string)
	if got != "" {
		t.Fatalf("清空后 avatar=%q, 期望空串", got)
	}
}

func TestUpdateAgent_AvatarDataURIPersists(t *testing.T) {
	setupAvatarDB(t)
	r := setupAvatarRouter()
	uid := createAvatarAgent(t, r, "")
	avatar := "data:image/webp;base64," + strings.Repeat("B", 2000)
	status, envelope := putJSON(t, r, "/api/v1/agents/"+uid,
		fmt.Sprintf(`{"avatar":%q}`, avatar))
	if status != http.StatusOK {
		t.Fatalf("期望 200, 实际 %d, body: %v", status, envelope)
	}
	got := envelope["data"].(map[string]interface{})["avatar"].(string)
	if got != avatar {
		t.Fatalf("长 data URI 更新后未原样返回(列可能被截断)")
	}
}

func TestUpdateAgent_AvatarInvalidRejected(t *testing.T) {
	setupAvatarDB(t)
	r := setupAvatarRouter()
	uid := createAvatarAgent(t, r, "")
	for _, avatar := range []string{
		"/avatar.png",
		"javascript:alert(1)",
		"data:image/svg+xml;base64,AAAA",
		"https://cdn.example.com/" + strings.Repeat("a", 2049),
	} {
		status, envelope := putJSON(t, r, "/api/v1/agents/"+uid,
			fmt.Sprintf(`{"avatar":%q}`, avatar))
		assertAvatar400(t, status, envelope)
	}
}
