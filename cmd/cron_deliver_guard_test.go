package cmd

import (
	"strings"
	"testing"
)

func TestGuardCronDelivery(t *testing.T) {
	vnBulletin := "Chào cả nhà! Bản tin sáng nay: thị trường chứng khoán tăng điểm nhẹ, giá vàng ổn định quanh mốc cũ. Chúc mọi người một ngày làm việc hiệu quả nhé!"

	tests := []struct {
		name        string
		input       string
		wantContent string
		wantDeliver bool
	}{
		{
			name:        "vietnamese bulletin passes unchanged",
			input:       vnBulletin,
			wantContent: vnBulletin,
			wantDeliver: true,
		},
		{
			name:        "vietnamese bulletin with english urls passes",
			input:       "Tin nóng hôm nay nè cả nhà:\n\n- Giá vàng tăng mạnh, xem tại https://goldprice.example.com/latest-news-update\n- Thị trường chứng khoán: https://stockmarket.example.com/daily-report",
			wantContent: "Tin nóng hôm nay nè cả nhà:\n\n- Giá vàng tăng mạnh, xem tại https://goldprice.example.com/latest-news-update\n- Thị trường chứng khoán: https://stockmarket.example.com/daily-report",
			wantDeliver: true,
		},
		{
			name:        "cot stripped vietnamese delivered",
			input:       "I don't have access to the group chat history. Let me look at the UIDs.\n\n" + vnBulletin,
			wantContent: vnBulletin,
			wantDeliver: true,
		},
		{
			name:        "all english failure suppressed",
			input:       "Error: failed to fetch the market data from the upstream API endpoint.",
			wantContent: "",
			wantDeliver: false,
		},
		{
			name:        "i was unable suppressed",
			input:       "I was unable to retrieve the group chat history for this scheduled run.",
			wantContent: "",
			wantDeliver: false,
		},
		{
			name:        "cross mark english failure suppressed",
			input:       "❌ Could not deliver the report because the channel rejected the message.",
			wantContent: "",
			wantDeliver: false,
		},
		{
			name:        "pure english cot suppressed",
			input:       "I don't have access to the group chat history right now.\n\nLet me look at the UIDs in the recent messages block to figure out who responded.",
			wantContent: "",
			wantDeliver: false,
		},
		{
			name:        "mixed vietnamese with one english brand sentence passes",
			input:       "Bản tin công nghệ sáng nay:\n\nApple Vision Pro 2 launches worldwide with a cheaper price tag this fall.\n\nNguồn tin cho biết giá bán tại Việt Nam sẽ được công bố vào tuần sau nhé cả nhà.",
			wantContent: "Bản tin công nghệ sáng nay:\n\nApple Vision Pro 2 launches worldwide with a cheaper price tag this fall.\n\nNguồn tin cho biết giá bán tại Việt Nam sẽ được công bố vào tuần sau nhé cả nhà.",
			wantDeliver: true,
		},
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
