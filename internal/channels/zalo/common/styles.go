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
// Lists pass through as literal text (`• item` / `1. item`) — Zalo mobile
// dumps lst_1/lst_2 spans as raw `<list>`/`<number>` XML in-band.
func RenderStyles(text string) (string, []Style) {
	if text == "" {
		return "", nil
	}
	text = renderMarkdownTables(text)

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
	text = neutralizeGluedUnderscores(text)
	text = collapseBlankAfterBoldHeader(text)
	text = indentUnderBoldHeader(text)

	out, styles := strings.Builder{}, []Style{}
	scan(text, &out, &styles)

	res := out.String()
	res = restoreDunders(res, dunders)
	res = restorePlaceholders(res, urls, inlineCodes, codeBlocks)
	res = restoreGluedUnderscores(res)
	res = reExcessiveNewlines.ReplaceAllString(res, "\n\n")
	res = strings.TrimSpace(res)
	res = reBullet.ReplaceAllString(res, "${1}• ")

	if len(styles) == 0 {
		return res, nil
	}
	return res, styles
}

// collapseBlankAfterBoldHeader drops blank lines between a bold-only header
// (e.g. `**Đánh giá:**`) and its following content. LLMs inconsistently
// insert blank padding after such headers; Zalo renders the blank as visible
// extra vertical space. Two adjacent headers keep their blank as section
// separator.
func collapseBlankAfterBoldHeader(text string) string {
	lines := strings.Split(text, "\n")
	if len(lines) < 3 {
		return text
	}
	out := make([]string, 0, len(lines))
	for i := 0; i < len(lines); i++ {
		out = append(out, lines[i])
		if !isBoldOnlyLine(lines[i]) {
			continue
		}
		j := i + 1
		for j < len(lines) && strings.TrimSpace(lines[j]) == "" {
			j++
		}
		if j == i+1 || j >= len(lines) || isBoldOnlyLine(lines[j]) {
			continue
		}
		i = j - 1
	}
	return strings.Join(out, "\n")
}

func isBoldOnlyLine(s string) bool {
	s = strings.TrimSpace(s)
	if len(s) < 5 || !strings.HasPrefix(s, "**") || !strings.HasSuffix(s, "**") {
		return false
	}
	inner := s[2 : len(s)-2]
	return inner != "" && !strings.Contains(inner, "**")
}

var (
	reOrderedPrefix = regexp.MustCompile(`^\s*\d+\.\s+`)
	reBulletPrefix  = regexp.MustCompile(`^\s*[-*+•]\s+`)
)

// indentUnderBoldHeader nests content under each bold-only header by 2 spaces.
// Bullets that immediately follow a numbered item get 4 spaces (sub-bullet).
// Section ends on the next bold-only header, OR on a blank line followed by
// non-list non-header prose (treated as closing remarks).
func indentUnderBoldHeader(text string) string {
	lines := strings.Split(text, "\n")
	out := make([]string, 0, len(lines))
	inSection := false
	prevOrdered := false
	for i, line := range lines {
		if isBoldOnlyLine(line) {
			inSection = true
			prevOrdered = false
			out = append(out, line)
			continue
		}
		if strings.TrimSpace(line) == "" {
			if inSection {
				if next := nextNonBlankIndex(lines, i+1); next >= 0 &&
					!isBoldOnlyLine(lines[next]) && !isListLine(lines[next]) {
					inSection = false
					prevOrdered = false
				}
			}
			out = append(out, line)
			continue
		}
		if !inSection {
			out = append(out, line)
			continue
		}
		indent := "  "
		isBullet := reBulletPrefix.MatchString(line)
		if prevOrdered && isBullet {
			indent = "    "
		}
		out = append(out, indent+line)
		if reOrderedPrefix.MatchString(line) {
			prevOrdered = true
		} else if !isBullet {
			prevOrdered = false
		}
	}
	return strings.Join(out, "\n")
}

func nextNonBlankIndex(lines []string, start int) int {
	for i := start; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) != "" {
			return i
		}
	}
	return -1
}

func isListLine(s string) bool {
	return reOrderedPrefix.MatchString(s) || reBulletPrefix.MatchString(s)
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

// neutralizeGluedUnderscores swaps `_` for a sentinel rune (\x02) when the
// underscore is glued to a letter/digit on BOTH sides — i.e. inside an
// identifier or filename like `BaoCao_DonHang_20260520.xlsx`. Italic regex
// then can't see these as markup. Restored 1:1 after scan. Sentinel is a
// 1-UTF-16-unit ASCII control char, so emitted style positions stay valid.
const gluedUnderscoreSentinel = '\x02'

func neutralizeGluedUnderscores(text string) string {
	if !strings.ContainsRune(text, '_') {
		return text
	}
	runes := []rune(text)
	for i, r := range runes {
		if r != '_' {
			continue
		}
		if i == 0 || i+1 >= len(runes) {
			continue
		}
		if isWordRune(runes[i-1]) && isWordRune(runes[i+1]) {
			runes[i] = gluedUnderscoreSentinel
		}
	}
	return string(runes)
}

func restoreGluedUnderscores(text string) string {
	if !strings.ContainsRune(text, gluedUnderscoreSentinel) {
		return text
	}
	return strings.ReplaceAll(text, string(gluedUnderscoreSentinel), "_")
}

func isWordRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r)
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
