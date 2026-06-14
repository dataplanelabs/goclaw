package http

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/nextlevelbuilder/goclaw/internal/codexreauth"
	"github.com/nextlevelbuilder/goclaw/internal/i18n"
	"github.com/nextlevelbuilder/goclaw/internal/permissions"
	"github.com/nextlevelbuilder/goclaw/internal/store"
	"github.com/nextlevelbuilder/goclaw/internal/workstation"
)

const codexAuthMaxAge = 10 * time.Minute

// CodexReauthHandler exposes two master-scope-only endpoints:
//
//	POST /v1/codex/reauth/start  — triggers device-auth on the coding-agent pod
//	GET  /v1/codex/reauth/status — reports whether auth.json is fresh
//
// Both are global operations (write to the pod, not a tenant table) so they
// require master scope (owner or master-tenant admin), not just RoleAdmin.
type CodexReauthHandler struct {
	wsStore      store.WorkstationStore
	backendCache *workstation.BackendCache
}

// NewCodexReauthHandler returns a handler wired to the workstation store and cache.
// If either is nil (e.g. lite edition, workstations not enabled) the handler will
// return 503 on every call — that is intentional.
func NewCodexReauthHandler(wsStore store.WorkstationStore, bc *workstation.BackendCache) *CodexReauthHandler {
	return &CodexReauthHandler{wsStore: wsStore, backendCache: bc}
}

func (h *CodexReauthHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /v1/codex/reauth/start", requireAuth(permissions.RoleAdmin, h.handleStart))
	mux.HandleFunc("GET /v1/codex/reauth/status", requireAuth(permissions.RoleAdmin, h.handleStatus))
}

func (h *CodexReauthHandler) handleStart(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	locale := store.LocaleFromContext(ctx)

	if !requireMasterScope(w, r) {
		return
	}

	if h.wsStore == nil || h.backendCache == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"error": i18n.T(locale, i18n.MsgWorkstationRequired),
		})
		return
	}

	tenantID := store.TenantIDFromContext(ctx)
	if tenantID == uuid.Nil {
		// master scope with no explicit tenant — use uuid.Nil (master tenant)
		tenantID = store.MasterTenantID
	}

	info, err := codexreauth.Trigger(ctx, h.wsStore, h.backendCache, tenantID, "")
	if err != nil {
		slog.Warn("codex.reauth.start.failed", "tenant_id", tenantID, "error", err)
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"url":  info.VerificationURL,
		"code": info.UserCode,
	})
}

func (h *CodexReauthHandler) handleStatus(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	locale := store.LocaleFromContext(ctx)

	if !requireMasterScope(w, r) {
		return
	}

	if h.wsStore == nil || h.backendCache == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"error": i18n.T(locale, i18n.MsgWorkstationRequired),
		})
		return
	}

	tenantID := store.TenantIDFromContext(ctx)
	if tenantID == uuid.Nil {
		tenantID = store.MasterTenantID
	}

	result, err := codexreauth.Status(ctx, h.wsStore, h.backendCache, tenantID, "", codexAuthMaxAge)
	if err != nil {
		slog.Warn("codex.reauth.status.failed", "tenant_id", tenantID, "error", err)
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, result)
}
