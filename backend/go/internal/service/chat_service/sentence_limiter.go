package chat_service

import "unicode"

// SentenceLimiter 在完整句边界限制流式输出(Global Constraints:只在完整句边界
// 停止,不得按字节截断)。纯内存无状态外依赖:不启动 goroutine、不读写数据库、
// 不记录正文。Task 9 起供 StreamChat 执行句数硬限制。
type SentenceLimiter struct {
	maxSentences int    // <=0 表示不限制
	buffer       []rune // 未完成的尾部内容(按 rune 保留,避免切坏 UTF-8)
	sentences    int    // 已发出的完整句数
}

// NewSentenceLimiter 构造限制器;maxSentences<=0 表示不限制。
func NewSentenceLimiter(maxSentences int) *SentenceLimiter {
	return &SentenceLimiter{maxSentences: maxSentences}
}

// Push 处理一个流式 chunk:返回应发出的内容与是否达到句数上限。
// 句末字符固定为 。！？!?;ASCII 句号仅在后面是空白、换行或 chunk 结束时计数。
// 达到第 N 个完整句末时,返回截至该句末的内容并 stop=true,丢弃同 chunk 后续。
// 未完成尾部内容按 rune 缓存,待后续 chunk 补齐标点后一并发出。
func (l *SentenceLimiter) Push(chunk string) (emit string, stop bool) {
	if l.maxSentences <= 0 {
		return chunk, false
	}
	l.buffer = append(l.buffer, []rune(chunk)...)
	lastEnd := -1 // 本次扫描最后一个完整句末的位置(含)
	for i := 0; i < len(l.buffer); i++ {
		if !sentenceEndAt(l.buffer, i) {
			continue
		}
		l.sentences++
		lastEnd = i
		if l.sentences >= l.maxSentences {
			// 达到上限:发出截至该句末的内容,丢弃同 chunk 后续
			out := string(l.buffer[:i+1])
			l.buffer = l.buffer[:0]
			return out, true
		}
	}
	if lastEnd < 0 {
		return "", false // 无完整句:全部缓存
	}
	out := string(l.buffer[:lastEnd+1])
	l.buffer = append([]rune(nil), l.buffer[lastEnd+1:]...) // 保留未完成尾部
	return out, false
}

// sentenceEndAt 判断 rune 序列中下标 i 处是否为完整句末。
func sentenceEndAt(runes []rune, i int) bool {
	switch r := runes[i]; r {
	case '。', '！', '？', '!', '?':
		return true
	case '.':
		// 英文句号:仅在后面是空白/换行或 chunk(buffer)末尾时计数
		if i+1 >= len(runes) {
			return true
		}
		return unicode.IsSpace(runes[i+1])
	}
	return false
}
