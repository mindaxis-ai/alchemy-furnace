// 道人详情(GET /api/v1/agents/:uuid)响应回归测试
// 任务 8 旧入口审计: 旧详情响应曾内嵌遗留 agent_pills 绑定(迁移后旧行保留供回滚,
// 移除能力后仍显示「已绑定」,与 effects 状态矛盾)。前端无任何消费方,已从响应移除;
// 能力展示走 GET /api/v1/agents/:uuid/effects。本测试锁定响应不再含 agent_pills 键。
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
)

// setupGetRouter 注册道人详情路由(仅测本路由;memory 参数本测试未涉及,传 nil)
func setupGetRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(middleware.RequestID())
	h := New(agent_service.New(dao.NewAgentDao(), dao.NewModelDao(),
		pill_inventory_service.New(dao.GetDB(), time.Now)), nil)
	r.GET("/api/v1/agents/:uuid", router.Wrapper(h.Get))
	return r
}

// TestAgentDetailResponseOmitsLegacyPills 即便存在遗留绑定行,详情响应也不输出 agent_pills
func TestAgentDetailResponseOmitsLegacyPills(t *testing.T) {
	db := setupTestDB(t)
	agentUUID, pillUUID := seedAgentForBindPill(t, db)

	// 预置一条遗留绑定行(迁移场景下旧表保留,回滚用): 详情响应不得再把它当活跃绑定输出
	var agent model.DaoAgent
	if err := db.Where("dao_agent_id = ?", agentUUID).First(&agent).Error; err != nil {
		t.Fatalf("查询道人失败: %v", err)
	}
	var pill model.ElixirPill
	if err := db.Where("elixir_pill_id = ?", pillUUID).First(&pill).Error; err != nil {
		t.Fatalf("查询金丹失败: %v", err)
	}
	if err := db.Create(&model.AgentPill{
		AgentID: agent.DaoAgentID, PillID: pill.ElixirPillID, Weight: 1.5, SortOrder: 0,
	}).Error; err != nil {
		t.Fatalf("创建遗留绑定失败: %v", err)
	}

	r := setupGetRouter()
	status, envelope := getJSON(t, r, fmt.Sprintf("/api/v1/agents/%s", agentUUID))
	if status != http.StatusOK {
		t.Fatalf("期望 HTTP 200, 实际 %d, body: %v", status, envelope)
	}
	data, ok := envelope["data"].(map[string]interface{})
	if !ok {
		t.Fatalf("响应缺少 data 对象: %v", envelope)
	}
	if _, has := data["agent_pills"]; has {
		t.Fatalf("详情响应仍包含 agent_pills(遗留绑定不得作为活跃绑定展示): %v", data["agent_pills"])
	}
}

// TestAgentDetailResponseKeepsLanguagePattern 详情响应仍保留语言模式缓存字段(防误删)
func TestAgentDetailResponseKeepsLanguagePattern(t *testing.T) {
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

	r := setupGetRouter()
	status, envelope := getJSON(t, r, fmt.Sprintf("/api/v1/agents/%s", agentUUID))
	if status != http.StatusOK {
		t.Fatalf("期望 HTTP 200, 实际 %d, body: %v", status, envelope)
	}
	data, ok := envelope["data"].(map[string]interface{})
	if !ok {
		t.Fatalf("响应缺少 data 对象: %v", envelope)
	}
	lp, has := data["language_pattern"].(map[string]interface{})
	if !has {
		t.Fatalf("详情响应缺少 language_pattern: %v", data)
	}
	if lp["is_valid"] != true {
		t.Fatalf("language_pattern.is_valid 期望 true, 实际 %v", lp["is_valid"])
	}
}
