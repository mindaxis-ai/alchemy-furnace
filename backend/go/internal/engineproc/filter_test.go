// filter_test.go - 验证 engineproc 过滤污染 env 的行为
package engineproc

import (
	"strings"
	"testing"
)

func TestFilterPythonEnv_DropsConflicting(t *testing.T) {
	in := []string{
		"PATH=/usr/bin",
		"HOME=/Users/x",
		"LOG_FORMAT=json", // 应被丢弃
		"LOG_LEVEL=DEBUG",  // 应被丢弃
		"RUST_LOG=warn",    // 应被丢弃
		"RANDOM=value",     // 应透传(非黑名单)
	}
	got := filterPythonEnv(in)
	joined := strings.Join(got, "\n")

	for _, must := range []string{"PATH=", "HOME=", "RANDOM="} {
		if !strings.Contains(joined, must) {
			t.Errorf("应保留 %s,得 %s", must, joined)
		}
	}
	for _, drop := range []string{"LOG_FORMAT", "LOG_LEVEL", "RUST_LOG"} {
		if strings.Contains(joined, drop+"=") {
			t.Errorf("应丢弃 %s,得 %s", drop, joined)
		}
	}
}

func TestFilterPythonEnv_PreservesAllByDefault(t *testing.T) {
	// 不在黑名单的都应透传(白名单是 PATH/HOME 等的语义提示,实际不过滤)
	in := []string{"SOMETHING=value", "ANOTHER=ok"}
	got := filterPythonEnv(in)
	if len(got) != 2 {
		t.Fatalf("应透传 2 个,得 %d: %v", len(got), got)
	}
}
