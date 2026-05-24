package pipeline

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/channels/schedule"
	"github.com/nextlevelbuilder/goclaw/internal/providers"
	"github.com/nextlevelbuilder/goclaw/internal/store"
)

// TestFinalize_StandbySuppressesContentKeepsMemory proves that standby mode:
//   1. zeroes Observe.FinalContent (no outbound delivery)
//   2. still calls FlushMessages (memory write)
//   3. still calls MaybeSummarize (episodic)
// Audit-revised assertion (Phase 3 Step 1).
func TestFinalize_StandbySuppressesContentKeepsMemory(t *testing.T) {
	var flushedCount, summarizedCount int
	deps := &PipelineDeps{
		FlushMessages: func(_ context.Context, _ string, _ []providers.Message) error {
			flushedCount++
			return nil
		},
		MaybeSummarize: func(_ context.Context, _ string) { summarizedCount++ },
	}
	stage := NewFinalizeStage(deps)
	state := &RunState{
		Input:       &RunInput{SessionKey: "s1"},
		Messages:    NewMessageBuffer(providers.Message{Role: "user", Content: "hi"}),
		StandbyMode: true,
		ExitCode:    AbortRun,
	}
	state.Observe.FinalContent = "would-be reply"

	if err := stage.Execute(context.Background(), state); err != nil {
		t.Fatalf("finalize: %v", err)
	}
	if state.Observe.FinalContent != "" {
		t.Fatalf("standby should clear FinalContent, got %q", state.Observe.FinalContent)
	}
	if flushedCount != 1 {
		t.Fatalf("FlushMessages calls = %d, want 1", flushedCount)
	}
	if summarizedCount != 1 {
		t.Fatalf("MaybeSummarize calls = %d, want 1", summarizedCount)
	}
}

func ctxWithTenant() context.Context {
	return store.WithTenantID(context.Background(), uuid.New())
}

func TestStandbyGate_NilResolverNoOp(t *testing.T) {
	deps := &PipelineDeps{}
	g := NewStandbyGate(deps)
	st := &RunState{Input: &RunInput{Channel: "tg", ChatID: "42", PeerKind: "direct"}}
	if err := g.Execute(ctxWithTenant(), st); err != nil {
		t.Fatal(err)
	}
	if st.StandbyMode || st.ExitCode == AbortRun {
		t.Fatalf("nil resolver should not set standby: %+v", st)
	}
	if g.Result() != Continue {
		t.Fatalf("nil resolver should report Continue, got %v", g.Result())
	}
}

func TestStandbyGate_MissingTenantOrChannel(t *testing.T) {
	called := false
	deps := &PipelineDeps{
		ResolveStandbyMode: func(ctx context.Context, tenantID, channelName, threadKey string, now time.Time) schedule.Mode {
			called = true
			return schedule.ModeStandby
		},
	}
	g := NewStandbyGate(deps)

	// no tenant in ctx
	st1 := &RunState{Input: &RunInput{Channel: "tg", ChatID: "42", PeerKind: "direct"}}
	if err := g.Execute(context.Background(), st1); err != nil {
		t.Fatal(err)
	}
	if st1.StandbyMode || called {
		t.Fatalf("no tenant: should skip, got standby=%v called=%v", st1.StandbyMode, called)
	}

	// empty channel
	st2 := &RunState{Input: &RunInput{ChatID: "42", PeerKind: "direct"}}
	if err := g.Execute(ctxWithTenant(), st2); err != nil {
		t.Fatal(err)
	}
	if st2.StandbyMode || called {
		t.Fatalf("empty channel: should skip")
	}

	// empty chat id
	st3 := &RunState{Input: &RunInput{Channel: "tg", PeerKind: "direct"}}
	if err := g.Execute(ctxWithTenant(), st3); err != nil {
		t.Fatal(err)
	}
	if st3.StandbyMode || called {
		t.Fatalf("empty chatID: should skip")
	}
}

func TestStandbyGate_ActiveDoesNothing(t *testing.T) {
	deps := &PipelineDeps{
		ResolveStandbyMode: func(_ context.Context, _, _, _ string, _ time.Time) schedule.Mode {
			return schedule.ModeActive
		},
	}
	g := NewStandbyGate(deps)
	st := &RunState{Input: &RunInput{Channel: "tg", ChatID: "42", PeerKind: "direct"}}
	if err := g.Execute(ctxWithTenant(), st); err != nil {
		t.Fatal(err)
	}
	if st.StandbyMode {
		t.Fatalf("active mode should not set StandbyMode")
	}
	if g.Result() != Continue {
		t.Fatalf("active mode should report Continue")
	}
}

func TestStandbyGate_StandbyAborts(t *testing.T) {
	var gotThreadKey string
	deps := &PipelineDeps{
		ResolveStandbyMode: func(_ context.Context, _, _, threadKey string, _ time.Time) schedule.Mode {
			gotThreadKey = threadKey
			return schedule.ModeStandby
		},
	}
	g := NewStandbyGate(deps)
	st := &RunState{Input: &RunInput{Channel: "tg", ChatID: "chat42", PeerKind: "group"}}
	if err := g.Execute(ctxWithTenant(), st); err != nil {
		t.Fatal(err)
	}
	if !st.StandbyMode {
		t.Fatalf("standby mode should set StandbyMode=true")
	}
	if st.ExitCode != AbortRun {
		t.Fatalf("standby should set ExitCode=AbortRun, got %v", st.ExitCode)
	}
	if g.Result() != AbortRun {
		t.Fatalf("Result should be AbortRun, got %v", g.Result())
	}
	if gotThreadKey != "group:chat42" {
		t.Fatalf("thread key: got %q want %q", gotThreadKey, "group:chat42")
	}
}

func TestBuildStandbyThreadKey(t *testing.T) {
	cases := []struct {
		kind, chat, want string
	}{
		{"direct", "peer123", "direct:peer123"},
		{"group", "chat456", "group:chat456"},
		{"", "x", "direct:x"},
	}
	for _, tc := range cases {
		got := BuildStandbyThreadKey(tc.kind, tc.chat)
		if got != tc.want {
			t.Fatalf("BuildStandbyThreadKey(%q,%q)=%q, want %q", tc.kind, tc.chat, got, tc.want)
		}
	}
}
