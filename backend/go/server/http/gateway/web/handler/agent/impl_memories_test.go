// 记忆管理接口回归测试(P3 Task 3)
// 桩 iservice.Memory + 真实 agent service + sqlite 内存库:
// 覆盖 5 个记忆端点(列表/创建/更新/删除/清空)与 memory_enabled 更新透传
package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alchemy-furnace/server/internal/dao"
	"github.com/alchemy-furnace/server/internal/errors"
	"github.com/alchemy-furnace/server/internal/interface/service"
	"github.com/alchemy-furnace/server/internal/service/agent_service"
	"github.com/alchemy-furnace/server/internal/service/pill_inventory_service"
	"github.com/alchemy-furnace/server/model"
	"github.com/alchemy-furnace/server/server/http/middleware"
	"github.com/alchemy-furnace/server/server/http/router"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// stubMemory 记忆服务桩:内存 map 保存记忆并记录查询参数
type stubMemory struct {
	mu       sync.Mutex
	memories map[string]*model.AgentMemory // key: memory UUID 字符串
	lastKind string
	lastAct  bool
}

func newStubMemory() *stubMemory {
	return &stubMemory{memories: make(map[string]*model.AgentMemory)}
}

func (s *stubMemory) ListMemories(_ context.Context, _ string, kind string, onlyActive bool) ([]*model.AgentMemory, errors.Error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastKind, s.lastAct = kind, onlyActive
	out := make([]*model.AgentMemory, 0, len(s.memories))
	for _, m := range s.memories {
		if onlyActive && m.Status != "active" {
			continue
		}
		out = append(out, m)
	}
	return out, nil
}

func (s *stubMemory) CreateMemory(_ context.Context, agentUID string, in service.MemoryInput) (*model.AgentMemory, errors.Error) {
	// 校验镜像真实 service 的 validateInput(创建语义: kind/content 必填)
	switch in.Kind {
	case "user_fact", "user_preference", "relationship", "open_loop", "episode":
	default:
		return nil, errors.New(errors.ErrorTypeInvalidRequest, "memory.invalid_kind", "非法的记忆类型")
	}
	if strings.TrimSpace(in.Content) == "" {
		return nil, errors.New(errors.ErrorTypeInvalidRequest, "memory.invalid_content", "记忆内容不能为空且不超过 500 字")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	m := &model.AgentMemory{
		UUID:       uuid.New(),
		AgentID:    agentUID,
		Kind:       in.Kind,
		Content:    in.Content,
		Importance: 3,
		Confidence: 0.8,
		Status:     "active",
	}
	if in.Importance != nil {
		m.Importance = *in.Importance
	}
	if in.Confidence != nil {
		m.Confidence = *in.Confidence
	}
	if in.Pinned != nil {
		m.Pinned = *in.Pinned
	}
	for _, kw := range in.Keywords {
		m.Keywords = append(m.Keywords, kw)
	}
	s.memories[m.UUID.String()] = m
	return m, nil
}

func (s *stubMemory) UpdateMemory(_ context.Context, _ string, memoryUUID uuid.UUID, in service.MemoryInput) (*model.AgentMemory, errors.Error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	m, ok := s.memories[memoryUUID.String()]
	if !ok {
		return nil, errors.ErrorRecordNotFound("stub.memory.update")
	}
	if in.Content != "" {
		m.Content = in.Content
	}
	if in.Kind != "" {
		m.Kind = in.Kind
	}
	if in.Importance != nil {
		m.Importance = *in.Importance
	}
	if in.Confidence != nil {
		m.Confidence = *in.Confidence
	}
	if in.Pinned != nil {
		m.Pinned = *in.Pinned
	}
	return m, nil
}

func (s *stubMemory) DeleteMemory(_ context.Context, _ string, memoryUUID uuid.UUID) errors.Error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.memories[memoryUUID.String()]; !ok {
		return errors.ErrorRecordNotFound("stub.memory.delete")
	}
	delete(s.memories, memoryUUID.String())
	return nil
}

