// UUID 业务键 HTTP 契约锁定测试(011 重构 Task 8)
// 契约: JSON 响应中所有名为 id 或以 _id 结尾的业务字段必须是 UUID 字符串
// (uuid.Parse 通过);响应不得出现 uuid 键,也不得出现数字内部主键
// (BIGSERIAL id 泄漏,JSON 解码后即 float64)。
// 说明: 011 Task 8 旧入口审计后,道人详情不再内嵌金丹/能力快照(能力展示迁至
// pill_inventory 包的 GET /api/v1/agents/:uuid/effects,其契约由该包同款测试锁定),
// 本包关系面为列表 id 与详情的 language_pattern 嵌套对象。
// 夹具: 复用同包既有 setupTestDB/getJSON/seedAgentForBindPill,真实 sqlite 内存库,
// 关系字段一律用父实体业务主键(.DaoAgentID 等,uuid 文本;禁用内部 .ID),经 httptest 发真实请求。
package agent

import (
	"fmt"
	"net/http"
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
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// setupContractRouter 注册道人列表与详情路由(包装器与真实网关 router.go 一致)
func setupContractRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(middleware.RequestID())
	h := New(agent_service.New(dao.NewAgentDao(), dao.NewModelDao(),
		pill_inventory_service.New(dao.GetDB(), time.Now)), nil)
	r.GET("/api/v1/agents", router.WrapperPage(h.List))
	r.GET("/api/v1/agents/:uuid", router.Wrapper(h.Get))
	return r
}

// seedContractAgents 直落库造 n 个道人,返回对外 UUID 列表
func seedContractAgents(t *testing.T, db *gorm.DB, names ...string) []string {
	t.Helper()
	uids := make([]string, 0, len(names))
	for _, name := range names {
		a := model.DaoAgent{Name: name, ModelName: "gpt-4o", Status: "active"}
		if err := db.Create(&a).Error; err != nil {
			t.Fatalf("创建道人 %s 失败: %v", name, err)
		}
		uids = append(uids, a.DaoAgentID)
	}
	return uids
}

// TestUUIDContractAgentList 道人列表: 每条 id 都是业务 UUID(集合等于种子 UUID),
// 整个响应包络(含嵌套)无 uuid 键、无数字 id
func TestUUIDContractAgentList(t *testing.T) {
	db := setupTestDB(t)
	seeded := seedContractAgents(t, db, "契约道人甲", "契约道人乙")
	r := setupContractRouter()

	status, envelope := getJSON(t, r, "/api/v1/agents?page=1&page_size=10")
	if status != http.StatusOK {
		t.Fatalf("期望 HTTP 200, 实际 %d, body: %v", status, envelope)
	}

	data, _ := envelope["data"].(map[string]interface{})
	list, ok := data["list"].([]interface{})
	if !ok || len(list) != len(seeded) {
		t.Fatalf("列表应含 %d 条, 实际: %v", len(seeded), envelope)
	}
	ids := make(map[string]bool, len(list))
	for _, item := range list {
		m, _ := item.(map[string]interface{})
		id, _ := m["id"].(string)
		ids[id] = true
	}
	for _, uid := range seeded {
		if !ids[uid] {
			t.Fatalf("列表缺少种子道人业务 UUID %s(内部主键泄漏或序列化错误): %v", uid, ids)
		}
	}

	assertUUIDContract(t, envelope, "GET /agents")
}

// TestUUIDContractAgentDetail 道人详情: id 为业务 UUID,语言模式嵌套对象可被
// 递归断言穿透(其内无 id 类字段,但嵌套遍历路径必须正确)
func TestUUIDContractAgentDetail(t *testing.T) {
	db := setupTestDB(t)
	agentUUID, _ := seedAgentForBindPill(t, db)

	var agent model.DaoAgent
	if err := db.Where("dao_agent_id = ?", agentUUID).First(&agent).Error; err != nil {
		t.Fatalf("查询道人失败: %v", err)
	}
	if err := db.Create(&model.LanguagePattern{
		AgentID: agent.DaoAgentID, SystemPrompt: "cached", SourceFingerprint: "sha256:x", IsValid: true,
	}).Error; err != nil {
		t.Fatalf("创建语言模式缓存失败: %v", err)
	}

	r := setupContractRouter()
	status, envelope := getJSON(t, r, fmt.Sprintf("/api/v1/agents/%s", agentUUID))
	if status != http.StatusOK {
		t.Fatalf("期望 HTTP 200, 实际 %d, body: %v", status, envelope)
	}

	data, _ := envelope["data"].(map[string]interface{})
	if data["id"] != agentUUID {
		t.Fatalf("详情 id 应为业务 UUID %s, 实际 %v", agentUUID, data["id"])
	}
	if _, has := data["language_pattern"]; !has {
		t.Fatalf("详情缺少 language_pattern 嵌套面: %v", data)
	}

	assertUUIDContract(t, envelope, fmt.Sprintf("GET /agents/%s", agentUUID))
}

