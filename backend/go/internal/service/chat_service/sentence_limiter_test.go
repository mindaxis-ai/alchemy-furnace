package chat_service

import "testing"

// TestSentenceLimiter 完整句边界限制器(Global Constraints:只在完整句边界停止)。
// 覆盖跨 chunk、中文标点、UTF-8、英文句号白名单与不限制模式。
func TestSentenceLimiter(t *testing.T) {
	// 跨 chunk + 中文标点 + 上限截断:上限 2 依次发出前两句并停止
	l := NewSentenceLimiter(2)
	if emit, stop := l.Push("我懂。"); emit != "我懂。" || stop {
		t.Fatalf("第1句: emit=%q stop=%v, want 我懂。/false", emit, stop)
	}
	emit, stop := l.Push("先歇会。还有第三句。")
	if emit != "先歇会。" || !stop {
		t.Fatalf("第2句: emit=%q stop=%v, want 先歇会。/true(丢弃第三句)", emit, stop)
	}

	// 英文句号:3.14 不算句末(句号后是数字),整段为一句,补标点后完整发出
	l2 := NewSentenceLimiter(1)
	if emit, stop := l2.Push("3.14"); emit != "" || stop {
		t.Fatalf("3.14 句号后是数字,不应计数: emit=%q stop=%v", emit, stop)
	}
	emit, stop = l2.Push(" 是圆周率。")
	if emit != "3.14 是圆周率。" || !stop {
		t.Fatalf("无句末时整段为一句,补标点后完整发出: emit=%q stop=%v", emit, stop)
	}
	// 句号后跟空白才计数
	l2b := NewSentenceLimiter(1)
	if emit, stop := l2b.Push("完了. "); emit != "完了." || !stop {
		t.Fatalf("句号后跟空白应计数: emit=%q stop=%v", emit, stop)
	}

	// 英文句号在 chunk 末尾计数
	l3 := NewSentenceLimiter(1)
	if emit, stop := l3.Push("约3."); emit != "约3." || !stop {
		t.Fatalf("chunk 末尾句号应计数: emit=%q stop=%v", emit, stop)
	}

	// 无标点不提前截断:内容保留到句号到达后一并发出
	l4 := NewSentenceLimiter(1)
	if emit, stop := l4.Push("你好"); emit != "" || stop {
		t.Fatalf("你好 无标点不应截断: emit=%q stop=%v", emit, stop)
	}
	emit, stop = l4.Push("。")
	if emit != "你好。" || !stop {
		t.Fatalf("补标点后应完整发出: emit=%q stop=%v", emit, stop)
	}

	// maxSentences<=0 表示不限制:原样透传
	l5 := NewSentenceLimiter(0)
	if emit, stop := l5.Push("一二三。四五六。"); emit != "一二三。四五六。" || stop {
		t.Fatalf("不限制应透传: emit=%q stop=%v", emit, stop)
	}
	if emit, stop := NewSentenceLimiter(-1).Push("随便。"); emit != "随便。" || stop {
		t.Fatalf("负数也应不限制: emit=%q stop=%v", emit, stop)
	}
}

// 流结束时 Flush 返回未完成缓冲内容并清空:无句末内容不得丢失(「好的」等短回复)
func TestSentenceLimiterFlush(t *testing.T) {
	l := NewSentenceLimiter(1)
	if emit, stop := l.Push("你好"); emit != "" || stop {
		t.Fatalf("无句末应缓存: emit=%q stop=%v", emit, stop)
	}
	if tail := l.Flush(); tail != "你好" {
		t.Fatalf("Flush = %q, want 你好", tail)
	}
	if tail := l.Flush(); tail != "" {
		t.Fatalf("第二次 Flush = %q, want 空", tail)
	}
	if tail := NewSentenceLimiter(0).Flush(); tail != "" {
		t.Fatalf("空缓冲 Flush = %q, want 空", tail)
	}
}
