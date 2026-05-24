package personal

import (
	"context"
	"log/slog"
	"regexp"
	"strings"
)

// bareMentionRE matches @ followed by 1–4 word groups of letters/digits/'_'.
// Vietnamese diacritics are letters under \p{L}. Word boundary before @ is
// enforced manually below (start-of-text or whitespace).
var bareMentionRE = regexp.MustCompile(`@([\p{L}\p{N}_]+(?:[ ][\p{L}\p{N}_]+){0,3})`)

// reservedBareTokens skip auto-wrap (already handled by parser or sentinel-like).
var reservedBareTokens = map[string]struct{}{
	"all":      {},
	"All":      {},
	"ALL":      {},
	"everyone": {},
}

// wrapBareMentions rewrites bare "@Name" or "@First Last" tokens into "@[Name]"
// markers when Name resolves to exactly one known group member (NFC + casefold
// match). Greedy longest-first: tries the full captured run, then shrinks one
// trailing word at a time until a member match is found. No match → leaves the
// bare text alone (the LLM may have meant a non-mention "@" usage).
//
// Skips:
//   - tokens already inside @[...] markers (parser handles those)
//   - reserved tokens (@all/@All/@everyone)
//   - matches whose preceding char isn't start-of-text or whitespace (avoids
//     wrapping suffixes inside email addresses or paths)
func (c *Channel) wrapBareMentions(ctx context.Context, threadID, text string) string {
	if !strings.Contains(text, "@") || c.memberCache == nil {
		return text
	}
	matches := bareMentionRE.FindAllStringSubmatchIndex(text, -1)
	if len(matches) == 0 {
		return text
	}

	var out strings.Builder
	out.Grow(len(text) + 32)
	cursor := 0
	for _, m := range matches {
		start, end := m[0], m[1]
		capStart, capEnd := m[2], m[3]
		captured := text[capStart:capEnd]

		// Word boundary check: char before `@` must be SOT or whitespace.
		if start > 0 {
			prev := rune(text[start-1])
			if !isWordBoundary(prev) {
				continue
			}
		}
		// Skip if already inside a marker — preceded by `[` or followed by `]`.
		// markerRE in parser.go owns @[...]; this regex only fires on bare @.
		if start > 0 && text[start-1] == '[' {
			continue
		}
		// Skip reserved.
		if _, reserved := reservedBareTokens[captured]; reserved {
			continue
		}
		// Try longest → shortest. Resolved name is wrapped.
		resolvedName, resolvedEnd, ok := c.resolveLongestPrefix(ctx, threadID, text, capStart, capEnd)
		if !ok {
			continue
		}
		// Emit text up to and including the @, then [<resolvedName>], then continue.
		out.WriteString(text[cursor:start])
		out.WriteString("@[")
		out.WriteString(resolvedName)
		out.WriteString("]")
		cursor = resolvedEnd
		_ = end // unused — resolvedEnd is the actual consumed boundary
	}
	out.WriteString(text[cursor:])
	return out.String()
}

// resolveLongestPrefix tries the captured Name, then drops the trailing word
// and retries, until a unique cache match is found or we exhaust to 1 word.
// Returns (displayName, end-offset-in-text, ok).
func (c *Channel) resolveLongestPrefix(ctx context.Context, threadID, text string, capStart, capEnd int) (string, int, bool) {
	captured := text[capStart:capEnd]
	words := strings.Split(captured, " ")
	for n := len(words); n >= 1; n-- {
		candidate := strings.Join(words[:n], " ")
		if _, dn, ok := c.LookupGroupMemberByName(ctx, threadID, candidate); ok {
			slog.Info("zalo_personal.mention.auto_wrapped",
				"thread_id", threadID,
				"bare", candidate,
				"resolved_to", dn)
			// end offset = capStart + byte-length of candidate
			return dn, capStart + len(candidate), true
		}
	}
	return "", 0, false
}

func isWordBoundary(r rune) bool {
	return r == ' ' || r == '\t' || r == '\n' || r == '\r' || r == '(' || r == ',' || r == '.' || r == '!' || r == '?' || r == ':' || r == ';'
}
