package http

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/bus"
	"github.com/nextlevelbuilder/goclaw/internal/i18n"
	"github.com/nextlevelbuilder/goclaw/internal/oauth"
	"github.com/nextlevelbuilder/goclaw/internal/permissions"
	"github.com/nextlevelbuilder/goclaw/internal/store"
)

// IntegrationsHandler serves the B3-01 per-operator OAuth flow for Google.
// Endpoints:
//   POST   /v1/integrations/google/start         (auth required — viewer+)
//   GET    /v1/auth/google/callback              (no session; auth via state token)
//   GET    /v1/integrations/me                   (auth required — viewer+)
//   DELETE /v1/integrations/{binary_name}        (auth required — viewer+, delete own only)
type IntegrationsHandler struct {
	store    store.SecureCLIStore
	google   *oauth.GoogleOAuthClient
	msgBus   *bus.MessageBus
	uiBaseURL string // e.g. https://dev.goclaw.example  — for popup redirect target
}

func NewIntegrationsHandler(s store.SecureCLIStore, g *oauth.GoogleOAuthClient, msgBus *bus.MessageBus, uiBaseURL string) *IntegrationsHandler {
	return &IntegrationsHandler{store: s, google: g, msgBus: msgBus, uiBaseURL: uiBaseURL}
}

func (h *IntegrationsHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /v1/integrations/google/start", requireAuth(permissions.RoleViewer, h.handleGoogleStart))
	// Callback is UNAUTHENTICATED — Google redirects here without our session cookie.
	// Auth happens via the single-use state token payload (tenant+user captured at /start).
	mux.HandleFunc("GET /v1/auth/google/callback", h.handleGoogleCallback)
	mux.HandleFunc("GET /v1/integrations/me", requireAuth(permissions.RoleViewer, h.handleListMyIntegrations))
	// NOTE: divergence from /v1/cli-credentials admin-only convention is intentional —
	// operators own their per-user credential row and can disconnect their own integration.
	mux.HandleFunc("DELETE /v1/integrations/{binary_name}", requireAuth(permissions.RoleViewer, h.handleDeleteMyIntegration))
}

// ---- POST /v1/integrations/google/start ----

type startResponse struct {
	AuthURL string `json:"auth_url"`
	State   string `json:"state"`
}

func (h *IntegrationsHandler) handleGoogleStart(w http.ResponseWriter, r *http.Request) {
	locale := store.LocaleFromContext(r.Context())
	if h.google == nil || !h.google.IsConfigured() {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": i18n.T(locale, i18n.MsgOAuthNotConfigured)})
		return
	}
	tenantID := store.TenantIDFromContext(r.Context())
	userIDStr := store.UserIDFromContext(r.Context())
	userID, err := uuid.Parse(userIDStr)
	if err != nil || tenantID == uuid.Nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": i18n.T(locale, i18n.MsgUnauthorized)})
		return
	}
	authURL, state, err := h.google.StartFlow(r.Context(), tenantID, userID)
	if err != nil {
		slog.Warn("oauth.google.start_failed", "user_id", userID, "tenant_id", tenantID, "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": i18n.T(locale, i18n.MsgInternalError, err.Error())})
		return
	}
	slog.Info("oauth.google.start", "user_id", userID, "tenant_id", tenantID)
	writeJSON(w, http.StatusOK, startResponse{AuthURL: authURL, State: state})
}

// ---- GET /v1/auth/google/callback ----

func (h *IntegrationsHandler) handleGoogleCallback(w http.ResponseWriter, r *http.Request) {
	locale := store.LocaleFromContext(r.Context())
	if h.google == nil || !h.google.IsConfigured() {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": i18n.T(locale, i18n.MsgOAuthNotConfigured)})
		return
	}
	code := r.URL.Query().Get("code")
	state := r.URL.Query().Get("state")
	if code == "" || state == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "code and state required"})
		return
	}

	refreshToken, email, tenantID, userID, err := h.google.CompleteFlow(r.Context(), code, state)
	if err != nil {
		// Distinguish state mismatch vs exchange failure for actionable UX.
		stage := "exchange"
		if strings.Contains(err.Error(), "state") {
			stage = "state"
		}
		slog.Warn("oauth.google.callback_failed", "stage", stage, "error", err)
		if stage == "state" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": i18n.T(locale, i18n.MsgOAuthStateMismatch)})
			return
		}
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": i18n.T(locale, i18n.MsgOAuthExchangeFailed, err.Error())})
		return
	}

	// Propagate the original tenant+user into ctx for the credential write.
	// (The callback request itself is unauthenticated; trust comes from the state-token payload.)
	writeCtx := store.WithTenantID(r.Context(), tenantID)
	writeCtx = store.WithUserID(writeCtx, userID.String())

	// Resolve the gws binary's UUID for this tenant.
	bin, err := h.store.GetByName(writeCtx, "gws")
	if err != nil || bin == nil {
		slog.Warn("oauth.google.callback.binary_not_found", "tenant_id", tenantID, "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": i18n.T(locale, i18n.MsgOAuthBinaryNotFound, "gws")})
		return
	}

	// Plaintext env — the store layer encrypts.
	envJSON, _ := json.Marshal(map[string]string{"GWS_REFRESH_TOKEN": refreshToken})
	metadata, _ := json.Marshal(map[string]any{
		"account_email": email,
		"scopes":        []string{oauth.ScopeCalendarReadonly, oauth.ScopeGmailReadonly},
		"connected_at":  time.Now().UTC().Format(time.RFC3339),
		"refreshed_at":  time.Now().UTC().Format(time.RFC3339),
	})
	if err := h.store.SetUserCredentials(writeCtx, bin.ID, userID.String(), envJSON, metadata); err != nil {
		slog.Error("oauth.google.callback.persist_failed", "tenant_id", tenantID, "user_id", userID, "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": i18n.T(locale, i18n.MsgInternalError, err.Error())})
		return
	}

	emitAudit(h.msgBus, r, "oauth.google.connected", "secure_cli_user_credentials", bin.ID.String()+"/"+userID.String())
	slog.Info("oauth.google.callback.success", "tenant_id", tenantID, "user_id", userID, "email", email)

	// HTML response: post a message to the opener (popup-aware) AND offer a
	// fallback redirect for browsers without window.opener.
	redirectURL := strings.TrimRight(h.uiBaseURL, "/") + "/integrations?success=true"
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(fmt.Sprintf(`<!doctype html>
<html><head><meta charset="utf-8"><title>Google connected</title></head>
<body>
<p>Google account connected. You can close this window.</p>
<script>
try { if (window.opener) { window.opener.postMessage("oauth-complete", "*"); } } catch (_) {}
try { window.close(); } catch (_) {}
setTimeout(function() { window.location = %q; }, 1500);
</script>
</body></html>`, redirectURL)))
}