func (s *stubMemory) ClearMemories(_ context.Context, _ string) (int64, errors.Error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := int64(len(s.memories))
	s.memories = make(map[string]*model.AgentMemory)
	return n, nil
}

func (s *stubMemory) Retrieve(_ context.Context, _ string, _ string) ([]service.MemorySnippet, errors.Error) {
	return []service.MemorySnippet{{Kind: "user_fact", Content: "用户喜欢围棋"}}, nil
}

func (s *stubMemory) EnqueueDistillation(_ context.Context, _ service.DistillationSpec) bool {
	return true
}
func (s *stubMemory) Close() {}

// setupMemoryRouter 装配记忆路由(记忆 service 用桩,agent service 真实)+ PUT 道人路由
func setupMemoryRouter(stub *stubMemory) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(middleware.RequestID())
	h := New(agent_service.New(dao.NewAgentDao(), dao.NewModelDao(),
		pill_inventory_service.New(dao.GetDB(), time.Now)), stub)
	r.PUT("/api/v1/agents/:uuid", router.Wrapper(h.Update))
	memories := r.Group("/api/v1/agents/:uuid/memories")
	{
		memories.GET("", router.Wrapper(h.ListMemories))
		memories.POST("", router.Wrapper(h.CreateMemory))
		memories.PATCH("/:memory_uuid", router.Wrapper(h.UpdateMemory))
		memories.DELETE("/:memory_uuid", router.Wrapper(h.DeleteMemory))
		memories.DELETE("", router.Wrapper(h.ClearMemories))
	}
	return r
}

// seedMemoryAgent 直落库造一个道人,返回其 UUID
func seedMemoryAgent(t *testing.T, db *gorm.DB) string {
	t.Helper()
	agent := model.DaoAgent{Name: "记忆道人", ModelName: "gpt-4o", Status: "active"}
	if err := db.Create(&agent).Error; err != nil {
		t.Fatalf("创建测试道人失败: %v", err)
	}
	return agent.UUID.String()
}

// doJSON 发送任意方法 JSON 请求并解析响应包络
func doJSON(t *testing.T, r *gin.Engine, method, path, body string) (int, map[string]interface{}) {
	t.Helper()
	req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	var envelope map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("解析响应包络失败: %v, body: %s", err, w.Body.String())
	}
	return w.Code, envelope
}

// createMemoryViaAPI 通过接口创建一条记忆,返回其 UUID
func createMemoryViaAPI(t *testing.T, r *gin.Engine, agentUUID, body string) string {
	t.Helper()
	status, envelope := doJSON(t, r, http.MethodPost, "/api/v1/agents/"+agentUUID+"/memories", body)
	if status != http.StatusCreated {
		t.Fatalf("预置记忆失败: %d %v", status, envelope)
	}
	memUUID, _ := envelope["data"].(map[string]interface{})["id"].(string)
	if memUUID == "" {
		t.Fatalf("创建记忆响应缺 id: %v", envelope)
	}
	return memUUID
}

// ---------- 列表 ----------

func TestListMemories(t *testing.T) {
	db := setupTestDB(t)
	stub := newStubMemory()
	r := setupMemoryRouter(stub)
	agentUUID := seedMemoryAgent(t, db)

	m := &model.AgentMemory{UUID: uuid.New(), Kind: "user_fact", Content: "用户喜欢围棋", Status: "active"}
	stub.mu.Lock()
	stub.memories[m.UUID.String()] = m
	stub.mu.Unlock()

	status, envelope := doJSON(t, r, http.MethodGet, "/api/v1/agents/"+agentUUID+"/memories", "")
	if status != http.StatusOK {
		t.Fatalf("期望 HTTP 200, 实际 %d, body: %v", status, envelope)
	}
	list, ok := envelope["data"].([]interface{})
	if !ok || len(list) != 1 {
		t.Fatalf("data 应为 1 条记忆数组: %v", envelope["data"])
	}
	item := list[0].(map[string]interface{})
	if item["id"] != m.UUID.String() || item["kind"] != "user_fact" || item["content"] != "用户喜欢围棋" {
		t.Fatalf("列表字段缺失: %v", item)
	}
	if _, ok := item["importance"]; !ok {
		t.Fatalf("列表缺 importance 字段: %v", item)
	}
	if _, ok := item["created_at"]; !ok {
		t.Fatalf("列表缺 created_at 字段: %v", item)
	}

	// kind/active 查询参数透传
	status, _ = doJSON(t, r, http.MethodGet,
		"/api/v1/agents/"+agentUUID+"/memories?kind=user_fact&active=false", "")
	if status != http.StatusOK {
		t.Fatalf("带参查询期望 200, 实际 %d", status)
	}
	stub.mu.Lock()
	defer stub.mu.Unlock()
	if stub.lastKind != "user_fact" || stub.lastAct {
		t.Fatalf("查询参数未透传: kind=%q onlyActive=%v", stub.lastKind, stub.lastAct)
	}
}

