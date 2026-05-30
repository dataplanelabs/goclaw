package browser

import (
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
