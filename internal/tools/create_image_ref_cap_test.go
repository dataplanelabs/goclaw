package tools

import (
	"context"
	"testing"

	"github.com/nextlevelbuilder/goclaw/internal/providers"
)

// Regression for trace 019e7369: a native provider with a custom instance name
// ("codex-cnb") must report the native cap (16), not the default 4.
func TestRefCapForProvider_NativeProviderCustomNameReports16(t *testing.T) {
	reg := providers.NewRegistry(nil)
	reg.Register(&nativeImageProvider{name: "codex-cnb", model: "gpt-5.3-codex"})
	tool := NewCreateImageTool(reg)

	if got := tool.refCapForProvider(context.Background(), "codex-cnb"); got != codexImageRefCap {
		t.Fatalf("refCapForProvider(codex-cnb) = %d, want %d (native providers cap at codex cap regardless of instance name)",
			got, codexImageRefCap)
	}
}

// Absent provider → best-effort name lookup.
func TestRefCapForProvider_AbsentProviderFallsBackToNameLookup(t *testing.T) {
	tool := NewCreateImageTool(providers.NewRegistry(nil))
	if got := tool.refCapForProvider(context.Background(), "gemini"); got != geminiRefCap {
		t.Fatalf("refCapForProvider(gemini, absent) = %d, want %d", got, geminiRefCap)
	}
}
