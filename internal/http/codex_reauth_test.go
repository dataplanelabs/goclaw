package http

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/nextlevelbuilder/goclaw/internal/store"
	"github.com/nextlevelbuilder/goclaw/internal/workstation"
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

// stubWsStore captures the tenant UUID seen in GetByKey and returns ErrNoRows.
// This lets tests verify the tenantID resolved by the handler without needing a real DB.
type stubWsStore struct {
	capturedTenant uuid.UUID
}

func (s *stubWsStore) GetByKey(ctx context.Context, _ string) (*store.Workstation, error) {
	s.capturedTenant = store.TenantIDFromContext(ctx)
	return nil, sql.ErrNoRows
}
func (s *stubWsStore) Create(_ context.Context, _ *store.Workstation) error { return nil }
func (s *stubWsStore) GetByID(_ context.Context, _ uuid.UUID) (*store.Workstation, error) {
	return nil, sql.ErrNoRows
}
func (s *stubWsStore) List(_ context.Context) ([]store.Workstation, error) { return nil, nil }
func (s *stubWsStore) Update(_ context.Context, _ uuid.UUID, _ map[string]any) error { return nil }
func (s *stubWsStore) SetActive(_ context.Context, _ uuid.UUID, _ bool) error { return nil }
func (s *stubWsStore) Delete(_ context.Context, _ uuid.UUID) error { return nil }

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

// TestCodexReauthHandler_MasterScopePropagatesTenant verifies that handleStart and
// handleStatus pass MasterTenantID (not uuid.Nil) to the workstation store when
// the request context carries uuid.Nil as the tenant (typical master-scope gateway token auth).
// Root cause guarded: GetByKey returns ErrNoRows on uuid.Nil — callers that omit the
// Nil→MasterTenantID coercion silently drop the lookup.
func TestCodexReauthHandler_MasterScopePropagatesTenant(t *testing.T) {
	stub := &stubWsStore{}
	bc := workstation.NewBackendCache(stub, time.Minute)
	h := NewCodexReauthHandler(stub, bc)

	for _, tc := range []struct {
		method string
		fn     func(http.ResponseWriter, *http.Request)
	}{
		{"POST", h.handleStart},
		{"GET", h.handleStatus},
	} {
		t.Run(tc.method, func(t *testing.T) {
			stub.capturedTenant = uuid.Nil // reset between sub-tests

			req := httptest.NewRequest(tc.method, "/v1/codex/reauth/x", nil)
			req = injectMasterCtx(req) // tenant = uuid.Nil in context
			w := httptest.NewRecorder()
			tc.fn(w, req)

			// Handler must resolve uuid.Nil → MasterTenantID before calling Trigger;
			// Trigger injects it into the store context, which stub captures.
			if stub.capturedTenant != store.MasterTenantID {
				t.Errorf("GetByKey saw tenant %v, want MasterTenantID %v",
					stub.capturedTenant, store.MasterTenantID)
			}
			// Stub returns ErrNoRows → 502 (workstation not found), not 503/403/panic.
			if w.Code != http.StatusBadGateway {
				t.Errorf("want 502, got %d", w.Code)
			}
		})
	}
}
