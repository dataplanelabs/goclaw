package common

import (
	"reflect"
	"testing"
)

func TestRenderStyles(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name       string
		in         string
		wantText   string
		wantStyles []Style
	}{
		{"empty", "", "", nil},
		{"plain", "hello world", "hello world", nil},
		{"bold_double_star", "hi **world**!", "hi world!", []Style{{3, 5, StyleBold}}},
		{"italic_single_star", "hi *world*!", "hi world!", []Style{{3, 5, StyleItalic}}},
		{"italic_underscore", "hi _world_!", "hi world!", []Style{{3, 5, StyleItalic}}},
		{"strikethrough", "~~old~~", "old", []Style{{0, 3, StyleStrikethrough}}},
		{"html_underline", "<u>x</u>", "x", []Style{{0, 1, StyleUnderline}}},
		{"bold_italic_triple", "***x***", "x", []Style{{0, 1, StyleBold}, {0, 1, StyleItalic}}},
		{"dunder_preserved", "call __init__ method", "call __init__ method", nil},
		{"vietnamese_bold", "**Đức**", "Đức", []Style{{0, 3, StyleBold}}},
		{"emoji_bold", "**🎉**", "🎉", []Style{{0, 2, StyleBold}}},
		{"link_to_text", "[click](https://a.com)", "click (https://a.com)", nil},
		{"image_stripped", "![alt](url)", "", nil},
		{"header_plain", "## h2", "h2", nil},
		{"blockquote_stripped", "> quoted", "quoted", nil},
		{"horizontal_rule_dropped", "before\n---\nafter", "before\n\nafter", nil},
		{"adjacent_bold", "**a** **b**", "a b", []Style{{0, 1, StyleBold}, {2, 1, StyleBold}}},
		{"unbalanced_preserved", "no closing **here", "no closing **here", nil},
		{"unordered_list", "- foo\n- bar", "foo\nbar", []Style{{0, 3, StyleListUnordered}, {4, 3, StyleListUnordered}}},
		{"ordered_list", "1. foo\n2. bar", "foo\nbar", []Style{{0, 3, StyleListOrdered}, {4, 3, StyleListOrdered}}},
		{"url_escapes_italic", "see https://a.com/foo_bar_baz for x", "see https://a.com/foo_bar_baz for x", nil},
		{"email_escapes_italic", "ping alice_doe@example.com", "ping alice_doe@example.com", nil},
		{"inline_code_kept_plain", "use `foo()` here", "use foo() here", nil},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			gotText, gotStyles := RenderStyles(c.in)
			if gotText != c.wantText {
				t.Errorf("text: got %q, want %q", gotText, c.wantText)
			}
			if !reflect.DeepEqual(gotStyles, c.wantStyles) {
				t.Errorf("styles: got %#v, want %#v", gotStyles, c.wantStyles)
			}
		})
	}
}

func TestRenderStyles_EmptyReturnsNilNotSlice(t *testing.T) {
	_, styles := RenderStyles("plain text")
	if styles != nil {
		t.Errorf("plain input must return nil styles slice, got %#v", styles)
	}
}

func TestRenderStyles_FencedCodeBlock_StripsBackticks(t *testing.T) {
	got, styles := RenderStyles("```go\nfmt.Println\n```")
	if got == "" {
		t.Error("expected non-empty output for fenced code")
	}
	if styles != nil {
		t.Errorf("code block emits no styles, got %#v", styles)
	}
}
