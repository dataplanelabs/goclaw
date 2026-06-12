package agent

import (
	"strings"
	"testing"

	"golang.org/x/text/unicode/norm"
)

func TestStripLeadingInternalReasoning(t *testing.T) {
	vn := "Chào cả nhà! Bản tin sáng nay: thị trường chứng khoán tăng điểm nhẹ, giá vàng ổn định quanh mốc cũ. Chúc mọi người một ngày làm việc hiệu quả nhé!"
	cot := "I don't have access to the group chat history. Let me look at the UIDs."

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "cot then vietnamese stripped",
			input: cot + "\n\n" + vn,
			want:  vn,
		},
		{
			name:  "two cot paragraphs then vietnamese",
			input: cot + "\n\nLooking at the recent messages, I should summarize the news now.\n\n" + vn + "\n\n" + vn,
			want:  vn + "\n\n" + vn,
		},
		{
			name:  "vietnamese only no-op",
			input: vn,
			want:  vn,
		},
		{
			name:  "english-only reply no-op even with stopwords",
			input: "I have reviewed the document and it looks good.\n\nLet me know if you need anything else from my side.",
			want:  "I have reviewed the document and it looks good.\n\nLet me know if you need anything else from my side.",
		},
		{
			name:  "english headline without stopword never stripped",
			input: "Apple unveils a new foldable iPhone at its annual September keynote event.\n\n" + vn,
			want:  "Apple unveils a new foldable iPhone at its annual September keynote event.\n\n" + vn,
		},
		{
			name:  "leading cot stripped delivers vietnamese mention",
			input: "I don't have access to the history. Let me check.\n\n@Công Ninh ơi, em chờ thông tin cự ly nhé!",
			want:  "@Công Ninh ơi, em chờ thông tin cự ly nhé!",
		},
		{
			name:  "nfd-decomposed vietnamese after cot stripped and preserved",
			input: cot + "\n\n" + norm.NFD.String(vn),
			want:  norm.NFD.String(vn),
		},
		{
			name:  "never strips inside fenced code",
			input: "```python\n# I should keep this comment\nprint('xin chào')\n```\n\n" + vn,
			want:  "```python\n# I should keep this comment\nprint('xin chào')\n```\n\n" + vn,
		},
		{
			name:  "cot stripped but following code fence preserved",
			input: cot + "\n\n```\nI need this code block\n```\n\n" + vn,
			want:  "```\nI need this code block\n```\n\n" + vn,
		},
		{
			name:  "utf8 vietnamese preserved exactly",
			input: cot + "\n\nNgày mai trời nắng đẹp, nhiệt độ 25–30°C. Mọi người nhớ mang theo nước uống nhé! 🌞",
			want:  "Ngày mai trời nắng đẹp, nhiệt độ 25–30°C. Mọi người nhớ mang theo nước uống nhé! 🌞",
		},
		{
			name:  "never strips past half the message",
			input: strings.Repeat(cot+" ", 10) + "\n\nGiá vàng ổn định.",
			want:  strings.Repeat(cot+" ", 10) + "\n\nGiá vàng ổn định.",
		},
		{
			name:  "empty no-op",
			input: "",
			want:  "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stripLeadingInternalReasoning(tt.input)
			if strings.TrimSpace(got) != strings.TrimSpace(tt.want) {
				t.Errorf("stripLeadingInternalReasoning() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSanitizeAssistantContent_StripsLeadingCoT(t *testing.T) {
	vn := "Bản tin sáng: thị trường tăng điểm, giá vàng ổn định quanh mốc cũ nhé cả nhà."
	input := "I don't have access to the group chat history. Let me look at the UIDs.\n\n" + vn
	got := SanitizeAssistantContent(input)
	if got != vn {
		t.Errorf("SanitizeAssistantContent() = %q, want %q", got, vn)
	}
}

func TestSanitizeAssistantContent_EnglishReplyUntouched(t *testing.T) {
	input := "I have checked the calendar and your meeting is at 3pm.\n\nLet me know if you want me to reschedule it."
	got := SanitizeAssistantContent(input)
	if got != input {
		t.Errorf("SanitizeAssistantContent() = %q, want unchanged %q", got, input)
	}
}
