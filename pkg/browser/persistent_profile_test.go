package browser

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/go-rod/rod"
)

// TestOldestEvictableLocked_ProtectsHumanTabs verifies the max-pages eviction only
// ever picks a goclaw-opened tab (in pageLastUsed) — never a human's noVNC tab.
func TestOldestEvictableLocked_ProtectsHumanTabs(t *testing.T) {
	now := time.Now()
	m := New(WithMaxPages(2))
	// two goclaw-opened master tabs (tracked, no tenant owner) + one untracked human tab
	m.pages = map[string]*rod.Page{"old": nil, "new": nil, "human": nil}
	m.pageLastUsed = map[string]time.Time{"old": now.Add(-10 * time.Minute), "new": now}

	if got := m.oldestEvictableLocked("", MasterTenantID); got != "old" {
		t.Fatalf("expected oldest tracked tab 'old', got %q (human tab must never be evicted)", got)
	}

	// Under the limit (1 tracked tab + 1 human) → nothing evictable.
	m.pages = map[string]*rod.Page{"new": nil, "human": nil}
	m.pageLastUsed = map[string]time.Time{"new": now}
	if got := m.oldestEvictableLocked("", MasterTenantID); got != "" {
		t.Fatalf("under maxPages: expected no eviction, got %q", got)
	}
}

// TestIsStalePageErr pins which errors trigger a re-resolve-and-retry on page ops.
func TestIsStalePageErr(t *testing.T) {
	for _, s := range []string{"open tab: context canceled", "Target closed", "no such target", "use of closed network connection"} {
		if !isStalePageErr(errors.New(s)) {
			t.Fatalf("want stale: %q", s)
		}
	}
	for _, s := range []string{"context deadline exceeded", "element not found"} {
		if isStalePageErr(errors.New(s)) {
			t.Fatalf("want NOT stale: %q", s)
		}
	}
	if isStalePageErr(nil) {
		t.Fatal("nil must not be stale")
	}
}

// TestTenantBrowserLocked_PersistentProfile verifies the persistent-profile mode
// routes EVERY tenant (including a non-master UUID) to the shared default-context
// browser instead of an ephemeral incognito context — the fix that lets a human's
// one-time login persist for the agent. Exercises only the early-return branches,
// so no real CDP connection is needed.
func TestTenantBrowserLocked_PersistentProfile(t *testing.T) {
	stub := &rod.Browser{} // sentinel; persistent/master paths return before any rod call
	m := &Manager{browser: stub, tenantCtxs: map[string]*rod.Browser{}}

	const nonMaster = "11111111-1111-1111-1111-111111111111"

	// Persistent mode: a non-master tenant gets the shared browser, NOT incognito.
	m.persistentProfile = true
	got, err := m.tenantBrowserLocked(nonMaster)
	if err != nil {
		t.Fatalf("persistent non-master: unexpected error: %v", err)
	}
	if got != stub {
		t.Fatalf("persistent non-master: got %p, want shared browser %p", got, stub)
	}
	if len(m.tenantCtxs) != 0 {
		t.Fatalf("persistent mode must NOT create an incognito context, got %d", len(m.tenantCtxs))
	}

	// Default mode unchanged: master + empty tenant still use the shared browser.
	m.persistentProfile = false
	for _, tid := range []string{"", MasterTenantID} {
		got, err := m.tenantBrowserLocked(tid)
		if err != nil || got != stub {
			t.Fatalf("default tenant %q: got %p err %v, want shared browser", tid, got, err)
		}
	}
	if len(m.tenantCtxs) != 0 {
		t.Fatalf("master/empty must not create incognito, got %d", len(m.tenantCtxs))
	}
}

// TestStop_CancelsBrowserContext guards the connect-lifetime fix: the browser's
// context now outlives Start() and is torn down by browserCancel on Stop. If a
// future change drops the cancel wiring, the CDP read-loop goroutine would leak.
// Remote mode is used so Stop drops the connection without calling Close() on the
// stub browser.
func TestStop_CancelsBrowserContext(t *testing.T) {
	m := New(WithRemoteURL("ws://chrome:9222"))
	m.browser = &rod.Browser{}
	cancelled := false
	m.browserCancel = func() { cancelled = true }

	if err := m.Stop(context.Background()); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if !cancelled {
		t.Fatal("Stop did not cancel the browser context")
	}
	if m.browserCancel != nil || m.browser != nil {
		t.Fatal("Stop did not clear browser/browserCancel")
	}
}

