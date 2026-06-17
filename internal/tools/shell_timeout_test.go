package tools

import (
	"context"
	"testing"
	"time"
)

func TestExecToolEffectiveTimeout_DefaultAtLeastThreeMinutes(t *testing.T) {
	tool := NewExecTool(t.TempDir(), false)
	tool.timeout = time.Minute

	got := tool.effectiveTimeout(context.Background(), nil)
	if got != minExecTimeout {
		t.Fatalf("effectiveTimeout default = %s, want %s", got, minExecTimeout)
	}
}

func TestExecToolEffectiveTimeout_SettingsAndArgs(t *testing.T) {
	tool := NewExecTool(t.TempDir(), false)
	ctx := WithBuiltinToolSettings(context.Background(), BuiltinToolSettings{
		"exec": []byte(`{"timeout_sec":240}`),
	})

	if got := tool.effectiveTimeout(ctx, nil); got != 240*time.Second {
		t.Fatalf("settings timeout = %s, want 240s", got)
	}
	if got := tool.effectiveTimeout(ctx, map[string]any{"timeout_sec": float64(300)}); got != 300*time.Second {
		t.Fatalf("arg timeout = %s, want 300s", got)
	}
}

func TestExecToolEffectiveTimeout_ClampsBounds(t *testing.T) {
	tool := NewExecTool(t.TempDir(), false)

	if got := tool.effectiveTimeout(context.Background(), map[string]any{"timeout_sec": 30}); got != minExecTimeout {
		t.Fatalf("short timeout = %s, want %s", got, minExecTimeout)
	}
	if got := tool.effectiveTimeout(context.Background(), map[string]any{"timeout_sec": 7200}); got != maxExecTimeout {
		t.Fatalf("long timeout = %s, want %s", got, maxExecTimeout)
	}
}

func TestExecToolParametersExposeTimeout(t *testing.T) {
	tool := NewExecTool(t.TempDir(), false)
	params := tool.Parameters()
	props, ok := params["properties"].(map[string]any)
	if !ok {
		t.Fatalf("parameters properties missing: %#v", params)
	}
	if _, ok := props["timeout_sec"]; !ok {
		t.Fatalf("timeout_sec parameter missing: %#v", props)
	}
}
