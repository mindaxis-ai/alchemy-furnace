package paths

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDataDirServeMode(t *testing.T) {
	SetDesktopMode(false)
	d, err := DataDir()
	if err != nil || d != "./data" {
		t.Fatalf("serve 模式 DataDir=%q,%v 应为 ./data", d, err)
	}
}

func TestDataDirDesktopMode(t *testing.T) {
	SetDesktopMode(true)
	defer SetDesktopMode(false)
	d, err := DataDir()
	if err != nil {
		t.Fatal(err)
	}
	base, _ := os.UserConfigDir()
	if d != filepath.Join(base, "AlchemyFurnace") {
		t.Fatalf("desktop DataDir=%q", d)
	}
}

func TestEnsureDataDirIdempotent(t *testing.T) {
	tmp := t.TempDir()
	SetDesktopMode(true)
	SetDataDirOverrideForTest(tmp)
	defer func() { SetDesktopMode(false); SetDataDirOverrideForTest("") }()
	d1, err1 := EnsureDataDir()
	d2, err2 := EnsureDataDir()
	if err1 != nil || err2 != nil || d1 != d2 {
		t.Fatalf("EnsureDataDir 非幂等: %q/%v vs %q/%v", d1, err1, d2, err2)
	}
}
