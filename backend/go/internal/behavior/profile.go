// Package behavior 道人行为引擎 - 金丹无损编译与提示词渲染(P1)
//
// 本包是「金丹编译 + 提示词渲染」的唯一事实源(spec §16.9: Go 是唯一策略源):
//   - CompileProfile     纯函数:基础性格 + 金丹列表 -> 完整结构化档案,类型合法字段
//     进对应字段,类型异常/未知键原值进 UnknownFields(无损,§6.2/§12)
//   - WithEmergence      合并涌现层(LLM 只产出涌现规则/冲突调和,不能覆盖金丹事实,§6.1)
//   - RenderSystemPrompt 确定性渲染分区提示词(§11;见 render.go)
//
// 本包不依赖 DB/网络/配置;同输入必同输出。
package behavior

import (
	"encoding/json"
	"fmt"

	"github.com/alchemy-furnace/server/internal/synthesis"
	"github.com/alchemy-furnace/server/model"
)

// ProfileVersion 行为档案版本;language_patterns.profile_version 不等于此值视为失效重建
const ProfileVersion = 2

// CompiledPillProfile 单颗金丹的无损编译结果(spec §6.2)
type CompiledPillProfile struct {
	PillID             string          `json:"pill_id"`
	Name               string          `json:"name"`
	Weight             float64         `json:"weight"`
	SortOrder          int             `json:"sort_order"`
	IdentityCard       string          `json:"identity_card"`
	Description        string          `json:"description"`
	ExpressionDNA      model.JSONMap   `json:"expression_dna"`
	MentalModels       []model.JSONMap `json:"mental_models"`
	DecisionHeuristics []model.JSONMap `json:"decision_heuristics"`
	Values             []string        `json:"values"`
	AntiPatterns       []string        `json:"anti_patterns"`
	HonestLimits       []string        `json:"honest_limits"`
	ExampleDialogues   []model.JSONMap `json:"example_dialogues"`
	UnknownFields      map[string]any  `json:"unknown_fields"`
}

// DaoistBehaviorProfile 完整结构化行为档案(spec §6.2)
type DaoistBehaviorProfile struct {
	Version           int                      `json:"version"`
	BasePersonality   string                   `json:"base_personality"`
	Pills             []CompiledPillProfile    `json:"pills"`
	EmergenceRules    []string                 `json:"emergence_rules"`
	InnerTensions     []synthesis.InnerTension `json:"inner_tensions"`
	EmergenceDegraded bool                     `json:"emergence_degraded"`
	DegradedReason    string                   `json:"degraded_reason,omitempty"`
}

// CompileProfile 纯函数:将基础性格与金丹列表编译为完整结构化档案。
// 已知九键按类型抽取;类型异常或未知键的原值进入 UnknownFields(§12:不静默修复、不丢数据)。
// pills 需已按 (sort_order, uuid字符串) 排序(调用方负责,与指纹排序一致)。
func CompileProfile(basePersonality string, pills []synthesis.PillInput) *DaoistBehaviorProfile {
	profile := &DaoistBehaviorProfile{
		Version:         ProfileVersion,
		BasePersonality: basePersonality,
		Pills:           make([]CompiledPillProfile, 0, len(pills)),
	}
	for _, p := range pills {
		profile.Pills = append(profile.Pills, compilePill(p))
	}
	return profile
}

// WithEmergence 将涌现层合并进档案(纯修改,返回接收者便于链式调用)。
// degraded=true 表示涌现层不可用:清空规则与张力并记录原因(档案本身无损)。
func (p *DaoistBehaviorProfile) WithEmergence(rules model.JSONList, tensions []synthesis.InnerTension, degraded bool, reason string) *DaoistBehaviorProfile {
	if degraded {
		p.EmergenceRules = nil
		p.InnerTensions = nil
	} else {
		p.EmergenceRules = stringifyRules(rules)
		p.InnerTensions = tensions
	}
	p.EmergenceDegraded = degraded
	p.DegradedReason = reason
	return p
}

