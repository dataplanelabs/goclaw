//go:build integration

package integration

import (
	"database/sql"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/store"
	"github.com/nextlevelbuilder/goclaw/internal/store/pg"
)

// seedFailedTrace creates a failed root trace + a captured replay payload row
// scoped to the given tenant/agent. Returns the new trace ID.
func seedFailedTrace(t *testing.T, db *sql.DB, tenantID, agentID uuid.UUID, sessionKey string, withPayload bool, outboundEmitted bool) uuid.UUID {
	return seedTraceWithStatus(t, db, tenantID, agentID, sessionKey, "error", withPayload, outboundEmitted)
}

// seedTraceWithStatus creates a root trace with the given status + optional captured payload.
func seedTraceWithStatus(t *testing.T, db *sql.DB, tenantID, agentID uuid.UUID, sessionKey, status string, withPayload bool, outboundEmitted bool) uuid.UUID {
	t.Helper()

	traceID := uuid.New()
	_, err := db.Exec(
		`INSERT INTO traces (id, tenant_id, agent_id, session_key, run_id, start_time, status, error, outbound_emitted, created_at)
		 VALUES ($1, $2, $3, $4, $5, NOW(), $6, 'seeded', $7, NOW())`,
		traceID, tenantID, agentID, sessionKey, "run-"+traceID.String()[:8], status, outboundEmitted)
	if err != nil {
		t.Fatalf("seed %s trace: %v", status, err)
	}

	if withPayload {
		envelope, _ := json.Marshal(store.RunRequestEnvelope{
			Version:  store.CurrentReplayPayloadVersion,
			Captured: time.Now().UTC(),
			Payload:  json.RawMessage(`{"session_key":"` + sessionKey + `","message":"hello"}`),
		})
		ctx := tenantCtx(tenantID)
		rps := pg.NewPGReplayPayloadStore(db)
		if err := rps.Capture(ctx, traceID, sessionKey, envelope); err != nil {
			t.Fatalf("capture payload: %v", err)
		}
	}
	return traceID
}

