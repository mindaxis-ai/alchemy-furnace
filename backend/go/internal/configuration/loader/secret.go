// secret.go - desktop 模式 MODEL_KEY_SECRET 兜底: 文件持久化,首启自动生成
package loader

import (
	"crypto/rand"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"

	"github.com/alchemy-furnace/server/internal/configuration"
	"github.com/alchemy-furnace/server/internal/paths"
)

// secretKeyFile desktop 模式存放 secret 的文件名(0600 权限)
const secretKeyFile = "secret.key"

// secretKeyBytes 32 字节 = 256 bit,加密学安全
const secretKeyBytes = 32

// resolveModelKeySecret 优先级: 已配置(env/toml) > 数据目录/secret.key > 首启生成
// 仅 desktop 模式做文件兜底;serve 模式缺省即空(维持现状,存 key 时报错引导)
func resolveModelKeySecret(c *configuration.Config) error {
	if strings.TrimSpace(c.ModelKeySecret) != "" || !paths.IsDesktop() {
		return nil
	}
	dir, err := paths.EnsureDataDir()
	if err != nil {
		return err
	}
	f := filepath.Join(dir, secretKeyFile)
	// 已有 secret 文件 → 读出来
	if data, err := os.ReadFile(f); err == nil {
		s := strings.TrimSpace(string(data))
		if s != "" {
			c.ModelKeySecret = s
			return nil
		}
	} else if !os.IsNotExist(err) {
		return err // 非"NotFound"错误要报
	}
	// 首启生成 32B 随机 hex,写 0600
	buf := make([]byte, secretKeyBytes)
	if _, err := rand.Read(buf); err != nil {
		return err
	}
	hexStr := hex.EncodeToString(buf)
	if err := os.WriteFile(f, []byte(hexStr), 0o600); err != nil {
		return err
	}
	c.ModelKeySecret = hexStr
	return nil
}
