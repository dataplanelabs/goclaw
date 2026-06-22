package textguard

import (
	"strings"
	"testing"

	"golang.org/x/text/unicode/norm"
)

func TestIsEnglishDominant(t *testing.T) {
	// NFD-decomposed "Chào buổi sáng" — base letters + combining diacritics.
	nfdVietnamese := norm.NFD.String("Chào buổi sáng cả nhà, hôm nay thị trường tăng nhẹ.")

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
			name:  "nfd-decomposed vietnamese classified vietnamese",
			input: nfdVietnamese,
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
			name:  "i dont have access cot",
			input: "I don't have access to the group chat history. Let me look at the UIDs.",
			want:  true,
		},
		{
			name:  "curly apostrophe cot",
			input: "I don’t have the data yet, so let me fetch it first before replying.",
			want:  true,
		},
		{
			name:  "let me planning",
			input: "Let me look at the UIDs to figure out who to mention in the post.",
			want:  true,
		},
		{
			name:  "i should planning",
			input: "I should summarize the news for the group before posting anything else.",
			want:  true,
		},
		{
			name:  "as an ai",
			input: "As an AI, I cannot directly access the calendar for this account today.",
			want:  true,
		},
		// Ambiguous openers REMOVED from the stopword set — these are legit reports.
		{
			name:  "based on the report no longer cot",
			input: "Based on the calendar, you have three meetings scheduled today.",
			want:  false,
		},
		{
			name:  "looking at metrics no longer cot",
			input: "Looking at this week's metrics, sales rose by twelve percent overall.",
			want:  false,
		},
		{
			name:  "i have posted no longer cot",
			input: "I have posted the schedule for next week to the team channel already.",
			want:  false,
		},
		{
			name:  "it seems no longer cot",
			input: "It seems engagement is up twenty percent this week across all posts.",
			want:  false,
		},
		{
			name:  "english brand sentence no stopwords",
			input: "Shopee Super Sale starts this weekend with discounts up to 50%.",
			want:  false,
		},
		{
			name:  "english product stock list no stopwords",
			input: "iPhone 15 Pro Max - out of stock\nGalaxy S24 - 12 units left",
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
			name:  "english content after cot preserved when not internal",
			input: cot + "\n\n" + "Weekly market update: gold steady, stocks up by two percent overall today." + "\n\n" + vn,
			want:  "Weekly market update: gold steady, stocks up by two percent overall today." + "\n\n" + vn,
		},
		{
			name:  "english headline without stopword never stripped",
			input: "Apple unveils a new foldable iPhone at its annual September keynote event.\n\n" + vn,
			want:  "Apple unveils a new foldable iPhone at its annual September keynote event.\n\n" + vn,
		},
		{
			// Leak shape: VN reasoning preamble naming internal plumbing ("retry crons").
			name:  "vietnamese internal-marker preamble stripped",
			input: "Nhìn lại ngữ cảnh hôm nay: học viên tick xanh từng task. Em gửi nhắc Piano thôi, không tạo retry crons — mỗi task một lần.\n\n@[100000000000000001] 🎹 3h rồi! Mở nắp đàn — 30 phút luyện ngón nhé!",
			want:  "@[100000000000000001] 🎹 3h rồi! Mở nắp đàn — 30 phút luyện ngón nhé!",
		},
		{
			// A normal VN reminder (no internal vocabulary) is never stripped.
			name:  "vietnamese reminder without markers kept",
			input: "@[100000000000000001] 🎹 3h rồi! Mở nắp đàn nhé!\n\n" + vn,
			want:  "@[100000000000000001] 🎹 3h rồi! Mở nắp đàn nhé!\n\n" + vn,
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
		// MUST-CATCH: pure English first-person CoT leaks.
		{
			name:  "pure cot message",
			input: "I don't have access to the group chat history. Let me look at the UIDs to figure out who to mention.",
			want:  true,
		},
		{
			name:  "i was unable cot",
			input: "I was unable to retrieve the group chat history, so let me check the UIDs instead.",
			want:  true,
		},
		{
			name:  "multi-paragraph pure cot",
			input: "I don't have access to the group chat history.\n\nLet me look at the UIDs in the recent messages.",
			want:  true,
		},
		// MUST-NOT-SUPPRESS: legit English content (no first-person CoT).
		{
			name:  "based on calendar report",
			input: "Based on the calendar, you have 3 meetings today.",
			want:  false,
		},
		{
			name:  "it seems engagement up",
			input: "It seems engagement is up 20% this week.",
			want:  false,
		},
		{
			name:  "looking at metrics",
			input: "Looking at this week's metrics, sales rose.",
			want:  false,
		},
		{
			name:  "error rate dropped alert",
			input: "Error rate dropped to 0.1% this week - great improvement team!",
			want:  false,
		},
		{
			name:  "sorry store closed",
			input: "Sorry, the store is closed on Sunday. We reopen Monday.",
			want:  false,
		},
		{
			name:  "cross-mark api down alert",
			input: "❌ API down: payments endpoint returning 503.",
			want:  false,
		},
		{
			name:  "cross-mark product stock list",
			input: "❌ iPhone 15 Pro Max - out of stock\n✅ Galaxy S24 - 12 units left",
			want:  false,
		},
		{
			name:  "failed prefix as status line",
			input: "Failed deploys: 0. All services healthy across every region today.",
			want:  false,
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
