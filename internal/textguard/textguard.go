// Package textguard classifies English-dominant first-person chain-of-thought
// text that reasoning models leak into the content channel, so outbound gates
// can strip or suppress it deterministically (no prompt reliance).
//
// Scope is intentionally narrow: only unambiguous first-person reasoning /
// inability / planning phrases trigger suppression. Legit English content
// (product/SKU stock lists, news headlines, status alerts) must pass through.
package textguard

import (
	"regexp"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// minLetters keeps tiny fragments (emoji, numbers, "OK") out of classification.
const minLetters = 4

var (
	urlPattern            = regexp.MustCompile(`(?i)https?://\S+|\bwww\.\S+`)
	paragraphSplitPattern = regexp.MustCompile(`\r?\n[ \t]*(?:\r?\n)+`)
	// Unambiguous first-person reasoning-about-the-task / inability / planning
	// phrases. Deliberately excludes ambiguous openers ("based on the",
	// "looking at", "i have", "it seems", bare "i need", bare "i'll") that
	// appear in legit English reports.
	cotStopwordPattern = regexp.MustCompile(`(?i)(?:i don't have access|i don't have|i do not have|i can't|i cannot|i'm unable|i am unable|i was unable|let me |i should |since i |i'll use|i'll mention|i'll just|i'll go with|as an ai|i need to figure|i need to determine|i don't have the ability)`)
	// Internal technical vocabulary that NEVER belongs in a user-facing message,
	// regardless of language. A leading paragraph containing one of these is the
	// agent narrating its own plumbing (observed: a VN reminder preceded by a
	// "...không tạo retry crons..." reasoning preamble that the English-only CoT
	// check let through). Low false-positive: no coaching/reminder text says these.
	internalMarkerPattern = regexp.MustCompile(`(?i)\b(?:retry cron|cron retry|escalation cron|target.?history|inject.?target|no_reply)`)
)

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

// nfc normalizes to NFC so decomposed (NFD) Vietnamese — base letter + combining
// diacritic — collapses to a single precomposed rune and counts as Vietnamese.
func nfc(s string) string {
	return norm.NFC.String(s)
}

func splitParagraphs(s string) []string {
	return paragraphSplitPattern.Split(s, -1)
}

// IsEnglishDominant reports whether text reads as English prose: mostly Latin
// letters with near-zero Vietnamese diacritic density. URLs, numbers, and
// emoji do not count toward either language. Input is NFC-normalized first so
// decomposed Vietnamese is recognized.
func IsEnglishDominant(text string) bool {
	stripped := urlPattern.ReplaceAllString(nfc(text), " ")
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
// either English-dominant first-person CoT, OR (any language) a paragraph that
// names internal plumbing (retry cron, target-history, NO_REPLY, …).
func IsInternalReasoning(paragraph string) bool {
	if internalMarkerPattern.MatchString(paragraph) {
		return true
	}
	if !IsEnglishDominant(paragraph) {
		return false
	}
	return cotStopwordPattern.MatchString(normalizeApostrophes(nfc(paragraph)))
}

// StripLeadingInternal removes internal-reasoning paragraphs from the top of
// content until the first non-internal paragraph. Bounded: stops at a code
// fence and is capped at half the message. The first leading CoT paragraph is
// always strippable when a real remainder follows (a single short CoT preamble
// can legitimately exceed half a two-paragraph message), but a single paragraph
// that dominates the whole message (>75%) is never stripped.
func StripLeadingInternal(content string) string {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return content
	}
	paras := splitParagraphs(trimmed)
	if len(paras) < 2 {
		return content
	}
	total := len([]rune(trimmed))
	budget := total / 2
	strippedRunes := 0
	idx := 0
	for idx < len(paras) {
		p := paras[idx]
		if strings.Contains(p, "```") || !IsInternalReasoning(p) {
			break
		}
		pRunes := len([]rune(p))
		if idx == 0 {
			// Allow the first leading CoT paragraph even past the half cap,
			// but not when it alone swallows the message (leak preamble, not body).
			if pRunes*4 > total*3 {
				break
			}
		} else if strippedRunes+pRunes > budget {
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

// IsMetaFailure reports whether the WHOLE message is a pure English-dominant
// first-person chain-of-thought leak (e.g. "I don't have access... Let me...")
// rather than deliverable content.
//
// It does NOT match on a leading cross-mark or generic failure prefixes
// ("error"/"sorry"/"failed"): those produce false positives on legit English
// status alerts and product lists. Only the combination English-dominant +
// narrowed first-person CoT stopword suppresses a message.
func IsMetaFailure(content string) bool {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return false
	}
	if !IsEnglishDominant(trimmed) {
		return false
	}
	return IsInternalReasoning(trimmed)
}
