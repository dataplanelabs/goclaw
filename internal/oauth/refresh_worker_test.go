package oauth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"golang.org/x/oauth2"

	"github.com/nextlevelbuilder/goclaw/internal/config"
	"github.com/nextlevelbuilder/goclaw/internal/store"
)

// --- stubRefreshStore: minimal SecureCLIStore for the worker tests ---

type stubRefreshStore struct {
	mu              sync.Mutex
	rows            []store.SecureCLIUserCredentialWithBinary
	setCalls        []setCall
	deleteCalls     []deleteCall
	listErr         error
}

type setCall struct {
	BinaryID  uuid.UUID
	UserID    string
	Env       []byte
	Metadata  json.RawMessage
	TenantID  uuid.UUID
}

type deleteCall struct {
	BinaryID uuid.UUID
	UserID   string
	TenantID uuid.UUID
}

func (s *stubRefreshStore) ListUserCredentialsByBinaryName(ctx context.Context, binaryName string) ([]store.SecureCLIUserCredentialWithBinary, error) {
	if s.listErr != nil {
		return nil, s.listErr
	}
	return s.rows, nil
}

func (s *stubRefreshStore) SetUserCredentials(ctx context.Context, binaryID uuid.UUID, userID string, encryptedEnv []byte, metadata json.RawMessage) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.setCalls = append(s.setCalls, setCall{
		BinaryID: binaryID, UserID: userID, Env: encryptedEnv, Metadata: metadata,
		TenantID: store.TenantIDFromContext(ctx),
	})
	return nil
}

func (s *stubRefreshStore) DeleteUserCredentials(ctx context.Context, binaryID uuid.UUID, userID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.deleteCalls = append(s.deleteCalls, deleteCall{
		BinaryID: binaryID, UserID: userID,
		TenantID: store.TenantIDFromContext(ctx),
	})
	return nil
}

// Zero-value stubs for interface satisfaction.
func (s *stubRefreshStore) Create(context.Context, *store.SecureCLIBinary) error { return nil }
func (s *stubRefreshStore) Get(context.Context, uuid.UUID) (*store.SecureCLIBinary, error) {
	return nil, nil
}
func (s *stubRefreshStore) Update(context.Context, uuid.UUID, map[string]any) error { return nil }
func (s *stubRefreshStore) Delete(context.Context, uuid.UUID) error                 { return nil }
func (s *stubRefreshStore) List(context.Context) ([]store.SecureCLIBinary, error) {
	return nil, nil
}
func (s *stubRefreshStore) ListEnabled(context.Context) ([]store.SecureCLIBinary, error) {
	return nil, nil
}
func (s *stubRefreshStore) ListForAgent(context.Context, uuid.UUID) ([]store.SecureCLIBinary, error) {
	return nil, nil
}
func (s *stubRefreshStore) IsRegisteredBinary(context.Context, string) (bool, error) {
	return false, nil
}
func (s *stubRefreshStore) LookupByBinary(context.Context, string, *uuid.UUID, string) (*store.SecureCLIBinary, error) {
	return nil, nil
}
func (s *stubRefreshStore) GetByName(context.Context, string) (*store.SecureCLIBinary, error) {
	return nil, nil
}
func (s *stubRefreshStore) GetUserCredentials(context.Context, uuid.UUID, string) (*store.SecureCLIUserCredential, error) {
	return nil, nil
}
func (s *stubRefreshStore) ListUserCredentials(context.Context, uuid.UUID) ([]store.SecureCLIUserCredential, error) {
	return nil, nil
}

// --- helpers ---

func googleClientWithFakeTokenServer(t *testing.T, response string, status int) *GoogleOAuthClient {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(response))
	}))
	t.Cleanup(srv.Close)
	c := NewGoogleClient(config.OAuthGoogleConfig{
		ClientID: "cid", ClientSecret: "sec",
		RedirectURL: "https://example.test/cb",
	})
	c.config.Endpoint = oauth2.Endpoint{
		AuthURL:   srv.URL + "/auth",
		TokenURL:  srv.URL + "/token",
		AuthStyle: oauth2.AuthStyleInParams,
	}
	return c
}

