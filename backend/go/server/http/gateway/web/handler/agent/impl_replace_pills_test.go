// 完整服丹编排接口(PUT /api/v1/agents/:uuid/pills)回归测试
// 任务 5 起该写路由恒 410 pill.legacy_api_removed(防绕过库存直接写绑定):
// 路径与载荷一律不解析,任何请求(含非法 UUID)都返回 410,不产生任何写入
// 真实 sqlite 内存库 + 真实 service + 真实 handler,不引入 mock
package agent

import (
	"fmt"
	"net/http"
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

// setupReplacePillsRouter 注册完整服丹编排路由(仅测本路由;memory 参数本测试未涉及,传 nil)
func setupReplacePillsRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(middleware.RequestID())
	h := New(agent_service.New(dao.NewAgentDao(), dao.NewModelDao(),
		pill_inventory_service.New(dao.GetDB(), time.Now)), nil)
	r.PUT("/api/v1/agents/:uuid/pills", router.Wrapper(h.ReplacePills))
	return r
}

// seedAgentWithPillsForReplace 造一个道人与 n 枚金丹,返回道人 UUID 与金丹 UUID 列表
func seedAgentWithPillsForReplace(t *testing.T, db *gorm.DB, pillCount int) (string, []string) {
	t.Helper()
	agent := model.DaoAgent{Name: "编排道人", ModelName: "gpt-4o", Status: "active"}
	if err := db.Create(&agent).Error; err != nil {
		t.Fatalf("创建道人失败: %v", err)
	}
	pillUUIDs := make([]string, 0, pillCount)
	for i := 0; i < pillCount; i++ {
		p := model.ElixirPill{Name: uuid.NewString(), SkillSchema: model.JSONMap{"identity_card": "x"}}
		if err := db.Create(&p).Error; err != nil {
			t.Fatalf("创建金丹失败: %v", err)
		}
		pillUUIDs = append(pillUUIDs, p.ElixirPillID)
	}
	return agent.DaoAgentID, pillUUIDs
}

// assertGone410 断言 410 + 稳定错误码 pill.legacy_api_removed
// envelope.code 是路由业务码(数字),稳定错误码在 error_code 字段(字符串)
func assertGone410(t *testing.T, status int, envelope map[string]interface{}) {
	t.Helper()
	if status != http.StatusGone {
		t.Fatalf("期望 HTTP 410, 实际 %d, body: %v", status, envelope)
	}
	code, ok := envelope["error_code"].(string)
	if !ok || code != "pill.legacy_api_removed" {
		t.Fatalf("期望 error_code=pill.legacy_api_removed, 实际 %v", envelope["error_code"])
	}
}

// TestReplacePills_ValidRequestReturnsGone 有效请求也 410: 完整编排入口已下线,
// 防绕过库存直接写绑定;410 不应产生任何写入
func TestReplacePills_ValidRequestReturnsGone(t *testing.T) {
	db := setupTestDB(t)
	agentUUID, pillUUIDs := seedAgentWithPillsForReplace(t, db, 3)
	r := setupReplacePillsRouter()

	body := fmt.Sprintf(`{"pills":[
		{"pill_id": %q, "weight": 2.5},
		{"pill_id": %q, "weight": 1.0}
	]}`, pillUUIDs[2], pillUUIDs[0])
	status, envelope := putJSON(t, r, fmt.Sprintf("/api/v1/agents/%s/pills", agentUUID), body)
	assertGone410(t, status, envelope)

	var count int64
	db.Model(&model.AgentPill{}).Count(&count)
	if count != 0 {
		t.Fatalf("410 后仍写入 %d 条绑定", count)
	}
}

// TestReplacePills_EmptyAlsoGone 空数组(旧清空语义)也 410: 移除能力改走 UnbindPill
func TestReplacePills_EmptyAlsoGone(t *testing.T) {
	db := setupTestDB(t)
	agentUUID, _ := seedAgentWithPillsForReplace(t, db, 1)
	r := setupReplacePillsRouter()
	status, envelope := putJSON(t, r, fmt.Sprintf("/api/v1/agents/%s/pills", agentUUID), `{"pills":[]}`)
	assertGone410(t, status, envelope)
}

// TestReplacePills_WeightOutOfRangeAlsoGone 权重越界不再做旧校验: 一律 410
func TestReplacePills_WeightOutOfRangeAlsoGone(t *testing.T) {
	db := setupTestDB(t)
	agentUUID, pillUUIDs := seedAgentWithPillsForReplace(t, db, 1)
	r := setupReplacePillsRouter()
	for _, w := range []string{"0", "11"} {
		body := fmt.Sprintf(`{"pills":[{"pill_id": %q, "weight": %s}]}`, pillUUIDs[0], w)
		status, envelope := putJSON(t, r, fmt.Sprintf("/api/v1/agents/%s/pills", agentUUID), body)
		assertGone410(t, status, envelope)
	}
}

// TestReplacePills_UnknownTargetsAlsoGone 未知道人/未知金丹同样 410(封禁先行,不做任何查询)
func TestReplacePills_UnknownTargetsAlsoGone(t *testing.T) {
	db := setupTestDB(t)
	agentUUID, _ := seedAgentWithPillsForReplace(t, db, 1)
	r := setupReplacePillsRouter()

	status, envelope := putJSON(t, r, fmt.Sprintf("/api/v1/agents/%s/pills", uuid.NewString()), `{"pills":[]}`)
	assertGone410(t, status, envelope)

	body := fmt.Sprintf(`{"pills":[{"pill_id": %q, "weight": 1}]}`, uuid.NewString())
	status, envelope = putJSON(t, r, fmt.Sprintf("/api/v1/agents/%s/pills", agentUUID), body)
	assertGone410(t, status, envelope)
}

// TestReplacePills_InvalidAgentUUID 非法道人 UUID 同样 410(入口已死,不做解析)
func TestReplacePills_InvalidAgentUUID(t *testing.T) {
	setupTestDB(t)
	r := setupReplacePillsRouter()
	status, envelope := putJSON(t, r, "/api/v1/agents/not-a-uuid/pills", `{"pills":[]}`)
	if status != http.StatusGone {
		t.Fatalf("非法道人 UUID 期望 410, 实际 %d", status)
	}
	if ec, _ := envelope["error_code"].(string); ec != "pill.legacy_api_removed" {
		t.Fatalf("期望 error_code=pill.legacy_api_removed, 实际 %v", envelope["error_code"])
	}
}

// TestReplacePills_InvalidPillUUIDInBody 载荷非法同样 410,且不产生任何写入
func TestReplacePills_InvalidPillUUIDInBody(t *testing.T) {
	db := setupTestDB(t)
	agentUUID, _ := seedAgentWithPillsForReplace(t, db, 1)
	r := setupReplacePillsRouter()
	status, envelope := putJSON(t, r, fmt.Sprintf("/api/v1/agents/%s/pills", agentUUID),
		`{"pills":[{"pill_id": "not-a-uuid", "weight": 1}]}`)
	if status != http.StatusGone {
		t.Fatalf("非法金丹 UUID 期望 410, 实际 %d", status)
	}
	if ec, _ := envelope["error_code"].(string); ec != "pill.legacy_api_removed" {
		t.Fatalf("期望 error_code=pill.legacy_api_removed, 实际 %v", envelope["error_code"])
	}
	var count int64
	db.Model(&model.AgentPill{}).Count(&count)
	if count != 0 {
		t.Fatalf("410 后仍写入 %d 条绑定", count)
	}
}
