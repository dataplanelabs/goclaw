package common

import (
	"regexp"
	"strings"

	pkgproto "github.com/nextlevelbuilder/goclaw/pkg/protocol"
)

const (
	StyleBold          = "b"
	StyleItalic        = "i"
	StyleUnderline     = "u"
	StyleStrikethrough = "s"
	StyleListUnordered = "lst_1"
	StyleListOrdered   = "lst_2"
)

// Style is one positional run of formatting over the final plain-text body.
// Positions are UTF-16 code units (matches Zalo client / zca-js wire shape).
type Style struct {
	Start int    `json:"start"`
	Len   int    `json:"len"`
	St    string `json:"st"`
}

// RenderStyles strips markdown markup and emits Zalo native Style spans.
// Positions are UTF-16 code units over the OUTPUT (stripped) text.
// Unmapped markdown (headers, blockquotes, code blocks, links, images) is
// stripped per existing StripMarkdown behavior but emits no style.
func RenderStyles(text string) (string, []Style) {
	if text == "" {
		return "", nil
	}
	// Stash code blocks and inline code FIRST so list / italic regex don't
	// touch their contents. Restore at end.
	codeBlocks, text := extractCodeBlocks(text)
	inlineCodes, text := extractInlineCodes(text)
	// URLs / emails likewise — protect from italic underscores.
	urls, text := extractURLs(text)
	// __identifier__ dunders: extract before bold/italic so they survive intact.
	dunders, text := extractDunders(text)

	// Strip non-mappable markup (no Style emitted): images, header markers,
	// blockquotes, horizontal rules, markdown links → "text (url)".
	text = reImage.ReplaceAllString(text, "")
	text = reLink.ReplaceAllString(text, "$1 ($2)")
	text = reHeader.ReplaceAllString(text, "$1")
	text = reBlockquote.ReplaceAllString(text, "$1")
	text = reHorizontalRule.ReplaceAllString(text, "")

	// Walk inline emphasis patterns in PRIORITY order — outer-first.
	out, styles := strings.Builder{}, []Style{}
	scan(text, &out, &styles)

	res := out.String()
	res = restoreDunders(res, dunders)
	res = restorePlaceholders(res, urls, inlineCodes, codeBlocks)
	res = reExcessiveNewlines.ReplaceAllString(res, "\n\n")
	res = strings.TrimSpace(res)

	// Handle list-prefix lines AFTER inline pass — strip `- ` / `1. ` and
	// emit lst_1 / lst_2 over the item text range.
	res, listStyles := emitListStyles(res)
	styles = append(styles, listStyles...)

	if len(styles) == 0 {
		return res, nil
	}
	return res, styles
}

// Regex constants for code blocks / images / links / headers / blockquotes /
// horizontal-rules / excessive-newlines / bold / strikethrough / identifier
// already live in markdown.go (this package) — reuse, don't redeclare.
var (
	reURL   = regexp.MustCompile(`https?://[^\s<>\)\]]+`)
	reEmail = regexp.MustCompile(`[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}`)

	reItalicStar    = regexp.MustCompile(`\*([^*\s][^*]*?)\*`)
	reItalicUnder   = regexp.MustCompile(`_([^_\s][^_]*?)_`)
	reHtmlUnderline = regexp.MustCompile(`(?is)<u>(.+?)</u>`)

	reListUnordered = regexp.MustCompile(`(?m)^(\s*)[-*+]\s+(.+)$`)
	reListOrdered   = regexp.MustCompile(`(?m)^(\s*)\d+\.\s+(.+)$`)
)

// scan walks `text` and emits matches for the inline emphasis patterns in
// outer-first order. Triple-star/under emit two styles (b+i) at the same span.
func scan(text string, out *strings.Builder, styles *[]Style) {
	// Process triple-emphasis first.
	text = applyPattern(text, reBoldItalicStar, out, styles, func(start, length int) {
		*styles = append(*styles, Style{Start: start, Len: length, St: StyleBold})
		*styles = append(*styles, Style{Start: start, Len: length, St: StyleItalic})
	})
	text = applyPattern(text, reBoldItalicUnder, out, styles, func(start, length int) {
		*styles = append(*styles, Style{Start: start, Len: length, St: StyleBold})
		*styles = append(*styles, Style{Start: start, Len: length, St: StyleItalic})
	})
	text = applyPattern(text, reBoldStar, out, styles, func(start, length int) {
		*styles = append(*styles, Style{Start: start, Len: length, St: StyleBold})
	})
	text = applyPatternFiltered(text, reBoldUnder, out, styles,
		func(inner string) bool { return !reIdentifier.MatchString(inner) },
		func(start, length int) {
			*styles = append(*styles, Style{Start: start, Len: length, St: StyleBold})
		})
	text = applyPattern(text, reStrikethrough, out, styles, func(start, length int) {
		*styles = append(*styles, Style{Start: start, Len: length, St: StyleStrikethrough})
	})
	text = applyPattern(text, reHtmlUnderline, out, styles, func(start, length int) {
		*styles = append(*styles, Style{Start: start, Len: length, St: StyleUnderline})
	})
	text = applyPattern(text, reItalicStar, out, styles, func(start, length int) {
		*styles = append(*styles, Style{Start: start, Len: length, St: StyleItalic})
	})
	text = applyPattern(text, reItalicUnder, out, styles, func(start, length int) {
		*styles = append(*styles, Style{Start: start, Len: length, St: StyleItalic})
	})
	out.WriteString(text)
}

