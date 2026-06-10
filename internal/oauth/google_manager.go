// Google OAuth client manager — per-tenant configuration resolution with env
// fallback. Owns the cross-tenant pendingFlows state map (callback URL is
// shared across tenants, so state must be resolvable globally).
//
// Config resolution order for a tenant:
//  1. tenant_id-scoped rows in config_secrets (client_id, client_secret)
//     + system_configs (redirect_url)
//  2. global env vars (GOCLAW_GOOGLE_{CLIENT_ID,CLIENT_SECRET,REDIRECT_URL})
//  3. unconfigured → endpoints return MsgOAuthNotConfigured (503)
//
// This is B3-01.1 follow-up to B3-01 P2. Existing GoogleOAuthClient stays for
// tests + single-org deploys (works exactly as before).
package oauth

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"

	"github.com/nextlevelbuilder/goclaw/internal/config"
	"github.com/nextlevelbuilder/goclaw/internal/store"
)

const (
	OAuthSecretKeyClientID     = "oauth.google.client_id"
	OAuthSecretKeyClientSecret = "oauth.google.client_secret"
	OAuthConfigKeyRedirectURL  = "oauth.google.redirect_url"
)

// GoogleClientManager resolves per-tenant Google OAuth configs from
// ConfigSecretsStore + SystemConfigStore, with env-var fallback.
type GoogleClientManager struct {
	envFallback config.OAuthGoogleConfig
	secrets     store.ConfigSecretsStore
	configs     store.SystemConfigStore

	// Cross-tenant state map — callback URL is shared.
	pendingFlows map[string]WebOAuthPendingFlow
	flowsMu      sync.RWMutex

	// Per-tenant oauth2.Config cache. Invalidated when tenant config changes
	// via Set/Clear (admin endpoints below).
	tenantConfigs sync.Map // map[uuid.UUID]*oauth2.Config

	// testFakeEndpoint, when non-nil, overrides Google's endpoint for ALL
	// per-tenant configs built via oauthConfigForTenant. Used by tests; never
	// set in production. Kept on the manager (not a global) so parallel tests
	// don't trip on shared state.
	testFakeEndpoint *oauth2.Endpoint
}

func NewGoogleClientManager(envFallback config.OAuthGoogleConfig, secrets store.ConfigSecretsStore, configs store.SystemConfigStore) *GoogleClientManager {
	return &GoogleClientManager{
		envFallback:  envFallback,
		secrets:      secrets,
		configs:      configs,
		pendingFlows: make(map[string]WebOAuthPendingFlow),
	}
}

// ConfigForTenant returns the effective per-tenant config, merging DB rows
// over env fallback. ClientSecret is included in the returned struct; do NOT
// surface it from HTTP handlers without explicit redaction.
func (m *GoogleClientManager) ConfigForTenant(ctx context.Context, tenantID uuid.UUID) (config.OAuthGoogleConfig, error) {
	cfg := m.envFallback // base copy
	if m.secrets == nil || m.configs == nil || tenantID == uuid.Nil {
		return cfg, nil
	}
	tenantCtx := store.WithTenantID(ctx, tenantID)
	if v, err := m.secrets.Get(tenantCtx, OAuthSecretKeyClientID); err == nil && v != "" {
		cfg.ClientID = v
	}
	if v, err := m.secrets.Get(tenantCtx, OAuthSecretKeyClientSecret); err == nil && v != "" {
		cfg.ClientSecret = v
	}
	if v, err := m.configs.Get(tenantCtx, OAuthConfigKeyRedirectURL); err == nil && v != "" {
		cfg.RedirectURL = v
	}
	return cfg, nil
}

// IsConfiguredForTenant reports whether the tenant has a resolvable config
// (either via DB or env fallback).
func (m *GoogleClientManager) IsConfiguredForTenant(ctx context.Context, tenantID uuid.UUID) bool {
	cfg, _ := m.ConfigForTenant(ctx, tenantID)
	return cfg.IsConfigured()
}

