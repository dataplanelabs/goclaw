package oa

import (
	"context"
	"log/slog"
	"strings"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/channels"
	"github.com/nextlevelbuilder/goclaw/internal/eventbus"
	"github.com/nextlevelbuilder/goclaw/internal/store"
)

// teamReplyCustomerContextChars caps the snippet of the latest customer
// message that gets fed to the judge agent. Avoids inflating prompt size
// on long inbound messages.
const teamReplyCustomerContextChars = 4000

// SetTeamReplyDeps wires Phase 4 dependencies into the channel. Called by
// FactoryWithDeps before Start(). Idempotent — last writer wins.
func (c *Channel) SetTeamReplyDeps(bus eventbus.DomainEventBus, sessions store.SessionStore,
	evals store.TeamReplyEvalStore, atomic store.AtomicTeamReplyWriter, tenantID string,
	judgeResolver JudgeAgentResolver) {
	c.teamReplyBus = bus
	c.teamReplySessions = sessions
	c.teamReplyEvals = evals
	c.teamReplyAtomic = atomic
	c.teamReplyTenantID = tenantID
	c.judgeResolver = judgeResolver
}

// startTeamReplyWorker is invoked from Start(). No-op when the channel
// hasn't been configured for capture (deps unwired OR flag false).
func (c *Channel) startTeamReplyWorker() {
	if c.cfg.CaptureTeamReplies == nil || !*c.cfg.CaptureTeamReplies {
		return
	}
	if c.teamReplyBus == nil || c.teamReplySessions == nil || c.teamReplyEvals == nil {
		slog.Warn("zalo_oa.team_reply.deps_missing",
			"instance", c.Name(),
			"hint", "channel.config.capture_team_replies=true but FactoryWithDeps was not used")
		return
	}
	creds := c.creds()
	if creds == nil || creds.OAID == "" {
		slog.Warn("zalo_oa.team_reply.no_oa_id", "instance", c.Name())
		return
	}
	if c.teamReplyTenantID == "" {
		// instance_loader.go stamps tenantID on BaseChannel via SetTenantID
		// before Start() is called. Read it directly — no DB lookup, no
		// tenant-scoped Get failure mode.
		tid := c.TenantID()
		if tid == uuid.Nil {
			slog.Warn("zalo_oa.team_reply.no_tenant_on_channel",
				"instance", c.Name(),
				"hint", "instance_loader did not stamp tenantID before Start")
			return
		}
		c.teamReplyTenantID = tid.String()
	}
	tokenSrc := func() string {
		tok, _ := c.tokens.Access(context.Background())
		return tok
	}
	onBehalf := NewOnBehalfClient(c.client, tokenSrc)
	sessions := c.teamReplySessions
	customerLast := func(ctx context.Context, sessionKey string) string {
		msgs := sessions.GetHistory(ctx, sessionKey)
		for i := len(msgs) - 1; i >= 0; i-- {
			if msgs[i].Role == "user" && strings.TrimSpace(msgs[i].Content) != "" {
				txt := msgs[i].Content
				if len(txt) > teamReplyCustomerContextChars {
					txt = txt[:teamReplyCustomerContextChars]
				}
				return txt
			}
		}
		return ""
	}
	deps := PollWorkerDeps{
		OnBehalf:     onBehalf,
		Sessions:     c.teamReplySessions,
		Evals:        c.teamReplyEvals,
		Atomic:       c.teamReplyAtomic,
		Bus:          c.teamReplyBus,
		CustomerLast: customerLast,
		JudgeMode:    c.cfg.JudgeEvaluationMode,
		AgentKey:     c.AgentID(),
	}
	w := NewPollWorker(c.instanceID, c.Name(), c.teamReplyTenantID, channels.TypeZaloOA,
		creds.OAID, c.cfg.TeamReplyPollInterval(), deps)
	c.teamReplyWorker = w
	go w.Run(context.Background())
	judgeEval := c.cfg.JudgeEvaluation != nil && *c.cfg.JudgeEvaluation
	slog.Info("zalo_oa.team_reply.started",
		"instance", c.Name(),
		"interval_seconds", int(c.cfg.TeamReplyPollInterval().Seconds()),
		"judge_evaluation", judgeEval)
	if judgeEval && c.judgeResolver != nil {
		judgeID, agentKey, resolveErr := c.judgeResolver(context.Background(), c.teamReplyTenantID, c.instanceID.String())
		if judgeID == uuid.Nil {
			slog.Warn("zalo_oa.team_reply.judge_misconfigured",
				"instance", c.Name(),
				"tenant", c.teamReplyTenantID,
				"configured_key", agentKey,
				"resolve_err", resolveErr)
		}
	}
}
