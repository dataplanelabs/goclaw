// Google OAuth refresh worker — scans secure_cli_user_credentials for tokens
// approaching expiry, refreshes via Google's token endpoint, writes back
// encrypted env + updated metadata. Runs on a 24h cron in production; clamped
// to a configurable tick (env var GOCLAW_OAUTH_REFRESH_TICK_SECONDS) for tests.
//
// On `oauth2.RetrieveError` with status 400 + "invalid_grant": refresh token is
// revoked; the row is deleted (UI status flips to disconnected). Other errors
// (5xx, network) are logged and retried next tick — no delete.
package oauth

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"sync/atomic"
	"time"

	"golang.org/x/oauth2"

	"github.com/nextlevelbuilder/goclaw/internal/store"
)

// RefreshWorker proactively refreshes per-operator OAuth tokens.
type RefreshWorker struct {
	store          store.SecureCLIStore
	google         *GoogleOAuthClient
	tick           time.Duration
	refreshThreshold time.Duration // refresh when expires_at < now + threshold
	healthLastTick atomic.Int64   // unix-nano of last successful tick
	binaryName     string         // "gws" for v0; extensible later
}

func NewRefreshWorker(s store.SecureCLIStore, g *GoogleOAuthClient, tick, refreshThreshold time.Duration) *RefreshWorker {
	if tick == 0 {
		tick = 24 * time.Hour
	}
	if refreshThreshold == 0 {
		refreshThreshold = 7 * 24 * time.Hour
	}
	return &RefreshWorker{
		store:            s,
		google:           g,
		tick:             tick,
		refreshThreshold: refreshThreshold,
		binaryName:       "gws",
	}
}

// Start launches the worker goroutine. Stops on ctx cancellation.
// First tick is delayed 60s to avoid racing with in-flight OAuth callbacks.
func (w *RefreshWorker) Start(ctx context.Context) {
	go w.run(ctx)
}

func (w *RefreshWorker) run(ctx context.Context) {
	if w.google == nil || !w.google.IsConfigured() {
		slog.Info("oauth.refresh_worker.disabled", "reason", "google_not_configured")
		return
	}
	// Initial delay to settle startup; honors test override via tick<1min.
	initialDelay := 60 * time.Second
	if w.tick < initialDelay {
		initialDelay = w.tick
	}
	select {
	case <-ctx.Done():
		return
	case <-time.After(initialDelay):
	}

	t := time.NewTicker(w.tick)
	defer t.Stop()
	w.runOnce(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			w.runOnce(ctx)
		}
	}
}

// runOnce is one tick worth of work — exposed for tests via direct invocation.
func (w *RefreshWorker) runOnce(ctx context.Context) {
	rows, err := w.store.ListUserCredentialsByBinaryName(store.WithCrossTenant(ctx), w.binaryName)
	if err != nil {
		slog.Error("oauth.refresh_worker.list_failed", "binary", w.binaryName, "error", err)
		return
	}
	refreshed, skipped, deleted, errored := 0, 0, 0, 0
	for _, row := range rows {
		action := w.handleRow(ctx, row)
		switch action {
		case actionRefreshed:
			refreshed++
		case actionSkipped:
			skipped++
		case actionDeleted:
			deleted++
		case actionErrored:
			errored++
		}
	}
	w.healthLastTick.Store(time.Now().UnixNano())
	slog.Info("oauth.refresh_worker.tick_done",
		"binary", w.binaryName,
		"refreshed", refreshed,
		"skipped_far_expiry", skipped,
		"deleted_revoked", deleted,
		"errored_transient", errored,
		"total", len(rows),
	)
}

type tickAction int

const (
	actionSkipped tickAction = iota
	actionRefreshed
	actionDeleted
	actionErrored
)