func TestReplayPayloadStore_CaptureAndDropOnSuccess(t *testing.T) {
	db := testDB(t)
	tenantID, agentID := seedTenantAgent(t, db)
	ctx := tenantCtx(tenantID)
	store_ := pg.NewPGReplayPayloadStore(db)

	sessionKey := "session-replay-test-" + uuid.NewString()[:8]
	traceID := seedFailedTrace(t, db, tenantID, agentID, sessionKey, true, false)

	got, err := store_.Get(ctx, traceID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Oversize || len(got.Payload) == 0 {
		t.Fatalf("expected non-oversize row with payload, got oversize=%v len=%d", got.Oversize, len(got.Payload))
	}
	if got.Version != store.CurrentReplayPayloadVersion {
		t.Fatalf("expected version %d, got %d", store.CurrentReplayPayloadVersion, got.Version)
	}

	dropped, err := store_.DropForSession(ctx, sessionKey, time.Now().UTC().Add(1*time.Second))
	if err != nil {
		t.Fatalf("drop: %v", err)
	}
	if dropped != 1 {
		t.Fatalf("expected 1 dropped, got %d", dropped)
	}

	_, err = store_.Get(ctx, traceID)
	if err == nil {
		t.Fatal("expected sql.ErrNoRows after drop, got nil")
	}
}

func TestReplayPayloadStore_TenantIsolation(t *testing.T) {
	db := testDB(t)
	tenantA, agentA := seedTenantAgent(t, db)
	tenantB, agentB := seedTenantAgent(t, db)
	rps := pg.NewPGReplayPayloadStore(db)

	sessionKey := "shared-key"
	traceA := seedFailedTrace(t, db, tenantA, agentA, sessionKey, true, false)
	_ = seedFailedTrace(t, db, tenantB, agentB, sessionKey, true, false)

	// Tenant B's DropForSession must not touch tenant A's row.
	ctxB := tenantCtx(tenantB)
	dropped, err := rps.DropForSession(ctxB, sessionKey, time.Now().UTC().Add(1*time.Second))
	if err != nil {
		t.Fatalf("drop B: %v", err)
	}
	if dropped != 1 {
		t.Fatalf("tenant B expected to drop 1 row, got %d", dropped)
	}

	// Tenant A's row must still exist.
	ctxA := tenantCtx(tenantA)
	rowA, err := rps.Get(ctxA, traceA)
	if err != nil || rowA == nil {
		t.Fatalf("tenant A row should remain; got err=%v row=%v", err, rowA)
	}
}

func TestReplayPayloadStore_OversizeSentinel(t *testing.T) {
	db := testDB(t)
	tenantID, _ := seedTenantAgent(t, db)
	ctx := tenantCtx(tenantID)
	rps := pg.NewPGReplayPayloadStore(db)

	// Seed a parent trace (FK to traces required).
	traceID := uuid.New()
	_, err := db.Exec(
		`INSERT INTO traces (id, tenant_id, session_key, start_time, status, created_at)
		 VALUES ($1, $2, $3, NOW(), 'error', NOW())`,
		traceID, tenantID, "oversize-session")
	if err != nil {
		t.Fatalf("seed trace: %v", err)
	}

	if err := rps.CaptureOversize(ctx, traceID, "oversize-session", 4_000_000); err != nil {
		t.Fatalf("capture oversize: %v", err)
	}
	got, err := rps.Get(ctx, traceID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if !got.Oversize {
		t.Fatal("expected oversize=true")
	}
	if got.ByteSize != 4_000_000 {
		t.Fatalf("expected byte_size=4_000_000, got %d", got.ByteSize)
	}
	if len(got.Payload) != 0 {
		t.Fatalf("expected empty payload for oversize, got %d bytes", len(got.Payload))
	}
}

func TestRetryLockStore_AcquireAndContention(t *testing.T) {
	db := testDB(t)
	tenantID, agentID := seedTenantAgent(t, db)
	ctx := tenantCtx(tenantID)
	locks := pg.NewPGRetryLockStore(db)

	traceID := seedFailedTrace(t, db, tenantID, agentID, "lock-session", true, false)
	user1 := uuid.New()
	user2 := uuid.New()

	acquired, err := locks.Acquire(ctx, traceID, user1, 60*time.Second)
	if err != nil || !acquired {
		t.Fatalf("first acquire should succeed: acquired=%v err=%v", acquired, err)
	}

	// Second concurrent acquire must fail.
	acquired2, err := locks.Acquire(ctx, traceID, user2, 60*time.Second)
	if err != nil {
		t.Fatalf("second acquire err: %v", err)
	}
	if acquired2 {
		t.Fatal("second acquire should fail while lock is hot")
	}

	// Release and re-acquire.
	if err := locks.Release(ctx, traceID); err != nil {
		t.Fatalf("release: %v", err)
	}
	acquired3, err := locks.Acquire(ctx, traceID, user2, 60*time.Second)
	if err != nil || !acquired3 {
		t.Fatalf("post-release acquire should succeed: acquired=%v err=%v", acquired3, err)
	}
}

func TestRetryLockStore_ExpiredLockReclaim(t *testing.T) {
	db := testDB(t)
	tenantID, agentID := seedTenantAgent(t, db)
	ctx := tenantCtx(tenantID)
	locks := pg.NewPGRetryLockStore(db)

	traceID := seedFailedTrace(t, db, tenantID, agentID, "expiry-session", true, false)
	user1 := uuid.New()
	user2 := uuid.New()

	// Acquire with a 1s TTL — sleep past it before the second call uses the same TTL.
	const ttl = 1 * time.Second
	acquired, err := locks.Acquire(ctx, traceID, user1, ttl)
	if err != nil || !acquired {
		t.Fatalf("first acquire: acquired=%v err=%v", acquired, err)
	}
	time.Sleep(ttl + 200*time.Millisecond)

	acquired2, err := locks.Acquire(ctx, traceID, user2, ttl)
	if err != nil {
		t.Fatalf("reclaim err: %v", err)
	}
	if !acquired2 {
		t.Fatal("expected expired lock to be reclaimed by second caller")
	}
}

func TestTracesStore_SetOutboundEmittedIdempotent(t *testing.T) {
	db := testDB(t)
	tenantID, agentID := seedTenantAgent(t, db)
	ctx := tenantCtx(tenantID)
	traces := pg.NewPGTracingStore(db)

	traceID := seedFailedTrace(t, db, tenantID, agentID, "outbound-session", false, false)

	if err := traces.SetOutboundEmitted(ctx, traceID); err != nil {
		t.Fatalf("first set: %v", err)
	}
	got, err := traces.GetTrace(ctx, traceID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if !got.OutboundEmitted {
		t.Fatal("expected outbound_emitted=true after first set")
	}

	// Second call is a no-op (guard predicate). Must not error.
	if err := traces.SetOutboundEmitted(ctx, traceID); err != nil {
		t.Fatalf("second set: %v", err)
	}
}

func TestReplayPayloadStore_CancelledTraceKeepsPayload(t *testing.T) {
	db := testDB(t)
	tenantID, agentID := seedTenantAgent(t, db)
	ctx := tenantCtx(tenantID)
	rps := pg.NewPGReplayPayloadStore(db)

	// Stopped (cancelled) runs MUST keep their captured payload so admin can retry.
	traceID := seedTraceWithStatus(t, db, tenantID, agentID, "stop-session", "cancelled", true, false)

	got, err := rps.Get(ctx, traceID)
	if err != nil || got == nil {
		t.Fatalf("cancelled trace's payload must survive: err=%v row=%v", err, got)
	}
	if got.Oversize || len(got.Payload) == 0 {
		t.Fatalf("expected non-oversize payload, got oversize=%v len=%d", got.Oversize, len(got.Payload))
	}
}

func TestReplayPayloadStore_DropOnlyOlderRows(t *testing.T) {
	db := testDB(t)
	tenantID, agentID := seedTenantAgent(t, db)
	ctx := tenantCtx(tenantID)
	rps := pg.NewPGReplayPayloadStore(db)

	sessionKey := "race-session"
	traceOld := seedFailedTrace(t, db, tenantID, agentID, sessionKey, true, false)

	// Insert a fresher payload row after sweep cutoff.
	cutoff := time.Now().UTC()
	time.Sleep(10 * time.Millisecond)

	traceNew := seedFailedTrace(t, db, tenantID, agentID, sessionKey, true, false)

	dropped, err := rps.DropForSession(ctx, sessionKey, cutoff)
	if err != nil {
		t.Fatalf("drop: %v", err)
	}
	if dropped != 1 {
		t.Fatalf("expected 1 dropped (old only), got %d", dropped)
	}

	// Old row gone, new row stays.
	if _, err := rps.Get(ctx, traceOld); err == nil {
		t.Fatal("old row should be gone")
	}
	rowNew, err := rps.Get(ctx, traceNew)
	if err != nil || rowNew == nil {
		t.Fatalf("new row should survive: err=%v row=%v", err, rowNew)
	}
}
