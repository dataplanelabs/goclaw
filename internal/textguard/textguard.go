// Package textguard classifies English-dominant chain-of-thought / meta /
// failure text that models leak into the content channel, so outbound gates
// can strip or suppress it deterministically (no prompt reliance).
package textguard

import (
	"regexp"
	"strings"
	"unicode"
)

// minLetters keeps tiny fragments (emoji, numbers, "OK") out of classification.
const minLetters = 4

var (
	urlPattern            = regexp.MustCompile(`(?i)https?://\S+|\bwww\.\S+`)
	paragraphSplitPattern = regexp.MustCompile(`\r?\n[ \t]*(?:\r?\n)+`)
	// First-person CoT/meta openers observed leaking from reasoning models.
	cotStopwordPattern = regexp.MustCompile(`(?i)\b(?:i don't|i should|i need|i'll|i can't|since i|let me|looking at|based on the|the cron|i have|i am unable|it seems|i cannot)\b`)
)

var failurePrefixes = []string{
	"error", "failed", "i was unable", "i am unable", "i cannot", "i can't", "sorry,",
}

var vietnameseRunes = func() map[rune]bool {
	const chars = "ăâđêôơưáàảãạắằẳẵặấầẩẫậéèẻẽẹếềểễệíìỉĩịóòỏõọốồổỗộớờởỡợúùủũụứừửữựýỳỷỹỵ"
	m := make(map[rune]bool, len(chars))
	for _, r := range chars {
		m[r] = true
	}
	return m
}()

func isVietnameseRune(r rune) bool {
	return vietnameseRunes[unicode.ToLower(r)]
}

func normalizeApostrophes(s string) string {
	return strings.NewReplacer("’", "'", "‘", "'").Replace(s)
}

func splitParagraphs(s string) []string {
	return paragraphSplitPattern.Split(s, -1)
}

// IsEnglishDominant reports whether text reads as English prose: mostly Latin
// letters with near-zero Vietnamese diacritic density. URLs, numbers, and
// emoji do not count toward either language.
func IsEnglishDominant(text string) bool {
	stripped := urlPattern.ReplaceAllString(text, " ")
	var letters, latin, vietnamese int
	for _, r := range stripped {
		if !unicode.IsLetter(r) {
			continue
		}
		letters++
		if r < 128 {
			latin++
		}
		if isVietnameseRune(r) {
			vietnamese++
		}
	}
	if letters < minLetters {
		return false
	}
	// Vietnamese prose carries diacritics on well over 5% of letters.
	if vietnamese*20 > letters {
		return false
	}
	return latin*4 >= letters*3
}

// IsInternalReasoning reports whether a paragraph is leaked internal reasoning:
// English-dominant AND matching first-person CoT/meta stopwords.
func IsInternalReasoning(paragraph string) bool {
	if !IsEnglishDominant(paragraph) {
		return false
	}
	return cotStopwordPattern.MatchString(normalizeApostrophes(paragraph))
}

// StripLeadingInternal removes internal-reasoning paragraphs from the top of
// content until the first non-internal paragraph. Bounded: stops at a code
// fence and never strips more than half of the message.
func StripLeadingInternal(content string) string {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return content
	}
	paras := splitParagraphs(trimmed)
	if len(paras) < 2 {
		return content
	}
	budget := len([]rune(trimmed)) / 2
	strippedRunes := 0
	idx := 0
	for idx < len(paras) {
		p := paras[idx]
		if strings.Contains(p, "```") || !IsInternalReasoning(p) {
			break
		}
		pRunes := len([]rune(p))
		if strippedRunes+pRunes > budget {
			break
		}
		strippedRunes += pRunes
		idx++
	}
	if idx == 0 {
		return content
	}
	return strings.TrimSpace(strings.Join(paras[idx:], "\n\n"))
}

// IsMetaFailure reports whether the WHOLE message is English-dominant
// meta/failure output (error reports, "I was unable...", residual CoT)
// rather than deliverable content.
func IsMetaFailure(content string) bool {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return false
	}
	if !IsEnglishDominant(trimmed) {
		return false
	}
	if strings.HasPrefix(trimmed, "❌") {
		return true
	}
	norm := strings.ToLower(normalizeApostrophes(trimmed))
	norm = strings.TrimLeft(norm, "❌⚠️🚫*_~#>- \t")
	for _, p := range failurePrefixes {
		if strings.HasPrefix(norm, p) {
			return true
		}
	}
	return IsInternalReasoning(splitParagraphs(trimmed)[0])
}
