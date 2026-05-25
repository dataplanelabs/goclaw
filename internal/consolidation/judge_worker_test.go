package consolidation

import (
	"context"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/eventbus"
	"github.com/nextlevelbuilder/goclaw/internal/store"
)

type fakeEvalStore struct {
	mu       sync.Mutex
	updates  []struct{ id, hypo, reason, model, prov, key string; score float64; latency int }
	errs     []struct{ id, msg string }
}

func (f *fakeEvalStore) Insert(context.Context, store.TeamReplyEvaluation) (string, error) {
	return "", nil
}
func (f *fakeEvalStore) UpdateJudgeVerdict(_ context.Context, id, hypo string, score float64, reason, model, prov, key string, latency int) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.updates = append(f.updates, struct{ id, hypo, reason, model, prov, key string; score float64; latency int }{id, hypo, reason, model, prov, key, score, latency})
	return nil
}
func (f *fakeEvalStore) MarkJudgeError(_ context.Context, id, msg string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.errs = append(f.errs, struct{ id, msg string }{id, msg})
	return nil
}
func (f *fakeEvalStore) List(context.Context, string, store.TeamReplyEvalFilter) ([]store.TeamReplyEvaluation, error) {
	return nil, nil
}
func (f *fakeEvalStore) Count(context.Context, string, store.TeamReplyEvalFilter) (int64, error) {
	return 0, nil
}
func (f *fakeEvalStore) GetByMessageID(context.Context, string, string) (*store.TeamReplyEvaluation, error) {
	return nil, nil
}
func (f *fakeEvalStore) ListFailedJudge(context.Context, string, int) ([]store.TeamReplyEvaluation, error) {
	return nil, nil
}
func (f *fakeEvalStore) ClearJudgeError(context.Context, []string) (int64, error) {
	return 0, nil
}
func (f *fakeEvalStore) ListPendingJudge(context.Context, int) ([]store.TeamReplyEvaluation, error) {
	return nil, nil
}
func (f *fakeEvalStore) DeleteByChannel(context.Context, string) (int64, error) { return 0, nil }

func TestJudgeWorker_PayloadTypeMismatch(t *testing.T) {
	w := NewJudgeWorker(JudgeDeps{Evals: &fakeEvalStore{}})
	if err := w.Handle(context.Background(), eventbus.DomainEvent{Payload: "string"}); err != nil {
		t.Fatalf("Handle should not propagate type mismatch: %v", err)
	}
}

