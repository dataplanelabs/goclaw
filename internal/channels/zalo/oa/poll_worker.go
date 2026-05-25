package oa

import (
	"context"
	"errors"
	"log/slog"
	"math"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/eventbus"
	"github.com/nextlevelbuilder/goclaw/internal/providers"
	"github.com/nextlevelbuilder/goclaw/internal/sessions"
	"github.com/nextlevelbuilder/goclaw/internal/store"
)

// PollWorker captures human-team-typed replies on a Zalo OA channel by
// polling /onbehalf/conversation per recently-active partner. OA-side
// messages (src_id == self UID) are persisted as assistant messages with
// metadata.source="team" + emit team.reply.observed events.
//
// v1 intentionally does NOT distinguish bot-API vs Manager-app sends —
// both look like "team" sends. Phase 5 judge worker dedups against recent
// content if needed.
type PollWorker struct {
	instanceID   uuid.UUID
	instanceName string
	tenantID     string
	channelType  string

	interval time.Duration
	onBehalf *OnBehalfClient

	sessions store.SessionCoreStore
	evals    store.TeamReplyEvalStore
	atomic   store.AtomicTeamReplyWriter
	bus      eventbus.DomainEventBus

	cursorMu sync.Mutex
	cursors  map[string]int64 // uid → last-seen msg time (Unix ms)

	customerLast func(ctx context.Context, sessionKey string) string
	selfUID      string
	judgeMode    string
	agentKey     string

	stopOnce sync.Once
	stopCh   chan struct{}
	runWG    sync.WaitGroup
}

// PollWorkerDeps groups required collaborators. Keeps the constructor
// signature stable as new fields are added.
type PollWorkerDeps struct {
	OnBehalf     *OnBehalfClient
	Sessions     store.SessionCoreStore
	Evals        store.TeamReplyEvalStore
	Atomic       store.AtomicTeamReplyWriter
	Bus          eventbus.DomainEventBus
	CustomerLast func(ctx context.Context, sessionKey string) string
	JudgeMode    string // "per_event" (default) or "scheduled" — when scheduled, publish is suppressed; JudgeScheduler grades pending rows on cron tick
	AgentKey     string // canonical agent identifier; "" falls back to legacy zalo_oa:<uid> session key
}

func NewPollWorker(instanceID uuid.UUID, name, tenantID, channelType, selfUID string,
	interval time.Duration, deps PollWorkerDeps) *PollWorker {
	if interval <= 0 {
		interval = 60 * time.Second
	}
	return &PollWorker{
		instanceID:   instanceID,
		instanceName: name,
		tenantID:     tenantID,
		channelType:  channelType,
		interval:     interval,
		onBehalf:     deps.OnBehalf,
		sessions:     deps.Sessions,
		evals:        deps.Evals,
		atomic:       deps.Atomic,
		bus:          deps.Bus,
		customerLast: deps.CustomerLast,
		selfUID:      selfUID,
		judgeMode:    deps.JudgeMode,
		agentKey:     deps.AgentKey,
		cursors:      make(map[string]int64),
		stopCh:       make(chan struct{}),
	}
}

// Run blocks until ctx is cancelled or Stop() is called. Safe to invoke
// in a goroutine. Stop waits via runWG for in-flight ticks to drain.
func (w *PollWorker) Run(ctx context.Context) {
	if w == nil || w.onBehalf == nil {
		return
	}
	w.runWG.Add(1)
	defer w.runWG.Done()
	t := time.NewTicker(w.interval)
	defer t.Stop()
	slog.Info("oa.poll_worker.start",
		"instance", w.instanceName, "interval", w.interval.String())
	// First tick immediately to avoid 60s warmup window.
	w.tick(ctx)
	for {
		select {
		case <-ctx.Done():
			slog.Info("oa.poll_worker.ctx_done", "instance", w.instanceName)
			return
		case <-w.stopCh:
			slog.Info("oa.poll_worker.stop", "instance", w.instanceName)
			return
		case <-t.C:
			w.tick(ctx)
		}
	}
}