// ---- GET /v1/integrations/me ----

type integrationStatus struct {
	BinaryName   string   `json:"binary_name"`
	AccountEmail string   `json:"account_email"`
	Scopes       []string `json:"scopes"`
	ConnectedAt  string   `json:"connected_at"`
}

func (h *IntegrationsHandler) handleListMyIntegrations(w http.ResponseWriter, r *http.Request) {
	locale := store.LocaleFromContext(r.Context())
	tenantID := store.TenantIDFromContext(r.Context())
	userID := store.UserIDFromContext(r.Context())
	if tenantID == uuid.Nil || userID == "" {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": i18n.T(locale, i18n.MsgUnauthorized)})
		return
	}
	// v0: hardcoded to "gws". Future per-tenant integrations can be looped here.
	rows, err := h.store.ListUserCredentialsByBinaryName(store.WithCrossTenant(r.Context()), "gws")
	if err != nil {
		slog.Error("integrations.me_failed", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": i18n.T(locale, i18n.MsgInternalError, err.Error())})
		return
	}
	out := []integrationStatus{}
	for _, row := range rows {
		// Filter to caller's (tenant, user) — handler layer enforces; the
		// list call was cross-tenant for the JOIN, but the response is scoped.
		if row.TenantID != tenantID || row.UserID != userID {
			continue
		}
		var meta struct {
			AccountEmail string   `json:"account_email"`
			Scopes       []string `json:"scopes"`
			ConnectedAt  string   `json:"connected_at"`
		}
		_ = json.Unmarshal(row.Metadata, &meta)
		out = append(out, integrationStatus{
			BinaryName:   row.BinaryName,
			AccountEmail: meta.AccountEmail,
			Scopes:       meta.Scopes,
			ConnectedAt:  meta.ConnectedAt,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"integrations": out})
}

// ---- DELETE /v1/integrations/{binary_name} ----

func (h *IntegrationsHandler) handleDeleteMyIntegration(w http.ResponseWriter, r *http.Request) {
	locale := store.LocaleFromContext(r.Context())
	name := strings.TrimSpace(r.PathValue("binary_name"))
	if name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "binary_name required"})
		return
	}
	tenantID := store.TenantIDFromContext(r.Context())
	userID := store.UserIDFromContext(r.Context())
	if tenantID == uuid.Nil || userID == "" {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": i18n.T(locale, i18n.MsgUnauthorized)})
		return
	}
	bin, err := h.store.GetByName(r.Context(), name)
	if err != nil || bin == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": i18n.T(locale, i18n.MsgOAuthIntegrationNotFound, name)})
		return
	}
	if err := h.store.DeleteUserCredentials(r.Context(), bin.ID, userID); err != nil && !errors.Is(err, errNotFoundSentinel) {
		slog.Error("integrations.delete_failed", "binary", name, "user_id", userID, "tenant_id", tenantID, "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": i18n.T(locale, i18n.MsgInternalError, err.Error())})
		return
	}
	emitAudit(h.msgBus, r, "oauth.google.disconnected", "secure_cli_user_credentials", bin.ID.String()+"/"+userID)
	slog.Info("oauth.google.disconnect", "binary", name, "user_id", userID, "tenant_id", tenantID)
	w.WriteHeader(http.StatusNoContent)
}

// errNotFoundSentinel — placeholder so the linter is happy if DeleteUserCredentials
// returns a typed not-found error in the future. Today the store returns nil on
// no-op deletes (idempotent), so this branch is never taken.
var errNotFoundSentinel = errors.New("not found")