func nearExpiry(rt string, when time.Time) store.SecureCLIUserCredentialWithBinary {
	env, _ := json.Marshal(map[string]string{"GWS_REFRESH_TOKEN": rt})
	meta, _ := json.Marshal(map[string]any{
		"account_email": "op@example.com",
		"expires_at":    when.UTC().Format(time.RFC3339),
	})
	return store.SecureCLIUserCredentialWithBinary{
		SecureCLIUserCredential: store.SecureCLIUserCredential{
			BinaryID: uuid.New(), UserID: uuid.NewString(),
			EncryptedEnv: env, Metadata: meta,
		},
		BinaryName: "gws",
		TenantID:   uuid.New(),
	}
}

// --- tests ---

func TestRefreshWorker_RunOnce_NoRows(t *testing.T) {
	w := NewRefreshWorker(&stubRefreshStore{}, googleClientWithFakeTokenServer(t, `{}`, 200), time.Hour, 7*24*time.Hour)
	w.runOnce(context.Background())
	if !w.Healthy() {
		t.Error("worker should be healthy after empty-tick run")
	}
}

func TestRefreshWorker_SkipsFarExpiry(t *testing.T) {
	st := &stubRefreshStore{rows: []store.SecureCLIUserCredentialWithBinary{
		nearExpiry("r-1", time.Now().Add(30*24*time.Hour)), // 30d > 7d threshold
	}}
	w := NewRefreshWorker(st, googleClientWithFakeTokenServer(t, `{}`, 200), time.Hour, 7*24*time.Hour)
	w.runOnce(context.Background())
	if len(st.setCalls) != 0 {
		t.Errorf("expected 0 set calls (far expiry), got %d", len(st.setCalls))
	}
	if len(st.deleteCalls) != 0 {
		t.Errorf("expected 0 delete calls, got %d", len(st.deleteCalls))
	}
}

func TestRefreshWorker_RefreshesNearExpiry(t *testing.T) {
	row := nearExpiry("r-old", time.Now().Add(3*24*time.Hour)) // 3d < 7d threshold
	st := &stubRefreshStore{rows: []store.SecureCLIUserCredentialWithBinary{row}}
	resp := `{"access_token":"ac-new","refresh_token":"r-new","expires_in":3600,"token_type":"Bearer"}`
	w := NewRefreshWorker(st, googleClientWithFakeTokenServer(t, resp, 200), time.Hour, 7*24*time.Hour)
	w.runOnce(context.Background())

	if len(st.setCalls) != 1 {
		t.Fatalf("expected 1 SetUserCredentials call, got %d", len(st.setCalls))
	}
	got := st.setCalls[0]
	if got.BinaryID != row.BinaryID || got.UserID != row.UserID || got.TenantID != row.TenantID {
		t.Errorf("set call scope mismatch: %+v vs row %+v", got, row)
	}
	var newEnv map[string]string
	_ = json.Unmarshal(got.Env, &newEnv)
	if newEnv["GWS_REFRESH_TOKEN"] != "r-new" {
		t.Errorf("expected rotated refresh_token, got %q", newEnv["GWS_REFRESH_TOKEN"])
	}
	if !strings.Contains(string(got.Metadata), "refreshed_at") {
		t.Errorf("metadata missing refreshed_at: %s", got.Metadata)
	}
	if !strings.Contains(string(got.Metadata), "account_email") {
		t.Errorf("metadata lost account_email: %s", got.Metadata)
	}
}

func TestRefreshWorker_RefreshesNearExpiry_PreservesRefreshTokenWhenNotRotated(t *testing.T) {
	row := nearExpiry("r-stick", time.Now().Add(time.Hour))
	st := &stubRefreshStore{rows: []store.SecureCLIUserCredentialWithBinary{row}}
	// Google returns no refresh_token (not rotated).
	resp := `{"access_token":"ac","expires_in":3600,"token_type":"Bearer"}`
	w := NewRefreshWorker(st, googleClientWithFakeTokenServer(t, resp, 200), time.Hour, 7*24*time.Hour)
	w.runOnce(context.Background())

	var newEnv map[string]string
	_ = json.Unmarshal(st.setCalls[0].Env, &newEnv)
	if newEnv["GWS_REFRESH_TOKEN"] != "r-stick" {
		t.Errorf("expected preserved refresh_token, got %q", newEnv["GWS_REFRESH_TOKEN"])
	}
}