// handleRow decides what to do with one credential row.
func (w *RefreshWorker) handleRow(ctx context.Context, row store.SecureCLIUserCredentialWithBinary) tickAction {
	expiresAt, hasExpiry := parseExpiresAt(row.Metadata)
	if hasExpiry && time.Until(expiresAt) > w.refreshThreshold {
		return actionSkipped
	}

	// Decrypt env to get refresh_token.
	var env map[string]string
	if err := json.Unmarshal(row.EncryptedEnv, &env); err != nil {
		slog.Warn("oauth.refresh_worker.bad_env", "tenant_id", row.TenantID, "user_id", row.UserID, "error", err)
		return actionErrored
	}
	refreshToken := env["GWS_REFRESH_TOKEN"]
	if refreshToken == "" {
		slog.Warn("oauth.refresh_worker.no_refresh_token", "tenant_id", row.TenantID, "user_id", row.UserID)
		return actionErrored
	}

	_, newRefresh, newExpiry, err := w.google.RefreshToken(ctx, refreshToken)
	if err != nil {
		if isInvalidGrant(err) {
			// Token revoked — drop the row, UI flips to disconnected next /me poll.
			delCtx := store.WithTenantID(ctx, row.TenantID)
			if delErr := w.store.DeleteUserCredentials(delCtx, row.BinaryID, row.UserID); delErr != nil {
				slog.Error("oauth.refresh_worker.delete_failed",
					"tenant_id", row.TenantID, "user_id", row.UserID, "error", delErr)
				return actionErrored
			}
			slog.Warn("goclaw.alert.oauth_revoked",
				"binary", row.BinaryName, "tenant_id", row.TenantID, "user_id", row.UserID)
			return actionDeleted
		}
		slog.Warn("oauth.refresh_worker.refresh_failed",
			"tenant_id", row.TenantID, "user_id", row.UserID, "error", err)
		return actionErrored
	}

	// Write back the (possibly-rotated) refresh_token + bumped metadata.
	newEnvJSON, _ := json.Marshal(map[string]string{"GWS_REFRESH_TOKEN": newRefresh})
	newMetadata := mergeMetadata(row.Metadata, map[string]any{
		"expires_at":   newExpiry.UTC().Format(time.RFC3339),
		"refreshed_at": time.Now().UTC().Format(time.RFC3339),
	})

	writeCtx := store.WithTenantID(ctx, row.TenantID)
	if err := w.store.SetUserCredentials(writeCtx, row.BinaryID, row.UserID, newEnvJSON, newMetadata); err != nil {
		slog.Error("oauth.refresh_worker.write_failed",
			"tenant_id", row.TenantID, "user_id", row.UserID, "error", err)
		return actionErrored
	}
	slog.Info("oauth.refresh_worker.refreshed",
		"binary", row.BinaryName, "tenant_id", row.TenantID, "user_id", row.UserID,
		"new_expires_at", newExpiry.UTC().Format(time.RFC3339))
	return actionRefreshed
}

// Healthy returns true when the last successful tick was within 2×interval.
// Used by the /v1/healthz/oauth-refresh-worker probe.
func (w *RefreshWorker) Healthy() bool {
	last := w.healthLastTick.Load()
	if last == 0 {
		// Before first tick — treat as healthy during startup window.
		return true
	}
	age := time.Since(time.Unix(0, last))
	return age < 2*w.tick
}

// --- helpers ---

func parseExpiresAt(rawMeta json.RawMessage) (time.Time, bool) {
	if len(rawMeta) == 0 {
		return time.Time{}, false
	}
	var meta struct {
		ExpiresAt string `json:"expires_at"`
	}
	if err := json.Unmarshal(rawMeta, &meta); err != nil || meta.ExpiresAt == "" {
		return time.Time{}, false
	}
	t, err := time.Parse(time.RFC3339, meta.ExpiresAt)
	if err != nil {
		return time.Time{}, false
	}
	return t, true
}

// mergeMetadata reads the existing JSON object and overlays new keys.
func mergeMetadata(existing json.RawMessage, overlay map[string]any) json.RawMessage {
	var cur map[string]any
	if len(existing) > 0 {
		_ = json.Unmarshal(existing, &cur)
	}
	if cur == nil {
		cur = make(map[string]any, len(overlay))
	}
	for k, v := range overlay {
		cur[k] = v
	}
	out, _ := json.Marshal(cur)
	return out
}

// isInvalidGrant detects the "refresh_token revoked" signal from Google.
// google-auth returns *oauth2.RetrieveError; we match the OAuth2 error code
// "invalid_grant" which is the canonical revocation signal.
func isInvalidGrant(err error) bool {
	var re *oauth2.RetrieveError
	if !errors.As(err, &re) {
		return false
	}
	return re.ErrorCode == "invalid_grant"
}
