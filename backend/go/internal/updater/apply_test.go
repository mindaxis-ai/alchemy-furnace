//go:build darwin

// apply_test.go - darwin 平台专用:apply 路径推导 + swap 脚本生成
package updater

import (
	"strings"
	"testing"
)

func TestAppBundlePath(t *testing.T) {
	got, err := appBundlePathFromExe("/Applications/AlchemyFurnace.app/Contents/MacOS/AlchemyFurnace")
	if err != nil || got != "/Applications/AlchemyFurnace.app" {
		t.Fatalf("bundle 路径=%q,%v", got, err)
	}
	if _, err := appBundlePathFromExe("/usr/local/bin/AlchemyFurnace"); err == nil {
		t.Fatal("非 .app 内运行应报错(开发模式不应用更新)")
	}
	if _, err := appBundlePathFromExe("/opt/dev.sh"); err == nil {
		t.Fatal("不含 .app 应报错")
	}
}

func TestSwapScript(t *testing.T) {
	s := swapScript(1234, "/tmp/new.app", "/Applications/AlchemyFurnace.app")
	for _, want := range []string{"kill -0 $OLD_PID", ".old", "open", "/tmp/new.app"} {
		if !strings.Contains(s, want) {
			t.Fatalf("脚本缺 %q:\n%s", want, s)
		}
	}
	// 必须可执行权限标记
	if !strings.HasPrefix(s, "#!/bin/bash") {
		t.Fatal("脚本缺 shebang")
	}
	// 必须含回滚分支
	if !strings.Contains(s, "mv \"$APP_PATH.old\" \"$APP_PATH\"") {
		t.Fatal("脚本缺回滚逻辑")
	}
}
