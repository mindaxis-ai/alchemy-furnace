// Package credential 提供模型调用凭证的类型与 API Key 加解密工具。
// 供应商/模型服务、对话服务、试炼服务共用，避免循环依赖。
// 加密密钥取自 internal/configuration.Configuration.ModelKeySecret（环境变量 MODEL_KEY_SECRET）。
package credential

import (
	"fmt"

	"github.com/alchemy-furnace/server/internal/configuration"
	alchemycrypto "github.com/alchemy-furnace/server/internal/util/crypto"
)

// ModelCredentials 解析后的模型调用凭证，按请求透传给 Python 语言引擎。
// BaseURL/APIKey 为空时 Python 回退到自身环境变量配置（向后兼容）。
type ModelCredentials struct {
	Model   string `json:"model"`
	BaseURL string `json:"base_url,omitempty"`
	APIKey  string `json:"api_key,omitempty"`
	// ProviderName/ProviderType 非敏感元数据(供应商名/协议):供编排快照填 model_ref
	// 路由信息,不参与解密链;未登记模型的回退凭证两者为空(向后兼容)。
	ProviderName string `json:"provider_name,omitempty"`
	ProviderType string `json:"provider_type,omitempty"`
}

// ErrNoSecret 未配置 MODEL_KEY_SECRET（创建/更新供应商 API Key 时返回，服务层映射为 HTTP 400）。
var ErrNoSecret = fmt.Errorf("未配置 MODEL_KEY_SECRET 环境变量，无法存储 API Key")

// MaskAPIKey 生成 api_key 掩码：长度 > 7 时显示前 3 位 + **** + 末 4 位（如 sk-****wxyz），否则 ****。
func MaskAPIKey(apiKey string) string {
	if apiKey == "" {
		return ""
	}
	if len(apiKey) > 7 {
		return apiKey[:3] + "****" + apiKey[len(apiKey)-4:]
	}
	return "****"
}

// EncryptAPIKey 加密明文 api_key；明文为空直接返回空（免密钥本地服务）。
// 明文非空但未配置 MODEL_KEY_SECRET 时返回 ErrNoSecret。
func EncryptAPIKey(plain string) (string, error) {
	if plain == "" {
		return "", nil
	}
	secret := configuration.Configuration.ModelKeySecret
	if secret == "" {
		return "", ErrNoSecret
	}
	enc, err := alchemycrypto.Encrypt(plain, secret)
	if err != nil {
		return "", fmt.Errorf("加密 API Key 失败: %w", err)
	}
	return enc, nil
}

// DecryptAPIKey 解密供应商的 api_key；未配置（空密文）返回空字符串。
func DecryptAPIKey(encrypted string) (string, error) {
	if encrypted == "" {
		return "", nil
	}
	secret := configuration.Configuration.ModelKeySecret
	plain, err := alchemycrypto.Decrypt(encrypted, secret)
	if err != nil {
		return "", fmt.Errorf("供应商凭证解密失败，请检查 MODEL_KEY_SECRET 配置: %w", err)
	}
	return plain, nil
}
