// Package language_pattern_service 语言模式缓存业务逻辑(新架构)
// 负责: 指纹计算 -> 缓存命中判断 -> 调用合成引擎重建 -> 写回缓存。
// 指纹算法与 Python 端 contracts/python-synthesis.md 严格一致:
// 排序键 (sort_order, str(id=uuid)) 字典序;序列化 json.dumps(ensure_ascii=False, sort_keys=True);
// 返回 "sha256:"+hex。跨端一致性金标准见 backend/python/app/tests/test_language_synthesis_service.py。
package language_pattern_service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/alchemy-furnace/server/internal/errors"
	"github.com/alchemy-furnace/server/internal/interface/dao"
	"github.com/alchemy-furnace/server/internal/service/credential"
	"github.com/alchemy-furnace/server/internal/synthesis"
	"github.com/alchemy-furnace/server/model"
	"go.uber.org/zap"
)

// LanguagePatternService service.LanguagePatternProvider 实现
type LanguagePatternService struct {
	agent     dao.Agent
	synthesis synthesis.Client
	creds     credential.Resolver
}

// New 构造语言模式服务实例
func New(agent dao.Agent, synthesis synthesis.Client, creds credential.Resolver) *LanguagePatternService {
	return &LanguagePatternService{agent: agent, synthesis: synthesis, creds: creds}
}

// GetOrBuildPattern 获取道人语言模式: 缓存命中(is_valid 且指纹一致)直接返回;
// 否则调用合成引擎重建并写回缓存。合成失败时若有旧缓存则降级返回
func (s *LanguagePatternService) GetOrBuildPattern(ctx context.Context, agentID uint) (*model.LanguagePattern, errors.Error) {
	agent, err := s.agent.TakeAgentDetailByID(ctx, agentID)
	if err != nil {
		return nil, err.Relation(errors.ErrorRecordNotFound("service.language_pattern.take_agent"))
	}

	fingerprint, fpErr := computeFingerprint(agent.Personality, agent.AgentPills)
	if fpErr != nil {
		return nil, errors.ErrorServerInternalError("service.language_pattern.fingerprint")
	}

	// 缓存命中判断
	if agent.LanguagePattern != nil && agent.LanguagePattern.IsValid && agent.LanguagePattern.SourceFingerprint == fingerprint {
		return agent.LanguagePattern, nil
	}

	// 缓存未命中: 组装合成输入
	pills := make([]synthesis.PillInput, 0, len(agent.AgentPills))
	for _, ap := range agent.AgentPills {
		pills = append(pills, synthesis.PillInput{
			ID:          ap.Pill.UUID.String(),
			Name:        ap.Pill.Name,
			Weight:      ap.Weight,
			SortOrder:   ap.SortOrder,
			SkillSchema: ap.Pill.SkillSchema,
		})
	}

	// 解析合成专用模型凭证(失败不阻塞合成: 回退环境变量模型配置)
	creds, credErr := s.creds.ResolveSynthesisCredentials(ctx)
	if credErr != nil {
		zap.L().Warn("[炼丹炉] 合成模型凭证解析失败，回退环境变量配置", zap.Error(credErr))
		creds = nil
	}

	resp, combineErr := s.synthesis.Combine(ctx, agent.Personality, pills, creds)
	if combineErr != nil {
		// 合成失败: 若存在旧缓存则降级返回(标记失效但可用)
		if agent.LanguagePattern != nil {
			zap.L().Warn("[炼丹炉] 语言模式合成失败，降级使用旧缓存",
				zap.Uint("agent_id", agentID), zap.Error(combineErr))
			return agent.LanguagePattern, nil
		}
		return nil, errors.New(errors.ErrorTypeServerInternalError, "service.language_pattern.combine", combineErr.Error())
	}

	// 写回缓存(upsert): 已有记录则更新(ID 非 0),否则创建
	innerTensions := toInnerTensions(resp.InnerTensions)
	emergenceRules := resp.EmergenceRules
	if emergenceRules == nil {
		emergenceRules = model.JSONList{}
	}

	// 降级结果(结构化合并兜底 prompt)不落库: 以 is_valid=false 的临时对象返回,
	// 本次论道可用;因指纹比对要求 is_valid=true,下次请求会重新合成,
	// 避免「指纹不变 → 兜底 prompt 被长期当有效缓存」的污染(本次 500 的深层原因)
	if resp.Degraded {
		zap.L().Warn("[炼丹炉] 语言模式合成为降级结果,本次不落库",
			zap.Uint("agent_id", agentID))
		return &model.LanguagePattern{
			AgentID:           agentID,
			SystemPrompt:      resp.SystemPrompt,
			EmergenceRules:    emergenceRules,
			InnerTensions:     innerTensions,
			SourceFingerprint: fingerprint,
			IsValid:           false,
		}, nil
	}

	if agent.LanguagePattern != nil {
		agent.LanguagePattern.SystemPrompt = resp.SystemPrompt
		agent.LanguagePattern.EmergenceRules = emergenceRules
		agent.LanguagePattern.InnerTensions = innerTensions
		agent.LanguagePattern.SourceFingerprint = fingerprint
		agent.LanguagePattern.IsValid = true
		if err := s.agent.SaveLanguagePattern(ctx, agent.LanguagePattern); err != nil {
			return nil, err.Relation(errors.ErrorServerInternalError("service.language_pattern.save"))
		}
		zap.L().Info("[炼丹炉] 语言模式合成完成(更新缓存)",
			zap.Uint("agent_id", agentID), zap.Int("pill_count", len(pills)))
		return agent.LanguagePattern, nil
	}

	pattern := &model.LanguagePattern{
		AgentID:           agentID,
		SystemPrompt:      resp.SystemPrompt,
		EmergenceRules:    emergenceRules,
		InnerTensions:     innerTensions,
		SourceFingerprint: fingerprint,
		IsValid:           true,
	}
	if err := s.agent.SaveLanguagePattern(ctx, pattern); err != nil {
		return nil, err.Relation(errors.ErrorServerInternalError("service.language_pattern.create"))
	}
	zap.L().Info("[炼丹炉] 语言模式合成完成(新建缓存)",
		zap.Uint("agent_id", agentID), zap.Int("pill_count", len(pills)))
	return pattern, nil
}