// TestCleanupDeadBrowser_CancelsContext verifies the reconnect path also cancels
// the old browser context before replacing it (else each reconnect leaks the
// previous CDP read loop).
func TestCleanupDeadBrowser_CancelsContext(t *testing.T) {
	m := New(WithRemoteURL("ws://chrome:9222"))
	m.browser = &rod.Browser{}
	cancelled := false
	m.browserCancel = func() { cancelled = true }

	m.mu.Lock()
	m.cleanupDeadBrowserLocked()
	m.mu.Unlock()

	if !cancelled {
		t.Fatal("cleanupDeadBrowserLocked did not cancel the browser context")
	}
	if m.browserCancel != nil {
		t.Fatal("cleanupDeadBrowserLocked did not clear browserCancel")
	}
}

// TestSessionTargetLocked verifies an empty targetID resolves to the calling
// session's OWN most-recently-used tab, never another session's, so concurrent
// sessions on the shared browser don't read each other's pages.
func TestSessionTargetLocked(t *testing.T) {
	now := time.Now()
	m := New()
	// Session A owns two tabs; session B owns one.
	m.pages = map[string]*rod.Page{"a-old": nil, "a-new": nil, "b1": nil}
	m.pageSessions = map[string]string{"a-old": "A", "a-new": "A", "b1": "B"}
	m.pageLastUsed = map[string]time.Time{
		"a-old": now.Add(-5 * time.Minute),
		"a-new": now,
		"b1":    now.Add(-time.Minute),
	}

	if got := m.sessionTargetLocked("A", ""); got != "a-new" {
		t.Fatalf("session A empty targetID: got %q, want its newest tab a-new", got)
	}
	if got := m.sessionTargetLocked("B", ""); got != "b1" {
		t.Fatalf("session B empty targetID: got %q, want b1", got)
	}
	// Explicit targetID is always honored, even cross-session.
	if got := m.sessionTargetLocked("A", "b1"); got != "b1" {
		t.Fatalf("explicit targetID must pass through, got %q", got)
	}
	// Unknown session and empty session → no tab / legacy global fallback.
	if got := m.sessionTargetLocked("ghost", ""); got != "" {
		t.Fatalf("unknown session: got %q, want \"\"", got)
	}
	if got := m.sessionTargetLocked("", ""); got != "" {
		t.Fatalf("empty session must keep global fallback (\"\"), got %q", got)
	}
}

// TestSessionTargetLocked_NoTabs verifies a known session with no open tabs resolves to "".
func TestSessionTargetLocked_NoTabs(t *testing.T) {
	m := New()
	m.pages = map[string]*rod.Page{"b1": nil}
	m.pageSessions = map[string]string{"b1": "B"}
	m.pageLastUsed = map[string]time.Time{"b1": time.Now()}

	if got := m.sessionTargetLocked("A", ""); got != "" {
		t.Fatalf("session with no tabs: got %q, want \"\"", got)
	}
}

// TestEviction_PerSession verifies maxPages is a per-session budget: session A
// opening a new tab at the limit never evicts session B's tab on the shared browser.
func TestEviction_PerSession(t *testing.T) {
	now := time.Now()
	m := New(WithMaxPages(1))
	m.pages = map[string]*rod.Page{"a1": nil, "b1": nil}
	m.pageSessions = map[string]string{"a1": "A", "b1": "B"}
	m.pageLastUsed = map[string]time.Time{"a1": now, "b1": now.Add(-time.Minute)}

	// Session A at its 1-tab limit → evicts only its own a1, never B's older b1.
	if got := m.oldestEvictableLocked("A", MasterTenantID); got != "a1" {
		t.Fatalf("session A eviction: got %q, want its own a1 (must not touch B's b1)", got)
	}
	// Session B at its 1-tab limit → evicts only b1.
	if got := m.oldestEvictableLocked("B", MasterTenantID); got != "b1" {
		t.Fatalf("session B eviction: got %q, want b1", got)
	}
}

// TestEviction_PerSession_HumanTabGuard verifies the human noVNC tab guard still
// holds under per-session eviction — an untracked tab is never evicted.
func TestEviction_PerSession_HumanTabGuard(t *testing.T) {
	now := time.Now()
	m := New(WithMaxPages(1))
	// Session A has one tracked tab; "human" is an untracked noVNC tab (no pageLastUsed).
	m.pages = map[string]*rod.Page{"a1": nil, "human": nil}
	m.pageSessions = map[string]string{"a1": "A"}
	m.pageLastUsed = map[string]time.Time{"a1": now}

	if got := m.oldestEvictableLocked("A", MasterTenantID); got != "a1" {
		t.Fatalf("got %q, want a1 — human tab must never be evicted", got)
	}
}