func TestListMemoriesUnknownAgent404(t *testing.T) {
	setupTestDB(t)
	stub := newStubMemory()
	r := setupMemoryRouter(stub)

	status, envelope := doJSON(t, r, http.MethodGet, "/api/v1/agents/"+uuid.NewString()+"/memories", "")
	if status != http.StatusNotFound {
		t.Fatalf("未知道人期望 404, 实际 %d, body: %v", status, envelope)
	}
}

// ---------- 创建 ----------

func TestCreateMemory(t *testing.T) {
	db := setupTestDB(t)
	stub := newStubMemory()
	r := setupMemoryRouter(stub)
	agentUUID := seedMemoryAgent(t, db)

	status, envelope := doJSON(t, r, http.MethodPost, "/api/v1/agents/"+agentUUID+"/memories",
		`{"kind":"user_fact","content":"用户喜欢围棋","keywords":["围棋"],"importance":4,"confidence":0.9}`)
	if status != http.StatusCreated {
		t.Fatalf("创建记忆期望 201, 实际 %d, body: %v", status, envelope)
	}
	data := envelope["data"].(map[string]interface{})
	if data["id"] == nil || data["content"] != "用户喜欢围棋" || data["kind"] != "user_fact" {
		t.Fatalf("创建响应字段缺失: %v", data)
	}
	if len(stub.memories) != 1 {
		t.Fatalf("桩中应有 1 条记忆, 实际 %d", len(stub.memories))
	}

	// 非法 kind → 400
	status, envelope = doJSON(t, r, http.MethodPost, "/api/v1/agents/"+agentUUID+"/memories",
		`{"kind":"bogus","content":"x"}`)
	if status != http.StatusBadRequest {
		t.Fatalf("非法 kind 期望 400, 实际 %d, body: %v", status, envelope)
	}

	// 缺 content → 400(service 层校验)
	status, envelope = doJSON(t, r, http.MethodPost, "/api/v1/agents/"+agentUUID+"/memories",
		`{"kind":"user_fact"}`)
	if status != http.StatusBadRequest {
		t.Fatalf("空内容期望 400, 实际 %d, body: %v", status, envelope)
	}
}

// ---------- 更新 ----------

func TestUpdateMemoryPartial(t *testing.T) {
	db := setupTestDB(t)
	stub := newStubMemory()
	r := setupMemoryRouter(stub)
	agentUUID := seedMemoryAgent(t, db)
	memUUID := createMemoryViaAPI(t, r, agentUUID, `{"kind":"user_preference","content":"用户偏好安静"}`)

	// 仅改 pinned(部分更新契约)
	status, envelope := doJSON(t, r, http.MethodPatch,
		"/api/v1/agents/"+agentUUID+"/memories/"+memUUID, `{"pinned":true}`)
	if status != http.StatusOK {
		t.Fatalf("部分更新期望 200, 实际 %d, body: %v", status, envelope)
	}
	data := envelope["data"].(map[string]interface{})
	if data["pinned"] != true {
		t.Fatalf("pinned 应为 true: %v", data)
	}
	if data["content"] != "用户偏好安静" {
		t.Fatalf("部分更新不应改动 content: %v", data)
	}
}

