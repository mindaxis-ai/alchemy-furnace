// Package integration 「金丹化性」端到端集成测试
//
// 本包覆盖端到端流程：内置金丹种子 → 创建道人 → 服用金丹 → 合成语言模式 → 创建会话 → 流式对话。
//
// 仓库中没有 sqlite 驱动或 testify 等测试依赖（仅 postgres 驱动），因此采用两级策略：
//
//  1. 默认可运行（go test ./... 无需任何基础设施）：
//     使用 httptest 伪造 Python 语言引擎（/api/v1/synthesis/combine 与
//     /api/v1/chat/completions/stream，见 contracts/internal-contract.md），
//     在「服务边界」上验证 SynthesisClient 与 ChatService.CallChatStream 的契约行为。
//
//  2. 完整 DB 流程（需要真实 PostgreSQL）：
//     设置环境变量 ALCHEMY_TEST_PG_DSN 后运行，例如：
//     ALCHEMY_TEST_PG_DSN="host=localhost port=5432 user=alchemy password=alchemy123 dbname=alchemy_test sslmode=disable" go test ./tests/integration/ -run TestChatFlowEndToEnd -v
//     未设置时自动跳过，保证默认 go test ./... 恒绿。
package integration

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/alchemy-furnace/server/dao"
	"github.com/alchemy-furnace/server/model"
	"github.com/alchemy-furnace/server/pkg/config"
	"github.com/alchemy-furnace/server/service"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// ---------- 测试辅助 ----------

