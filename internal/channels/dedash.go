package channels

import "strings"

// dedashContent swaps the "fancy" dashes that read as AI-generated — em-dash
// (U+2014) and horizontal bar (U+2015) — for a plain hyphen. En-dash (U+2013) is
// left alone: it carries range semantics (2020–2024, Mon–Fri) that a hyphen loses.
func dedashContent(s string) string {
	if !strings.ContainsRune(s, '—') && !strings.ContainsRune(s, '―') {
		return s
	}
	s = strings.ReplaceAll(s, "—", "-")
	s = strings.ReplaceAll(s, "―", "-")
	return s
}