// SetConfigForTenant writes the tenant's client_id/secret/redirect_url to
// ConfigSecrets + SystemConfigs. Any nil/empty field is unset (so callers
// can update partial fields by re-reading first + merging client-side).
func (m *GoogleClientManager) SetConfigForTenant(ctx context.Context, tenantID uuid.UUID, cfg config.OAuthGoogleConfig) error {
	if m.secrets == nil || m.configs == nil {
		return errors.New("oauth manager: stores not configured")
	}
	if tenantID == uuid.Nil {
		return errors.New("oauth manager: tenantID required")
	}
	tenantCtx := store.WithTenantID(ctx, tenantID)
	if cfg.ClientID != "" {
		if err := m.secrets.Set(tenantCtx, OAuthSecretKeyClientID, cfg.ClientID); err != nil {
			return fmt.Errorf("save client_id: %w", err)
		}
	}
	if cfg.ClientSecret != "" {
		if err := m.secrets.Set(tenantCtx, OAuthSecretKeyClientSecret, cfg.ClientSecret); err != nil {
			return fmt.Errorf("save client_secret: %w", err)
		}
	}
	if cfg.RedirectURL != "" {
		if err := m.configs.Set(tenantCtx, OAuthConfigKeyRedirectURL, cfg.RedirectURL); err != nil {
			return fmt.Errorf("save redirect_url: %w", err)
		}
	}
	// Invalidate cache so the next ClientForTenant rebuilds from DB.
	m.tenantConfigs.Delete(tenantID)
	return nil
}

// ClearConfigForTenant removes the tenant's DB rows (falls back to env).
func (m *GoogleClientManager) ClearConfigForTenant(ctx context.Context, tenantID uuid.UUID) error {
	if m.secrets == nil || m.configs == nil {
		return errors.New("oauth manager: stores not configured")
	}
	if tenantID == uuid.Nil {
		return errors.New("oauth manager: tenantID required")
	}
	tenantCtx := store.WithTenantID(ctx, tenantID)
	_ = m.secrets.Delete(tenantCtx, OAuthSecretKeyClientID)
	_ = m.secrets.Delete(tenantCtx, OAuthSecretKeyClientSecret)
	_ = m.configs.Delete(tenantCtx, OAuthConfigKeyRedirectURL)
	m.tenantConfigs.Delete(tenantID)
	return nil
}