// Stop signals the Run loop to exit and blocks until in-flight work
// drains. Idempotent.
func (w *PollWorker) Stop() {
	w.stopOnce.Do(func() {
		close(w.stopCh)
	})
	w.runWG.Wait()
}

// pollPageCap is Zalo's hard cap on `count` for /listrecentchat. Asking
// for >10 returns error -210 "maximum count is 10".
const pollPageCap = 10

// tick fetches recent messages and groups by peer uid so applyMessages
// can advance per-peer cursors. /v2.0/oa/listrecentchat returns a flat
// list of messages across all users; group locally — no per-peer
// follow-up API call needed.
func (w *PollWorker) tick(ctx context.Context) {
	msgs, err := w.onBehalf.ListRecentMessages(ctx, 0, pollPageCap)
	if err != nil {
		w.classifyErr(err, "list_recent_chat")
		return
	}
	if len(msgs) == 0 {
		return
	}
	byPeer := make(map[string][]ConversationMessage)
	for _, m := range msgs {
		// Peer = the OTHER side of the conversation. OA→user: peer is to_id.
		// user→OA: peer is from_id. Use whichever != selfUID.
		peer := m.SrcID
		if w.selfUID != "" && m.SrcID == w.selfUID {
			peer = m.DstID
		}
		if peer == "" {
			continue
		}
		byPeer[peer] = append(byPeer[peer], m)
	}
	for peer, group := range byPeer {
		w.applyMessages(ctx, peer, group)
	}
}

func (w *PollWorker) applyMessages(ctx context.Context, uid string, msgs []ConversationMessage) {
	last := w.cursor(uid)
	ctx2 := ctx
	if tid, err := uuid.Parse(w.tenantID); err == nil {
		ctx2 = store.WithTenantID(ctx, tid)
	}
	sessionKey := w.sessionKeyFor(uid)
	threadKey := "direct:" + uid
	type outcome struct {
		t     int64
		retry bool
	}
	outcomes := make([]outcome, 0, len(msgs))
	for _, m := range msgs {
		if m.Time <= last || m.MsgID == "" {
			outcomes = append(outcomes, outcome{t: m.Time, retry: false})
			continue
		}
		if m.SrcID == uid {
			outcomes = append(outcomes, outcome{t: m.Time, retry: false})
			continue
		}
		if w.selfUID != "" && m.SrcID != w.selfUID {
			outcomes = append(outcomes, outcome{t: m.Time, retry: false})
			continue
		}
		if err := w.persistTeamReply(ctx2, uid, threadKey, sessionKey, m); err != nil {
			outcomes = append(outcomes, outcome{t: m.Time, retry: true})
			continue
		}
		outcomes = append(outcomes, outcome{t: m.Time, retry: false})
	}
	// Never advance cursor past a failed message — next tick must retry it.
	var minRetry int64 = math.MaxInt64
	var maxOK int64 = last
	for _, o := range outcomes {
		if o.retry && o.t > 0 && o.t < minRetry {
			minRetry = o.t
		}
	}
	for _, o := range outcomes {
		if o.retry {
			continue
		}
		if o.t > maxOK && o.t < minRetry {
			maxOK = o.t
		}
	}
	if maxOK > last {
		w.setCursor(uid, maxOK)
	}
}

