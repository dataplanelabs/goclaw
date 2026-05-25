package consolidation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"golang.org/x/time/rate"

	"github.com/nextlevelbuilder/goclaw/internal/agent"
	"github.com/nextlevelbuilder/goclaw/internal/eventbus"
	"github.com/nextlevelbuilder/goclaw/internal/store"
)

const (
	throttleRetryMaxAttempts = 4
	throttleRetryBaseDelay   = 5 * time.Second
	throttleRetryMaxDelay    = 60 * time.Second
	throttleRetryInFlightCap = 1000
	throttleRetrySuffix      = "?throttle_retry="
)

// JudgeAgentResolver returns the agent UUID to use as the judge for the
// given tenant/channel. Lookup precedence: channel override > tenant
// default > nil (no judge configured for this tenant).
type JudgeAgentResolver func(ctx context.Context, tenantID, channelInstanceID string) (uuid.UUID, string, error)

// JudgeDeps bundles dependencies for the JudgeWorker.
type JudgeDeps struct {
	Evals    store.TeamReplyEvalStore
	Router   *agent.Router
	Resolver JudgeAgentResolver
	Bus      eventbus.DomainEventBus
	Timeout  time.Duration
}

// JudgeWorker subscribes to team.reply.observed events and invokes a
// per-tenant judge agent to grade the captured reply. Runs inline in the
// eventbus worker pool — Handle spawns a goroutine immediately so the
// worker pool stays unblocked, and per-tenant semaphore + rate limiter
// keep runaway concurrency in check.
type JudgeWorker struct {
	evals    store.TeamReplyEvalStore
	router   *agent.Router
	resolver JudgeAgentResolver
	bus      eventbus.DomainEventBus
	timeout  time.Duration

	rateLimits sync.Map // tenantID → *rate.Limiter

	inFlightRetries atomic.Int64

	rootCtx    context.Context
	rootCancel context.CancelFunc

	nowFn func() time.Time
}

func NewJudgeWorker(deps JudgeDeps) *JudgeWorker {
	timeout := deps.Timeout
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	rootCtx, rootCancel := context.WithCancel(context.Background())
	return &JudgeWorker{
		evals:      deps.Evals,
		router:     deps.Router,
		resolver:   deps.Resolver,
		bus:        deps.Bus,
		timeout:    timeout,
		rootCtx:    rootCtx,
		rootCancel: rootCancel,
		nowFn:      time.Now,
	}
}

// Stop cancels in-flight throttle-retry schedulers. AfterFunc bodies check rootCtx before publishing.
func (w *JudgeWorker) Stop() {
	if w != nil && w.rootCancel != nil {
		w.rootCancel()
	}
}

// Handle is the eventbus subscriber entry. Returns nil on permanent
// errors (so the bus doesn't retry budget/parse failures); errors are
// captured in team_reply_evaluations.judge_error.
func (w *JudgeWorker) Handle(ctx context.Context, e eventbus.DomainEvent) error {
	if w == nil {
		return nil
	}
	payload, ok := payloadAsTeamReply(e.Payload)
	if !ok {
		slog.Warn("judge.payload_type_mismatch", "event_type", e.Type)
		return nil
	}
	if !w.limiter(payload.TenantID).Allow() {
		attempt := parseThrottleAttempt(e.SourceID)
		slog.Warn("judge.throttle",
			"tenant", payload.TenantID, "evaluation_id", payload.EvaluationID, "attempt", attempt)
		w.scheduleThrottleRetry(e, payload, attempt)
		return nil
	}
	go w.process(ctx, payload)
	return nil
}

func parseThrottleAttempt(sourceID string) int {
	idx := strings.LastIndex(sourceID, throttleRetrySuffix)
	if idx < 0 {
		return 0
	}
	n, err := strconv.Atoi(sourceID[idx+len(throttleRetrySuffix):])
	if err != nil || n < 0 {
		return 0
	}
	return n
}