// oauthConfigForTenant returns a cached/built oauth2.Config for the tenant.
// Returns nil + error if the tenant is unconfigured.
func (m *GoogleClientManager) oauthConfigForTenant(ctx context.Context, tenantID uuid.UUID) (*oauth2.Config, error) {
	if v, ok := m.tenantConfigs.Load(tenantID); ok {
		return v.(*oauth2.Config), nil
	}
	cfg, err := m.ConfigForTenant(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	if !cfg.IsConfigured() {
		return nil, errors.New("oauth google: not configured for tenant")
	}
	endpoint := google.Endpoint
	if m.testFakeEndpoint != nil {
		endpoint = *m.testFakeEndpoint
	}
	oauthCfg := &oauth2.Config{
		ClientID:     cfg.ClientID,
		ClientSecret: cfg.ClientSecret,
		RedirectURL:  cfg.RedirectURL,
		Endpoint:     endpoint,
		Scopes: []string{
			ScopeCalendarReadonly,
			ScopeGmailReadonly,
			ScopeUserinfoEmail,
		},
	}
	m.tenantConfigs.Store(tenantID, oauthCfg)
	return oauthCfg, nil
}

// StartFlow generates a single-use state token bound to (tenantID, userID)
// and returns the per-tenant-configured Google auth URL.
func (m *GoogleClientManager) StartFlow(ctx context.Context, tenantID, userID uuid.UUID) (authURL, stateToken string, err error) {
	oauthCfg, err := m.oauthConfigForTenant(ctx, tenantID)
	if err != nil {
		return "", "", err
	}
	state, err := randomToken(16)
	if err != nil {
		return "", "", fmt.Errorf("state token: %w", err)
	}
	m.flowsMu.Lock()
	m.pendingFlows[state] = WebOAuthPendingFlow{
		TenantID:  tenantID,
		UserID:    userID,
		ExpiresAt: time.Now().Add(pendingFlowTTL),
	}
	m.flowsMu.Unlock()
	url := oauthCfg.AuthCodeURL(state,
		oauth2.AccessTypeOffline,
		oauth2.ApprovalForce,
	)
	return url, state, nil
}

// CompleteFlow looks up the state (single-use, TTL-bound), resolves the
// tenant's oauth config from the state payload, exchanges the code, fetches
// the user email, and returns the refresh_token + email + tenant/user.
func (m *GoogleClientManager) CompleteFlow(ctx context.Context, code, state string) (refreshToken, email string, tenantID, userID uuid.UUID, err error) {
	m.flowsMu.Lock()
	pf, ok := m.pendingFlows[state]
	if ok {
		delete(m.pendingFlows, state)
	}
	m.flowsMu.Unlock()
	if !ok {
		return "", "", uuid.Nil, uuid.Nil, errors.New("state mismatch")
	}
	if time.Now().After(pf.ExpiresAt) {
		return "", "", uuid.Nil, uuid.Nil, errors.New("state expired")
	}
	oauthCfg, err := m.oauthConfigForTenant(ctx, pf.TenantID)
	if err != nil {
		return "", "", uuid.Nil, uuid.Nil, err
	}
	tok, err := oauthCfg.Exchange(ctx, code)
	if err != nil {
		return "", "", uuid.Nil, uuid.Nil, fmt.Errorf("exchange: %w", err)
	}
	if tok.RefreshToken == "" {
		return "", "", uuid.Nil, uuid.Nil, errors.New("no refresh token returned (re-consent required)")
	}
	email, _ = fetchUserEmail(ctx, tok)
	return tok.RefreshToken, email, pf.TenantID, pf.UserID, nil
}

// RefreshToken refreshes an access token using the tenant's oauth config.
// Used by the refresh worker — needs the tenantID since each tenant may
// have its own GCP project/client.
func (m *GoogleClientManager) RefreshToken(ctx context.Context, tenantID uuid.UUID, refreshToken string) (newAccessToken, newRefreshToken string, expiresAt time.Time, err error) {
	oauthCfg, err := m.oauthConfigForTenant(ctx, tenantID)
	if err != nil {
		return "", "", time.Time{}, err
	}
	src := oauthCfg.TokenSource(ctx, &oauth2.Token{RefreshToken: refreshToken})
	tok, err := src.Token()
	if err != nil {
		return "", "", time.Time{}, err
	}
	rt := tok.RefreshToken
	if rt == "" {
		rt = refreshToken
	}
	return tok.AccessToken, rt, tok.Expiry, nil
}

// CleanupExpiredFlows removes pending flows past their TTL.
func (m *GoogleClientManager) CleanupExpiredFlows() {
	m.flowsMu.Lock()
	defer m.flowsMu.Unlock()
	now := time.Now()
	for state, pf := range m.pendingFlows {
		if now.After(pf.ExpiresAt) {
			delete(m.pendingFlows, state)
		}
	}
}

// PendingFlowsCount is for tests + observability probes.
func (m *GoogleClientManager) PendingFlowsCount() int {
	m.flowsMu.RLock()
	defer m.flowsMu.RUnlock()
	return len(m.pendingFlows)
}

// HasEnvFallback reports whether the manager has env-var fallback configured.
// Used by the admin UI to show "config inherits from env" vs "configure now".
func (m *GoogleClientManager) HasEnvFallback() bool { return m.envFallback.IsConfigured() }
