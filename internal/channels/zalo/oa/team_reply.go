package oa

import (
	"context"
	"log/slog"
	"strings"

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
	evals store.TeamReplyEvalStore, tenantID string) {
	c.teamReplyBus = bus
	c.teamReplySessions = sessions
	c.teamReplyEvals = evals
	c.teamReplyTenantID = tenantID
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
		inst, err := c.ciStore.Get(context.Background(), c.instanceID)
		if err != nil || inst == nil {
			slog.Warn("zalo_oa.team_reply.tenant_lookup_fail",
				"instance", c.Name(), "err", err)
			return
		}
		c.teamReplyTenantID = inst.TenantID.String()
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
		Bus:          c.teamReplyBus,
		CustomerLast: customerLast,
	}
	w := NewPollWorker(c.instanceID, c.Name(), c.teamReplyTenantID, channels.TypeZaloOA,
		creds.OAID, c.cfg.TeamReplyPollInterval(), deps)
	c.teamReplyWorker = w
	go w.Run(context.Background())
	slog.Info("zalo_oa.team_reply.started",
		"instance", c.Name(),
		"interval_seconds", int(c.cfg.TeamReplyPollInterval().Seconds()),
		"judge_evaluation", c.cfg.JudgeEvaluation != nil && *c.cfg.JudgeEvaluation)
}
