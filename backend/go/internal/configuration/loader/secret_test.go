package loader

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/alchemy-furnace/server/internal/configuration"
	"github.com/alchemy-furnace/server/internal/paths"
)

func TestSecretKeepConfigured(t *testing.T) {
	c := &configuration.Config{ModelKeySecret: "from-env"}
	if err := resolveModelKeySecret(c); err != nil || c.ModelKeySecret != "from-env" {
		t.Fatalf("已配置 secret 不应被覆盖: %q %v", c.ModelKeySecret, err)
	}
}

func TestSecretGenerateDesktop(t *testing.T) {
	tmp := t.TempDir()
	paths.SetDesktopMode(true)
	paths.SetDataDirOverrideForTest(tmp)
	defer func() { paths.SetDesktopMode(false); paths.SetDataDirOverrideForTest("") }()

	c := &configuration.Config{}
	if err := resolveModelKeySecret(c); err != nil {
		t.Fatal(err)
	}
	if len(c.ModelKeySecret) != 64 { // 32B hex
		t.Fatalf("生成 secret 长度=%d", len(c.ModelKeySecret))
	}
	fi, err := os.Stat(filepath.Join(tmp, "secret.key"))
	if err != nil || fi.Mode().Perm() != 0o600 {
		t.Fatalf("secret.key 权限 err=%v mode=%v", err, fi.Mode().Perm())
	}
	// 二次调用读文件,值不变(幂等)
	c2 := &configuration.Config{}
	if err := resolveModelKeySecret(c2); err != nil || c2.ModelKeySecret != c.ModelKeySecret {
		t.Fatal("二次加载未复用 secret.key")
	}
}
