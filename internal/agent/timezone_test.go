package agent

import "testing"

func TestResolveUserTimezone(t *testing.T) {
	cases := []struct {
		name             string
		channelTZ        string
		workspaceDefault string
		want             string
	}{
		{"channel wins", "Asia/Ho_Chi_Minh", "UTC", "Asia/Ho_Chi_Minh"},
		{"workspace fallback", "", "Asia/Ho_Chi_Minh", "Asia/Ho_Chi_Minh"},
		{"both empty", "", "", ""},
		{"invalid channel skipped", "Garbage/Invalid", "Asia/Ho_Chi_Minh", "Asia/Ho_Chi_Minh"},
		{"both invalid", "Garbage/Invalid", "Also/Bad", ""},
		{"channel over workspace", "America/New_York", "Asia/Ho_Chi_Minh", "America/New_York"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ResolveUserTimezone(tc.channelTZ, tc.workspaceDefault)
			if got != tc.want {
				t.Errorf("ResolveUserTimezone(%q, %q) = %q, want %q", tc.channelTZ, tc.workspaceDefault, got, tc.want)
			}
		})
	}
}
