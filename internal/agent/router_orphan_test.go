package agent

import (
	"context"
	"database/sql"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/store"
)

// orphanTracingStore is a TracingStore stub configurable per test case.
// Unused methods panic via the embedded interface.
type orphanTracingStore struct {
	store.TracingStore

	returnID     uuid.UUID
	returnStatus string
	returnErr    error
	calls        atomic.Int64
	gotTenant    atomic.Pointer[uuid.UUID]
}

func (s *orphanTracingStore) GetTraceByRunID(_ context.Context, _ string, tenantID uuid.UUID) (uuid.UUID, string, error) {
	s.calls.Add(1)
	tid := tenantID
	s.gotTenant.Store(&tid)
	return s.returnID, s.returnStatus, s.returnErr
}

func newOrphanRouter(ts store.TracingStore) (*Router, *mockTraceCollector) {
	tc := &mockTraceCollector{storeReturn: ts}
	r := NewRouter()
	r.SetTraceCollector(tc)
	return r, tc
}

// TestAbortRun_Orphan_DBHasRunning: map miss + DB has running row → Orphaned=true.
func TestAbortRun_Orphan_DBHasRunning(t *testing.T) {
	traceID := uuid.New()
	tenantID := uuid.New()
	ts := &orphanTracingStore{returnID: traceID, returnStatus: "running"}
	r, tc := newOrphanRouter(ts)

	res := r.AbortRun("missing-run", "", tenantID)
	if !res.Orphaned || res.NotFound {
		t.Fatalf("expected Orphaned=true NotFound=false, got %+v", res)
	}
	// FinishTrace must have been called with cancelled status.
	time.Sleep(20 * time.Millisecond) // FinishTrace is sync; small slack
	if tc.callCount() != 1 {
		t.Fatalf("expected 1 FinishTrace call, got %d", tc.callCount())
	}
	if tc.calls[0].TraceID != traceID || tc.calls[0].Status != "cancelled" {
		t.Fatalf("FinishTrace wrong args: %+v", tc.calls[0])
	}
	if got := ts.gotTenant.Load(); got == nil || *got != tenantID {
		t.Fatalf("expected tenant %s passed to GetTraceByRunID, got %v", tenantID, got)
	}
}

// TestAbortRun_Orphan_DBHasTerminal: map miss + DB row already terminal → NotFound.
func TestAbortRun_Orphan_DBHasTerminal(t *testing.T) {
	ts := &orphanTracingStore{returnID: uuid.New(), returnStatus: "completed"}
	r, tc := newOrphanRouter(ts)

	res := r.AbortRun("missing-run", "", uuid.Nil)
	if res.Orphaned || !res.NotFound {
		t.Fatalf("expected Orphaned=false NotFound=true, got %+v", res)
	}
	if tc.callCount() != 0 {
		t.Fatalf("expected 0 FinishTrace calls on terminal row, got %d", tc.callCount())
	}
}

// TestAbortRun_Orphan_DBNoRow: map miss + DB returns ErrNoRows → NotFound.
func TestAbortRun_Orphan_DBNoRow(t *testing.T) {
	ts := &orphanTracingStore{returnErr: sql.ErrNoRows}
	r, tc := newOrphanRouter(ts)

	res := r.AbortRun("missing-run", "", uuid.Nil)
	if res.Orphaned || !res.NotFound {
		t.Fatalf("expected Orphaned=false NotFound=true, got %+v", res)
	}
	if tc.callCount() != 0 {
		t.Fatalf("expected 0 FinishTrace calls on no row, got %d", tc.callCount())
	}
}

// TestAbortRun_Orphan_NilCollector: collector nil → NotFound, no panic.
func TestAbortRun_Orphan_NilCollector(t *testing.T) {
	r := NewRouter() // no SetTraceCollector
	res := r.AbortRun("missing-run", "", uuid.Nil)
	if res.Orphaned || !res.NotFound {
		t.Fatalf("expected Orphaned=false NotFound=true with nil collector, got %+v", res)
	}
}

// TestAbortRun_Orphan_TenantPassedThrough: tenant filter reaches GetTraceByRunID.
func TestAbortRun_Orphan_TenantPassedThrough(t *testing.T) {
	tenantA := uuid.New()
	ts := &orphanTracingStore{returnErr: sql.ErrNoRows}
	r, _ := newOrphanRouter(ts)

	_ = r.AbortRun("any", "", tenantA)
	if got := ts.gotTenant.Load(); got == nil || *got != tenantA {
		t.Fatalf("expected tenant %s, got %v", tenantA, got)
	}
}
