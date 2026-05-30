package browser

import (
	"context"
	"testing"

	"github.com/go-rod/rod"
)

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
