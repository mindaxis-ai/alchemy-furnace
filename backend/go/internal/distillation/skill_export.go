// Skill 导出: 规范化导出模型 + 服务端重校验 + Python 引擎调用客户端方法。
// 与 plan §2 的 ExportableSkill 对齐: 前端/Go 独立定义,绝不直接序列化数据库 Pill。
// 权限边界: 接口绝不接收 API Key/凭据字段(RejectCredentialFields),内容级密钥由
// 字段校验与 Python 渲染器双重拦截。
package distillation

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"time"
	"unicode/utf8"

	"github.com/alchemy-furnace/server/model"
)

const (
	// MaxExportNameLength 名称最大长度(与 Python skill_export.py 对齐)
	MaxExportNameLength = 80
	// MaxExportDescriptionLength 描述最大长度
	MaxExportDescriptionLength = 500
	// MaxExportTags 标签数量上限
	MaxExportTags = 12
	// MaxExportTagLength 单标签长度上限
	MaxExportTagLength = 30
	// MaxExportSources 来源数量上限
	MaxExportSources = 50
	// MaxExportSourceTitleLength 来源标题长度上限
	MaxExportSourceTitleLength = 200
	// MaxExportSourceDimensionLength 来源维度长度上限
	MaxExportSourceDimensionLength = 60
	// MaxExportURLLength 来源 URL 长度上限
	MaxExportURLLength = 2048
)