// scheduleThrottleRetry republishes the event after exponential backoff.
// Capped at throttleRetryMaxAttempts retries; on overflow or final exhaustion
// the row is marked judge_error so it stops being silently Pending.
func (w *JudgeWorker) scheduleThrottleRetry(orig eventbus.DomainEvent, payload eventbus.TeamReplyObservedPayload, attempt int) {
	if w.bus == nil {
		w.markErr(context.Background(), payload.EvaluationID, "throttle_no_bus")
		return
	}
	if attempt >= throttleRetryMaxAttempts {
		w.markErr(context.Background(), payload.EvaluationID, "throttle_max_retries")
		return
	}
	if w.inFlightRetries.Load() >= throttleRetryInFlightCap {
		w.markErr(context.Background(), payload.EvaluationID, "throttle_overflow")
		return
	}

	delay := time.Duration(1<<attempt) * throttleRetryBaseDelay
	if delay > throttleRetryMaxDelay {
		delay = throttleRetryMaxDelay
	}

	baseSourceID := orig.SourceID
	if idx := strings.LastIndex(baseSourceID, throttleRetrySuffix); idx >= 0 {
		baseSourceID = baseSourceID[:idx]
	}
	next := eventbus.DomainEvent{
		ID:        uuid.NewString(),
		Type:      orig.Type,
		SourceID:  baseSourceID + throttleRetrySuffix + strconv.Itoa(attempt+1),
		TenantID:  orig.TenantID,
		Timestamp: time.Now().UTC(),
		Payload:   payload,
	}

	w.inFlightRetries.Add(1)
	time.AfterFunc(delay, func() {
		defer w.inFlightRetries.Add(-1)
		if w.rootCtx.Err() != nil {
			return
		}
		w.bus.Publish(next)
	})
}

func (w *JudgeWorker) process(ctx context.Context, payload eventbus.TeamReplyObservedPayload) {
	ctx, cancel := context.WithTimeout(ctx, w.timeout)
	defer cancel()

	tenantUUID, err := uuid.Parse(payload.TenantID)
	if err != nil {
		w.markErr(ctx, payload.EvaluationID, "invalid_tenant_id: "+err.Error())
		return
	}

	if w.resolver == nil {
		w.markErr(ctx, payload.EvaluationID, "no_judge_resolver_configured")
		return
	}
	judgeID, agentKey, err := w.resolver(ctx, payload.TenantID, payload.ChannelInstanceID)
	if err != nil {
		w.markErr(ctx, payload.EvaluationID, "judge_resolver_error: "+err.Error())
		return
	}
	if judgeID == uuid.Nil {
		w.markErr(ctx, payload.EvaluationID, "no_judge_agent_configured")
		return
	}

	if w.router == nil {
		w.markErr(ctx, payload.EvaluationID, "router_unavailable")
		return
	}

	ctx2 := store.WithTenantID(ctx, tenantUUID)
	ctx2 = store.WithAgentID(ctx2, judgeID)
	ctx2 = store.WithUserID(ctx2, "system:judge-worker")

	loop, err := w.router.Get(ctx2, judgeID.String())
	if err != nil {
		w.markErr(ctx, payload.EvaluationID, "judge_agent_unavailable: "+err.Error())
		return
	}
	prompt := RenderJudgePrompt(JudgeInput{
		CustomerMessage: payload.CustomerMessage,
		TeamReply:       payload.TeamReply,
	})
	start := w.nowFn()
	result, err := loop.Run(ctx2, agent.RunRequest{
		SessionKey:    "judge:eval:" + payload.EvaluationID,
		Message:       prompt,
		UserID:        "system:judge-worker",
		ChannelType:   "system",
		MaxIterations: 1,
		RunKind:       "judge_eval",
		HideInput:     true,
		LightContext:  true,
		// Sentinel name no real tool matches → intersectWithSpec returns
		// empty set, judge gets ZERO tools. Plain empty slice means
		// "no group-level restriction" and exposes every tool to the LLM.
		ToolAllow: []string{"__judge_no_tools__"},
	})
	latency := int(w.nowFn().Sub(start).Milliseconds())
	if err != nil {
		w.markErr(ctx, payload.EvaluationID, "judge_rpc_error: "+err.Error())
		return
	}
	verdict, err := ParseJudgeResponse(result.Content)
	if err != nil {
		w.markErr(ctx, payload.EvaluationID, "judge_parse_error: "+err.Error())
		return
	}
	if err := w.evals.UpdateJudgeVerdict(ctx2, payload.EvaluationID,
		verdict.HypothesizedBotReply, verdict.DiffScore, verdict.DiffReasoning,
		loop.Model(), loop.ProviderName(), agentKey, latency); err != nil {
		slog.Warn("judge.update_failed",
			"evaluation_id", payload.EvaluationID, "err", err)
		return
	}
	slog.Info("judge.complete",
		"evaluation_id", payload.EvaluationID,
		"diff_score", verdict.DiffScore,
		"latency_ms", latency)
}

