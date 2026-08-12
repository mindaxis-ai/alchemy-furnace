// 演示模式种子数据(007-demo-mode)
// 硬编码 9 条 agent / 9 条 pill / 9 条 provider / 9 条 model / 9 条 session / 9 条 message,
// 加 9 条 agent_pill 1:1 绑定。UUID 采用语义前缀,便于前端/测试肉眼识别:
//   a = agent, b = pill, c = provider, d = model, e = session, f = message
// 每条 UUID 形如 {prefix×8/4/4/4/12} 满足 v4 形态(版本位 4,变体位 8)
package memory

import (
	"fmt"
	"time"

	"github.com/alchemy-furnace/server/model"
	"github.com/google/uuid"
)

// 语义前缀 UUID(启动期解析一次)
var (
	agentUUID    [9]uuid.UUID
	pillUUID     [9]uuid.UUID
	providerUUID [9]uuid.UUID
	modelUUID    [9]uuid.UUID
	sessionUUID  [9]uuid.UUID
	messageUUID  [9]uuid.UUID
)

func init() {
	tmpl := map[byte]string{
		'a': "aaaaaaaa-aaaa-4aaa-8aaa-00000000000%d",
		'b': "bbbbbbbb-bbbb-4bbb-8bbb-00000000000%d",
		'c': "cccccccc-cccc-4ccc-8ccc-00000000000%d",
		'd': "dddddddd-dddd-4ddd-8ddd-00000000000%d",
		'e': "eeeeeeee-eeee-4eee-8eee-00000000000%d",
		'f': "ffffffff-ffff-4fff-8fff-00000000000%d",
	}
	for i := 0; i < 9; i++ {
		n := i + 1
		agentUUID[i] = uuid.MustParse(fmt.Sprintf(tmpl['a'], n))
		pillUUID[i] = uuid.MustParse(fmt.Sprintf(tmpl['b'], n))
		providerUUID[i] = uuid.MustParse(fmt.Sprintf(tmpl['c'], n))
		modelUUID[i] = uuid.MustParse(fmt.Sprintf(tmpl['d'], n))
		sessionUUID[i] = uuid.MustParse(fmt.Sprintf(tmpl['e'], n))
		messageUUID[i] = uuid.MustParse(fmt.Sprintf(tmpl['f'], n))
	}
}

