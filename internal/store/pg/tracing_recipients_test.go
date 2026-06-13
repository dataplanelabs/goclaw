package pg

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/nextlevelbuilder/goclaw/internal/store"
)

func tracingTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skipf("TEST_DATABASE_URL not set; skipping PG tracing store tests")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("open DB: %v", err)
	}
	if err := db.Ping(); err != nil {
		db.Close()
		t.Skipf("PG not reachable: %v", err)
	}
	m, err := migrate.New("file://../../../migrations", dsn)
	if err != nil {
		db.Close()
		t.Fatalf("migrate.New: %v", err)
	}
	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		db.Close()
		t.Fatalf("migrate up: %v", err)
	}
	InitSqlx(db)
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestPGListTraceRecipients_DistinctTenantScoped(t *testing.T) {
	db := tracingTestDB(t)
	s := NewPGTracingStore(db)
	ctx := store.WithTenantID(context.Background(), store.MasterTenantID)
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	mk := func(userID, sessionKey, channel string, start time.Time) {
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
			t.Fatalf("CreateTrace(%s): %v", userID, err)
		}
		t.Cleanup(func() { _, _ = db.Exec(`DELETE FROM traces WHERE id = $1`, tr.ID) })
	}

	uid := "pgrec-" + store.GenNewID().String()
	mk(uid, "agent:default:telegram:direct:"+uid, "tg", base)
	mk(uid, "agent:default:telegram:direct:"+uid+"-new", "tg2", base.Add(time.Hour))
	mk("", "agent:default:cron:tick", "", base)

	recipients, err := s.ListTraceRecipients(ctx, store.MasterTenantID)
	if err != nil {
		t.Fatalf("ListTraceRecipients: %v", err)
	}

	var found *store.TraceRecipient
	for i := range recipients {
		if recipients[i].UserID == uid {
			found = &recipients[i]
		}
		if recipients[i].UserID == "" {
			t.Error("empty user_id must be excluded")
		}
	}
	if found == nil {
		t.Fatalf("expected recipient %q", uid)
	}
	if found.SessionKey != "agent:default:telegram:direct:"+uid+"-new" {
		t.Errorf("expected newest session_key, got %q", found.SessionKey)
	}
}
