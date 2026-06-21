package cmd

import (
	"strings"
	"testing"

	"golang.org/x/text/unicode/norm"
)

func TestGuardCronDelivery(t *testing.T) {
	vnBulletin := "Chào cả nhà! Bản tin sáng nay: thị trường chứng khoán tăng điểm nhẹ, giá vàng ổn định quanh mốc cũ. Chúc mọi người một ngày làm việc hiệu quả nhé!"
	nfdVietnamese := norm.NFD.String("Chào cả nhà, hôm nay trời nắng đẹp, mọi người nhớ uống đủ nước nhé!")

	tests := []struct {
		name        string
		input       string
		wantContent string
		wantDeliver bool
	}{
		// --- MUST-DELIVER / MUST-NOT-STRIP (legit content) ---
		{
			name:        "vietnamese bulletin passes unchanged",
			input:       vnBulletin,
			wantContent: vnBulletin,
			wantDeliver: true,
		},
		{
			name:        "nfd-decomposed vietnamese passes unchanged",
			input:       nfdVietnamese,
			wantContent: nfdVietnamese,
			wantDeliver: true,
		},
		{
			name:        "based on calendar report delivered",
			input:       "Based on the calendar, you have 3 meetings today.",
			wantContent: "Based on the calendar, you have 3 meetings today.",
			wantDeliver: true,
		},
		{
			name:        "it seems engagement up delivered",
			input:       "It seems engagement is up 20% this week.",
			wantContent: "It seems engagement is up 20% this week.",
			wantDeliver: true,
		},
		{
			name:        "mention prefixed i have posted delivered",
			input:       "@[850123] I have posted the schedule for next week.",
			wantContent: "@[850123] I have posted the schedule for next week.",
			wantDeliver: true,
		},
		{
			name:        "looking at metrics delivered",
			input:       "Looking at this week's metrics, sales rose.",
			wantContent: "Looking at this week's metrics, sales rose.",
			wantDeliver: true,
		},
		{
			name:        "error rate alert delivered",
			input:       "Error rate dropped to 0.1% this week - great improvement team!",
			wantContent: "Error rate dropped to 0.1% this week - great improvement team!",
			wantDeliver: true,
		},
		{
			name:        "sorry store closed delivered",
			input:       "Sorry, the store is closed on Sunday. We reopen Monday.",
			wantContent: "Sorry, the store is closed on Sunday. We reopen Monday.",
			wantDeliver: true,
		},
		{
			name:        "cross-mark api down alert delivered",
			input:       "❌ API down: payments endpoint returning 503.",
			wantContent: "❌ API down: payments endpoint returning 503.",
			wantDeliver: true,
		},
		{
			name:        "cross-mark product stock list single newline delivered",
			input:       "❌ iPhone 15 Pro Max - out of stock\n✅ Galaxy S24 - 12 units left",
			wantContent: "❌ iPhone 15 Pro Max - out of stock\n✅ Galaxy S24 - 12 units left",
			wantDeliver: true,
		},
		{
			name:        "english headline then vietnamese summary headline preserved",
			input:       "Apple unveils a new foldable iPhone at its annual September keynote event.\n\n" + vnBulletin,
			wantContent: "Apple unveils a new foldable iPhone at its annual September keynote event.\n\n" + vnBulletin,
			wantDeliver: true,
		},
		{
			name:        "vietnamese bulletin with english urls passes",
			input:       "Tin nóng hôm nay nè cả nhà:\n\n- Giá vàng tăng mạnh, xem tại https://goldprice.example.com/latest-news-update\n- Thị trường chứng khoán: https://stockmarket.example.com/daily-report",
			wantContent: "Tin nóng hôm nay nè cả nhà:\n\n- Giá vàng tăng mạnh, xem tại https://goldprice.example.com/latest-news-update\n- Thị trường chứng khoán: https://stockmarket.example.com/daily-report",
			wantDeliver: true,
		},
		{
			name:        "mixed vietnamese with one english brand sentence passes",
			input:       "Bản tin công nghệ sáng nay:\n\nApple Vision Pro 2 launches worldwide with a cheaper price tag this fall.\n\nNguồn tin cho biết giá bán tại Việt Nam sẽ được công bố vào tuần sau nhé cả nhà.",
			wantContent: "Bản tin công nghệ sáng nay:\n\nApple Vision Pro 2 launches worldwide with a cheaper price tag this fall.\n\nNguồn tin cho biết giá bán tại Việt Nam sẽ được công bố vào tuần sau nhé cả nhà.",
			wantDeliver: true,
		},
		{
			name:        "legit english newsletter passes",
			input:       "Weekly market update: gold steady, stocks up two percent across all sectors.",
			wantContent: "Weekly market update: gold steady, stocks up two percent across all sectors.",
			wantDeliver: true,
		},
		{
			name:        "vietnamese error message passes",
			input:       "❌ Lỗi: không thể lấy dữ liệu thị trường hôm nay, mình sẽ thử lại sau nhé.",
			wantContent: "❌ Lỗi: không thể lấy dữ liệu thị trường hôm nay, mình sẽ thử lại sau nhé.",
			wantDeliver: true,
		},

		// --- MUST-CATCH (suppress at cron) ---
		{
			name:        "pure english cot suppressed",
			input:       "I don't have access to the group chat history. Let me look at the UIDs to figure out who to mention.",
			wantContent: "",
			wantDeliver: false,
		},
		{
			name:        "leading cot stripped vietnamese delivered",
			input:       "I don't have access to the history. Let me check.\n\n@Công Ninh ơi, em chờ thông tin cự ly nhé!",
			wantContent: "@Công Ninh ơi, em chờ thông tin cự ly nhé!",
			wantDeliver: true,
		},
		{
			name:        "multi-paragraph english cot suppressed",
			input:       "I don't have access to the group chat history right now.\n\nLet me look at the UIDs in the recent messages block to figure out who responded.",
			wantContent: "",
			wantDeliver: false,
		},
		{
			name:        "cot stripped vietnamese delivered",
			input:       "I don't have access to the group chat history. Let me look at the UIDs.\n\n" + vnBulletin,
			wantContent: vnBulletin,
			wantDeliver: true,
		},
		{
			name:        "i was unable cot suppressed",
			input:       "I was unable to retrieve the group chat history, so let me check the UIDs instead.",
			wantContent: "",
			wantDeliver: false,
		},
		{
			name:        "trace no more sending suppressed",
			input:       "Chị Trân đã báo dọn cát mèo + bếp xong trong ngày hôm nay rồi. Đã xoá cron nhắc. Không gửi gì thêm.",
			wantContent: "",
			wantDeliver: false,
		},
		{
			name:        "explicit english no reply suppressed",
			input:       "Already confirmed. No action needed for the next cycle.",
			wantContent: "",
			wantDeliver: false,
		},
		{
			name:        "vietnamese no reminder suppressed",
			input:       "Đã xong rồi, không cần nhắc nữa.",
			wantContent: "",
			wantDeliver: false,
		},

		// --- edge cases ---
		{
			name:        "empty content suppressed",
			input:       "",
			wantContent: "",
			wantDeliver: false,
		},
		{
			name:        "whitespace only suppressed",
			input:       "  \n\n  ",
			wantContent: "",
			wantDeliver: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, deliver := guardCronDelivery(tt.input)
			if deliver != tt.wantDeliver {
				t.Fatalf("guardCronDelivery() deliver = %v, want %v (content %q)", deliver, tt.wantDeliver, got)
			}
			if strings.TrimSpace(got) != strings.TrimSpace(tt.wantContent) {
				t.Errorf("guardCronDelivery() content = %q, want %q", got, tt.wantContent)
			}
		})
	}
}
