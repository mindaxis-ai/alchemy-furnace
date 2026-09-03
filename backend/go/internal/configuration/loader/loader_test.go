package loader

import (
	"strings"
	"testing"

	"github.com/alchemy-furnace/server/internal/configuration"
	"github.com/alchemy-furnace/server/internal/paths"
)

// TestResolveDriver_DesktopRedirectsSQLiteToDataDir
// desktop 模式 + SQLite 相对路径应重定向到 DataDir
// serve 模式或显式绝对路径应保持原值
func TestResolveDriver_DesktopRedirectsSQLiteToDataDir(t *testing.T) {
	dir := t.TempDir()
	paths.SetDataDirOverrideForTest(dir)
	defer func() { paths.SetDesktopMode(false); paths.SetDataDirOverrideForTest("") }()

	// 1. desktop + 空路径 → 用默认相对路径 → 重定向到 DataDir
	paths.SetDesktopMode(true)
	d := &configuration.DatabaseConfig{}
	resolveDriver(d)
	if !strings.HasPrefix(d.SQLitePath, dir) {
		t.Fatalf("desktop 模式应把 SQLitePath 解析到 DataDir,得 %q", d.SQLitePath)
	}

	// 2. desktop + 显式相对路径 → 同样重定向
	d2 := &configuration.DatabaseConfig{SQLitePath: "./data/foo.db"}
	resolveDriver(d2)
	if !strings.HasPrefix(d2.SQLitePath, dir) {
		t.Fatalf("显式相对路径也应重定向,得 %q", d2.SQLitePath)
	}

	// 3. serve 模式 → 保持原相对路径
	paths.SetDesktopMode(false)
	d3 := &configuration.DatabaseConfig{}
	resolveDriver(d3)
	if d3.SQLitePath != "./data/alchemy.db" {
		t.Fatalf("serve 模式应保持默认相对路径,得 %q", d3.SQLitePath)
	}

	// 4. 显式绝对路径(desktop 模式)→ 保持
	paths.SetDesktopMode(true)
	abs := "/tmp/explicit/alchemy.db"
	d4 := &configuration.DatabaseConfig{SQLitePath: abs}
	resolveDriver(d4)
	if d4.SQLitePath != abs {
		t.Fatalf("显式绝对路径应保持,得 %q", d4.SQLitePath)
	}
}

// TestResolveOrchestrationEngine 编排引擎迁移开关(临时,设计 §12):
// 空 → legacy(零值兼容,存量部署零改动);合法值归一化保留;非法值启动期明确报错。
func TestResolveOrchestrationEngine(t *testing.T) {
	// 1. 空 → legacy
	cfg := &configuration.Config{}
	if err := resolveOrchestrationEngine(cfg); err != nil {
		t.Fatalf("empty engine: %v", err)
	}
	if cfg.OrchestrationEngine != "legacy" {
		t.Fatalf("empty engine = %q, want legacy", cfg.OrchestrationEngine)
	}

	// 2. 显式 legacy 原样保留
	cfg = &configuration.Config{OrchestrationEngine: "legacy"}
	if err := resolveOrchestrationEngine(cfg); err != nil {
		t.Fatalf("legacy engine: %v", err)
	}
	if cfg.OrchestrationEngine != "legacy" {
		t.Fatalf("legacy engine = %q", cfg.OrchestrationEngine)
	}

	// 3. 大小写/空白归一化
	cfg = &configuration.Config{OrchestrationEngine: " LangGraph "}
	if err := resolveOrchestrationEngine(cfg); err != nil {
		t.Fatalf("normalized engine: %v", err)
	}
	if cfg.OrchestrationEngine != "langgraph" {
		t.Fatalf("normalized engine = %q, want langgraph", cfg.OrchestrationEngine)
	}

	// 4. 非法值明确报错,不静默回退
	cfg = &configuration.Config{OrchestrationEngine: "bogus"}
	if err := resolveOrchestrationEngine(cfg); err == nil {
		t.Fatal("bogus engine: error = nil, want explicit config error")
	}
}