func TestRefreshWorker_RevokedTokenDeletesRow(t *testing.T) {
	row := nearExpiry("r-revoked", time.Now().Add(time.Hour))
	st := &stubRefreshStore{rows: []store.SecureCLIUserCredentialWithBinary{row}}
	// Google returns invalid_grant — refresh token revoked.
	resp := `{"error":"invalid_grant","error_description":"Token has been expired or revoked."}`
	w := NewRefreshWorker(st, googleClientWithFakeTokenServer(t, resp, 400), time.Hour, 7*24*time.Hour)
	w.runOnce(context.Background())

	if len(st.deleteCalls) != 1 {
		t.Fatalf("expected 1 Delete call for revoked token, got %d", len(st.deleteCalls))
	}
	if st.deleteCalls[0].TenantID != row.TenantID {
		t.Errorf("delete tenant scope mismatch: got %v want %v", st.deleteCalls[0].TenantID, row.TenantID)
	}
	if len(st.setCalls) != 0 {
		t.Errorf("expected 0 set calls on revoke path, got %d", len(st.setCalls))
	}
}

func TestRefreshWorker_TransientErrorNoDelete(t *testing.T) {
	row := nearExpiry("r-transient", time.Now().Add(time.Hour))
	st := &stubRefreshStore{rows: []store.SecureCLIUserCredentialWithBinary{row}}
	// 503 from Google — transient, no revocation.
	w := NewRefreshWorker(st, googleClientWithFakeTokenServer(t, `{}`, 503), time.Hour, 7*24*time.Hour)
	w.runOnce(context.Background())

	if len(st.deleteCalls) != 0 {
		t.Errorf("transient error must NOT delete row, got %d delete calls", len(st.deleteCalls))
	}
	if len(st.setCalls) != 0 {
		t.Errorf("transient error must NOT write back, got %d set calls", len(st.setCalls))
	}
}

func TestRefreshWorker_Healthy_BeforeFirstTick(t *testing.T) {
	w := NewRefreshWorker(&stubRefreshStore{}, nil, time.Hour, time.Hour)
	if !w.Healthy() {
		t.Error("worker should be healthy before first tick (startup window)")
	}
}

func TestRefreshWorker_Healthy_FlipsUnhealthyAfterStaleTick(t *testing.T) {
	w := NewRefreshWorker(&stubRefreshStore{}, googleClientWithFakeTokenServer(t, `{}`, 200), 50*time.Millisecond, time.Hour)
	w.runOnce(context.Background())
	if !w.Healthy() {
		t.Fatal("worker should be healthy right after tick")
	}
	time.Sleep(200 * time.Millisecond) // > 2× tick
	if w.Healthy() {
		t.Error("worker should be unhealthy after stale window (2× tick)")
	}
}

func TestRefreshWorker_DisabledWhenGoogleNotConfigured(t *testing.T) {
	st := &stubRefreshStore{rows: []store.SecureCLIUserCredentialWithBinary{
		nearExpiry("r", time.Now().Add(time.Hour)),
	}}
	bare := NewGoogleClient(config.OAuthGoogleConfig{}) // not configured
	w := NewRefreshWorker(st, bare, time.Hour, time.Hour)
	ctx, cancel := context.WithCancel(context.Background())
	w.Start(ctx)
	time.Sleep(50 * time.Millisecond)
	cancel()
	if len(st.setCalls) != 0 || len(st.deleteCalls) != 0 {
		t.Errorf("worker should no-op when google not configured: %d set, %d delete",
			len(st.setCalls), len(st.deleteCalls))
	}
}

// silence unused-import lint when only some tests run
var _ = fmt.Sprintf