// TestUUIDContractAgentMemories 锁定记忆端点契约: 对外键为 id(禁止 uuid 键),
// 无来源(手工录入)的记忆缺省 source_session_id/source_message_id(禁止空串 *_id)。
func TestUUIDContractAgentMemories(t *testing.T) {
	db := setupTestDB(t)
	r := setupMemoryRouter(newStubMemory())
	agentUUID := seedMemoryAgent(t, db)

	memUUID := createMemoryViaAPI(t, r, agentUUID,
		`{"kind":"user_preference","content":"用户偏好安静"}`)

	status, envelope := doJSON(t, r, http.MethodGet, "/api/v1/agents/"+agentUUID+"/memories", "")
	if status != http.StatusOK {
		t.Fatalf("GET memories 期望 200, 实际 %d: %v", status, envelope)
	}
	list, ok := envelope["data"].([]interface{})
	if !ok || len(list) != 1 {
		t.Fatalf("data 应为 1 条记忆数组: %v", envelope["data"])
	}
	item := list[0].(map[string]interface{})
	if item["id"] != memUUID {
		t.Fatalf("记忆 id = %v, want %s(011 契约: 对外键为 id)", item["id"], memUUID)
	}
	if _, has := item["uuid"]; has {
		t.Fatalf("记忆响应出现 uuid 键(011 契约禁止): %v", item)
	}
	if _, has := item["source_session_id"]; has {
		t.Fatalf("无来源记忆不应出现 source_session_id 键(空串违契): %v", item)
	}
	assertUUIDContract(t, envelope, "GET /agents/:uuid/memories")
}

// TestUUIDContractDetectorSelfCheck 违约检测器自检: 构造含数字 id / uuid 键 /
// 嵌套违约 / 非法字符串 id 的假响应喂给检测器,断言逐一定位报错;守约响应零误报。
// 锁定检测器本身有效,防止断言函数退化为恒真。
func TestUUIDContractDetectorSelfCheck(t *testing.T) {
	// 数字内部主键泄漏: id=7
	violations := uuidContractViolations(map[string]interface{}{
		"code": float64(0), "data": map[string]interface{}{"id": float64(7)},
	}, "fake")
	if len(violations) != 1 || !strings.Contains(violations[0], "fake.data.id") {
		t.Fatalf("检测器未定位数字 id 违约: %v", violations)
	}

	// uuid 键违约(值即使是合法 UUID 也违约: 对外标识必须命名为 id)
	violations = uuidContractViolations(map[string]interface{}{
		"data": map[string]interface{}{"uuid": uuid.NewString()},
	}, "fake")
	if len(violations) != 1 || !strings.Contains(violations[0], "fake.data.uuid") {
		t.Fatalf("检测器未报告 uuid 键违约: %v", violations)
	}

	// 嵌套数组内 *_id 数字泄漏,路径须可定位
	violations = uuidContractViolations(map[string]interface{}{
		"data": map[string]interface{}{"list": []interface{}{
			map[string]interface{}{"id": uuid.NewString()},
			map[string]interface{}{"recipe_id": float64(3)},
		}},
	}, "fake")
	if len(violations) != 1 || !strings.Contains(violations[0], "fake.data.list[1].recipe_id") {
		t.Fatalf("检测器未定位嵌套违约路径: %v", violations)
	}

	// 非法字符串 id 同样违约
	violations = uuidContractViolations(map[string]interface{}{"item_id": "not-a-uuid"}, "fake")
	if len(violations) != 1 || !strings.Contains(violations[0], "fake.item_id") {
		t.Fatalf("检测器未报告非法字符串 id: %v", violations)
	}

	// 守约响应零误报(含 request_id、非 id 的数字/字符串字段、数组)
	clean := map[string]interface{}{
		"code": float64(0), "request_id": uuid.NewString(),
		"data": map[string]interface{}{"list": []interface{}{
			map[string]interface{}{"id": uuid.NewString(), "recipe_id": uuid.NewString(),
				"name": "x", "count": float64(2)},
		}},
	}
	if got := uuidContractViolations(clean, "ok"); len(got) != 0 {
		t.Fatalf("守约响应被误报: %v", got)
	}

	// memory_id 编排复合键 "<agentUUID>#<序号>" 豁免(非 HTTP 契约业务键);
	// 垃圾形态仍违约,豁免不得放行任意文本
	if got := uuidContractViolations(map[string]interface{}{"memory_id": uuid.NewString() + "#3"}, "ok"); len(got) != 0 {
		t.Fatalf("编排复合键被误报: %v", got)
	}
	if got := uuidContractViolations(map[string]interface{}{"memory_id": "junk#x"}, "ok"); len(got) != 1 {
		t.Fatalf("垃圾 memory_id 应违约: %v", got)
	}
}

