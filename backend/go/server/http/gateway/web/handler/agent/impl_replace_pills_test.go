// 完整服丹编排接口(PUT /api/v1/agents/:uuid/pills)回归测试
// 真实 sqlite 内存库 + 真实 service + 真实 handler,不引入 mock
package agent

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/alchemy-furnace/server/internal/dao"
	"github.com/alchemy-furnace/server/internal/service/agent_service"
	"github.com/alchemy-furnace/server/model"
	"github.com/alchemy-furnace/server/server/http/middleware"
	"github.com/alchemy-furnace/server/server/http/router"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// setupReplacePillsRouter 注册完整服丹编排路由(与既有逐项 POST 共存由 router.go 保证,此处只测本路由)
func setupReplacePillsRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(middleware.RequestID())
	h := New(agent_service.New(dao.NewAgentDao(), dao.NewPillDao(), dao.NewModelDao()))
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
		pillUUIDs = append(pillUUIDs, p.UUID.String())
	}
	return agent.UUID.String(), pillUUIDs
}

func TestReplacePills_SuccessReturnsOrderedDetail(t *testing.T) {
	db := setupTestDB(t)
	agentUUID, pillUUIDs := seedAgentWithPillsForReplace(t, db, 3)
	r := setupReplacePillsRouter()

	// 故意乱序 + 自定义权重
	body := fmt.Sprintf(`{"pills":[
		{"pill_id": %q, "weight": 2.5},
		{"pill_id": %q, "weight": 1.0}
	]}`, pillUUIDs[2], pillUUIDs[0])
	status, envelope := putJSON(t, r, fmt.Sprintf("/api/v1/agents/%s/pills", agentUUID), body)

	if status != http.StatusOK {
		t.Fatalf("期望 HTTP 200, 实际 %d, body: %v", status, envelope)
	}
	if envelope["code"].(float64) != 0 {
		t.Fatalf("期望 code=0, 实际 %v", envelope)
	}
	data, ok := envelope["data"].(map[string]interface{})
	if !ok {
		t.Fatalf("响应缺 data: %v", envelope)
	}
	agentPills, ok := data["agent_pills"].([]interface{})
	if !ok || len(agentPills) != 2 {
		t.Fatalf("agent_pills = %v, 期望 2 条", data["agent_pills"])
	}
	first := agentPills[0].(map[string]interface{})
	second := agentPills[1].(map[string]interface{})
	if first["pill_id"].(string) != pillUUIDs[2] || second["pill_id"].(string) != pillUUIDs[0] {
		t.Fatalf("顺序未保持: got %s,%s want %s,%s", first["pill_id"], second["pill_id"], pillUUIDs[2], pillUUIDs[0])
	}
	if first["weight"].(float64) != 2.5 || second["weight"].(float64) != 1.0 {
		t.Fatalf("权重错误: %v,%v", first["weight"], second["weight"])
	}
	if first["sort_order"].(float64) != 1 || second["sort_order"].(float64) != 2 {
		t.Fatalf("sort_order 应为 1,2: %v,%v", first["sort_order"], second["sort_order"])
	}
}

func TestReplacePills_EmptyClearsRelations(t *testing.T) {
	db := setupTestDB(t)
	agentUUID, pillUUIDs := seedAgentWithPillsForReplace(t, db, 2)
	r := setupReplacePillsRouter()

	// 先绑定两枚
	body := fmt.Sprintf(`{"pills":[{"pill_id": %q, "weight": 1},{"pill_id": %q, "weight": 1}]}`, pillUUIDs[0], pillUUIDs[1])
	if status, env := putJSON(t, r, fmt.Sprintf("/api/v1/agents/%s/pills", agentUUID), body); status != http.StatusOK {
		t.Fatalf("预置编排失败: %d %v", status, env)
	}
	// 空数组清空
	status, envelope := putJSON(t, r, fmt.Sprintf("/api/v1/agents/%s/pills", agentUUID), `{"pills":[]}`)
	if status != http.StatusOK {
		t.Fatalf("清空期望 200, 实际 %d, body: %v", status, envelope)
	}
	data := envelope["data"].(map[string]interface{})
	if aps, ok := data["agent_pills"].([]interface{}); ok && len(aps) != 0 {
		t.Fatalf("清空后仍有 %d 条服用记录", len(aps))
	}
	var count int64
	db.Model(&model.AgentPill{}).Count(&count)
	if count != 0 {
		t.Fatalf("数据库仍残留 %d 条服用记录", count)
	}
}

func TestReplacePills_InvalidAgentUUID(t *testing.T) {
	setupTestDB(t)
	r := setupReplacePillsRouter()
	status, _ := putJSON(t, r, "/api/v1/agents/not-a-uuid/pills", `{"pills":[]}`)
	if status != http.StatusBadRequest {
		t.Fatalf("非法道人 UUID 期望 400, 实际 %d", status)
	}
}

func TestReplacePills_InvalidPillUUIDInBody(t *testing.T) {
	db := setupTestDB(t)
	agentUUID, _ := seedAgentWithPillsForReplace(t, db, 1)
	r := setupReplacePillsRouter()
	status, _ := putJSON(t, r, fmt.Sprintf("/api/v1/agents/%s/pills", agentUUID),
		`{"pills":[{"pill_id": "not-a-uuid", "weight": 1}]}`)
	if status != http.StatusBadRequest {
		t.Fatalf("非法金丹 UUID 期望 400, 实际 %d", status)
	}
}

func TestReplacePills_WeightOutOfRange(t *testing.T) {
	db := setupTestDB(t)
	agentUUID, pillUUIDs := seedAgentWithPillsForReplace(t, db, 1)
	r := setupReplacePillsRouter()
	for _, w := range []string{"0", "11"} {
		body := fmt.Sprintf(`{"pills":[{"pill_id": %q, "weight": %s}]}`, pillUUIDs[0], w)
		status, envelope := putJSON(t, r, fmt.Sprintf("/api/v1/agents/%s/pills", agentUUID), body)
		if status != http.StatusBadRequest {
			t.Fatalf("权重 %s 期望 400, 实际 %d, body: %v", w, status, envelope)
		}
	}
}

func TestReplacePills_PillNotFound(t *testing.T) {
	db := setupTestDB(t)
	agentUUID, _ := seedAgentWithPillsForReplace(t, db, 1)
	r := setupReplacePillsRouter()
	body := fmt.Sprintf(`{"pills":[{"pill_id": %q, "weight": 1}]}`, uuid.NewString())
	status, _ := putJSON(t, r, fmt.Sprintf("/api/v1/agents/%s/pills", agentUUID), body)
	if status != http.StatusNotFound {
		t.Fatalf("金丹不存在期望 404, 实际 %d", status)
	}
}

func TestReplacePills_AgentNotFound(t *testing.T) {
	setupTestDB(t)
	r := setupReplacePillsRouter()
	status, _ := putJSON(t, r, fmt.Sprintf("/api/v1/agents/%s/pills", uuid.NewString()), `{"pills":[]}`)
	if status != http.StatusNotFound {
		t.Fatalf("道人不存在期望 404, 实际 %d", status)
	}
}