// loadSeed 把 9×6 + 9 绑定写入 store(演示模式启动期调用一次)
func loadSeed(s *Store) {
	now := time.Date(2026, 8, 10, 9, 0, 0, 0, time.UTC)

	// ---------- 9 providers ----------
	providerNames := []struct{ Name, Display, Protocol, BaseURL string }{
		{"openai", "OpenAI", "openai-compatible", "https://api.openai.com/v1"},
		{"anthropic", "Anthropic", "anthropic", "https://api.anthropic.com/v1"},
		{"google", "Google Gemini", "google", "https://generativelanguage.googleapis.com/v1"},
		{"mistral", "Mistral AI", "openai-compatible", "https://api.mistral.ai/v1"},
		{"deepseek", "DeepSeek", "openai-compatible", "https://api.deepseek.com/v1"},
		{"qwen", "通义千问", "openai-compatible", "https://dashscope.aliyuncs.com/compatible-mode/v1"},
		{"wenxin", "文心一言", "wenxin", "https://aip.baidubce.com/v1"},
		{"zhipu", "智谱 GLM", "openai-compatible", "https://open.bigmodel.cn/api/paas/v4"},
		{"moonshot", "月之暗面 Kimi", "openai-compatible", "https://api.moonshot.cn/v1"},
	}
	for i, p := range providerNames {
		prov := &model.LLMProvider{
			ID:          uint(i + 1),
			UUID:        providerUUID[i],
			Name:        p.Name,
			DisplayName: p.Display,
			Protocol:    p.Protocol,
			BaseURL:     p.BaseURL,
			IsEnabled:   true,
			SortOrder:   i,
			CreatedAt:   now,
			UpdatedAt:   now,
		}
		s.providers[providerUUID[i].String()] = prov
	}
	s.nextProviderID = 9

	// ---------- 9 models(每条对齐一个 provider) ----------
	modelNames := []struct {
		Name, Display                  string
		Temp                           float64
		MaxTokens                      int
		IsDefault, IsSynthesis, IsFusion, IsEn bool
	}{
		{"gpt-4o", "GPT-4o", 0.7, 4096, true, false, false, true},
		{"claude-3-5-sonnet", "Claude 3.5 Sonnet", 0.7, 4096, false, false, false, true},
		{"gemini-1.5-pro", "Gemini 1.5 Pro", 0.7, 4096, false, false, false, true},
		{"mistral-large", "Mistral Large", 0.7, 4096, false, false, false, true},
		{"deepseek-chat", "DeepSeek Chat", 0.7, 4096, false, true, false, true},
		{"qwen-max", "通义千问 Max", 0.7, 4096, false, false, false, true},
		{"ernie-4.0", "文心 ERNIE 4.0", 0.7, 4096, false, false, false, true},
		{"glm-4", "智谱 GLM-4", 0.7, 4096, false, false, false, true},
		{"moonshot-v1-32k", "Kimi v1 32k", 0.7, 4096, false, false, false, true},
	}
	for i, m := range modelNames {
		mm := &model.LLMModel{
			ID:          uint(i + 1),
			UUID:        modelUUID[i],
			ProviderID:  uint(i + 1),
			Name:        m.Name,
			DisplayName: m.Display,
			Temperature: m.Temp,
			MaxTokens:   m.MaxTokens,
			IsDefault:   m.IsDefault,
			IsSynthesis: m.IsSynthesis,
			IsFusion:    m.IsFusion,
			IsEnabled:   m.IsEn,
			SortOrder:   i,
			CreatedAt:   now,
			UpdatedAt:   now,
		}
		s.models[modelUUID[i].String()] = mm
	}
	s.nextModelID = 9

	// ---------- 9 pills(与 agent 1:1 绑定) ----------
	pillData := []struct{ Name, Desc, Author string }{
		{"凝神丸", "凝神静气,助道人专注当下,不为外物所扰。", "玄真子"},
		{"烈阳散", "壮阳助火,提升道人行动力与决断。", "火舞儿"},
		{"问道丸", "开启慧根,引道人入深思之境。", "望舒"},
		{"逍遥露", "化郁解忧,使道人言语之间带几分洒脱。", "苍羽"},
		{"金刚丸", "稳固心性,临大变而不惊。", "玄铁"},
		{"灵犀丹", "通灵达意,善解来客所言。", "灵犀"},
		{"纯阳丹", "去阴扶阳,言语明亮而少阴郁。", "丹心"},
		{"墨韵散", "润泽文气,使道人应答如行云流水。", "墨言"},
		{"紫电丸", "锋锐通透,言辞明快如电光。", "紫电"},
	}
	for i, p := range pillData {
		pill := &model.ElixirPill{
			ID:          uint(i + 1),
			UUID:        pillUUID[i],
			Name:        p.Name,
			Description: p.Desc,
			SkillSchema: model.JSONMap{
				"trigger":        "当道人涉及" + p.Name + "主题时启用",
				"anti_trigger":   "用户明确拒绝时停用",
				"language_style": "古意、温润、含譬喻",
				"value_tendency": "平和、中正、不偏激",
			},
			Tags:      model.JSONList{p.Author, "内置"},
			Author:    p.Author,
			Version:   "1.0.0",
			IsBuiltin: true,
			CreatedAt: now,
			UpdatedAt: now,
		}
		s.pills[pillUUID[i].String()] = pill
	}
	s.nextPillID = 9

	// ---------- 9 agents ----------
	agentData := []struct{ Name, Avatar, Personality, ModelName, Status string }{
		{"玄真子", "/avatars/xuanzhen.png", "清修之人,寡言而深沉,言语带道家古意。", "gpt-4o", "active"},
		{"火舞儿", "/avatars/huowu.png", "热烈如焰,言辞明快,常有惊人之语。", "claude-3-5-sonnet", "active"},
		{"望舒", "/avatars/wangshu.png", "月华之性,温柔细腻,善解人意。", "gemini-1.5-pro", "active"},
		{"苍羽", "/avatars/cangyu.png", "闲云野鹤,洒脱不羁,言带江湖气。", "mistral-large", "active"},
		{"玄铁", "/avatars/xuantie.png", "刚毅沉稳,临事不乱,语如金石。", "deepseek-chat", "active"},
		{"灵犀", "/avatars/lingxi.png", "灵秀通透,一点即通,善察言观色。", "qwen-max", "active"},
		{"丹心", "/avatars/danxin.png", "赤诚正直,言出必行,光明磊落。", "ernie-4.0", "active"},
		{"墨言", "/avatars/moyan.png", "文气深厚,出口成章,善用典故。", "glm-4", "active"},
		{"紫电", "/avatars/zidian.png", "锋锐明快,直指核心,不喜绕弯。", "moonshot-v1-32k", "active"},
	}
	for i, a := range agentData {
		ag := &model.DaoAgent{
			ID:          uint(i + 1),
			UUID:        agentUUID[i],
			Name:        a.Name,
			Avatar:      a.Avatar,
			Personality: a.Personality,
			ModelName:   a.ModelName,
			Status:      a.Status,
			CreatedAt:   now,
		}
		s.agents[agentUUID[i].String()] = ag
	}
	s.nextAgentID = 9

	// ---------- 9 agent_pill 绑定(1:1) ----------
	for i := 0; i < 9; i++ {
		ap := &model.AgentPill{
			ID:        uint(i + 1),
			AgentID:   uint(i + 1),
			PillID:    uint(i + 1),
			Weight:    1.0,
			SortOrder: i,
			CreatedAt: now,
		}
		agentID := uint(i + 1)
		pillID := uint(i + 1)
		if s.agentPill[agentID] == nil {
			s.agentPill[agentID] = map[uint]*model.AgentPill{}
		}
		s.agentPill[agentID][pillID] = ap
	}
	s.nextApID = 9

	// ---------- 9 sessions(每道人一个) ----------
	for i := 0; i < 9; i++ {
		sess := &model.ChatSession{
			ID:        uint(i + 1),
			UUID:      sessionUUID[i],
			AgentID:   uint(i + 1),
			Title:     "与" + agentData[i].Name + "的对话",
			CreatedAt: now,
			UpdatedAt: now,
		}
		s.sessions[sessionUUID[i].String()] = sess
	}
	s.nextSessionID = 9

	// ---------- 9 messages(每会话一条用户消息) ----------
	userQuestions := []string{
		"你好,初次拜访,请多指教。",
		"我想了解炼丹之道,该从何处入手?",
		"我最近身体有些不适,该如何调理?",
		"能否为我推荐一些日常修行的建议?",
		"道家有那么多戒律,我该从何守起?",
		"如何看待命运的安排?",
		"佛家讲明心见性,与金丹化性有何异同?",
		"我的气血不太好,有什么调养之法?",
		"灵、魂、神三者如何区分?",
	}
	for i := 0; i < 9; i++ {
		msg := &model.ChatMessage{
			ID:        uint(i + 1),
			UUID:      messageUUID[i],
			SessionID: uint(i + 1),
			Role:      "user",
			Content:   userQuestions[i],
			CreatedAt: now,
		}
		s.messages[uint(i+1)] = []*model.ChatMessage{msg}
	}
	s.nextMessageID = 9
}
