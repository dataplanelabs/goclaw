//go:build sqlite || sqliteonly

package sqlitestore

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/nextlevelbuilder/goclaw/internal/store"
)

func newTestHabitStore(t *testing.T) *SQLiteHabitChecklistStore {
	t.Helper()
	db, err := OpenDB(filepath.Join(t.TempDir(), "habit.db"))
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := EnsureSchema(db); err != nil {
		t.Fatalf("EnsureSchema: %v", err)
	}
	return NewSQLiteHabitChecklistStore(db)
}

func TestHabitChecklist_DispatcherGateAndCompletion(t *testing.T) {
	s := newTestHabitStore(t)
	ctx := context.Background()
	sc := store.HabitScope{
		TenantID: "00000000-0000-0000-0000-000000000001",
		AgentID:  "00000000-0000-0000-0000-0000000000a1",
		UserID:   "group:testchan:900000000000000001",
	}
	const day = "2026-06-23"
	seed := func(key, title, at string) {
		if err := s.Seed(ctx, store.HabitEntry{TenantID: sc.TenantID, AgentID: sc.AgentID, UserID: sc.UserID, PlanDate: day, TaskKey: key, Title: title, ScheduledLocal: at}); err != nil {
			t.Fatalf("seed %s: %v", key, err)
		}
	}
	seed("guzheng", "Luyện guzheng", "10:00")
	seed("run", "Chạy bộ", "16:00")
	seed("read", "Đọc sách", "") // anytime

	keys := func(es []store.HabitEntry) []string {
		var ks []string
		for _, e := range es {
			ks = append(ks, e.TaskKey)
		}
		return ks
	}

	// At 12:00: guzheng (due) + read (anytime) are pending+due; run (16:00) is not yet due.
	due, err := s.ListPendingDue(ctx, sc, day, "12:00")
	if err != nil {
		t.Fatalf("ListPendingDue: %v", err)
	}
	if got := keys(due); len(got) != 2 || got[0] != "guzheng" || got[1] != "read" {
		t.Fatalf("pending+due at 12:00 = %v, want [guzheng read]", got)
	}

	// Seed is idempotent: re-seeding does not duplicate or reset.
	seed("guzheng", "Luyện guzheng (changed title ignored)", "10:00")
	if all, _ := s.List(ctx, sc, day); len(all) != 3 {
		t.Fatalf("after re-seed, rows = %d, want 3", len(all))
	}

	// Mark guzheng done → drops out of the due set; idempotent.
	if ok, err := s.MarkDone(ctx, sc, day, "guzheng", "english tick xanh"); err != nil || !ok {
		t.Fatalf("MarkDone guzheng: ok=%v err=%v", ok, err)
	}
	due, _ = s.ListPendingDue(ctx, sc, day, "12:00")
	if got := keys(due); len(got) != 1 || got[0] != "read" {
		t.Fatalf("after MarkDone, pending+due = %v, want [read]", got)
	}

	// IncrementNudge bumps the counter (escalation = still pending after N ticks).
	if err := s.IncrementNudge(ctx, sc, day, []string{"read"}, time.Now()); err != nil {
		t.Fatalf("IncrementNudge: %v", err)
	}
	all, _ := s.List(ctx, sc, day)
	for _, e := range all {
		if e.TaskKey == "read" {
			if e.NudgeCount != 1 || e.LastNudgedAt == nil {
				t.Fatalf("read nudge_count=%d lastNudged=%v, want 1 + set", e.NudgeCount, e.LastNudgedAt)
			}
		}
	}

	// MarkDone on a non-existent task reports false (no row).
	if ok, _ := s.MarkDone(ctx, sc, day, "nope", ""); ok {
		t.Fatal("MarkDone nonexistent returned true")
	}
}

func TestHabitChecklist_ScopeIsolation(t *testing.T) {
	s := newTestHabitStore(t)
	ctx := context.Background()
	const day = "2026-06-23"
	a := store.HabitScope{TenantID: "t-aaaa", AgentID: "ag-1", UserID: "group:testchan:900000000000000001"}
	b := store.HabitScope{TenantID: "t-bbbb", AgentID: "ag-1", UserID: "group:testchan:900000000000000002"}

	if err := s.Seed(ctx, store.HabitEntry{TenantID: a.TenantID, AgentID: a.AgentID, UserID: a.UserID, PlanDate: day, TaskKey: "guzheng", Title: "A"}); err != nil {
		t.Fatalf("seed a: %v", err)
	}
	// Different tenant/user must not see A's rows.
	if got, _ := s.List(ctx, b, day); len(got) != 0 {
		t.Fatalf("scope B sees %d rows, want 0 (tenant/user isolation breach)", len(got))
	}
	if ok, _ := s.MarkDone(ctx, b, day, "guzheng", ""); ok {
		t.Fatal("scope B could MarkDone A's task (isolation breach)")
	}
	if got, _ := s.List(ctx, a, day); len(got) != 1 {
		t.Fatalf("scope A sees %d rows, want 1", len(got))
	}
}
