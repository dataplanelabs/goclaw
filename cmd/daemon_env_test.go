package cmd

import (
	"slices"
	"testing"
)

func TestFilteredDaemonEnv(t *testing.T) {
	got := filteredDaemonEnv([]string{
		"PATH=/usr/bin",
		"PYTHONPATH=/app/data/.runtime/pip:",
		"PIP_TARGET=/app/data/.runtime/pip",
		"PIP_CACHE_DIR=/app/data/.runtime/pip-cache",
		"NPM_CONFIG_PREFIX=/app/data/.runtime/npm-global",
		"NODE_PATH=/usr/local/lib/node_modules:",
		"HF_HOME=/somewhere/bad",
		"XDG_CACHE_HOME=/somewhere/bad",
		"HOME=/app",
		"=malformed",
	})

	wantKept := []string{"PATH=/usr/bin", "HOME=/app", "=malformed"}
	if !slices.Equal(got, wantKept) {
		t.Errorf("filteredDaemonEnv = %q, want %q", got, wantKept)
	}
}