// slugPattern 用户可见 slug 规则: [a-z0-9][a-z0-9-]{0,48}(与 plan §2 一致)
var slugPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,48}$`)

// uuidPattern 数据库标识形态,禁止作为用户可见名称
var uuidPattern = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// secretPatterns 疑似密钥/凭据内容(与 Python skill_export.py 的 SECRET_PATTERNS 对齐)
var secretPatterns = []*regexp.Regexp{
	regexp.MustCompile(`\bsk-[A-Za-z0-9]{16,}\b`),
	regexp.MustCompile(`(?i)\bapi[_-]?key\s*[:=]\s*\S{8,}`),
	regexp.MustCompile(`(?i)\bbearer\s+[A-Za-z0-9._~+/=-]{16,}`),
}

// credentialFieldKeys 接口边界拒绝的顶层凭据键(绝不接收 API Key)
var credentialFieldKeys = []string{"api_key", "apikey", "apiKey", "model_key", "secret", "token", "password", "credential"}

// ErrExportCredentialRejected 请求体携带凭据字段,接口边界拒绝(→403)
var ErrExportCredentialRejected = errors.New("导出接口不接受任何密钥或凭据字段")

// ExportableSkill 规范化导出模型(plan §2)。
// instructions/attribution 由服务端按结构化字段稳定渲染,客户端传入值不参与组装;
// slug 由 Python 渲染器从名称派生,此处仅做格式重校验。
type ExportableSkill struct {
	Name          string            `json:"name"`
	Slug          string            `json:"slug"`
	Description   string            `json:"description"`
	Instructions  string            `json:"instructions"`
	SkillSchema   model.JSONMap     `json:"skillSchema"`
	Tags          []string          `json:"tags"`
	Sources       []Source          `json:"sources"`
	Attribution   map[string]string `json:"attribution"`
	GeneratedAt   string            `json:"generatedAt"`
	EvidenceLevel string            `json:"evidence_level,omitempty"`
}

// ExportResult 导出产物: ZIP 字节 + 下载文件名(纯 ASCII,§3.4 命名)
type ExportResult struct {
	Filename string
	Content  []byte
}

// SkillExportInput 导出接口入参（任务 5 消耗品重构后）。
// 目标三选一：旧 pill_id（仅经 LegacyMap 解析，不读取可用库存）、
// recipe_id（当前版本）+ 可选 revision_id（指定版本）、skill（结构化数据）。
type SkillExportInput struct {
	RecipeID   string
	RevisionID string
	Skill      *ExportableSkill
	Format     string
}

// ExportValidationError 导出内容校验失败(服务端重校验,→400)
type ExportValidationError struct {
	Field  string
	Reason string
}

func (e *ExportValidationError) Error() string {
	return "Skill 导出内容无效: " + e.Field + " " + e.Reason
}

// RejectCredentialFields 扫描请求体顶层键,发现凭据字段即返回 ErrExportCredentialRejected。
// 语法错误不在此拦截(交给绑定层);嵌套内容级密钥由字段校验拦截。
func RejectCredentialFields(raw []byte) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return nil
	}
	for _, key := range credentialFieldKeys {
		if _, ok := fields[key]; ok {
			return ErrExportCredentialRejected
		}
	}
	return nil
}

// ValidateExportable 服务端重校验: 字段长度、slug 格式、来源协议与敏感内容。
// 与 Python skill_export.py 的 validate_exportable 同口径,保证 Go 前置拦截与
// Python 最终校验语义一致。
func ValidateExportable(skill *ExportableSkill) error {
	if skill == nil {
		return &ExportValidationError{Field: "skill", Reason: "必须提供金丹结构化数据"}
	}
	if err := requireExportText(skill.Name, "name", 1, MaxExportNameLength); err != nil {
		return err
	}
	if uuidPattern.MatchString(skill.Name) {
		return &ExportValidationError{Field: "name", Reason: "名称疑似数据库标识,不能作为用户可见名称"}
	}
	if err := requireExportText(skill.Description, "description", 1, MaxExportDescriptionLength); err != nil {
		return err
	}
	if skill.Slug != "" && !slugPattern.MatchString(skill.Slug) {
		return &ExportValidationError{Field: "slug", Reason: "只允许小写字母数字与短横线(最长 49)"}
	}
	if skill.SkillSchema == nil {
		return &ExportValidationError{Field: "skill_schema", Reason: "必须是对象"}
	}
	if len(skill.Tags) > MaxExportTags {
		return &ExportValidationError{Field: "tags", Reason: fmt.Sprintf("数量超过 %d", MaxExportTags)}
	}
	for _, tag := range skill.Tags {
		if err := requireExportText(tag, "tags", 1, MaxExportTagLength); err != nil {
			return err
		}
	}
	if len(skill.Sources) > MaxExportSources {
		return &ExportValidationError{Field: "sources", Reason: fmt.Sprintf("数量超过 %d", MaxExportSources)}
	}
	for _, source := range skill.Sources {
		if err := requireExportText(source.Title, "sources.title", 1, MaxExportSourceTitleLength); err != nil {
			return err
		}
		if err := requireExportText(source.Dimension, "sources.dimension", 1, MaxExportSourceDimensionLength); err != nil {
			return err
		}
		if err := validateExportURL(source.URL); err != nil {
			return err
		}
	}
	if skill.GeneratedAt == "" || !isValidExportTime(skill.GeneratedAt) {
		return &ExportValidationError{Field: "generated_at", Reason: "必须是合法的 ISO 时间"}
	}
	if skill.EvidenceLevel != "" && !validEvidenceLevel(skill.EvidenceLevel) {
		return &ExportValidationError{Field: "evidence_level", Reason: "必须是 insufficient/limited/standard 之一"}
	}
	return nil
}

func validEvidenceLevel(level string) bool {
	switch level {
	case "insufficient", "limited", "standard":
		return true
	}
	return false
}

func isValidExportTime(value string) bool {
	for _, layout := range []string{time.RFC3339, time.RFC3339Nano} {
		if _, err := time.Parse(layout, value); err == nil {
			return true
		}
	}
	return false
}

func requireExportText(value, field string, minLen, maxLen int) error {
	if len(value) == 0 {
		return &ExportValidationError{Field: field, Reason: "不能为空"}
	}
	if runeLen := utf8.RuneCountInString(value); runeLen < minLen || runeLen > maxLen {
		return &ExportValidationError{Field: field, Reason: fmt.Sprintf("长度必须在 %d-%d 之间", minLen, maxLen)}
	}
	for _, r := range value {
		if r < 32 || r == 127 {
			return &ExportValidationError{Field: field, Reason: "包含危险控制字符"}
		}
	}
	if containsSecret(value) {
		return &ExportValidationError{Field: field, Reason: "疑似包含密钥或凭据"}
	}
	return nil
}

func containsSecret(text string) bool {
	for _, pattern := range secretPatterns {
		if pattern.MatchString(text) {
			return true
		}
	}
	return false
}

func validateExportURL(rawURL string) error {
	if len(rawURL) == 0 || len(rawURL) > MaxExportURLLength {
		return &ExportValidationError{Field: "sources.url", Reason: fmt.Sprintf("必须是长度不超过 %d 的非空字符串", MaxExportURLLength)}
	}
	for _, r := range rawURL {
		if r < 32 || r == 127 {
			return &ExportValidationError{Field: "sources.url", Reason: "包含控制字符"}
		}
		if r == ' ' || r == '\t' || r == '\n' || r == '\r' {
			return &ExportValidationError{Field: "sources.url", Reason: "包含空白字符"}
		}
	}
	parsed, err := url.Parse(rawURL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return &ExportValidationError{Field: "sources.url", Reason: "协议仅允许 http/https"}
	}
	if parsed.Hostname() == "" {
		return &ExportValidationError{Field: "sources.url", Reason: "缺少主机名"}
	}
	if parsed.User != nil {
		return &ExportValidationError{Field: "sources.url", Reason: "不允许携带凭据"}
	}
	return nil
}
