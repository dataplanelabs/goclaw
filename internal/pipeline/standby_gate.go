package pipeline

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/channels/schedule"
	"github.com/nextlevelbuilder/goclaw/internal/store"
)

// StandbyGate is the FIRST iteration stage. When the resolver returns
// ModeStandby for the current (tenant, channel, thread), it sets
// state.StandbyMode + state.ExitCode=AbortRun. Pipeline.Run breaks the inner
// loop at pipeline.go:77 and outer at pipeline.go:84 — FinalizeStage still
// runs (memory writes preserved). Outbound from cron paths bypasses this
// gate (cron-fired publishes are intentional and do not enter the pipeline).
type StandbyGate struct {
	deps   *PipelineDeps
	result StageResult
}

func NewStandbyGate(d *PipelineDeps) *StandbyGate { return &StandbyGate{deps: d} }

func (s *StandbyGate) Name() string { return "standby_gate" }

func (s *StandbyGate) Execute(ctx context.Context, state *RunState) error {
	s.result = Continue
	if s.deps == nil || s.deps.ResolveStandbyMode == nil {
		return nil
	}
	tid := store.TenantIDFromContext(ctx)
	channelName := state.Input.Channel
	chatID := state.Input.ChatID
	if tid == uuid.Nil || channelName == "" || chatID == "" {
		return nil
	}
	tenantID := tid.String()
	threadKey := BuildStandbyThreadKey(state.Input.PeerKind, chatID)
	mode := s.deps.ResolveStandbyMode(ctx, tenantID, channelName, threadKey, time.Now())
	if mode == schedule.ModeStandby {
		state.StandbyMode = true
		state.ExitCode = AbortRun
		s.result = AbortRun
		slog.Info("pipeline.standby_gate active",
			"tenant_id", tenantID,
			"channel", channelName,
			"thread_key", threadKey,
		)
	}
	return nil
}

func (s *StandbyGate) Result() StageResult { return s.result }

// BuildStandbyThreadKey is the canonical thread-key format for
// channel_thread_schedules. Both StandbyGate (read) and the enter_standby
// tool (write) MUST use this — schedules silently won't match otherwise.
//
//	DM:    "direct:{peerID}"
//	Group: "group:{chatID}"
//	Default: "{kind}:{chatID}"
func BuildStandbyThreadKey(kind, chatID string) string {
	if kind == "" {
		kind = "direct"
	}
	return fmt.Sprintf("%s:%s", kind, chatID)
}
