package common

import (
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

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

// Style positions are UTF-16 code units over the OUTPUT text — matches the
// Zalo client / zca-js wire shape.
type Style struct {
	Start int    `json:"start"`
	Len   int    `json:"len"`
	St    string `json:"st"`
}

// RenderStyles strips markdown markup and emits Zalo native Style spans.
// Lists are NOT styled: Zalo mobile dumps lst_1/lst_2 spans as raw
// `<list>`/`<number>` XML in-band, so list lines pass through as literal text.
func RenderStyles(text string) (string, []Style) {
	if text == "" {
		return "", nil
	}
	codeBlocks, text := extractCodeBlocks(text)
	inlineCodes, text := extractInlineCodes(text)
	urls, text := extractURLs(text)
	dunders, text := extractDunders(text)

	text = reImage.ReplaceAllString(text, "")
	text = reLink.ReplaceAllString(text, "$1 ($2)")
	text = reHeader.ReplaceAllString(text, "$1")
	text = reBlockquote.ReplaceAllString(text, "$1")
	text = reHorizontalRule.ReplaceAllString(text, "")

	text = stripFragmentBoldStar(text)

	out, styles := strings.Builder{}, []Style{}
	scan(text, &out, &styles)

	res := out.String()
	res = restoreDunders(res, dunders)
	res = restorePlaceholders(res, urls, inlineCodes, codeBlocks)
	res = reExcessiveNewlines.ReplaceAllString(res, "\n\n")
	res = strings.TrimSpace(res)

	if len(styles) == 0 {
		return res, nil
	}
	return res, styles
}

var (
	reURL   = regexp.MustCompile(`https?://[^\s<>\)\]]+`)
	reEmail = regexp.MustCompile(`[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}`)

	reItalicStar    = regexp.MustCompile(`\*([^*\s][^*]*?)\*`)
	reItalicUnder   = regexp.MustCompile(`_([^_\s][^_]*?)_`)
	reHtmlUnderline = regexp.MustCompile(`(?is)<u>(.+?)</u>`)
)

// scan walks `text` outer-first; triple-emphasis emits b+i at the same span.
func scan(text string, out *strings.Builder, styles *[]Style) {
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

// stripFragmentBoldStar drops `**…**` glued to a letter/digit on either side
// (e.g. `ove**rtrain**g`) — emitting a Style over the fragment renders as
// broken partial-word bold. Triple-emphasis `***x***` skipped via the `*`
// outer guard so scan still emits b+i for it.
func stripFragmentBoldStar(text string) string {
	matches := reBoldStar.FindAllStringSubmatchIndex(text, -1)
	if len(matches) == 0 {
		return text
	}
	var out strings.Builder
	cursor := 0
	for _, m := range matches {
		matchStart, matchEnd := m[0], m[1]
		innerStart, innerEnd := m[2], m[3]
		if matchStart > 0 && text[matchStart-1] == '*' {
			continue
		}
		if matchEnd < len(text) && text[matchEnd] == '*' {
			continue
		}
		if !boundaryGluedToWord(text, matchStart, true) && !boundaryGluedToWord(text, matchEnd, false) {
			continue
		}
		out.WriteString(text[cursor:matchStart])
		out.WriteString(text[innerStart:innerEnd])
		cursor = matchEnd
	}
	if cursor == 0 {
		return text
	}
	out.WriteString(text[cursor:])
	return out.String()
}

func boundaryGluedToWord(text string, idx int, before bool) bool {
	var r rune
	if before {
		if idx <= 0 {
			return false
		}
		r, _ = utf8.DecodeLastRuneInString(text[:idx])
	} else {
		if idx >= len(text) {
			return false
		}
		r, _ = utf8.DecodeRuneInString(text[idx:])
	}
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_'
}
