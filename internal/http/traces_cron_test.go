package http

import (
	"testing"
	"time"
)

func TestCronJobIDFromTrace(t *testing.T) {
	cases := []struct {
		name       string
		runID      string
		sessionKey string
		want       string
	}{
		{
			name:  "run id wins",
			runID: "cron:job-1",
			want:  "job-1",
		},
		{
			name:       "session fallback",
			sessionKey: "agent:assistant:cron:job-2",
			want:       "job-2",
		},
		{
			name:       "empty cron run id falls back to session",
			runID:      "cron:",
			sessionKey: "agent:assistant:cron:job-3",
			want:       "job-3",
		},
		{
			name:       "non cron ignored",
			runID:      "chat:1",
			sessionKey: "agent:assistant:zalo:direct:123",
			want:       "",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := cronJobIDFromTrace(tc.runID, tc.sessionKey)
			if got != tc.want {
				t.Fatalf("cronJobIDFromTrace() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestRewriteLeadingTraceStamp(t *testing.T) {
	start := time.Date(2026, 6, 17, 1, 0, 1, 0, time.UTC)
	input := "[2026-06-17 01:00 +00]\nbriefing"

	got := rewriteLeadingTraceStamp(input, start, "Asia/Ho_Chi_Minh")
	want := "[2026-06-17 08:00 +07]\nbriefing"
	if got != want {
		t.Fatalf("rewriteLeadingTraceStamp = %q, want %q", got, want)
	}
}

func TestRewriteLeadingTraceStamp_LeavesNonStampText(t *testing.T) {
	start := time.Date(2026, 6, 17, 1, 0, 1, 0, time.UTC)
	input := "[not a timestamp]\nbriefing"

	if got := rewriteLeadingTraceStamp(input, start, "Asia/Ho_Chi_Minh"); got != input {
		t.Fatalf("rewriteLeadingTraceStamp changed non-stamp input: %q", got)
	}
}