// ProfileToJSONMap 将档案序列化为 JSONMap,用于 language_patterns.behavior_profile 缓存
func ProfileToJSONMap(p *DaoistBehaviorProfile) (model.JSONMap, error) {
	raw, err := json.Marshal(p)
	if err != nil {
		return nil, err
	}
	var m model.JSONMap
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, err
	}
	return m, nil
}

func compilePill(p synthesis.PillInput) CompiledPillProfile {
	out := CompiledPillProfile{
		PillID:             p.ID,
		Name:               p.Name,
		Weight:             p.Weight,
		SortOrder:          p.SortOrder,
		ExpressionDNA:      model.JSONMap{},
		MentalModels:       []model.JSONMap{},
		DecisionHeuristics: []model.JSONMap{},
		Values:             []string{},
		AntiPatterns:       []string{},
		HonestLimits:       []string{},
		ExampleDialogues:   []model.JSONMap{},
		UnknownFields:      map[string]any{},
	}
	for key, raw := range p.SkillSchema {
		switch key {
		case "identity_card":
			if s, ok := raw.(string); ok {
				out.IdentityCard = s
			} else {
				out.UnknownFields[key] = raw
			}
		case "description":
			if s, ok := raw.(string); ok {
				out.Description = s
			} else {
				out.UnknownFields[key] = raw
			}
		case "expression_dna":
			if m, ok := asMap(raw); ok {
				out.ExpressionDNA = m
			} else {
				out.UnknownFields[key] = raw
			}
		case "mental_models":
			if l, ok := asMapList(raw); ok {
				out.MentalModels = l
			} else {
				out.UnknownFields[key] = raw
			}
		case "decision_heuristics":
			if l, ok := asMapList(raw); ok {
				out.DecisionHeuristics = l
			} else {
				out.UnknownFields[key] = raw
			}
		case "example_dialogues":
			if l, ok := asMapList(raw); ok {
				out.ExampleDialogues = l
			} else {
				out.UnknownFields[key] = raw
			}
		case "values":
			if l, ok := asStringList(raw); ok {
				out.Values = l
			} else {
				out.UnknownFields[key] = raw
			}
		case "anti_patterns":
			if l, ok := asStringList(raw); ok {
				out.AntiPatterns = l
			} else {
				out.UnknownFields[key] = raw
			}
		case "honest_limits":
			if l, ok := asStringList(raw); ok {
				out.HonestLimits = l
			} else {
				out.UnknownFields[key] = raw
			}
		default:
			// 未知键:原值保留(如真实种子金丹的 agentic_protocol),渲染时进【扩展字段】分区
			out.UnknownFields[key] = raw
		}
	}
	return out
}

// asMap 提取 map 值。两种来源都必须接受: 代码/测试构造的是 JSONMap(定义类型),
// 数据库 JSON 反序列化得到 map[string]any。
func asMap(v any) (model.JSONMap, bool) {
	switch m := v.(type) {
	case model.JSONMap:
		return m, true
	case map[string]any:
		return model.JSONMap(m), true
	default:
		return nil, false
	}
}

// asMapList 提取对象数组;任一元素非对象即整体判定类型异常(整段进 UnknownFields)
func asMapList(v any) ([]model.JSONMap, bool) {
	raw, ok := v.([]any)
	if !ok {
		return nil, false
	}
	out := make([]model.JSONMap, 0, len(raw))
	for _, item := range raw {
		m, ok := asMap(item)
		if !ok {
			return nil, false
		}
		out = append(out, m)
	}
	return out, true
}

// asStringList 提取字符串数组;任一元素非字符串即整体判定类型异常
func asStringList(v any) ([]string, bool) {
	raw, ok := v.([]any)
	if !ok {
		return nil, false
	}
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		s, ok := item.(string)
		if !ok {
			return nil, false
		}
		out = append(out, s)
	}
	return out, true
}

// stringifyRules 将涌现规则转为字符串列表(Python 端已强制字符串,这里兜底)
func stringifyRules(rules model.JSONList) []string {
	out := make([]string, 0, len(rules))
	for _, r := range rules {
		out = append(out, fmt.Sprint(r))
	}
	return out
}
