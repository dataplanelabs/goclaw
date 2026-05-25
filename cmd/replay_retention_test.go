package cmd

import (
	"testing"
	"time"

	"github.com/nextlevelbuilder/goclaw/internal/config"
)

// Regression: trace 019e5f22-… could not be retried because every subsequent
// successful run swept its replay payload with cutoff=runStart. Sweeping by
// age-based cutoff (now-retention) keeps captures retryable within the window.
func TestResolveReplayRetention(t *testing.T) {
	cases := []struct {
		name string
		days int
		want time.Duration
	}{
		{"default (7 days, seeded by config.Default)", config.DefaultReplayRetentionDays, 7 * 24 * time.Hour},
		{"explicit 1 day", 1, 24 * time.Hour},
		{"explicit 14 days", 14, 14 * 24 * time.Hour},
		{"zero opts out → legacy runStart cutoff", 0, 0},
		{"negative opts out → legacy runStart cutoff", -1, 0},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			got := resolveReplayRetention(c.days)
			if got != c.want {
				t.Errorf("resolveReplayRetention(%d) = %v, want %v", c.days, got, c.want)
			}
		})
	}
}

// Default config seeds ReplayRetentionDays so seedConfigForContext writes the
// value into system_configs and operators see + can edit it in the UI.
func TestConfigDefault_SeedsReplayRetentionDays(t *testing.T) {
	cfg := config.Default()
	if cfg.Gateway.ReplayRetentionDays != config.DefaultReplayRetentionDays {
		t.Errorf("Gateway.ReplayRetentionDays = %d, want %d (so system_configs picks it up)",
			cfg.Gateway.ReplayRetentionDays, config.DefaultReplayRetentionDays)
	}
}
