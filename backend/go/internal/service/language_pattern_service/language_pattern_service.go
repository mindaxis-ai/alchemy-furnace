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

	"github.com/alchemy-furnace/server/internal/behavior"
	"github.com/alchemy-furnace/server/internal/errors"
	"github.com/alchemy-furnace/server/internal/interface/dao"
	"github.com/alchemy-furnace/server/internal/service/credential"
	"github.com/alchemy-furnace/server/internal/synthesis"
	"github.com/alchemy-furnace/server/model"
	"github.com/google/uuid"
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

// GetOrBuildPattern 获取道人语言模式: 缓存命中(is_valid + 指纹一致 + 新档案结构)直接返回;
// 否则走三段式重建: 确定性编译(Go) -> 涌现合成(Python) -> 确定性渲染(Go)。
// 合成失败/降级时返回无损确定性渲染(is_valid=false 临时对象,不落库),聊天不阻断。
// 缓存保护(§3.2): 写回前事务内核对 EffectsRevision(读取时记录值),并发编排变更导致
// 冲突时丢弃本次结果重读重试,最多重试 2 次;仍冲突返回 409 agent.effects_conflict,
// 不返回旧能力拼装结果、不覆盖新能力。
func (s *LanguagePatternService) GetOrBuildPattern(ctx context.Context, agentUID string) (*model.LanguagePattern, errors.Error) {
	for attempt := 0; attempt < 3; attempt++ {
		pattern, err := s.buildOnce(ctx, agentUID)
		if err == nil {
			return pattern, nil
		}
		if err.GetCode() != "agent.effects_conflict" || attempt == 2 {
			return nil, err
		}
		// 编排已变更: 本次编译基于过期能力,丢弃并重读当前状态(重读若缓存命中则不重合成)
		zap.L().Warn("[炼丹炉] 语言模式缓存写入与并发编排变更冲突,丢弃结果重试",
			zap.String("agent_uid", agentUID), zap.Int("attempt", attempt+1))
	}
	return nil, errors.ErrorServerInternalError("service.language_pattern.retry_exhausted")
}

// buildOnce 单次「读取→命中判断→(编译+合成+渲染)→带版本核对写回」。
// 冲突错误(agent.effects_conflict)由 GetOrBuildPattern 外层决定重试,不在本层吞掉。
func (s *LanguagePatternService) buildOnce(ctx context.Context, agentUID string) (*model.LanguagePattern, errors.Error) {
	uid, parseErr := uuid.Parse(agentUID)
	if parseErr != nil {
		return nil, errors.ErrorRecordNotFound("service.language_pattern.take_agent")
	}
	agent, err := s.agent.TakeAgentDetailByUUID(ctx, uid)
	if err != nil {
		return nil, err.Relation(errors.ErrorRecordNotFound("service.language_pattern.take_agent"))
	}

	pills := buildPillInputs(agent)
	fingerprint, fpErr := computeFingerprint(agent.Personality, pills)
	if fpErr != nil {
		return nil, errors.ErrorServerInternalError("service.language_pattern.fingerprint")
	}

	// 缓存命中判断: 旧缓存(无 behavior_profile)或版本不一致视为失效,按 spec §15 首次使用时自动重建
	if agent.LanguagePattern != nil && agent.LanguagePattern.IsValid &&
		agent.LanguagePattern.SourceFingerprint == fingerprint &&
		agent.LanguagePattern.BehaviorProfile != nil &&
		agent.LanguagePattern.ProfileVersion == behavior.ProfileVersion {
		return agent.LanguagePattern, nil
	}

	// 解析合成专用模型凭证(失败不阻塞合成: 回退环境变量模型配置)
	creds, credErr := s.creds.ResolveSynthesisCredentials(ctx)
	if credErr != nil {
		zap.L().Warn("[炼丹炉] 合成模型凭证解析失败，回退环境变量配置", zap.Error(credErr))
		creds = nil
	}

	resp, combineErr := s.synthesis.Combine(ctx, agent.Personality, pills, creds)
	if combineErr != nil {
		// 合成调用失败: 内存中无损编译+渲染(无涌现层),返回 is_valid=false 临时对象不落库。
		// 旧逻辑「失败时降级用旧缓存」删除: 旧缓存缺 behavior_profile 已被缓存判定排除
		zap.L().Warn("[炼丹炉] 语言模式合成失败，返回无损确定性渲染(不落库)",
			zap.String("agent_uid", agentUID), zap.Error(combineErr))
		return s.losslessTempPattern(agent.DaoAgentID, agent.Name, agent.Personality, fingerprint, "combine_error", pills), nil
	}

	// 降级结果(涌现层不可用)不落库: is_valid=false 临时对象,下次请求重试合成
	if resp.Degraded {
		zap.L().Warn("[炼丹炉] 语言模式合成降级,本次不落库",
			zap.String("agent_uid", agentUID), zap.String("reason", resp.DegradedReason))
		return s.losslessTempPattern(agent.DaoAgentID, agent.Name, agent.Personality, fingerprint, resp.DegradedReason, pills), nil
	}

	// 合成成功: 确定性编译 + 合并涌现层 + 渲染 + 写回缓存
	profile := behavior.CompileProfile(agent.Personality, pills)
	profile.WithEmergence(resp.EmergenceRules, resp.InnerTensions, false, "")
	bp, bpErr := behavior.ProfileToJSONMap(profile)
	if bpErr != nil {
		return nil, errors.ErrorServerInternalError("service.language_pattern.profile_marshal")
	}

	innerTensions := toInnerTensions(resp.InnerTensions)
	emergenceRules := resp.EmergenceRules
	if emergenceRules == nil {
		emergenceRules = model.JSONList{}
	}

	var pattern *model.LanguagePattern
	if agent.LanguagePattern != nil {
		agent.LanguagePattern.SystemPrompt = behavior.RenderSystemPrompt(profile, agent.Name)
		agent.LanguagePattern.EmergenceRules = emergenceRules
		agent.LanguagePattern.InnerTensions = innerTensions
		agent.LanguagePattern.BehaviorProfile = bp
		agent.LanguagePattern.ProfileVersion = behavior.ProfileVersion
		agent.LanguagePattern.SourceFingerprint = fingerprint
		agent.LanguagePattern.IsValid = true
		pattern = agent.LanguagePattern
	} else {
		pattern = &model.LanguagePattern{
			AgentID:           agent.DaoAgentID,
			SystemPrompt:      behavior.RenderSystemPrompt(profile, agent.Name),
			EmergenceRules:    emergenceRules,
			InnerTensions:     innerTensions,
			BehaviorProfile:   bp,
			ProfileVersion:    behavior.ProfileVersion,
			SourceFingerprint: fingerprint,
			IsValid:           true,
		}
	}
	// 写回缓存前事务内核对 EffectsRevision(读取时记录值);冲突由外层重读重试
	if err := s.agent.SaveLanguagePatternIfRevision(ctx, pattern, agent.EffectsRevision); err != nil {
		return nil, err
	}
	zap.L().Info("[炼丹炉] 语言模式合成完成(缓存写入)",
		zap.String("agent_uid", agentUID), zap.Int("pill_count", len(pills)))
	return pattern, nil
}