// Returns err only when atomic write fails; bus.Publish dedupes via SourceID.
func (w *PollWorker) persistTeamReply(ctx context.Context, uid, threadKey, sessionKey string, m ConversationMessage) error {
	captured := time.UnixMilli(m.Time).UTC()
	if m.Time == 0 {
		captured = time.Now().UTC()
	}
	evalRow := store.TeamReplyEvaluation{
		ChannelInstanceID: w.instanceID.String(),
		TenantID:          w.tenantID,
		ThreadKey:         threadKey,
		SessionKey:        sessionKey,
		TeamMsgID:         m.MsgID,
		CapturedAt:        captured,
		TeamReply:         m.Text,
	}
	customer := ""
	if w.customerLast != nil {
		customer = w.customerLast(ctx, sessionKey)
		evalRow.CustomerMessage = customer
	}
	msg := providers.Message{
		Role:    "assistant",
		Content: m.Text,
		Metadata: map[string]any{
			"source":      providers.MessageSourceTeam,
			"team_msg_id": m.MsgID,
			"captured_at": captured.Format(time.RFC3339Nano),
			"poll_origin": true,
		},
	}

	var evalID string
	var wasNew bool
	if w.atomic != nil {
		var err error
		evalID, wasNew, err = w.atomic.WriteTeamReplyAtomic(ctx, evalRow, sessionKey, msg)
		if err != nil {
			slog.Warn("oa.poll_worker.atomic_write_fail",
				"instance", w.instanceName, "msg_id", m.MsgID, "err", err)
			return err
		}
	} else {
		if existing, _ := w.evals.GetByMessageID(ctx, w.instanceID.String(), m.MsgID); existing != nil {
			return nil
		}
		var err error
		evalID, err = w.evals.Insert(ctx, evalRow)
		if err != nil {
			slog.Warn("oa.poll_worker.eval_insert_fail",
				"instance", w.instanceName, "msg_id", m.MsgID, "err", err)
			return err
		}
		w.sessions.AddMessage(ctx, sessionKey, msg)
		if err := w.sessions.Save(ctx, sessionKey); err != nil {
			slog.Warn("oa.poll_worker.session_save_fail",
				"instance", w.instanceName, "session", sessionKey, "err", err)
		}
		wasNew = true
	}

	if !wasNew {
		return nil
	}

	event := eventbus.DomainEvent{
		ID:        uuid.NewString(),
		Type:      eventbus.EventTeamReplyObserved,
		SourceID:  eventbus.TeamReplyObservedSourceID(w.instanceID.String(), m.MsgID),
		TenantID:  w.tenantID,
		Timestamp: time.Now().UTC(),
		Payload: eventbus.TeamReplyObservedPayload{
			EvaluationID:      evalID,
			TenantID:          w.tenantID,
			ChannelInstanceID: w.instanceID.String(),
			ChannelName:       w.instanceName,
			ThreadKey:         threadKey,
			SessionKey:        sessionKey,
			TeamMsgID:         m.MsgID,
			TeamReply:         m.Text,
			CustomerMessage:   customer,
			CapturedAt:        captured,
		},
	}
	if w.bus != nil && w.judgeMode != "scheduled" {
		w.bus.Publish(event)
	}
	return nil
}

func (w *PollWorker) classifyErr(err error, op string) {
	if errors.Is(err, ErrInvalidRefreshToken) {
		slog.Error("oa.poll_worker.refresh_token_invalid",
			"instance", w.instanceName, "op", op,
			"action_required", "re-consent OA in Credentials tab")
		return
	}
	if errors.Is(err, ErrRateLimit) {
		slog.Warn("oa.poll_worker.rate_limited", "instance", w.instanceName, "op", op)
		return
	}
	slog.Warn("oa.poll_worker.tick_error", "instance", w.instanceName, "op", op, "err", err)
}

func (w *PollWorker) cursor(uid string) int64 {
	w.cursorMu.Lock()
	defer w.cursorMu.Unlock()
	return w.cursors[uid]
}

func (w *PollWorker) setCursor(uid string, t int64) {
	w.cursorMu.Lock()
	defer w.cursorMu.Unlock()
	w.cursors[uid] = t
}

// SeedCursorsForTest lets tests pre-populate cursors. Production cursor
// state is per-pod in-memory (single-pod assumption documented in plan).
func (w *PollWorker) SeedCursorsForTest(c map[string]int64) {
	w.cursorMu.Lock()
	defer w.cursorMu.Unlock()
	for k, v := range c {
		w.cursors[k] = v
	}
}

func (w *PollWorker) sessionKeyFor(uid string) string {
	if w.agentKey != "" {
		return sessions.BuildSessionKey(w.agentKey, w.instanceName, sessions.PeerDirect, uid)
	}
	return w.channelType + ":" + uid
}
