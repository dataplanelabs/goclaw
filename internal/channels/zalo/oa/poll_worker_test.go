package oa

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/eventbus"
	"github.com/nextlevelbuilder/goclaw/internal/providers"
	"github.com/nextlevelbuilder/goclaw/internal/store"
)

type fakeSessionCore struct {
	mu     sync.Mutex
	added  []providers.Message
	saved  int
}

func (f *fakeSessionCore) GetOrCreate(context.Context, string) *store.SessionData { return nil }
func (f *fakeSessionCore) Get(context.Context, string) *store.SessionData         { return nil }
func (f *fakeSessionCore) AddMessage(_ context.Context, _ string, msg providers.Message) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.added = append(f.added, msg)
}
func (f *fakeSessionCore) GetHistory(context.Context, string) []providers.Message { return nil }
func (f *fakeSessionCore) GetSummary(context.Context, string) string              { return "" }
func (f *fakeSessionCore) SetSummary(context.Context, string, string)             {}
func (f *fakeSessionCore) GetLabel(context.Context, string) string                { return "" }
func (f *fakeSessionCore) SetLabel(context.Context, string, string)               {}
func (f *fakeSessionCore) SetAgentInfo(context.Context, string, uuid.UUID, string) {}
func (f *fakeSessionCore) TruncateHistory(context.Context, string, int)            {}
func (f *fakeSessionCore) SetHistory(context.Context, string, []providers.Message) {}
func (f *fakeSessionCore) Reset(context.Context, string)                           {}
func (f *fakeSessionCore) Delete(context.Context, string) error                    { return nil }
func (f *fakeSessionCore) Save(context.Context, string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.saved++
	return nil
}

type fakeEvalStore struct {
	mu          sync.Mutex
	rows        []store.TeamReplyEvaluation
	byKey       map[string]string // (channel_instance|msg_id) → id
	lastInsertTenant uuid.UUID    // captures store.TenantIDFromContext on Insert
}

func newFakeEvalStore() *fakeEvalStore {
	return &fakeEvalStore{byKey: make(map[string]string)}
}

func (f *fakeEvalStore) Insert(ctx context.Context, e store.TeamReplyEvaluation) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.lastInsertTenant = store.TenantIDFromContext(ctx)
	key := e.ChannelInstanceID + "|" + e.TeamMsgID
	if id, ok := f.byKey[key]; ok {
		return id, nil
	}
	id := uuid.NewString()
	f.byKey[key] = id
	e.ID = id
	f.rows = append(f.rows, e)
	return id, nil
}
func (f *fakeEvalStore) UpdateJudgeVerdict(context.Context, string, string, float64, string, string, string, string, int) error {
	return nil
}
func (f *fakeEvalStore) MarkJudgeError(context.Context, string, string) error { return nil }
func (f *fakeEvalStore) List(context.Context, string, store.TeamReplyEvalFilter) ([]store.TeamReplyEvaluation, error) {
	return nil, nil
}
func (f *fakeEvalStore) GetByMessageID(context.Context, string, string) (*store.TeamReplyEvaluation, error) {
	return nil, nil
}
func (f *fakeEvalStore) ListPendingJudge(context.Context, int) ([]store.TeamReplyEvaluation, error) {
	return nil, nil
}
func (f *fakeEvalStore) DeleteByChannel(context.Context, string) (int64, error) { return 0, nil }

type fakeBus struct {
	mu        sync.Mutex
	published []eventbus.DomainEvent
}

func (f *fakeBus) Publish(e eventbus.DomainEvent) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.published = append(f.published, e)
}
func (f *fakeBus) Subscribe(eventbus.EventType, eventbus.DomainEventHandler) func() { return func() {} }
func (f *fakeBus) Start(context.Context)                                            {}
func (f *fakeBus) Drain(time.Duration) error                                        { return nil }

func TestPollWorker_PersistsOASideMessages(t *testing.T) {
	tenantID := uuid.NewString()
	instID := uuid.New()
	sess := &fakeSessionCore{}
	ev := newFakeEvalStore()
	bus := &fakeBus{}
	w := NewPollWorker(instID, "zalo-oa-test", tenantID, "zalo_oa", "oa-self",
		1*time.Second, PollWorkerDeps{Sessions: sess, Evals: ev, Bus: bus})

	msgs := []ConversationMessage{
		{MsgID: "m1", SrcID: "oa-self", DstID: "u1", Type: "text", Text: "hi sir", Time: 1735041000000},
		{MsgID: "m2", SrcID: "u1", DstID: "oa-self", Type: "text", Text: "who?", Time: 1735041001000},
		{MsgID: "m3", SrcID: "oa-self", DstID: "u1", Type: "text", Text: "boss", Time: 1735041002000},
	}
	w.applyMessages(context.Background(), "u1", msgs)

	if len(ev.rows) != 2 {
		t.Fatalf("expected 2 OA-side rows, got %d", len(ev.rows))
	}
	if len(sess.added) != 2 || sess.added[0].Content != "hi sir" || sess.added[1].Content != "boss" {
		t.Fatalf("unexpected session adds: %+v", sess.added)
	}
	if sess.added[0].Metadata["source"] != providers.MessageSourceTeam {
		t.Fatalf("source tag missing: %+v", sess.added[0].Metadata)
	}
	if len(bus.published) != 2 || bus.published[0].Type != eventbus.EventTeamReplyObserved {
		t.Fatalf("wrong events: %+v", bus.published)
	}
	if bus.published[0].SourceID != eventbus.TeamReplyObservedSourceID(instID.String(), "m1") {
		t.Fatalf("source ID wrong: %s", bus.published[0].SourceID)
	}
}

