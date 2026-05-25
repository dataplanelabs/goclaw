package tools

import (
	"os"
	"testing"
	"time"
)

func TestToolTimeoutFromEnv(t *testing.T) {
	cases := []struct {
		name string
		env  string
		want time.Duration
	}{
		{"unset returns default", "", defaultToolTimeout},
		{"valid positive", "60", 60 * time.Second},
		{"large valid", "600", 600 * time.Second},
		{"zero falls back to default", "0", defaultToolTimeout},
		{"negative falls back to default", "-5", defaultToolTimeout},
		{"non-numeric falls back to default", "abc", defaultToolTimeout},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.env == "" {
				t.Setenv("TESTTOOL_TIMEOUT_SEC", "")
				os.Unsetenv("TESTTOOL_TIMEOUT_SEC")
			} else {
				t.Setenv("TESTTOOL_TIMEOUT_SEC", tc.env)
			}
			got := toolTimeoutFromEnv("TESTTOOL")
			if got != tc.want {
				t.Errorf("toolTimeoutFromEnv(%q)=%v want %v", tc.env, got, tc.want)
			}
		})
	}
}
