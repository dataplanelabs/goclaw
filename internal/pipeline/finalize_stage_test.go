package pipeline

import (
	"context"
	"testing"
)

// Durable no-silence (fix/durable-no-silence): FinalizeStage must DELIVER a
// fallback for genuine aborts with empty content, but keep suppressing legit
// empty completions (BreakLoop NO_REPLY) and StandbyMode.
func TestFinalizeStage_EmptyContent_DeliveryDecision(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		exitCode     StageResult
		standby      bool
		silentReply  bool // IsSilentReply wired to always-true
		loopKilled   bool
		cancelled    bool
		fallbackDeps string
		wantContent  string
		wantSilent   bool // expected to be suppressed (delivered content == "")
	}{
		{
			name:        "aborted run delivers default fallback",
			exitCode:    AbortRun,
			wantContent: defaultAbortFallbackMessage,
			wantSilent:  false,
		},
		{
			name:        "loop-detector kill (BreakLoop + LoopKilled) delivers fallback — never silent",
			exitCode:    BreakLoop,
			loopKilled:  true,
			wantContent: defaultAbortFallbackMessage,
			wantSilent:  false,
		},
		{
			name:       "user-cancelled run (AbortRun + Cancelled) is suppressed — no 'please resend'",
			exitCode:   AbortRun,
			cancelled:  true,
			wantSilent: true,
		},
		{
			name:         "aborted run delivers custom fallback from deps",
			exitCode:     AbortRun,
			fallbackDeps: "Xin lỗi, hệ thống đang quá tải. Vui lòng gửi lại.",
			wantContent:  "Xin lỗi, hệ thống đang quá tải. Vui lòng gửi lại.",
			wantSilent:   false,
		},
		{
			name:       "normal completion (BreakLoop) empty content is suppressed",
			exitCode:   BreakLoop,
			wantSilent: true,
		},
		{
			name:       "standby mode is suppressed (no fallback) even when aborted",
			exitCode:   AbortRun,
			standby:    true,
			wantSilent: true,
		},
		{
			name:        "silent reply match is suppressed (no fallback) even when aborted",
			exitCode:    AbortRun,
			silentReply: true,
			wantSilent:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			deps := &PipelineDeps{AbortFallbackMessage: tt.fallbackDeps}
			if tt.silentReply {
				deps.IsSilentReply = func(string) bool { return true }
			}
			stage := NewFinalizeStage(deps)
			state := defaultState()
			state.ExitCode = tt.exitCode
			state.StandbyMode = tt.standby
			state.Cancelled = tt.cancelled
			state.Tool.LoopKilled = tt.loopKilled
			// FinalContent left empty — the abort-with-empty-content scenario.

			if err := stage.Execute(context.Background(), state); err != nil {
				t.Fatalf("Execute() error: %v", err)
			}

			if tt.wantSilent {
				if state.Observe.FinalContent != "" {
					t.Errorf("FinalContent = %q, want suppressed (empty)", state.Observe.FinalContent)
				}
				return
			}
			if state.Observe.FinalContent != tt.wantContent {
				t.Errorf("FinalContent = %q, want %q (delivered fallback)", state.Observe.FinalContent, tt.wantContent)
			}
		})
	}
}

// A non-empty final answer must pass through untouched regardless of exit code.
func TestFinalizeStage_NonEmptyContent_NotOverwritten(t *testing.T) {
	t.Parallel()
	stage := NewFinalizeStage(&PipelineDeps{})
	state := defaultState()
	state.ExitCode = AbortRun
	state.Observe.FinalContent = "real answer"

	if err := stage.Execute(context.Background(), state); err != nil {
		t.Fatalf("Execute() error: %v", err)
	}
	if state.Observe.FinalContent != "real answer" {
		t.Errorf("FinalContent = %q, want unchanged 'real answer'", state.Observe.FinalContent)
	}
}