// buildPillInputs 从已吸收能力快照构建合成输入(任务 3 起的事实来源):
// 身份=被服用实例 UUID(传播到行为指纹与 turn policy);内容=吸收时的名称/完整 schema 快照,
// 不依赖丹方当前内容与金丹是否还在库存。权重/顺序=吸收时编排值。
func buildPillInputs(agent *model.DaoAgent) []synthesis.PillInput {
	effects := agent.AgentPillEffects
	pills := make([]synthesis.PillInput, 0, len(effects))
	for _, ef := range effects {
		pills = append(pills, synthesis.PillInput{
			ID:          ef.Item.PillItemID,
			Name:        ef.NameSnapshot,
			Weight:      ef.Weight,
			SortOrder:   ef.SortOrder,
			SkillSchema: ef.SchemaSnapshot,
		})
	}
	return pills
}

// losslessTempPattern 合成失败/降级时返回的无损确定性渲染(不落库):
// 在内存中完成编译+渲染,全部金丹字段保留(§12 无损降级);
// is_valid=false 保证下次请求重新合成,避免无涌现层结果被长期缓存。
func (s *LanguagePatternService) losslessTempPattern(agentUID string, agentName, personality, fingerprint, reason string, pills []synthesis.PillInput) *model.LanguagePattern {
	profile := behavior.CompileProfile(personality, pills)
	profile.WithEmergence(nil, nil, true, reason)
	bp, err := behavior.ProfileToJSONMap(profile)
	if err != nil {
		bp = nil
	}
	return &model.LanguagePattern{
		AgentID:           agentUID,
		SystemPrompt:      behavior.RenderSystemPrompt(profile, agentName),
		EmergenceRules:    model.JSONList{},
		InnerTensions:     model.JSONList{},
		BehaviorProfile:   bp,
		ProfileVersion:    behavior.ProfileVersion,
		SourceFingerprint: fingerprint,
		IsValid:           false,
	}
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

// computeFingerprint 计算来源指纹: SHA256(personality + 排序后的已吸收能力)
// 排序键 (sort_order, id字符串) 字典序,id=被服用实例 UUID(任务 3 起,与 turn policy 身份一致);
// 序列化与 Python json.dumps(ensure_ascii=False, sort_keys=True) 字节一致;
// 返回 "sha256:"+hex
func computeFingerprint(personality string, pills []synthesis.PillInput) (string, error) {
	// 按 (sort_order, id字符串) 字典序排序(与 Python 端 key=lambda p: (sort_order, str(id)) 一致)
	ordered := make([]synthesis.PillInput, len(pills))
	copy(ordered, pills)
	sort.SliceStable(ordered, func(i, j int) bool {
		if ordered[i].SortOrder != ordered[j].SortOrder {
			return ordered[i].SortOrder < ordered[j].SortOrder
		}
		return ordered[i].ID < ordered[j].ID
	})

	// 构建 payload: {personality, pills: [{id, name, weight, sort_order, skill_schema}]}
	list := make([]any, 0, len(ordered))
	for _, p := range ordered {
		list = append(list, map[string]any{
			"id":           p.ID,
			"name":         p.Name,
			"weight":       p.Weight,
			"sort_order":   p.SortOrder,
			"skill_schema": p.SkillSchema,
		})
	}
	payload := map[string]any{
		"personality": personality,
		"pills":       list,
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