func TestUpdateMemoryNotFound404(t *testing.T) {
	db := setupTestDB(t)
	stub := newStubMemory()
	r := setupMemoryRouter(stub)
	agentUUID := seedMemoryAgent(t, db)

	status, envelope := doJSON(t, r, http.MethodPatch,
		"/api/v1/agents/"+agentUUID+"/memories/"+uuid.NewString(), `{"pinned":true}`)
	if status != http.StatusNotFound {
		t.Fatalf("未知记忆期望 404, 实际 %d, body: %v", status, envelope)
	}
}

// ---------- 删除 / 清空 ----------

func TestDeleteMemory(t *testing.T) {
	db := setupTestDB(t)
	stub := newStubMemory()
	r := setupMemoryRouter(stub)
	agentUUID := seedMemoryAgent(t, db)
	memUUID := createMemoryViaAPI(t, r, agentUUID, `{"kind":"episode","content":"一次对话"}`)

	status, envelope := doJSON(t, r, http.MethodDelete,
		"/api/v1/agents/"+agentUUID+"/memories/"+memUUID, "")
	if status != http.StatusOK {
		t.Fatalf("删除期望 200, 实际 %d, body: %v", status, envelope)
	}
	if envelope["data"].(map[string]interface{})["deleted"] != true {
		t.Fatalf("删除响应缺 deleted: %v", envelope["data"])
	}
	stub.mu.Lock()
	defer stub.mu.Unlock()
	if len(stub.memories) != 0 {
		t.Fatalf("删除后桩中应无记忆, 实际 %d", len(stub.memories))
	}
}

func TestClearMemories(t *testing.T) {
	db := setupTestDB(t)
	stub := newStubMemory()
	r := setupMemoryRouter(stub)
	agentUUID := seedMemoryAgent(t, db)
	createMemoryViaAPI(t, r, agentUUID, `{"kind":"episode","content":"第一次对话"}`)
	createMemoryViaAPI(t, r, agentUUID, `{"kind":"user_fact","content":"用户喜欢茶"}`)

	status, envelope := doJSON(t, r, http.MethodDelete, "/api/v1/agents/"+agentUUID+"/memories", "")
	if status != http.StatusOK {
		t.Fatalf("清空期望 200, 实际 %d, body: %v", status, envelope)
	}
	data := envelope["data"].(map[string]interface{})
	if data["deleted_count"] != float64(2) {
		t.Fatalf("清空应删除 2 条: %v", data)
	}
	stub.mu.Lock()
	defer stub.mu.Unlock()
	if len(stub.memories) != 0 {
		t.Fatalf("清空后桩中应无记忆, 实际 %d", len(stub.memories))
	}
}

// ---------- memory_enabled 更新 ----------

func TestUpdateAgentMemoryEnabled(t *testing.T) {
	setupAvatarDB(t)
	r := setupAvatarRouter()
	uid := createAvatarAgent(t, r, "")

	status, envelope := putJSON(t, r, "/api/v1/agents/"+uid, `{"memory_enabled": false}`)
	if status != http.StatusOK {
		t.Fatalf("关闭记忆期望 200, 实际 %d, body: %v", status, envelope)
	}
	data := envelope["data"].(map[string]interface{})
	if data["memory_enabled"] != false {
		t.Fatalf("响应 memory_enabled 应为 false: %v", data)
	}

	// 落库验证
	var agent model.DaoAgent
	if err := dao.DB.Where("uuid = ?", uid).First(&agent).Error; err != nil {
		t.Fatalf("查询道人失败: %v", err)
	}
	if agent.MemoryEnabled {
		t.Fatal("memory_enabled=false 应落库")
	}

	// 置回 true
	status, envelope = putJSON(t, r, "/api/v1/agents/"+uid, `{"memory_enabled": true}`)
	if status != http.StatusOK {
		t.Fatalf("开启记忆期望 200, 实际 %d, body: %v", status, envelope)
	}
	if envelope["data"].(map[string]interface{})["memory_enabled"] != true {
		t.Fatalf("响应 memory_enabled 应为 true: %v", envelope["data"])
	}
}