// toInnerTensions 将合成响应的内在冲突列表转为 JSONList 缓存格式
func toInnerTensions(tensions []synthesis.InnerTension) model.JSONList {
	if len(tensions) == 0 {
		return model.JSONList{}
	}
	raw, err := json.Marshal(tensions)
	if err != nil {
		return model.JSONList{}
	}
	var list model.JSONList
	if err := json.Unmarshal(raw, &list); err != nil {
		return model.JSONList{}
	}
	return list
}

// computeFingerprint 计算来源指纹: SHA256(personality + 排序后的金丹)
// 排序键 (sort_order, uuid字符串) 字典序;序列化与 Python json.dumps(ensure_ascii=False, sort_keys=True) 字节一致;
// 返回 "sha256:"+hex
func computeFingerprint(personality string, agentPills []model.AgentPill) (string, error) {
	// 按 (sort_order, uuid字符串) 字典序排序(与 Python 端 key=lambda p: (sort_order, str(id)) 一致)
	ordered := make([]model.AgentPill, len(agentPills))
	copy(ordered, agentPills)
	sort.SliceStable(ordered, func(i, j int) bool {
		if ordered[i].SortOrder != ordered[j].SortOrder {
			return ordered[i].SortOrder < ordered[j].SortOrder
		}
		return ordered[i].Pill.UUID.String() < ordered[j].Pill.UUID.String()
	})

	// 构建 payload: {personality, pills: [{id, name, weight, sort_order, skill_schema}]}
	pills := make([]any, 0, len(ordered))
	for _, ap := range ordered {
		pills = append(pills, map[string]any{
			"id":           ap.Pill.UUID.String(),
			"name":         ap.Pill.Name,
			"weight":       ap.Weight,
			"sort_order":   ap.SortOrder,
			"skill_schema": ap.Pill.SkillSchema,
		})
	}
	payload := map[string]any{
		"personality": personality,
		"pills":       pills,
	}

	raw, err := canonicalJSON(payload)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256([]byte(raw))
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

// ==================== Python 兼容规范化 JSON ====================
//
// canonicalJSON 序列化为与 Python json.dumps(ensure_ascii=False, sort_keys=True) 字节一致的 JSON。
// 与 Go encoding/json 默认行为的差异(均需修正以匹配 Python):
//   - 分隔符使用 ", " 与 ": "(Python 默认,非紧凑),Go 默认为 "," 与 ":"
//   - 关闭 HTML 转义(< > & 不转义),Go 默认转义
//   - 整数值 float64 渲染为 "N.0"(Python float repr),Go strconv 'g' 给出 "N"
//   - map 键字典序(Go 默认即字典序,与 Python sort_keys=True 一致)
//   - 非 ASCII(含中文)以 UTF-8 原样输出(ensure_ascii=False),Go 默认亦如此
func canonicalJSON(v any) (string, error) {
	var buf bytes.Buffer
	if err := writeCanonical(&buf, v); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func writeCanonical(buf *bytes.Buffer, v any) error {
	switch val := v.(type) {
	case nil:
		buf.WriteString("null")
	case bool:
		if val {
			buf.WriteString("true")
		} else {
			buf.WriteString("false")
		}
	case string:
		writeCanonicalString(buf, val)
	case int:
		buf.WriteString(strconv.Itoa(val))
	case int64:
		buf.WriteString(strconv.FormatInt(val, 10))
	case float64:
		writePythonFloat(buf, val)
	case model.JSONMap:
		return writeCanonicalMap(buf, map[string]any(val))
	case model.JSONList:
		return writeCanonicalSlice(buf, []any(val))
	case map[string]any:
		return writeCanonicalMap(buf, val)
	case []any:
		return writeCanonicalSlice(buf, val)
	default:
		// 兜底:其余类型(罕见)交由 encoding/json 处理后写入
		b, err := json.Marshal(v)
		if err != nil {
			return err
		}
		buf.Write(b)
	}
	return nil
}

func writeCanonicalMap(buf *bytes.Buffer, m map[string]any) error {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys) // 字典序,与 Python sort_keys=True 一致
	buf.WriteByte('{')
	for i, k := range keys {
		if i > 0 {
			buf.WriteString(", ") // Python 默认 item 分隔符
		}
		writeCanonicalString(buf, k)
		buf.WriteString(": ") // Python 默认 key 分隔符
		if err := writeCanonical(buf, m[k]); err != nil {
			return err
		}
	}
	buf.WriteByte('}')
	return nil
}

func writeCanonicalSlice(buf *bytes.Buffer, s []any) error {
	buf.WriteByte('[')
	for i, item := range s {
		if i > 0 {
			buf.WriteString(", ")
		}
		if err := writeCanonical(buf, item); err != nil {
			return err
		}
	}
	buf.WriteByte(']')
	return nil
}

// writeCanonicalString 写入 JSON 字符串字面量,转义规则对齐 Python json(ensure_ascii=False):
// " 与 \ 及控制字符转义;非 ASCII(含中文)以 UTF-8 原样输出;不做 HTML 转义。
func writeCanonicalString(buf *bytes.Buffer, s string) {
	buf.WriteByte('"')
	for i := 0; i < len(s); i++ {
		b := s[i]
		switch b {
		case '"':
			buf.WriteString(`\"`)
		case '\\':
			buf.WriteString(`\\`)
		case '\n':
			buf.WriteString(`\n`)
		case '\r':
			buf.WriteString(`\r`)
		case '\t':
			buf.WriteString(`\t`)
		case '\b':
			buf.WriteString(`\b`)
		case '\f':
			buf.WriteString(`\f`)
		default:
			if b < 0x20 {
				// 控制字符: \u00xx(4 位小写十六进制,与 Python 一致)
				buf.WriteString(fmt.Sprintf(`\u%04x`, b))
			} else {
				// ASCII 可见字符与 UTF-8 多字节序列(>= 0x80)原样输出
				buf.WriteByte(b)
			}
		}
	}
	buf.WriteByte('"')
}

// writePythonFloat 写入与 Python float repr 一致的浮点字面量
// Go strconv.FormatFloat(f,'g',-1,64) 给出最短往返表示,但对整数值浮点给出 "1" 而非 "1.0";
// Python float repr 对整数值给出 "1.0"。检测纯整数表示(无 '.' 'e' 'E')补 ".0"。
func writePythonFloat(buf *bytes.Buffer, f float64) {
	s := strconv.FormatFloat(f, 'g', -1, 64)
	if !strings.ContainsAny(s, ".eEnNiI") {
		s += ".0"
	}
	buf.WriteString(s)
}