func TestPollWorker_SkipsCustomerSideMessages(t *testing.T) {
	tenantID := uuid.NewString()
	instID := uuid.New()
	sess := &fakeSessionCore{}
	ev := newFakeEvalStore()
	bus := &fakeBus{}
	w := NewPollWorker(instID, "zalo-oa-test", tenantID, "zalo_oa", "oa-self",
		1*time.Second, PollWorkerDeps{Sessions: sess, Evals: ev, Bus: bus})

	msgs := []ConversationMessage{
		{MsgID: "c1", SrcID: "u1", DstID: "oa-self", Type: "text", Text: "hi", Time: 1735041000000},
	}
	w.applyMessages(context.Background(), "u1", msgs)
	if len(ev.rows) != 0 || len(sess.added) != 0 || len(bus.published) != 0 {
		t.Fatalf("customer-side leaked: rows=%d added=%d bus=%d", len(ev.rows), len(sess.added), len(bus.published))
	}
}

func TestPollWorker_CursorPreventsDuplicate(t *testing.T) {
	tenantID := uuid.NewString()
	instID := uuid.New()
	sess := &fakeSessionCore{}
	ev := newFakeEvalStore()
	bus := &fakeBus{}
	w := NewPollWorker(instID, "zalo-oa-test", tenantID, "zalo_oa", "oa-self",
		1*time.Second, PollWorkerDeps{Sessions: sess, Evals: ev, Bus: bus})

	msgs := []ConversationMessage{
		{MsgID: "m1", SrcID: "oa-self", DstID: "u1", Type: "text", Text: "first", Time: 1735041000000},
	}
	w.applyMessages(context.Background(), "u1", msgs)
	w.applyMessages(context.Background(), "u1", msgs) // re-run same batch
	if len(ev.rows) != 1 {
		t.Fatalf("cursor failed dedup: %d rows", len(ev.rows))
	}
}

func TestPollWorker_DropsForeignSrcID(t *testing.T) {
	tenantID := uuid.NewString()
	instID := uuid.New()
	sess := &fakeSessionCore{}
	ev := newFakeEvalStore()
	bus := &fakeBus{}
	w := NewPollWorker(instID, "zalo-oa-test", tenantID, "zalo_oa", "oa-self",
		1*time.Second, PollWorkerDeps{Sessions: sess, Evals: ev, Bus: bus})

	msgs := []ConversationMessage{
		{MsgID: "x1", SrcID: "other-oa", DstID: "u1", Type: "text", Text: "x", Time: 1735041000000},
	}
	w.applyMessages(context.Background(), "u1", msgs)
	if len(ev.rows) != 0 {
		t.Fatalf("foreign src leaked: %d rows", len(ev.rows))
	}
}

// TestPollWorker_PropagatesTenantContext is a regression guard against
// silently losing tenant scope on downstream store calls. Pre-v3.22.1
// the worker resolved tenant via a tenant-scoped Get(ctx, instanceID)
// from background ctx, which returned ErrNoRows and prevented startup.
// Even after startup is fixed, the chain that follows MUST stamp the
// tenant on every store call or PG writes fall back to master tenant
// and leak cross-tenant.
func TestPollWorker_PropagatesTenantContext(t *testing.T) {
	tenantUUID := uuid.New()
	instID := uuid.New()
	sess := &fakeSessionCore{}
	ev := newFakeEvalStore()
	bus := &fakeBus{}
	w := NewPollWorker(instID, "zalo-oa-test", tenantUUID.String(), "zalo_oa", "oa-self",
		1*time.Second, PollWorkerDeps{Sessions: sess, Evals: ev, Bus: bus})

	msgs := []ConversationMessage{
		{MsgID: "tenant-ctx-1", SrcID: "oa-self", DstID: "u1", Type: "text", Text: "x", Time: 1735041000000},
	}
	// background ctx — emulates exactly what tick() passes in production.
	w.applyMessages(context.Background(), "u1", msgs)

	if ev.lastInsertTenant == uuid.Nil {
		t.Fatal("Insert received ctx with NO tenant — downstream PG would fall back to master_tenant")
	}
	if ev.lastInsertTenant != tenantUUID {
		t.Fatalf("Insert tenant ctx = %s, want %s", ev.lastInsertTenant, tenantUUID)
	}
}