func TestJudgeWorker_NoResolverConfigured(t *testing.T) {
	ev := &fakeEvalStore{}
	w := NewJudgeWorker(JudgeDeps{Evals: ev})
	tenantID := uuid.NewString()
	err := w.Handle(context.Background(), eventbus.DomainEvent{
		Type: eventbus.EventTeamReplyObserved,
		Payload: eventbus.TeamReplyObservedPayload{
			EvaluationID: "eval-1",
			TenantID:     tenantID,
			TeamReply:    "non-empty so empty-check passes",
		},
	})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	// process() runs in a goroutine — give it a moment.
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		ev.mu.Lock()
		done := len(ev.errs) == 1
		ev.mu.Unlock()
		if done {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	ev.mu.Lock()
	defer ev.mu.Unlock()
	if len(ev.errs) != 1 || ev.errs[0].msg != "no_judge_resolver_configured" {
		t.Fatalf("expected resolver-missing error, got %+v", ev.errs)
	}
}

func TestJudgeWorker_ResolverReturnsNoAgent(t *testing.T) {
	ev := &fakeEvalStore{}
	w := NewJudgeWorker(JudgeDeps{
		Evals: ev,
		Resolver: func(context.Context, string, string) (uuid.UUID, string, error) {
			return uuid.Nil, "", nil
		},
	})
	tenantID := uuid.NewString()
	_ = w.Handle(context.Background(), eventbus.DomainEvent{
		Type: eventbus.EventTeamReplyObserved,
		Payload: eventbus.TeamReplyObservedPayload{
			EvaluationID: "eval-2",
			TenantID:     tenantID,
			TeamReply:    "non-empty so empty-check passes",
		},
	})
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		ev.mu.Lock()
		done := len(ev.errs) == 1
		ev.mu.Unlock()
		if done {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	ev.mu.Lock()
	defer ev.mu.Unlock()
	if len(ev.errs) != 1 || ev.errs[0].msg != "no_judge_agent_configured" {
		t.Fatalf("expected no-agent error, got %+v", ev.errs)
	}
}

func TestJudgeWorker_SkipsEmptyTeamReply(t *testing.T) {
	ev := &fakeEvalStore{}
	w := NewJudgeWorker(JudgeDeps{Evals: ev})
	tenantID := uuid.NewString()
	_ = w.Handle(context.Background(), eventbus.DomainEvent{
		Type: eventbus.EventTeamReplyObserved,
		Payload: eventbus.TeamReplyObservedPayload{
			EvaluationID: "eval-empty",
			TenantID:     tenantID,
			TeamReply:    "   ",
		},
	})
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		ev.mu.Lock()
		done := len(ev.errs) == 1
		ev.mu.Unlock()
		if done {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	ev.mu.Lock()
	defer ev.mu.Unlock()
	if len(ev.errs) != 1 || ev.errs[0].msg != "empty_team_reply" {
		t.Fatalf("expected empty_team_reply mark, got %+v", ev.errs)
	}
}

func TestJudgeWorker_BatchGradeFiltersEmptyTeamReply(t *testing.T) {
	ev := &fakeEvalStore{}
	bus := &stubPublishBus{}
	w := NewJudgeWorker(JudgeDeps{Evals: ev, Bus: bus})
	rows := []store.TeamReplyEvaluation{
		{ID: "r1", TenantID: uuid.NewString(), ChannelInstanceID: uuid.NewString(), TeamReply: ""},
		{ID: "r2", TenantID: uuid.NewString(), ChannelInstanceID: uuid.NewString(), TeamReply: "   "},
		{ID: "r3", TenantID: uuid.NewString(), ChannelInstanceID: uuid.NewString(), TeamReply: ""},
	}
	_ = w.BatchGrade(context.Background(), rows, "test-channel")
	ev.mu.Lock()
	defer ev.mu.Unlock()
	if len(ev.errs) != 3 {
		t.Fatalf("expected 3 empty_team_reply marks, got %d: %+v", len(ev.errs), ev.errs)
	}
	for _, e := range ev.errs {
		if e.msg != "empty_team_reply" {
			t.Fatalf("expected empty_team_reply for all, got %q", e.msg)
		}
	}
}

func TestJudgeWorker_InvalidTenantID(t *testing.T) {
	ev := &fakeEvalStore{}
	w := NewJudgeWorker(JudgeDeps{Evals: ev,
		Resolver: func(context.Context, string, string) (uuid.UUID, string, error) {
			return uuid.New(), "k", nil
		}})
	_ = w.Handle(context.Background(), eventbus.DomainEvent{
		Type: eventbus.EventTeamReplyObserved,
		Payload: eventbus.TeamReplyObservedPayload{
			EvaluationID: "eval-3",
			TenantID:     "not-a-uuid",
		},
	})
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		ev.mu.Lock()
		done := len(ev.errs) == 1
		ev.mu.Unlock()
		if done {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	ev.mu.Lock()
	defer ev.mu.Unlock()
	if len(ev.errs) != 1 || ev.errs[0].id != "eval-3" {
		t.Fatalf("expected invalid_tenant_id err: %+v", ev.errs)
	}
}

func TestJudgeWorker_RateLimiter_PerTenantIsolation(t *testing.T) {
	w := NewJudgeWorker(JudgeDeps{Evals: &fakeEvalStore{}})
	tenantA := uuid.NewString()
	tenantB := uuid.NewString()
	for i := 0; i < 5; i++ {
		if !w.limiter(tenantA).Allow() {
			t.Fatalf("tenant A burst exhausted at iter %d", i)
		}
	}
	if w.limiter(tenantA).Allow() {
		t.Fatal("tenant A should be at burst limit")
	}
	if !w.limiter(tenantB).Allow() {
		t.Fatal("tenant B must be independent")
	}
}

type stubPublishBus struct {
	mu        sync.Mutex
	published []eventbus.DomainEvent
}

func (b *stubPublishBus) Publish(e eventbus.DomainEvent) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.published = append(b.published, e)
}
func (b *stubPublishBus) Subscribe(eventbus.EventType, eventbus.DomainEventHandler) func() {
	return func() {}
}
func (b *stubPublishBus) Close()                          {}
func (b *stubPublishBus) Start(context.Context)           {}
func (b *stubPublishBus) Drain(time.Duration) error       { return nil }

func TestParseThrottleAttempt(t *testing.T) {
	cases := []struct {
		in   string
		want int
	}{
		{"", 0},
		{"foo", 0},
		{"foo?throttle_retry=3", 3},
		{"foo?throttle_retry=abc", 0},
		{"foo?rejudge=x?throttle_retry=2", 2},
		{"foo?throttle_retry=-1", 0},
	}
	for _, c := range cases {
		if got := parseThrottleAttempt(c.in); got != c.want {
			t.Errorf("parseThrottleAttempt(%q) = %d, want %d", c.in, got, c.want)
		}
	}
}

func TestJudgeWorker_ThrottleSchedulesRetry(t *testing.T) {
	evals := &fakeEvalStore{}
	bus := &stubPublishBus{}
	w := NewJudgeWorker(JudgeDeps{Evals: evals, Bus: bus})
	tenant := uuid.NewString()
	for i := 0; i < 5; i++ {
		w.limiter(tenant).Allow()
	}
	ev := eventbus.DomainEvent{
		Type:     eventbus.EventTeamReplyObserved,
		SourceID: "src-1",
		Payload:  eventbus.TeamReplyObservedPayload{EvaluationID: "eid-1", TenantID: tenant},
	}
	if err := w.Handle(context.Background(), ev); err != nil {
		t.Fatalf("Handle err: %v", err)
	}
	if got := w.inFlightRetries.Load(); got != 1 {
		t.Fatalf("inFlightRetries=%d want 1", got)
	}
}

func TestJudgeWorker_ThrottleMaxRetries(t *testing.T) {
	evals := &fakeEvalStore{}
	bus := &stubPublishBus{}
	w := NewJudgeWorker(JudgeDeps{Evals: evals, Bus: bus})
	tenant := uuid.NewString()
	for i := 0; i < 5; i++ {
		w.limiter(tenant).Allow()
	}
	ev := eventbus.DomainEvent{
		Type:     eventbus.EventTeamReplyObserved,
		SourceID: "src-2?throttle_retry=" + strconv.Itoa(throttleRetryMaxAttempts),
		Payload:  eventbus.TeamReplyObservedPayload{EvaluationID: "eid-2", TenantID: tenant},
	}
	if err := w.Handle(context.Background(), ev); err != nil {
		t.Fatalf("Handle err: %v", err)
	}
	if w.inFlightRetries.Load() != 0 {
		t.Fatalf("should not schedule retry at max attempts")
	}
	found := false
	for _, e := range evals.errs {
		if e.id == "eid-2" && e.msg == "throttle_max_retries" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected throttle_max_retries markErr; got %+v", evals.errs)
	}
}

func TestJudgeWorker_ThrottleOverflowCap(t *testing.T) {
	evals := &fakeEvalStore{}
	bus := &stubPublishBus{}
	w := NewJudgeWorker(JudgeDeps{Evals: evals, Bus: bus})
	w.inFlightRetries.Store(throttleRetryInFlightCap)
	tenant := uuid.NewString()
	for i := 0; i < 5; i++ {
		w.limiter(tenant).Allow()
	}
	ev := eventbus.DomainEvent{
		Type:     eventbus.EventTeamReplyObserved,
		SourceID: "src-3",
		Payload:  eventbus.TeamReplyObservedPayload{EvaluationID: "eid-3", TenantID: tenant},
	}
	_ = w.Handle(context.Background(), ev)
	found := false
	for _, e := range evals.errs {
		if e.id == "eid-3" && e.msg == "throttle_overflow" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected throttle_overflow markErr; got %+v", evals.errs)
	}
}

func TestPayloadAsTeamReply_MapRoundTrip(t *testing.T) {
	m := map[string]any{
		"EvaluationID": "eid",
		"TenantID":     "tid",
		"TeamReply":    "yo",
	}
	v, ok := payloadAsTeamReply(m)
	if !ok || v.EvaluationID != "eid" || v.TeamReply != "yo" {
		t.Fatalf("roundtrip failed: %+v ok=%v", v, ok)
	}
}
