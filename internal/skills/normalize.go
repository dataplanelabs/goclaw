package skills

import (
	"strings"
	"unicode"

	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
)

// diacriticsFolder strips Unicode combining marks (Mn category): "Tạo" → "Tao",
// "Nhiên" → "Nhien". Applied to both index and query so Vietnamese skills
// match romanized queries — fixes the trace 019e62ff failure where
// `skill_search("design poster annhien")` missed `design-annhien` because the
// description "Tạo... An Nhiên Safety..." indexed `nhiên` not `nhien`.
var diacriticsFolder = transform.Chain(
	norm.NFD,
	runes.Remove(runes.In(unicode.Mn)),
	norm.NFC,
)

// FoldDiacritics strips combining marks from s. Returns s unchanged on
// transform error (defensive; in practice norm.NFD never fails).
func FoldDiacritics(s string) string {
	if s == "" {
		return s
	}
	out, _, err := transform.String(diacriticsFolder, s)
	if err != nil {
		return s
	}
	return out
}

// normalizeForSearch lowercases + folds diacritics. The canonical pre-tokenize
// pass shared by index Build and query Search.
func normalizeForSearch(s string) string {
	return FoldDiacritics(strings.ToLower(s))
}
