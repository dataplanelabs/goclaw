package http

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/nextlevelbuilder/goclaw/internal/store"
)

// injectMasterCtx bypasses requireAuth by pre-enriching the request context with
// master-scope identity, matching the pattern used in other unit tests in this package.
func injectMasterCtx(r *http.Request) *http.Request {
	ctx := r.Context()
	ctx = store.WithRole(ctx, store.RoleOwner)
	ctx = store.WithUserID(ctx, uuid.New().String())
	ctx = store.WithTenantID(ctx, uuid.Nil)
	return r.WithContext(ctx)
}

// injectNonMasterCtx pre-enriches request with a non-master scope.
func injectNonMasterCtx(r *http.Request) *http.Request {
	ctx := r.Context()
	ctx = store.WithRole(ctx, "admin")
	ctx = store.WithUserID(ctx, uuid.New().String())
	ctx = store.WithTenantID(ctx, uuid.New()) // non-nil, non-master tenant
	return r.WithContext(ctx)
}

// TestCodexReauthHandler_NilDeps_MasterScope verifies that calling start/status with nil
// workstation deps returns 503 (not 200 or panic). No LLM is involved.
func TestCodexReauthHandler_NilDeps_MasterScope(t *testing.T) {
	h := NewCodexReauthHandler(nil, nil)

	for _, tc := range []struct {
		method string
		path   string
	}{
		{"POST", "/v1/codex/reauth/start"},
		{"GET", "/v1/codex/reauth/status"},
	} {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.path, nil)
			req = injectMasterCtx(req)
			w := httptest.NewRecorder()

			// Call the handler directly (skip requireAuth wrapper for unit testing)
			if tc.method == "POST" {
				h.handleStart(w, req)
			} else {
				h.handleStatus(w, req)
			}

			if w.Code != http.StatusServiceUnavailable {
				t.Errorf("want 503, got %d", w.Code)
			}
		})
	}
}

// TestCodexReauthHandler_NonMasterRejected verifies requireMasterScope gate rejects
// non-master-scope callers with 403.
func TestCodexReauthHandler_NonMasterRejected(t *testing.T) {
	h := NewCodexReauthHandler(nil, nil)

	for _, tc := range []struct {
		method string
		fn     func(http.ResponseWriter, *http.Request)
	}{
		{"POST", h.handleStart},
		{"GET", h.handleStatus},
	} {
		t.Run(tc.method, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, "/v1/codex/reauth/x", nil)
			req = injectNonMasterCtx(req)
			w := httptest.NewRecorder()
			tc.fn(w, req)
			if w.Code != http.StatusForbidden {
				t.Errorf("want 403, got %d (requireMasterScope gate must fire before nil-dep check)", w.Code)
			}
		})
	}
}
