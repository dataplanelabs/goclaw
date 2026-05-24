package oauth

import (
	"context"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/config"
)

func newTestClient() *GoogleOAuthClient {
	return NewGoogleClient(config.OAuthGoogleConfig{
		ClientID:     "client-abc",
		ClientSecret: "secret-xyz",
		RedirectURL:  "https://example.test/v1/auth/google/callback",
	})
}

func TestGoogleClient_IsConfigured(t *testing.T) {
	if !newTestClient().IsConfigured() {
		t.Fatal("client with id+secret should be configured")
	}
	bare := NewGoogleClient(config.OAuthGoogleConfig{})
	if bare.IsConfigured() {
		t.Fatal("client without id+secret should NOT be configured")
	}
}

func TestStartFlow_ReturnsValidAuthURLAndState(t *testing.T) {
	c := newTestClient()
	tenantID := uuid.New()
	userID := uuid.New()
	authURL, state, err := c.StartFlow(context.Background(), tenantID, userID)
	if err != nil {
		t.Fatalf("StartFlow: %v", err)
	}
	if len(state) != 32 { // 16 bytes hex-encoded
		t.Errorf("state length=%d, want 32", len(state))
	}
	u, err := url.Parse(authURL)
	if err != nil {
		t.Fatalf("parse auth URL: %v", err)
	}
	q := u.Query()
	if q.Get("state") != state {
		t.Errorf("state in URL %q != returned %q", q.Get("state"), state)
	}
	if q.Get("client_id") != "client-abc" {
		t.Errorf("client_id mismatch: %s", q.Get("client_id"))
	}
	if q.Get("access_type") != "offline" {
		t.Errorf("access_type=%s, want offline", q.Get("access_type"))
	}
	if !strings.Contains(q.Get("scope"), ScopeCalendarReadonly) {
		t.Errorf("scope missing calendar.readonly: %s", q.Get("scope"))
	}
	if !strings.Contains(q.Get("scope"), ScopeGmailReadonly) {
		t.Errorf("scope missing gmail.readonly: %s", q.Get("scope"))
	}
	if c.PendingFlowsCount() != 1 {
		t.Errorf("pending flows=%d, want 1", c.PendingFlowsCount())
	}
}

func TestStartFlow_NotConfiguredReturnsError(t *testing.T) {
	c := NewGoogleClient(config.OAuthGoogleConfig{})
	_, _, err := c.StartFlow(context.Background(), uuid.New(), uuid.New())
	if err == nil {
		t.Fatal("expected error when not configured")
	}
}

func TestCompleteFlow_RejectsUnknownState(t *testing.T) {
	c := newTestClient()
	_, _, _, _, err := c.CompleteFlow(context.Background(), "code", "never-issued-state")
	if err == nil || !strings.Contains(err.Error(), "state mismatch") {
		t.Fatalf("expected state mismatch error, got %v", err)
	}
}

func TestCompleteFlow_RejectsExpiredState(t *testing.T) {
	c := newTestClient()
	expiredState := "expired-state-token"
	c.mu.Lock()
	c.pendingFlows[expiredState] = WebOAuthPendingFlow{
		TenantID:  uuid.New(),
		UserID:    uuid.New(),
		ExpiresAt: time.Now().Add(-1 * time.Minute),
	}
	c.mu.Unlock()
	_, _, _, _, err := c.CompleteFlow(context.Background(), "code", expiredState)
	if err == nil || !strings.Contains(err.Error(), "expired") {
		t.Fatalf("expected expired error, got %v", err)
	}
}

func TestCompleteFlow_StateIsSingleUse(t *testing.T) {
	c := newTestClient()
	_, state, err := c.StartFlow(context.Background(), uuid.New(), uuid.New())
	if err != nil {
		t.Fatal(err)
	}
	// First Complete attempt (will fail at code exchange — no live Google) but
	// MUST delete the state from pendingFlows regardless. Then second attempt
	// should see "state mismatch".
	_, _, _, _, _ = c.CompleteFlow(context.Background(), "bogus-code", state)
	if c.PendingFlowsCount() != 0 {
		t.Errorf("state should be deleted after first lookup; pending=%d", c.PendingFlowsCount())
	}
	_, _, _, _, err = c.CompleteFlow(context.Background(), "bogus-code", state)
	if err == nil || !strings.Contains(err.Error(), "state mismatch") {
		t.Errorf("second use should be rejected, got %v", err)
	}
}

func TestCleanupExpiredFlows(t *testing.T) {
	c := newTestClient()
	c.mu.Lock()
	c.pendingFlows["fresh"] = WebOAuthPendingFlow{
		TenantID: uuid.New(), UserID: uuid.New(),
		ExpiresAt: time.Now().Add(5 * time.Minute),
	}
	c.pendingFlows["stale"] = WebOAuthPendingFlow{
		TenantID: uuid.New(), UserID: uuid.New(),
		ExpiresAt: time.Now().Add(-1 * time.Minute),
	}
	c.mu.Unlock()
	c.CleanupExpiredFlows()
	if c.PendingFlowsCount() != 1 {
		t.Errorf("expected 1 flow after cleanup, got %d", c.PendingFlowsCount())
	}
}

func TestRefreshToken_NotConfigured(t *testing.T) {
	c := NewGoogleClient(config.OAuthGoogleConfig{})
	_, _, _, err := c.RefreshToken(context.Background(), "anything")
	if err == nil {
		t.Fatal("expected error when not configured")
	}
}
