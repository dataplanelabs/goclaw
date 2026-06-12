package textguard

import (
	"strings"
	"testing"
)

func TestIsEnglishDominant(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{
			name:  "plain english prose",
			input: "I don't have access to the group chat history right now.",
			want:  true,
		},
		{
			name:  "vietnamese with diacritics",
			input: "Chào buổi sáng cả nhà, hôm nay thị trường tăng nhẹ.",
			want:  false,
		},
		{
			name:  "vietnamese with english urls",
			input: "Xem chi tiết tại https://example.com/very-long-english-path-name nhé cả nhà.",
			want:  false,
		},
		{
			name:  "english with urls",
			input: "Check the dashboard at https://thị-trường.example.com for details.",
			want:  true,
		},
		{
			name:  "emoji and numbers only",
			input: "🎉 2024 ❌ 100%",
			want:  false,
		},
		{
			name:  "too short",
			input: "OK.",
			want:  false,
		},
		{
			name:  "empty",
			input: "",
			want:  false,
		},
		{
			name:  "english cot quoting one vietnamese term",
			input: "Let me check the thị trường data before answering the question properly.",
			want:  true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsEnglishDominant(tt.input); got != tt.want {
				t.Errorf("IsEnglishDominant(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestIsInternalReasoning(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{
			name:  "first person cot",
			input: "I don't have access to the group chat history. Let me look at the UIDs.",
			want:  true,
		},
		{
			name:  "curly apostrophe cot",
			input: "I don’t have the data yet, so I’ll fetch it first.",
			want:  true,
		},
		{
			name:  "cron meta",
			input: "The cron job requires me to post a summary to the chat.",
			want:  true,
		},
		{
			name:  "english brand sentence no stopwords",
			input: "Shopee Super Sale starts this weekend with discounts up to 50%.",
			want:  false,
		},
		{
			name:  "vietnamese paragraph",
			input: "Hôm nay mình sẽ gửi bản tin thị trường cho cả nhà nhé.",
			want:  false,
		},
		{
			name:  "vietnamese containing let me quote",
			input: "Bài hát Let Me Down Slowly đang đứng đầu bảng xếp hạng tuần này.",
			want:  false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsInternalReasoning(tt.input); got != tt.want {
				t.Errorf("IsInternalReasoning(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestStripLeadingInternal(t *testing.T) {
	vn := "Chào cả nhà! Bản tin sáng nay: thị trường chứng khoán tăng điểm nhẹ, giá vàng ổn định quanh mốc cũ. Chúc mọi người một ngày làm việc hiệu quả nhé!"
	cot := "I don't have access to the group chat history."
	cot2 := "Let me look at the UIDs in the recent messages block."

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "cot then vietnamese",
			input: cot + "\n\n" + vn,
			want:  vn,
		},
		{
			name:  "two cot paragraphs then vietnamese",
			input: cot + "\n\n" + cot2 + "\n\n" + vn + "\n\n" + vn,
			want:  vn + "\n\n" + vn,
		},
		{
			name:  "vietnamese only no-op",
			input: vn + "\n\n" + vn,
			want:  vn + "\n\n" + vn,
		},
		{
			name:  "single paragraph never stripped",
			input: cot,
			want:  cot,
		},
		{
			name:  "never strips paragraph containing code fence",
			input: "```go\n// I should keep this\nlet me := 1\n```\n\n" + vn,
			want:  "```go\n// I should keep this\nlet me := 1\n```\n\n" + vn,
		},
		{
			name:  "stops at code fence after stripping cot",
			input: cot + "\n\n```\nI need this code\n```\n\n" + vn,
			want:  "```\nI need this code\n```\n\n" + vn,
		},
		{
			name:  "never strips past half the message",
			input: strings.Repeat(cot+" ", 10) + "\n\nGiá vàng hôm nay ổn định.",
			want:  strings.Repeat(cot+" ", 10) + "\n\nGiá vàng hôm nay ổn định.",
		},
		{
			name:  "crlf paragraph breaks",
			input: cot + "\r\n\r\n" + vn,
			want:  vn,
		},
		{
			name:  "empty",
			input: "",
			want:  "",
		},
		{
			name:  "english content after cot is preserved when not internal",
			input: cot + "\n\n" + "Weekly market update: gold steady, stocks up by two percent overall today." + "\n\n" + vn,
			want:  "Weekly market update: gold steady, stocks up by two percent overall today." + "\n\n" + vn,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := StripLeadingInternal(tt.input)
			if strings.TrimSpace(got) != strings.TrimSpace(tt.want) {
				t.Errorf("StripLeadingInternal() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestIsMetaFailure(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{
			name:  "error prefix",
			input: "Error: failed to fetch data from the upstream API endpoint.",
			want:  true,
		},
		{
			name:  "failed prefix",
			input: "Failed to complete the scheduled job because the tool timed out.",
			want:  true,
		},
		{
			name:  "cross mark prefix english",
			input: "❌ Could not deliver the report to the requested channel.",
			want:  true,
		},
		{
			name:  "i was unable",
			input: "I was unable to retrieve the group chat history for this session.",
			want:  true,
		},
		{
			name:  "pure cot message",
			input: "I don't have access to the group chat history.\n\nLet me look at the UIDs in the recent messages.",
			want:  true,
		},
		{
			name:  "it seems meta",
			input: "It seems the tool is unavailable right now, so nothing was posted.",
			want:  true,
		},
		{
			name:  "vietnamese bulletin",
			input: "Bản tin sáng: thị trường tăng điểm, giá vàng ổn định quanh mốc cũ.",
			want:  false,
		},
		{
			name:  "vietnamese error message protected",
			input: "❌ Lỗi: không thể lấy dữ liệu thị trường hôm nay, mình sẽ thử lại sau nhé.",
			want:  false,
		},
		{
			name:  "legit english newsletter",
			input: "Weekly market update: gold steady, stocks up two percent across all sectors.",
			want:  false,
		},
		{
			name:  "empty",
			input: "",
			want:  false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsMetaFailure(tt.input); got != tt.want {
				t.Errorf("IsMetaFailure(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}