// newMockPythonEngine 返回一个伪造的 Python 语言引擎 httptest 服务器
// 实现 internal-contract.md 中的两个端点：
//   - POST /api/v1/synthesis/combine        语言模式合成
//   - POST /api/v1/chat/completions/stream  SSE 流式对话
//
// seenSynthesis / seenChat 用于回传引擎收到的请求体，供断言使用
func newMockPythonEngine(t *testing.T, seenSynthesis, seenChat *map[string]interface{}) *httptest.Server {
	t.Helper()

	mux := http.NewServeMux()

	mux.HandleFunc("/api/v1/synthesis/combine", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		body, _ := io.ReadAll(r.Body)
		var req map[string]interface{}
		if err := json.Unmarshal(body, &req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		*seenSynthesis = req

		personality, _ := req["personality"].(string)
		pills, _ := req["pills"].([]interface{})

		prompt := fmt.Sprintf("【合成提示词】基础性格=%s，金丹数=%d", personality, len(pills))
		resp := map[string]interface{}{
			"system_prompt":   prompt,
			"emergence_rules": []string{"融合规则一：以第一枚金丹为基调", "融合规则二：冲突处以性格为准"},
			"inner_tensions":  []map[string]string{{"dimension": "sentence_length", "description": "性格偏长句，金丹偏短句", "severity": "medium"}},
			"fingerprint":     "sha256:mockfingerprint",
			"model":           "gpt-4o-mini",
			"usage":           map[string]int{"prompt_tokens": 800, "completion_tokens": 200, "total_tokens": 1000},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	})

	mux.HandleFunc("/api/v1/chat/completions/stream", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		body, _ := io.ReadAll(r.Body)
		var req map[string]interface{}
		if err := json.Unmarshal(body, &req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		*seenChat = req

		// 按 websocket-contract.md / internal-contract.md 输出 SSE 流
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flusher, _ := w.(http.Flusher)
		for _, chunk := range []string{"你", "好", "，", "道", "友"} {
			fmt.Fprintf(w, "data: {\"content\": %q}\n\n", chunk)
			if flusher != nil {
				flusher.Flush()
			}
		}
		fmt.Fprint(w, "data: [DONE]\n\n")
		if flusher != nil {
			flusher.Flush()
		}
	})

	return httptest.NewServer(mux)
}

// pointConfigAtEngine 将全局配置的 Python 引擎地址指向 mock 服务器并重新加载
func pointConfigAtEngine(t *testing.T, engineURL string) {
	t.Helper()
	// config.Load 通过 Viper 读取环境变量（前缀 AF），两处都设置以确保命中
	t.Setenv("AF_PYTHON_ENGINE_BASE_URL", engineURL)
	t.Setenv("PYTHON_ENGINE_BASE_URL", engineURL)
	if _, err := config.Load(); err != nil {
		t.Fatalf("加载测试配置失败: %v", err)
	}
	if got := config.Get().PythonEngine.BaseURL; got != engineURL {
		t.Fatalf("配置未指向 mock 引擎: got %s, want %s", got, engineURL)
	}
}

// ---------- 默认运行的测试（无外部基础设施） ----------

// TestSynthesisClientContract 验证 SynthesisClient 与合成端点的契约：
// 请求体包含 personality/pills/model，响应可解析为 CombineResponse
func TestSynthesisClientContract(t *testing.T) {
	var seen map[string]interface{}
	engine := newMockPythonEngine(t, &seen, &map[string]interface{}{})
	defer engine.Close()
	pointConfigAtEngine(t, engine.URL)

	client := service.NewSynthesisClient()
	resp, err := client.Combine("沉稳内敛，喜好引经据典", []service.SynthesisPillInput{
		{
			ID:        1,
			Name:      "文言文金丹",
			Weight:    1.0,
			SortOrder: 1,
			SkillSchema: model.JSONMap{
				"identity_card": "吾乃文言丹所化",
				"expression_dna": map[string]interface{}{
					"sentence_length": "short",
					"formality":       0.9,
					"vocabulary":      []string{"之", "乎", "者", "也"},
					"taboo_words":     []string{"yyds"},
					"rhythm":          "四字为节",
					"humor_type":      "冷幽默",
					"certainty_style": "断然下判",
					"citation_habit":  "引《论语》《庄子》",
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("Combine 调用失败: %v", err)
	}

	// 断言请求契约
	if seen["personality"] != "沉稳内敛，喜好引经据典" {
		t.Errorf("合成请求 personality 不符: %v", seen["personality"])
	}
	pills, ok := seen["pills"].([]interface{})
	if !ok || len(pills) != 1 {
		t.Fatalf("合成请求 pills 不符: %v", seen["pills"])
	}
	firstPill, _ := pills[0].(map[string]interface{})
	if firstPill["name"] != "文言文金丹" {
		t.Errorf("合成请求首枚金丹名不符: %v", firstPill["name"])
	}
	if seen["model"] == "" {
		t.Errorf("合成请求缺少 model 字段")
	}

	// 断言响应解析
	if !strings.Contains(resp.SystemPrompt, "沉稳内敛") {
		t.Errorf("system_prompt 未包含基础性格: %s", resp.SystemPrompt)
	}
	if len(resp.EmergenceRules) != 2 {
		t.Errorf("emergence_rules 数量不符: %d", len(resp.EmergenceRules))
	}
	if len(resp.InnerTensions) != 1 || resp.InnerTensions[0].Severity != "medium" {
		t.Errorf("inner_tensions 解析不符: %+v", resp.InnerTensions)
	}
}

// TestChatStreamContract 验证 ChatService.CallChatStream 与流式端点的契约：
// 请求携带 messages+model，返回可读取的 SSE 流并以 [DONE] 结束
func TestChatStreamContract(t *testing.T) {
	var seenChat map[string]interface{}
	engine := newMockPythonEngine(t, &map[string]interface{}{}, &seenChat)
	defer engine.Close()
	pointConfigAtEngine(t, engine.URL)

	svc := service.NewChatService()
	stream, err := svc.CallChatStream([]map[string]string{
		{"role": "system", "content": "【合成提示词】你是服用文言文金丹的道人"},
		{"role": "user", "content": "道友请讲"},
	}, "gpt-4o")
	if err != nil {
		t.Fatalf("CallChatStream 调用失败: %v", err)
	}
	defer stream.Close()

	raw, err := io.ReadAll(stream)
	if err != nil {
		t.Fatalf("读取 SSE 流失败: %v", err)
	}
	body := string(raw)

	// 断言 SSE 内容符合契约：data 行 + [DONE] 终止符
	if !strings.Contains(body, "data: {\"content\": \"你\"}") {
		t.Errorf("SSE 流缺少首个内容块: %q", body)
	}
	if !strings.Contains(body, "data: [DONE]") {
		t.Errorf("SSE 流缺少 [DONE] 终止符: %q", body)
	}

	// 断言发送给引擎的请求
	messages, ok := seenChat["messages"].([]interface{})
	if !ok || len(messages) != 2 {
		t.Fatalf("流式请求 messages 不符: %v", seenChat["messages"])
	}
	first, _ := messages[0].(map[string]interface{})
	if first["role"] != "system" || !strings.Contains(first["content"].(string), "合成提示词") {
		t.Errorf("首条消息应为合成后的 system 提示词: %v", first)
	}
	if seenChat["model"] != "gpt-4o" {
		t.Errorf("流式请求 model 不符: %v", seenChat["model"])
	}
}

// TestChatStreamEngineError 验证引擎返回非 200 时 CallChatStream 返回错误
func TestChatStreamEngineError(t *testing.T) {
	engine := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":"engine down"}`, http.StatusInternalServerError)
	}))
	defer engine.Close()
	pointConfigAtEngine(t, engine.URL)

	svc := service.NewChatService()
	_, err := svc.CallChatStream([]map[string]string{{"role": "user", "content": "hi"}}, "gpt-4o")
	if err == nil {
		t.Fatal("引擎返回 500 时应当报错")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("错误信息应包含状态码: %v", err)
	}
}

// ---------- 完整 DB 流程（需 ALCHEMY_TEST_PG_DSN，未设置则跳过） ----------

// TestChatFlowEndToEnd 端到端流程：
// 迁移 → 种子金丹（含幂等性验证）→ 创建道人 → 服用金丹 → 合成语言模式（mock 引擎）
// → 创建会话 → 保存消息 → 流式对话
//
// 运行方式：
//
//	ALCHEMY_TEST_PG_DSN="host=... dbname=alchemy_test sslmode=disable" go test ./tests/integration/ -run TestChatFlowEndToEnd -v
func TestChatFlowEndToEnd(t *testing.T) {
	dsn := os.Getenv("ALCHEMY_TEST_PG_DSN")
	if dsn == "" {
		t.Skip("跳过：未设置 ALCHEMY_TEST_PG_DSN（无可用 PostgreSQL）")
	}

	var seenSynthesis, seenChat map[string]interface{}
	engine := newMockPythonEngine(t, &seenSynthesis, &seenChat)
	defer engine.Close()
	pointConfigAtEngine(t, engine.URL)

	// 连接测试数据库（独立 schema 表，建议使用专用测试库）
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: gormlogger.Default.LogMode(gormlogger.Silent)})
	if err != nil {
		t.Fatalf("连接测试数据库失败: %v", err)
	}
	dao.DB = db

	// 迁移 + 种子
	if err := dao.AutoMigrate(db); err != nil {
		t.Fatalf("自动迁移失败: %v", err)
	}
	if err := dao.SeedBuiltinPills(db); err != nil {
		t.Fatalf("种子写入失败: %v", err)
	}

	// 幂等性：再次种子不应新增
	var countBefore, countAfter int64
	db.Model(&model.ElixirPill{}).Where("is_builtin = ?", true).Count(&countBefore)
	if countBefore < 3 {
		t.Fatalf("内置金丹数量不足: %d", countBefore)
	}
	if err := dao.SeedBuiltinPills(db); err != nil {
		t.Fatalf("二次种子写入失败: %v", err)
	}
	db.Model(&model.ElixirPill{}).Where("is_builtin = ?", true).Count(&countAfter)
	if countAfter != countBefore {
		t.Fatalf("种子不幂等: before=%d after=%d", countBefore, countAfter)
	}

	// 取一枚内置金丹：鲁迅风金丹
	var pill model.ElixirPill
	if err := db.Where("name = ?", "鲁迅风金丹").First(&pill).Error; err != nil {
		t.Fatalf("未找到内置金丹「鲁迅风金丹」: %v", err)
	}
	// 校验 skill_schema 关键字段齐全（对照 data-model.md）
	if _, ok := pill.SkillSchema["identity_card"]; !ok {
		t.Error("skill_schema 缺少 identity_card")
	}
	dna, ok := pill.SkillSchema["expression_dna"].(map[string]interface{})
	if !ok {
		t.Fatal("skill_schema 缺少 expression_dna")
	}
	for _, field := range []string{"sentence_length", "formality", "vocabulary", "taboo_words", "rhythm", "humor_type", "certainty_style", "citation_habit"} {
		if _, ok := dna[field]; !ok {
			t.Errorf("expression_dna 缺少字段 %s", field)
		}
	}
	for _, field := range []string{"mental_models", "decision_heuristics", "values", "anti_patterns", "honest_limits", "example_dialogues"} {
		if _, ok := pill.SkillSchema[field]; !ok {
			t.Errorf("skill_schema 缺少 %s", field)
		}
	}

	// 创建道人
	agentSvc := service.NewAgentService()
	agent, err := agentSvc.CreateAgent(&model.CreateAgentRequest{
		Name:        "集成测试道人",
		Personality: "外冷内热，寡言而锋利",
	})
	if err != nil {
		t.Fatalf("创建道人失败: %v", err)
	}
	t.Cleanup(func() {
		db.Delete(&model.DaoAgent{}, agent.ID)
	})

	// 服用金丹
	if err := agentSvc.BindPill(agent.ID, pill.ID, 1.5, 0); err != nil {
		t.Fatalf("服用金丹失败: %v", err)
	}
	pillIDs, err := agentSvc.GetAgentPillIDs(agent.ID)
	if err != nil || len(pillIDs) != 1 || pillIDs[0] != pill.ID {
		t.Fatalf("服用记录查询不符: ids=%v err=%v", pillIDs, err)
	}

	// 合成语言模式（应调用 mock 引擎并写回缓存）
	patternSvc := service.NewLanguagePatternService()
	pattern, err := patternSvc.GetOrBuildPattern(agent.ID)
	if err != nil {
		t.Fatalf("合成语言模式失败: %v", err)
	}
	if !pattern.IsValid {
		t.Error("合成后的语言模式应为有效")
	}
	if !strings.Contains(pattern.SystemPrompt, "外冷内热") {
		t.Errorf("合成提示词未包含道人性格: %s", pattern.SystemPrompt)
	}
	if pattern.SourceFingerprint == "" {
		t.Error("来源指纹不应为空")
	}
	if pills, _ := seenSynthesis["pills"].([]interface{}); len(pills) != 1 {
		t.Errorf("合成引擎应收到 1 枚金丹: %v", seenSynthesis["pills"])
	}

	// 缓存命中：第二次调用应直接返回缓存（指纹一致）
	pattern2, err := patternSvc.GetOrBuildPattern(agent.ID)
	if err != nil {
		t.Fatalf("二次获取语言模式失败: %v", err)
	}
	if pattern2.ID != pattern.ID || pattern2.SourceFingerprint != pattern.SourceFingerprint {
		t.Error("缓存未命中：指纹或记录不一致")
	}

	// 创建会话
	chatSvc := service.NewChatService()
	session, err := chatSvc.CreateSession(&model.CreateSessionRequest{AgentID: agent.ID})
	if err != nil {
		t.Fatalf("创建会话失败: %v", err)
	}

	// 保存用户消息
	if _, err := chatSvc.SaveMessage(session.ID, "user", "道友，如何看待内卷？", nil); err != nil {
		t.Fatalf("保存用户消息失败: %v", err)
	}

	// 流式对话（mock 引擎）
	stream, err := chatSvc.CallChatStream([]map[string]string{
		{"role": "system", "content": pattern.SystemPrompt},
		{"role": "user", "content": "道友，如何看待内卷？"},
	}, agent.ModelName)
	if err != nil {
		t.Fatalf("流式对话失败: %v", err)
	}
	defer stream.Close()
	raw, _ := io.ReadAll(stream)
	if !strings.Contains(string(raw), "data: [DONE]") {
		t.Errorf("流式响应缺少 [DONE]: %q", string(raw))
	}

	// 保存助手消息并验证历史
	if _, err := chatSvc.SaveMessage(session.ID, "assistant", "内卷者，陀螺之谓也……", nil); err != nil {
		t.Fatalf("保存助手消息失败: %v", err)
	}
	messages, total, err := chatSvc.GetMessages(session.ID, 1, 20)
	if err != nil || total != 2 || len(messages) != 2 {
		t.Fatalf("消息历史不符: total=%d len=%d err=%v", total, len(messages), err)
	}
	if messages[0].Role != "user" || messages[1].Role != "assistant" {
		t.Errorf("消息顺序/角色不符: %s -> %s", messages[0].Role, messages[1].Role)
	}

	// 解除绑定后缓存应失效
	if err := agentSvc.UnbindPill(agent.ID, pill.ID); err != nil {
		t.Fatalf("解除绑定失败: %v", err)
	}
	var stale model.LanguagePattern
	if err := db.Where("agent_id = ?", agent.ID).First(&stale).Error; err != nil {
		t.Fatalf("查询语言模式缓存失败: %v", err)
	}
	if stale.IsValid {
		t.Error("解除绑定后语言模式缓存应被置为失效")
	}
}
