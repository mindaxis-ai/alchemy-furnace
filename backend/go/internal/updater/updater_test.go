// updater_test.go - IsNewer / SelectAsset / VerifyChecksums 单测
// CheckLatest / Download 用 httptest mock GitHub API
package updater

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestIsNewer(t *testing.T) {
	for _, c := range []struct {
		latest, current string
		want            bool
	}{
		{"v0.2.0", "v0.1.0", true},
		{"0.1.1", "v0.1.0", true},
		{"v0.1.0", "v0.1.0", false},
		{"v0.1.0", "v0.2.0", false},
		{"v0.2.0-beta.1", "v0.1.0", false}, // prerelease 不推
		{"dev", "v0.1.0", false},           // 非法版本不推
		{"", "v0.1.0", false},              // 空不推
	} {
		if got := IsNewer(c.latest, c.current); got != c.want {
			t.Fatalf("IsNewer(%q,%q)=%v want %v", c.latest, c.current, got, c.want)
		}
	}
}

func TestSelectAsset(t *testing.T) {
	r := &ReleaseInfo{Assets: []Asset{
		{Name: "AlchemyFurnace-mac-arm64.zip", URL: "https://x/mac-arm64"},
		{Name: "AlchemyFurnace-mac-x64.zip", URL: "https://x/mac-x64"},
		{Name: "AlchemyFurnace-Setup.exe", URL: "https://x/win"},
	}}
	if a := SelectAsset(r, "darwin", "arm64"); a == nil || a.Name != "AlchemyFurnace-mac-arm64.zip" {
		t.Fatalf("darwin/arm64 选择错误: %+v", a)
	}
	if a := SelectAsset(r, "darwin", "amd64"); a == nil || a.Name != "AlchemyFurnace-mac-x64.zip" {
		t.Fatalf("darwin/amd64 选择错误: %+v", a)
	}
	if a := SelectAsset(r, "windows", "amd64"); a == nil || a.Name != "AlchemyFurnace-Setup.exe" {
		t.Fatalf("windows/amd64 选择错误: %+v", a)
	}
	if a := SelectAsset(r, "linux", "amd64"); a != nil {
		t.Fatalf("linux 应无 asset: %+v", a)
	}
	if a := SelectAsset(nil, "darwin", "arm64"); a != nil {
		t.Fatalf("nil release 应返回 nil")
	}
}

func TestVerifyChecksums(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "a.zip")
	if err := os.WriteFile(f, []byte("abc"), 0o644); err != nil {
		t.Fatal(err)
	}
	sum := fmt.Sprintf("%x", sha256.Sum256([]byte("abc")))
	good := strings.NewReader(sum + "  a.zip\n" + strings.Repeat("0", 64) + "  other.zip\n")
	if err := VerifyChecksums(f, "a.zip", good); err != nil {
		t.Fatalf("正确 sha256 应通过: %v", err)
	}

	bad := strings.NewReader(strings.Repeat("1", 64) + "  a.zip\n")
	if err := VerifyChecksums(f, "a.zip", bad); err == nil {
		t.Fatal("校验和不符应报错")
	}

	if err := VerifyChecksums(f, "missing.zip", strings.NewReader("")); err == nil {
		t.Fatal("checksums 中无此文件应报错")
	}
}
