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
		// Lists pass through as literal text; `- ` / `* ` / `+ ` prefixes
		// rewritten to `• ` for nicer rendering (UTF-16 width unchanged).
		// Ordered keeps the numeric prefix as-is.
		{"unordered_list", "- foo\n- bar", "• foo\n• bar", nil},
		{"ordered_list", "1. foo\n2. bar", "1. foo\n2. bar", nil},
		{"ordered_list_three_items", "1. a\n2. b\n3. c", "1. a\n2. b\n3. c", nil},
		{"ordered_list_single_isolated", "before\n1. foo\nafter", "before\n1. foo\nafter", nil},
		{"unordered_list_single_isolated", "before\n- foo\nafter", "before\n• foo\nafter", nil},
		{"nested_numbered_with_unicode_bullets", "1. Header\n• sub\n2. Header2", "1. Header\n• sub\n2. Header2", nil},
		{"mixed_ordered_unordered_adjacent", "1. a\n- b", "1. a\n• b", nil},
		{"ordered_with_blank_line_break", "1. a\n\n2. b", "1. a\n\n2. b", nil},
		{"unordered_two_blocks_separated", "- a\n- b\n\n- c\n- d", "• a\n• b\n\n• c\n• d", nil},
		{"url_escapes_italic", "see https://a.com/foo_bar_baz for x", "see https://a.com/foo_bar_baz for x", nil},
		{"email_escapes_italic", "ping alice_doe@example.com", "ping alice_doe@example.com", nil},
		{"inline_code_kept_plain", "use `foo()` here", "use foo() here", nil},

		// Fragment-bold: ** glued to a word char on either side renders as
		// broken partial-word bold on Zalo. Strip markers, emit no style.
		{"fragment_bold_both_sides_ascii", "Bo**ld**er", "Bolder", nil},
		{"fragment_bold_left_glued", "pre**bold**", "prebold", nil},
		{"fragment_bold_right_glued", "**bold**post", "boldpost", nil},
		{
			"fragment_bold_vietnamese",
			"Dễ ove**rtrain nếu khôn**g kiểm soát",
			"Dễ overtrain nếu không kiểm soát",
			nil,
		},
		{
			"clean_bold_with_punct_preserved",
			"this is **bold**, ok?",
			"this is bold, ok?",
			[]Style{{8, 4, StyleBold}},
		},
		{"triple_bold_glued_left_unchanged", "pre***x***", "prex", []Style{{3, 1, StyleBold}, {3, 1, StyleItalic}}},

		// Bold-only header collapses the blank before its content AND
		// indents that content 2 spaces. Bullets that follow a numbered item
		// get 4 spaces (sub-bullet inference).
		{
			"bold_header_blank_collapsed_before_ordered",
			"**Đánh giá:**\n\n1. Doanh thu giảm",
			"Đánh giá:\n  1. Doanh thu giảm",
			[]Style{{0, 9, StyleBold}},
		},
		{
			"bold_header_blank_collapsed_before_bullet",
			"**Dữ liệu:**\n\n- Tổng đơn: 17",
			"Dữ liệu:\n  • Tổng đơn: 17",
			[]Style{{0, 8, StyleBold}},
		},
		{
			"bold_header_blank_preserved_between_two_headers",
			"**A:**\n\n**B:**\n- x",
			"A:\n\nB:\n  • x",
			[]Style{{0, 2, StyleBold}, {4, 2, StyleBold}},
		},
		{
			"bold_header_sub_bullet_under_ordered",
			"**Đánh giá:**\n1. Doanh thu giảm mạnh\n- Giảm 52% so hôm trước\n\n2. AOV ở mức khá\n- 3.5tr/don",
			"Đánh giá:\n  1. Doanh thu giảm mạnh\n    • Giảm 52% so hôm trước\n\n  2. AOV ở mức khá\n    • 3.5tr/don",
			[]Style{{0, 9, StyleBold}},
		},
		{
			"bold_header_section_ends_at_trailing_prose",
			"**Đề xuất:**\n- Tìm hiểu nguyên nhân\n\nAnh có file chi tiết không?",
			"Đề xuất:\n  • Tìm hiểu nguyên nhân\n\nAnh có file chi tiết không?",
			[]Style{{0, 8, StyleBold}},
		},
		{
			"bold_header_with_trailing_emoji",
			"**Điểm mạnh** 💪\n\n- AOV ổn định\n- Sales star",
			"Điểm mạnh 💪\n  • AOV ổn định\n  • Sales star",
			[]Style{{0, 9, StyleBold}},
		},
		{
			"bold_header_with_trailing_punct",
			"**Cảnh báo**!\n\n- 98.7% chưa thanh toán",
			"Cảnh báo!\n  • 98.7% chưa thanh toán",
			[]Style{{0, 8, StyleBold}},
		},
		{
			"bold_prefix_with_prose_is_not_header",
			"**Note** this is important content here\n- not a list item under a header",
			"Note this is important content here\n• not a list item under a header",
			[]Style{{0, 4, StyleBold}},
		},

		// Filename / identifier underscores: glued on both sides → not italic.
		{
			"filename_xlsx_no_italic",
			"file BaoCao_DonHang_20260520.xlsx",
			"file BaoCao_DonHang_20260520.xlsx",
			nil,
		},
		{
			"identifier_three_segments_no_italic",
			"use the_quick_brown_fox variable",
			"use the_quick_brown_fox variable",
			nil,
		},
		{
			"italic_underscore_with_spaces_still_works",
			"this is _italic_ text",
			"this is italic text",
			[]Style{{8, 6, StyleItalic}},
		},
		{
			"italic_underscore_around_word_with_inner_underscore",
			"this is _user_id_ field",
			"this is user_id field",
			[]Style{{8, 7, StyleItalic}},
		},

		// Tables convert to bulleted labeled blocks (same as StripMarkdown).
		{
			"table_two_col_native",
			"| Key | Value |\n|---|---|\n| Total | 8,370,300đ |\n| Bank | BIDV |",
			"• Total\n  Value: 8,370,300đ\n• Bank\n  Value: BIDV",
			nil,
		},
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
