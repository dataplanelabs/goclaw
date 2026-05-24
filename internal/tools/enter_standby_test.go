package tools

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/store"
)

type fakeStandbyStore struct {
	resolveResp string
	resolveErr  error
	setCalls    []store.ThreadSchedule
	setErr      error
}

func (f *fakeStandbyStore) ResolveInstanceIDByName(_ context.Context, _, _ string) (string, error) {
	return f.resolveResp, f.resolveErr
}
func (f *fakeStandbyStore) SetThreadSchedule(_ context.Context, t store.ThreadSchedule) error {
	f.setCalls = append(f.setCalls, t)
	return f.setErr
}

func ctxWithStandbyChannel(t *testing.T) context.Context {
	t.Helper()
	ctx := store.WithTenantID(context.Background(), uuid.New())
	ctx = WithToolChannel(ctx, "tg")
	ctx = WithToolChatID(ctx, "chat42")
	ctx = WithToolPeerKind(ctx, "direct")
	return ctx
}

func errIfErr(r *Result) string {
	if r == nil || !r.IsError {
		return ""
	}
	return r.ForLLM
}

func TestEnterStandby_NoChannelContext(t *testing.T) {
	tool := NewEnterStandbyTool(&fakeStandbyStore{resolveResp: "inst-1"}, nil)
	res := tool.Execute(context.Background(), map[string]any{"duration_seconds": 300})
	if errIfErr(res) == "" {
		t.Fatalf("expected error for missing channel ctx, got %+v", res)
	}
}

func TestEnterStandby_DurationOutOfRange(t *testing.T) {
	tool := NewEnterStandbyTool(&fakeStandbyStore{resolveResp: "inst-1"}, nil)
	for _, bad := range []int{0, 30, 100000} {
		res := tool.Execute(ctxWithStandbyChannel(t), map[string]any{"duration_seconds": bad})
		if errIfErr(res) == "" {
			t.Fatalf("duration=%d: expected error, got %+v", bad, res)
		}
	}
}

func TestEnterStandby_ResolveMissing(t *testing.T) {
	tool := NewEnterStandbyTool(&fakeStandbyStore{resolveResp: ""}, nil)
	res := tool.Execute(ctxWithStandbyChannel(t), map[string]any{"duration_seconds": 600})
	if errIfErr(res) == "" {
		t.Fatalf("missing instance should error, got %+v", res)
	}
}

func TestEnterStandby_HappyPath(t *testing.T) {
	store := &fakeStandbyStore{resolveResp: "inst-1"}
	var reloaded string
	tool := NewEnterStandbyTool(store, func(id string) { reloaded = id })
	res := tool.Execute(ctxWithStandbyChannel(t), map[string]any{
		"duration_seconds": 7200,
		"reason":           "lunch",
	})
	if errIfErr(res) != "" {
		t.Fatalf("unexpected error: %v", errIfErr(res))
	}
	if !strings.Contains(res.ForLLM, "lunch") {
		t.Fatalf("expected reason in result, got %q", res.ForLLM)
	}
	if !strings.Contains(res.ForLLM, "2.0h") {
		t.Fatalf("expected humanized duration in result, got %q", res.ForLLM)
	}
	if len(store.setCalls) != 1 {
		t.Fatalf("expected 1 SetThreadSchedule call, got %d", len(store.setCalls))
	}
	got := store.setCalls[0]
	if got.ChannelInstanceID != "inst-1" {
		t.Fatalf("instance id: %q", got.ChannelInstanceID)
	}
	if got.ThreadKey != "direct:chat42" {
		t.Fatalf("thread key: %q", got.ThreadKey)
	}
	if got.Reason != "lunch" {
		t.Fatalf("reason: %q", got.Reason)
	}
	if got.ExpiresAt == nil {
		t.Fatalf("expires_at must be set")
	}
	if reloaded != "inst-1" {
		t.Fatalf("reload not called with instance id, got %q", reloaded)
	}
}

func TestHumanDuration(t *testing.T) {
	cases := map[int]string{
		60:    "1m",
		3600:  "1.0h",
		7200:  "2.0h",
		86400: "24.0h",
	}
	for in, want := range cases {
		if got := humanDuration(in); got != want {
			t.Fatalf("humanDuration(%d)=%q, want %q", in, got, want)
		}
	}
}
