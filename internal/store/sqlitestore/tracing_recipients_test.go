//go:build sqlite || sqliteonly

package sqlitestore

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/store"
)

func newTestSQLiteTracingStore(t *testing.T) (*SQLiteTracingStore, context.Context) {
	t.Helper()
	db, err := OpenDB(filepath.Join(t.TempDir(), "tracing.db"))
	if err != nil {
		t.Fatalf("OpenDB error: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := EnsureSchema(db); err != nil {
		t.Fatalf("EnsureSchema error: %v", err)
	}
	ctx := store.WithTenantID(context.Background(), store.MasterTenantID)
	return NewSQLiteTracingStore(db), ctx
}

func mustCreateTrace(t *testing.T, s *SQLiteTracingStore, ctx context.Context, userID, sessionKey, channel string, start time.Time) {
	t.Helper()
	tr := &store.TraceData{
		ID:         store.GenNewID(),
		UserID:     userID,
		SessionKey: sessionKey,
		Channel:    channel,
		StartTime:  start,
		Status:     store.TraceStatusCompleted,
		CreatedAt:  start,
	}
	if err := s.CreateTrace(ctx, tr); err != nil {
		t.Fatalf("CreateTrace(%s) error: %v", userID, err)
	}
}

func TestSQLiteListTraceRecipients_DistinctPerUser(t *testing.T) {
	s, ctx := newTestSQLiteTracingStore(t)
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	// alice has two traces; the later one should be the representative row.
	mustCreateTrace(t, s, ctx, "alice", "agent:default:telegram:direct:alice", "tg", base)
	mustCreateTrace(t, s, ctx, "alice", "agent:default:telegram:direct:alice-newer", "tg2", base.Add(time.Hour))
	mustCreateTrace(t, s, ctx, "group:telegram:-100", "agent:default:telegram:group:-100", "tg", base)
	// empty user_id rows must be excluded.
	mustCreateTrace(t, s, ctx, "", "agent:default:cron:tick", "", base)

	recipients, err := s.ListTraceRecipients(ctx, store.MasterTenantID)
	if err != nil {
		t.Fatalf("ListTraceRecipients error: %v", err)
	}

	if len(recipients) != 2 {
		t.Fatalf("expected 2 distinct recipients, got %d: %#v", len(recipients), recipients)
	}

	byUser := make(map[string]store.TraceRecipient, len(recipients))
	for _, r := range recipients {
		byUser[r.UserID] = r
	}

	alice, ok := byUser["alice"]
	if !ok {
		t.Fatal("expected alice in recipients")
	}
	if alice.SessionKey != "agent:default:telegram:direct:alice-newer" {
		t.Errorf("expected newest session_key for alice, got %q", alice.SessionKey)
	}
	if alice.Channel != "tg2" {
		t.Errorf("expected channel from newest trace, got %q", alice.Channel)
	}
	if _, ok := byUser["group:telegram:-100"]; !ok {
		t.Error("expected group recipient")
	}
	if _, ok := byUser[""]; ok {
		t.Error("empty user_id must be excluded")
	}
}

func TestSQLiteListTraceRecipients_TenantScoped(t *testing.T) {
	s, ctx := newTestSQLiteTracingStore(t)
	mustCreateTrace(t, s, ctx, "alice", "agent:default:telegram:direct:alice", "tg", time.Now())

	other := uuid.MustParse("0193a5b0-7000-7000-8000-0000000000ff")
	recipients, err := s.ListTraceRecipients(ctx, other)
	if err != nil {
		t.Fatalf("ListTraceRecipients error: %v", err)
	}
	if len(recipients) != 0 {
		t.Fatalf("expected no recipients for other tenant, got %#v", recipients)
	}

	none, err := s.ListTraceRecipients(ctx, uuid.Nil)
	if err != nil {
		t.Fatalf("ListTraceRecipients(nil) error: %v", err)
	}
	if len(none) != 0 {
		t.Fatalf("expected no recipients for nil tenant, got %#v", none)
	}
}
