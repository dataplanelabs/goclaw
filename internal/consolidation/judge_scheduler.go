package consolidation

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"
	"time"

	"github.com/adhocore/gronx"
	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/eventbus"
	"github.com/nextlevelbuilder/goclaw/internal/store"
)

const (
	defaultJudgeSchedule  = "0 8-18 * * 1-5"
	judgeSchedulerTick    = 60 * time.Second
	judgeSchedulerMaxRows = 500
	judgeBatchSizeMax     = 50
)

// JudgeScheduler ticks every minute, identifies channels in scheduled mode
// whose cron expression matched the previous-minute window, and publishes
// team.reply.observed events for their pending rows. JudgeWorker then grades
// them via the standard per-row path.
type JudgeScheduler struct {
	evals     store.TeamReplyEvalStore
	instances store.ChannelInstanceStore
	bus       eventbus.DomainEventBus
	worker    *JudgeWorker

	mu       sync.Mutex
	gronx    *gronx.Gronx
	stopOnce sync.Once
	stopCh   chan struct{}
}

type JudgeSchedulerDeps struct {
	Evals     store.TeamReplyEvalStore
	Instances store.ChannelInstanceStore
	Bus       eventbus.DomainEventBus
	Worker    *JudgeWorker
}

func NewJudgeScheduler(deps JudgeSchedulerDeps) *JudgeScheduler {
	return &JudgeScheduler{
		evals:     deps.Evals,
		instances: deps.Instances,
		bus:       deps.Bus,
		worker:    deps.Worker,
		gronx:     gronx.New(),
		stopCh:    make(chan struct{}),
	}
}

func (s *JudgeScheduler) Start(ctx context.Context) {
	if s == nil || s.evals == nil || s.instances == nil || s.bus == nil {
		slog.Info("judge_scheduler.skipped", "reason", "missing deps")
		return
	}
	go s.run(ctx)
}

func (s *JudgeScheduler) Stop() {
	s.stopOnce.Do(func() { close(s.stopCh) })
}

func (s *JudgeScheduler) run(ctx context.Context) {
	t := time.NewTicker(judgeSchedulerTick)
	defer t.Stop()
	slog.Info("judge_scheduler.started", "tick_seconds", int(judgeSchedulerTick.Seconds()))
	for {
		select {
		case <-ctx.Done():
			return
		case <-s.stopCh:
			return
		case now := <-t.C:
			s.tick(ctx, now)
		}
	}
}

// scheduledCfg is the subset of ZaloOAConfig we need; declared locally to
// avoid importing config (which would create a cycle via channels).
type scheduledCfg struct {
	CaptureTeamReplies      *bool  `json:"capture_team_replies,omitempty"`
	JudgeEvaluationMode     string `json:"judge_evaluation_mode,omitempty"`
	JudgeEvaluationSchedule string `json:"judge_evaluation_schedule,omitempty"`
	JudgeBatchSize          int    `json:"judge_batch_size,omitempty"`
}

func (s *JudgeScheduler) tick(ctx context.Context, now time.Time) {
	insts, err := s.instances.ListAllInstances(ctx)
	if err != nil {
		slog.Warn("judge_scheduler.list_failed", "err", err)
		return
	}
	for i := range insts {
		s.tickInstance(ctx, &insts[i], now)
	}
}

func (s *JudgeScheduler) tickInstance(ctx context.Context, ci *store.ChannelInstanceData, now time.Time) {
	if len(ci.Config) == 0 {
		return
	}
	var cfg scheduledCfg
	if json.Unmarshal(ci.Config, &cfg) != nil {
		return
	}
	if cfg.JudgeEvaluationMode != "scheduled" {
		return
	}
	if cfg.CaptureTeamReplies == nil || !*cfg.CaptureTeamReplies {
		return
	}
	expr := cfg.JudgeEvaluationSchedule
	if expr == "" {
		expr = defaultJudgeSchedule
	}
	due, err := s.gronx.IsDue(expr, now)
	if err != nil || !due {
		return
	}
	s.gradePending(ctx, ci, cfg.JudgeBatchSize)
}

func (s *JudgeScheduler) gradePending(ctx context.Context, ci *store.ChannelInstanceData, batchSize int) {
	rows, err := s.evals.ListPendingJudge(ctx, judgeSchedulerMaxRows)
	if err != nil {
		slog.Warn("judge_scheduler.list_pending_failed", "instance", ci.Name, "err", err)
		return
	}
	if len(rows) == 0 {
		return
	}
	var ours []store.TeamReplyEvaluation
	for _, r := range rows {
		if r.ChannelInstanceID == ci.ID.String() {
			ours = append(ours, r)
		}
	}
	if len(ours) == 0 {
		return
	}
	if batchSize > 1 && s.worker != nil {
		if batchSize > judgeBatchSizeMax {
			batchSize = judgeBatchSizeMax
		}
		for i := 0; i < len(ours); i += batchSize {
			end := i + batchSize
			if end > len(ours) {
				end = len(ours)
			}
			_ = s.worker.BatchGrade(ctx, ours[i:end], ci.Name)
		}
		slog.Info("judge_scheduler.tick_batched", "instance", ci.Name, "rows", len(ours), "batch_size", batchSize)
		return
	}
	for _, r := range ours {
		s.publish(ci, r)
	}
	slog.Info("judge_scheduler.tick", "instance", ci.Name, "published", len(ours))
}

func (s *JudgeScheduler) publish(ci *store.ChannelInstanceData, r store.TeamReplyEvaluation) {
	s.bus.Publish(eventbus.DomainEvent{
		ID:        uuid.NewString(),
		Type:      eventbus.EventTeamReplyObserved,
		SourceID:  eventbus.TeamReplyObservedSourceID(r.ChannelInstanceID, r.TeamMsgID) + "?scheduled=" + uuid.NewString()[:8],
		TenantID:  r.TenantID,
		Timestamp: time.Now().UTC(),
		Payload: eventbus.TeamReplyObservedPayload{
			EvaluationID:      r.ID,
			TenantID:          r.TenantID,
			ChannelInstanceID: r.ChannelInstanceID,
			ChannelName:       ci.Name,
			ThreadKey:         r.ThreadKey,
			SessionKey:        r.SessionKey,
			TeamMsgID:         r.TeamMsgID,
			TeamReply:         r.TeamReply,
			CustomerMessage:   r.CustomerMessage,
			CapturedAt:        r.CapturedAt,
		},
	})
}
