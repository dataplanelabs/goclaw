package http

import (
	"net/http"
)

// healthProbe is the surface exposed by *oauth.RefreshWorker that this
// handler queries. Decoupled so we don't import internal/oauth into this file.
type healthProbe interface {
	Healthy() bool
}

// OAuthRefreshHealthHandler exposes a liveness probe for the per-operator
// OAuth refresh worker (B3-01 Phase 4 Rev-1). Returns 200 when the worker's
// last successful tick is within 2× tick interval; 503 otherwise.
type OAuthRefreshHealthHandler struct {
	worker healthProbe
}

func NewOAuthRefreshHealthHandler(w healthProbe) *OAuthRefreshHealthHandler {
	return &OAuthRefreshHealthHandler{worker: w}
}

func (h *OAuthRefreshHealthHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /v1/healthz/oauth-refresh-worker", h.handle)
}

func (h *OAuthRefreshHealthHandler) handle(w http.ResponseWriter, r *http.Request) {
	if h.worker == nil || !h.worker.Healthy() {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{
			"status": "unhealthy",
			"worker": "oauth-refresh",
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"status": "ok",
		"worker": "oauth-refresh",
	})
}
