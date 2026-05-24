// Google OAuth web flow for B3-01 per-operator integrations.
//
// This is a NEW state-token-indexed map for web flows — DISTINCT from the
// existing internal/oauth/openai.go (desktop localhost-callback) and the
// composite-key map at internal/http/oauth.go:34 (also desktop). The web
// flow needs single-use state tokens because Google redirects to our
// /v1/auth/google/callback URL without our session cookie.
package oauth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"

	"github.com/nextlevelbuilder/goclaw/internal/config"
)

const (
	ScopeCalendarReadonly = "https://www.googleapis.com/auth/calendar.readonly"
	ScopeGmailReadonly    = "https://www.googleapis.com/auth/gmail.readonly"
	ScopeUserinfoEmail    = "https://www.googleapis.com/auth/userinfo.email"

	pendingFlowTTL = 6 * time.Minute
	userinfoURL    = "https://openidconnect.googleapis.com/v1/userinfo"
)

// WebOAuthPendingFlow is the per-state payload in the in-memory map.
// Tenant + user are captured at /start time and used at /callback to write
// the credential to the right (tenant_id, user_id) row.
type WebOAuthPendingFlow struct {
	TenantID  uuid.UUID
	UserID    uuid.UUID
	ExpiresAt time.Time
}

// GoogleOAuthClient handles the web OAuth flow for Google Workspace.
type GoogleOAuthClient struct {
	cfg          config.OAuthGoogleConfig
	config       *oauth2.Config
	pendingFlows map[string]WebOAuthPendingFlow
	mu           sync.RWMutex
	httpClient   *http.Client
}

func NewGoogleClient(cfg config.OAuthGoogleConfig) *GoogleOAuthClient {
	c := &GoogleOAuthClient{
		cfg: cfg,
		config: &oauth2.Config{
			ClientID:     cfg.ClientID,
			ClientSecret: cfg.ClientSecret,
			RedirectURL:  cfg.RedirectURL,
			Endpoint:     google.Endpoint,
			Scopes: []string{
				ScopeCalendarReadonly,
				ScopeGmailReadonly,
				ScopeUserinfoEmail, // returns the operator's email at /userinfo
			},
		},
		pendingFlows: make(map[string]WebOAuthPendingFlow),
		httpClient:   &http.Client{Timeout: 15 * time.Second},
	}
	return c
}

// IsConfigured reports whether the client has client_id + client_secret wired.
func (c *GoogleOAuthClient) IsConfigured() bool { return c.cfg.IsConfigured() }

// StartFlow generates a single-use state token bound to (tenantID, userID),
// stores it with a 6-min TTL, and returns the Google auth URL.
func (c *GoogleOAuthClient) StartFlow(_ context.Context, tenantID, userID uuid.UUID) (authURL, stateToken string, err error) {
	if !c.IsConfigured() {
		return "", "", fmt.Errorf("oauth google: not configured")
	}
	state, err := randomToken(16)
	if err != nil {
		return "", "", fmt.Errorf("state token: %w", err)
	}
	c.mu.Lock()
	c.pendingFlows[state] = WebOAuthPendingFlow{
		TenantID:  tenantID,
		UserID:    userID,
		ExpiresAt: time.Now().Add(pendingFlowTTL),
	}
	c.mu.Unlock()

	url := c.config.AuthCodeURL(state,
		oauth2.AccessTypeOffline,
		oauth2.ApprovalForce, // force refresh_token even on re-consent
	)
	return url, state, nil
}

// CompleteFlow validates state (single-use, TTL-bound), exchanges code for
// tokens, fetches the user's email via /userinfo, and returns the refresh
// token + email + original tenant/user.
func (c *GoogleOAuthClient) CompleteFlow(ctx context.Context, code, state string) (refreshToken, email string, tenantID, userID uuid.UUID, err error) {
	if !c.IsConfigured() {
		return "", "", uuid.Nil, uuid.Nil, fmt.Errorf("oauth google: not configured")
	}
	c.mu.Lock()
	pf, ok := c.pendingFlows[state]
	if ok {
		delete(c.pendingFlows, state) // single-use
	}
	c.mu.Unlock()
	if !ok {
		return "", "", uuid.Nil, uuid.Nil, fmt.Errorf("state mismatch")
	}
	if time.Now().After(pf.ExpiresAt) {
		return "", "", uuid.Nil, uuid.Nil, fmt.Errorf("state expired")
	}

	tok, err := c.config.Exchange(ctx, code)
	if err != nil {
		return "", "", uuid.Nil, uuid.Nil, fmt.Errorf("exchange: %w", err)
	}
	if tok.RefreshToken == "" {
		return "", "", uuid.Nil, uuid.Nil, fmt.Errorf("no refresh token returned (re-consent required)")
	}
	email, _ = c.fetchUserEmail(ctx, tok)
	return tok.RefreshToken, email, pf.TenantID, pf.UserID, nil
}

// RefreshToken exchanges a refresh_token for a fresh access_token.
// Used by the B3-01 refresh worker (Phase 4).
// Returns the (possibly-rotated) refresh_token, the new access_token,
// expires_at, and any error.
func (c *GoogleOAuthClient) RefreshToken(ctx context.Context, refreshToken string) (newAccessToken, newRefreshToken string, expiresAt time.Time, err error) {
	if !c.IsConfigured() {
		return "", "", time.Time{}, fmt.Errorf("oauth google: not configured")
	}
	src := c.config.TokenSource(ctx, &oauth2.Token{RefreshToken: refreshToken})
	tok, err := src.Token()
	if err != nil {
		return "", "", time.Time{}, err
	}
	// Google sometimes returns a rotated refresh_token; preserve old if blank.
	rt := tok.RefreshToken
	if rt == "" {
		rt = refreshToken
	}
	return tok.AccessToken, rt, tok.Expiry, nil
}

// CleanupExpiredFlows removes pending flows past their TTL. Call from a
// background goroutine started by NewGoogleClient's owner (gateway startup).
func (c *GoogleOAuthClient) CleanupExpiredFlows() {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := time.Now()
	for state, pf := range c.pendingFlows {
		if now.After(pf.ExpiresAt) {
			delete(c.pendingFlows, state)
		}
	}
}

// PendingFlowsCount is used by tests + observability probes.
func (c *GoogleOAuthClient) PendingFlowsCount() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.pendingFlows)
}

func (c *GoogleOAuthClient) fetchUserEmail(ctx context.Context, tok *oauth2.Token) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, userinfoURL, nil)
	if err != nil {
		return "", err
	}
	tok.SetAuthHeader(req)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("userinfo: status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
	if err != nil {
		return "", err
	}
	var ui struct {
		Email string `json:"email"`
	}
	if err := json.Unmarshal(body, &ui); err != nil {
		return "", err
	}
	return ui.Email, nil
}

func randomToken(nBytes int) (string, error) {
	b := make([]byte, nBytes)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
