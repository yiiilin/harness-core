package preview

import "unicode/utf8"

const marker = "..."

type Result struct {
	Text          string
	Truncated     bool
	OriginalBytes int
	ReturnedBytes int
	OriginalChars int
	ReturnedChars int
	HeadBytes     int
	TailBytes     int
	ElidedBytes   int
	HeadChars     int
	TailChars     int
	ElidedChars   int
}

func TruncateHeadTailBytes(text string, limit int) Result {
	result := fullResult(text)
	if limit <= 0 || result.OriginalBytes <= limit {
		return result
	}
	if limit <= len(marker)+1 {
		return prefixBytesFallback(text, limit, result)
	}

	contentBudget := limit - len(marker)
	headBudget, tailBudget := splitBudget(contentBudget)
	headText, headBytes, headChars := prefixWithinBytes(text, headBudget)
	tailText, tailBytes, tailChars := suffixWithinBytes(text, tailBudget)
	if headBytes == 0 || tailBytes == 0 {
		return prefixBytesFallback(text, limit, result)
	}

	result.Truncated = true
	result.Text = headText + marker + tailText
	result.ReturnedBytes = len(result.Text)
	result.ReturnedChars = utf8.RuneCountInString(result.Text)
	result.HeadBytes = headBytes
	result.TailBytes = tailBytes
	result.ElidedBytes = max(result.OriginalBytes-headBytes-tailBytes, 0)
	result.HeadChars = headChars
	result.TailChars = tailChars
	result.ElidedChars = max(result.OriginalChars-headChars-tailChars, 0)
	return result
}

func TruncateHeadTailChars(text string, limit int) Result {
	result := fullResult(text)
	if limit <= 0 || result.OriginalChars <= limit {
		return result
	}
	if limit <= len(marker)+1 {
		return prefixCharsFallback(text, limit, result)
	}

	runes := []rune(text)
	contentBudget := limit - len(marker)
	headBudget, tailBudget := splitBudget(contentBudget)
	if headBudget <= 0 || tailBudget <= 0 {
		return prefixCharsFallback(text, limit, result)
	}

	head := string(runes[:headBudget])
	tail := string(runes[len(runes)-tailBudget:])
	result.Truncated = true
	result.Text = head + marker + tail
	result.ReturnedBytes = len(result.Text)
	result.ReturnedChars = utf8.RuneCountInString(result.Text)
	result.HeadBytes = len(head)
	result.TailBytes = len(tail)
	result.ElidedBytes = max(result.OriginalBytes-result.HeadBytes-result.TailBytes, 0)
	result.HeadChars = headBudget
	result.TailChars = tailBudget
	result.ElidedChars = max(result.OriginalChars-headBudget-tailBudget, 0)
	return result
}

func fullResult(text string) Result {
	return Result{
		Text:          text,
		OriginalBytes: len(text),
		ReturnedBytes: len(text),
		OriginalChars: utf8.RuneCountInString(text),
		ReturnedChars: utf8.RuneCountInString(text),
		HeadBytes:     len(text),
		HeadChars:     utf8.RuneCountInString(text),
	}
}

func prefixBytesFallback(text string, limit int, result Result) Result {
	truncated, bytes, chars := prefixWithinBytes(text, limit)
	result.Truncated = true
	result.Text = truncated
	result.ReturnedBytes = bytes
	result.ReturnedChars = chars
	result.HeadBytes = bytes
	result.TailBytes = 0
	result.ElidedBytes = max(result.OriginalBytes-bytes, 0)
	result.HeadChars = chars
	result.TailChars = 0
	result.ElidedChars = max(result.OriginalChars-chars, 0)
	return result
}

func prefixCharsFallback(text string, limit int, result Result) Result {
	runes := []rune(text)
	if limit > len(runes) {
		limit = len(runes)
	}
	truncated := string(runes[:limit])
	result.Truncated = true
	result.Text = truncated
	result.ReturnedBytes = len(truncated)
	result.ReturnedChars = utf8.RuneCountInString(truncated)
	result.HeadBytes = len(truncated)
	result.TailBytes = 0
	result.ElidedBytes = max(result.OriginalBytes-len(truncated), 0)
	result.HeadChars = result.ReturnedChars
	result.TailChars = 0
	result.ElidedChars = max(result.OriginalChars-result.ReturnedChars, 0)
	return result
}

func splitBudget(contentBudget int) (int, int) {
	headBudget := contentBudget * 4 / 10
	tailBudget := contentBudget - headBudget
	if contentBudget >= 2 {
		if headBudget == 0 {
			headBudget = 1
			tailBudget--
		}
		if tailBudget == 0 {
			tailBudget = 1
			headBudget--
		}
	}
	return headBudget, tailBudget
}

func prefixWithinBytes(text string, limit int) (string, int, int) {
	if limit <= 0 || text == "" {
		return "", 0, 0
	}
	end := 0
	for idx := range text {
		if idx > limit {
			break
		}
		end = idx
	}
	if end == 0 && len(text) <= limit {
		return text, len(text), utf8.RuneCountInString(text)
	}
	truncated := text[:end]
	return truncated, len(truncated), utf8.RuneCountInString(truncated)
}

func suffixWithinBytes(text string, limit int) (string, int, int) {
	if limit <= 0 || text == "" {
		return "", 0, 0
	}
	start := len(text)
	for start > 0 {
		_, size := utf8.DecodeLastRuneInString(text[:start])
		next := start - size
		if len(text[next:]) > limit {
			break
		}
		start = next
	}
	truncated := text[start:]
	return truncated, len(truncated), utf8.RuneCountInString(truncated)
}

func max(left, right int) int {
	if left > right {
		return left
	}
	return right
}