// ---------- 递归断言器 ----------

// assertUUIDContract 递归断言 v 满足 UUID 契约,违约逐条 t.Errorf(路径可定位)
func assertUUIDContract(t *testing.T, v interface{}, path string) {
	t.Helper()
	for _, violation := range uuidContractViolations(v, path) {
		t.Errorf("UUID 契约违约: %s", violation)
	}
}

// uuidContractViolations 深度遍历 v(json.Unmarshal 产物: map/slice/标量),
// 收集 UUID 契约违约描述;空返回 = 守约。规则:
//   - 键名恰为 id 或以 _id 结尾: 值必须是 uuid.Parse 通过的字符串;
//     数字(float64,即内部 BIGSERIAL 主键)/null/其他类型一律违约
//   - 键名恰为 uuid: 违约(对外标识一律命名为 id)
//   - 例外: memory_id 允许编排内部复合键 "<agentUUID>#<序号>"
//     (internal/service/orchestration/types.go 的图状态键,非 HTTP 契约业务键),
//     但前缀仍须为合法 UUID 且序号为纯数字
func uuidContractViolations(v interface{}, path string) []string {
	switch node := v.(type) {
	case map[string]interface{}:
		var out []string
		for key, val := range node {
			child := path + "." + key
			if key == "uuid" {
				out = append(out, child+" 出现 uuid 键(对外标识必须命名为 id)")
				continue
			}
			if key == "id" || strings.HasSuffix(key, "_id") {
				if msg := contractIDValueMsg(key, val, child); msg != "" {
					out = append(out, msg)
				}
				continue
			}
			out = append(out, uuidContractViolations(val, child)...)
		}
		return out
	case []interface{}:
		var out []string
		for i, item := range node {
			out = append(out, uuidContractViolations(item, fmt.Sprintf("%s[%d]", path, i))...)
		}
		return out
	}
	return nil
}

// contractIDValueMsg 校验单个 id 类键值;空串 = 守约
func contractIDValueMsg(key string, val interface{}, path string) string {
	s, ok := val.(string)
	if !ok {
		return fmt.Sprintf("%s 须为 UUID 字符串, 实际 %T(%v)(内部主键泄漏)", path, val, val)
	}
	if _, err := uuid.Parse(s); err == nil {
		return ""
	}
	if key == "memory_id" && isOrchestrationMemoryKey(s) {
		return ""
	}
	return fmt.Sprintf("%s 不是合法 UUID: %q", path, s)
}

// isOrchestrationMemoryKey 判断 s 是否编排内部复合键 "<UUID>#<纯数字序号>"
func isOrchestrationMemoryKey(s string) bool {
	idx := strings.LastIndex(s, "#")
	if idx < 0 {
		return false
	}
	if _, err := uuid.Parse(s[:idx]); err != nil {
		return false
	}
	seq := s[idx+1:]
	if seq == "" {
		return false
	}
	for _, r := range seq {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
