// 旧服用绑定接口(POST /api/v1/agents/:uuid/pills)回归测试
// 任务 5 起该写路由恒 410 pill.legacy_api_removed(防绕过库存直接绑定):
// 路径与载荷一律不解析,任何请求(含非法 UUID)都返回 410,不产生任何写入。
// 服用改走 POST /api/v1/agents/:id/consume(Idempotency-Key 幂等契约)。
// 真实 sqlite 内存库 + 真实 service + 真实 handler,不引入 mock。
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

// setupBindPillRouter 注册旧绑定路由(仅测本路由;memory 参数本测试未涉及,传 nil)
func setupBindPillRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(middleware.RequestID())
	h := New(agent_service.New(dao.NewAgentDao(), dao.NewModelDao(),
		pill_inventory_service.New(dao.GetDB(), time.Now)), nil)
	r.POST("/api/v1/agents/:uuid/pills", router.Wrapper(h.BindPill))
	return r
}

// seedAgentForBindPill 造一个道人与一枚金丹,返回 (道人 UUID, 金丹 UUID)
func seedAgentForBindPill(t *testing.T, db *gorm.DB) (string, string) {
	t.Helper()
	agent := model.DaoAgent{Name: "绑定道人", ModelName: "gpt-4o", Status: "active"}
	if err := db.Create(&agent).Error; err != nil {
		t.Fatalf("创建道人失败: %v", err)
	}
	p := model.ElixirPill{Name: uuid.NewString(), SkillSchema: model.JSONMap{"identity_card": "x"}}
	if err := db.Create(&p).Error; err != nil {
		t.Fatalf("创建金丹失败: %v", err)
	}
	return agent.DaoAgentID, p.ElixirPillID
}

// TestBindPillLegacyRemoved 合法载荷也 410: 服用入口已下线,防绕过库存直接绑定
func TestBindPillLegacyRemoved(t *testing.T) {
	db := setupTestDB(t)
	agentUUID, pillUUID := seedAgentForBindPill(t, db)
	r := setupBindPillRouter()

	body := fmt.Sprintf(`{"pill_id":%q,"weight":2.5,"sort_order":1}`, pillUUID)
	status, envelope := postJSON(t, r, fmt.Sprintf("/api/v1/agents/%s/pills", agentUUID), body)
	if status != http.StatusGone {
		t.Fatalf("期望 HTTP 410, 实际 %d, body: %v", status, envelope)
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

// TestBindPillLegacyRemovedIgnoresPayload 非法载荷(缺字段/坏 UUID)同样 410
func TestBindPillLegacyRemovedIgnoresPayload(t *testing.T) {
	db := setupTestDB(t)
	agentUUID, _ := seedAgentForBindPill(t, db)
	r := setupBindPillRouter()

	for _, body := range []string{
		`{}`,
		`{"pill_id":"not-a-uuid","weight":1}`,
		`{"pill_id":%q,"weight":99}`,
	} {
		status, envelope := postJSON(t, r, fmt.Sprintf("/api/v1/agents/%s/pills", agentUUID), fmt.Sprintf(body, agentUUID))
		if status != http.StatusGone {
			t.Fatalf("载荷 %s 期望 410, 实际 %d, body: %v", body, status, envelope)
		}
		if ec, _ := envelope["error_code"].(string); ec != "pill.legacy_api_removed" {
			t.Fatalf("期望 error_code=pill.legacy_api_removed, 实际 %v", envelope["error_code"])
		}
	}
	var count int64
	db.Model(&model.AgentPill{}).Count(&count)
	if count != 0 {
		t.Fatalf("410 后仍写入 %d 条绑定", count)
	}
}

// TestBindPillLegacyRemovedIgnoresPath 非法道人 UUID 同样 410(入口已死,不做解析)
func TestBindPillLegacyRemovedIgnoresPath(t *testing.T) {
	setupTestDB(t)
	r := setupBindPillRouter()
	status, _ := postJSON(t, r, "/api/v1/agents/not-a-uuid/pills",
		fmt.Sprintf(`{"pill_id":%q,"weight":1}`, uuid.NewString()))
	if status != http.StatusGone {
		t.Fatalf("非法道人 UUID 期望 410, 实际 %d", status)
	}
}