func (w *JudgeWorker) markErr(ctx context.Context, evalID, msg string) {
	if w.evals == nil {
		return
	}
	if err := w.evals.MarkJudgeError(ctx, evalID, msg); err != nil {
		slog.Warn("judge.mark_error_failed", "evaluation_id", evalID, "err", err)
	}
}

func (w *JudgeWorker) limiter(tenantID string) *rate.Limiter {
	if v, ok := w.rateLimits.Load(tenantID); ok {
		return v.(*rate.Limiter)
	}
	l := rate.NewLimiter(rate.Every(6*time.Second), 5) // ~10/min, burst 5
	actual, _ := w.rateLimits.LoadOrStore(tenantID, l)
	return actual.(*rate.Limiter)
}

// payloadAsTeamReply extracts the typed payload. Eventbus may pass the
// payload as the typed struct OR (post-JSON-roundtrip) as a generic map.
func payloadAsTeamReply(p any) (eventbus.TeamReplyObservedPayload, bool) {
	if v, ok := p.(eventbus.TeamReplyObservedPayload); ok {
		return v, true
	}
	if v, ok := p.(*eventbus.TeamReplyObservedPayload); ok && v != nil {
		return *v, true
	}
	// Last-resort: JSON roundtrip (defensive — production path uses the typed
	// struct, but tests/serialization may convert).
	if m, ok := p.(map[string]any); ok {
		b, err := json.Marshal(m)
		if err != nil {
			return eventbus.TeamReplyObservedPayload{}, false
		}
		var out eventbus.TeamReplyObservedPayload
		if json.Unmarshal(b, &out) != nil {
			return eventbus.TeamReplyObservedPayload{}, false
		}
		return out, true
	}
	return eventbus.TeamReplyObservedPayload{}, false
}

// NewTenantJudgeResolver builds a JudgeAgentResolver that resolves the
// judge agent in precedence: channel_instances.config.judge_agent_key →
// tenants.settings.judge_agent_key. Wraps the resolver ctx with the
// tenant UUID before calling agent/instance stores so tenant-scoped reads
// pick up the right row.
func NewTenantJudgeResolver(tenants store.TenantStore, agents store.AgentCRUDStore, instances store.ChannelInstanceStore) JudgeAgentResolver {
	return func(ctx context.Context, tenantID, channelInstanceID string) (uuid.UUID, string, error) {
		if tenants == nil || agents == nil {
			return uuid.Nil, "", errors.New("tenant/agent store unavailable")
		}
		tid, err := uuid.Parse(tenantID)
		if err != nil {
			return uuid.Nil, "", fmt.Errorf("parse tenant id: %w", err)
		}
		ctx = store.WithTenantID(ctx, tid)

		// 1. Channel override beats tenant default.
		var agentKey string
		if instances != nil && channelInstanceID != "" {
			if cid, err := uuid.Parse(channelInstanceID); err == nil {
				if ci, err := instances.Get(ctx, cid); err == nil && ci != nil && len(ci.Config) > 0 {
					var ccfg struct {
						JudgeAgentKey string `json:"judge_agent_key,omitempty"`
					}
					_ = json.Unmarshal(ci.Config, &ccfg)
					agentKey = ccfg.JudgeAgentKey
				}
			}
		}

		// 2. Fall back to tenant.settings.judge_agent_key.
		if agentKey == "" {
			td, err := tenants.GetTenant(ctx, tid)
			if err != nil || td == nil {
				return uuid.Nil, "", fmt.Errorf("get tenant: %w", err)
			}
			var settings struct {
				JudgeAgentKey string `json:"judge_agent_key,omitempty"`
			}
			if len(td.Settings) > 0 {
				_ = json.Unmarshal(td.Settings, &settings)
			}
			agentKey = settings.JudgeAgentKey
		}

		if agentKey == "" {
			return uuid.Nil, "", nil
		}
		ad, err := agents.GetByKey(ctx, agentKey)
		if err != nil || ad == nil {
			return uuid.Nil, agentKey, fmt.Errorf("resolve agent key %q: %w", agentKey, err)
		}
		return ad.ID, agentKey, nil
	}
}