// applyPattern walks `text`, replaces every match with its capture-group-1
// content, appends to `out`, and emits styles via the callback. Returns
// the remaining text (which is empty after a full sweep) so the calling
// pipeline can chain into the next pattern by re-feeding `out.String()`.
//
// Implementation: writes already-stripped prefix + emitted inner content to
// `out` and returns empty string. The next pattern call receives a FRESH
// builder seeded with the current `out` contents. This is simpler than
// threading state through multiple passes.
func applyPattern(text string, re *regexp.Regexp, out *strings.Builder, styles *[]Style, emit func(start, length int)) string {
	return applyPatternFiltered(text, re, out, styles, nil, emit)
}

func applyPatternFiltered(text string, re *regexp.Regexp, out *strings.Builder, styles *[]Style, keep func(inner string) bool, emit func(start, length int)) string {
	indexes := re.FindAllStringSubmatchIndex(text, -1)
	if len(indexes) == 0 {
		return text
	}
	var next strings.Builder
	cursor := 0
	for _, m := range indexes {
		matchStart, matchEnd := m[0], m[1]
		innerStart, innerEnd := m[2], m[3]
		inner := text[innerStart:innerEnd]
		if keep != nil && !keep(inner) {
			next.WriteString(text[cursor:matchEnd])
			cursor = matchEnd
			continue
		}
		next.WriteString(text[cursor:matchStart])
		styleStart := pkgproto.UTF16Len(out.String()) + pkgproto.UTF16Len(next.String())
		styleLen := pkgproto.UTF16Len(inner)
		if styleLen > 0 {
			emit(styleStart, styleLen)
		}
		next.WriteString(inner)
		cursor = matchEnd
	}
	next.WriteString(text[cursor:])
	return next.String()
}

// emitListStyles walks the (post-strip) text line by line, strips bullet/number
// prefixes, and emits Style{St:lst_1|lst_2} over each item's text range.
func emitListStyles(text string) (string, []Style) {
	if text == "" {
		return text, nil
	}
	lines := strings.Split(text, "\n")
	var out strings.Builder
	var styles []Style
	outOffset := 0
	for i, line := range lines {
		var stripped string
		var st string
		if m := reListUnordered.FindStringSubmatchIndex(line); m != nil {
			stripped = line[:m[2]] + line[m[4]:m[5]]
			st = StyleListUnordered
		} else if m := reListOrdered.FindStringSubmatchIndex(line); m != nil {
			stripped = line[:m[2]] + line[m[4]:m[5]]
			st = StyleListOrdered
		} else {
			stripped = line
		}
		if i > 0 {
			out.WriteString("\n")
			outOffset += 1 // "\n" is 1 UTF-16 unit
		}
		if st != "" {
			leadingLen := pkgproto.UTF16Len(stripped) - pkgproto.UTF16Len(strings.TrimLeft(stripped, " \t"))
			itemTextLen := pkgproto.UTF16Len(strings.TrimLeft(stripped, " \t"))
			if itemTextLen > 0 {
				styles = append(styles, Style{
					Start: outOffset + leadingLen,
					Len:   itemTextLen,
					St:    st,
				})
			}
		}
		out.WriteString(stripped)
		outOffset += pkgproto.UTF16Len(stripped)
	}
	return out.String(), styles
}

// extractCodeBlocks pulls fenced ``` blocks out into placeholders before any
// other regex sees them. Returns (placeholders, text-with-placeholders).
// Inner-content stripped of leading language hint.
func extractCodeBlocks(text string) ([]string, string) {
	var stash []string
	out := reFencedCode.ReplaceAllStringFunc(text, func(match string) string {
		body := reFencedCode.FindStringSubmatch(match)[1]
		stash = append(stash, body)
		return placeholder("CB", len(stash)-1)
	})
	return stash, out
}

func extractInlineCodes(text string) ([]string, string) {
	var stash []string
	out := reInlineCode.ReplaceAllStringFunc(text, func(match string) string {
		body := reInlineCode.FindStringSubmatch(match)[1]
		stash = append(stash, body)
		return placeholder("IC", len(stash)-1)
	})
	return stash, out
}

var reDunder = regexp.MustCompile(`__[A-Za-z][A-Za-z0-9_]*__`)

func extractDunders(text string) ([]string, string) {
	var stash []string
	out := reDunder.ReplaceAllStringFunc(text, func(m string) string {
		inner := m[2 : len(m)-2]
		if reIdentifier.MatchString(inner) {
			stash = append(stash, m)
			return placeholder("DD", len(stash)-1)
		}
		return m
	})
	return stash, out
}

func restoreDunders(text string, dunders []string) string {
	for i := len(dunders) - 1; i >= 0; i-- {
		text = strings.Replace(text, placeholder("DD", i), dunders[i], 1)
	}
	return text
}

func extractURLs(text string) ([]string, string) {
	var stash []string
	out := reURL.ReplaceAllStringFunc(text, func(m string) string {
		stash = append(stash, m)
		return placeholder("URL", len(stash)-1)
	})
	out = reEmail.ReplaceAllStringFunc(out, func(m string) string {
		stash = append(stash, m)
		return placeholder("URL", len(stash)-1)
	})
	return stash, out
}

func placeholder(kind string, idx int) string {
	var b strings.Builder
	b.WriteByte(0)
	b.WriteString(kind)
	for i := 0; i < idx; i++ {
		b.WriteByte('X')
	}
	b.WriteByte(0)
	return b.String()
}

func restorePlaceholders(text string, urls, inlineCodes, codeBlocks []string) string {
	for i := len(urls) - 1; i >= 0; i-- {
		text = strings.Replace(text, placeholder("URL", i), urls[i], 1)
	}
	for i := len(inlineCodes) - 1; i >= 0; i-- {
		text = strings.Replace(text, placeholder("IC", i), inlineCodes[i], 1)
	}
	for i := len(codeBlocks) - 1; i >= 0; i-- {
		text = strings.Replace(text, placeholder("CB", i), codeBlocks[i], 1)
	}
	return text
}
