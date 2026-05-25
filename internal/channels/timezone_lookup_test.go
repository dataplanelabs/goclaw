package channels

import (
	"encoding/json"
	"testing"
)

func TestTimezoneFromConfig(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want string
	}{
		{"empty", "", ""},
		{"missing key", `{"dm_policy":"open"}`, ""},
		{"present", `{"timezone":"Asia/Ho_Chi_Minh"}`, "Asia/Ho_Chi_Minh"},
		{"present with siblings", `{"dm_policy":"open","timezone":"America/New_York"}`, "America/New_York"},
		{"empty string value", `{"timezone":""}`, ""},
		{"malformed json", `not-json`, ""},
		{"null value", `{"timezone":null}`, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := TimezoneFromConfig(json.RawMessage(tc.raw))
			if got != tc.want {
				t.Errorf("TimezoneFromConfig(%q) = %q, want %q", tc.raw, got, tc.want)
			}
		})
	}
}
