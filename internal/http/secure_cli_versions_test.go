package http

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/store"
)

type stubVersionsStore struct {
	rows []store.SecureCLIBinary
	err  error
}

func (s *stubVersionsStore) Create(ctx context.Context, b *store.SecureCLIBinary) error { return nil }
func (s *stubVersionsStore) Get(ctx context.Context, id uuid.UUID) (*store.SecureCLIBinary, error) {
	return nil, nil
}
func (s *stubVersionsStore) Update(ctx context.Context, id uuid.UUID, updates map[string]any) error {
	return nil
}
func (s *stubVersionsStore) Delete(ctx context.Context, id uuid.UUID) error { return nil }
func (s *stubVersionsStore) List(ctx context.Context) ([]store.SecureCLIBinary, error) {
	return s.rows, s.err
}
func (s *stubVersionsStore) ListEnabled(ctx context.Context) ([]store.SecureCLIBinary, error) {
	return s.rows, s.err
}
func (s *stubVersionsStore) ListForAgent(ctx context.Context, agentID uuid.UUID) ([]store.SecureCLIBinary, error) {
	return nil, nil
}
func (s *stubVersionsStore) LookupByBinary(ctx context.Context, binaryName string, agentID *uuid.UUID, userID string) (*store.SecureCLIBinary, error) {
	return nil, nil
}
func (s *stubVersionsStore) IsRegisteredBinary(ctx context.Context, binaryName string) (bool, error) {
	return false, nil
}
func (s *stubVersionsStore) GetByName(ctx context.Context, binaryName string) (*store.SecureCLIBinary, error) {
	return nil, nil
}
func (s *stubVersionsStore) ListUserCredentialsByBinaryName(ctx context.Context, binaryName string) ([]store.SecureCLIUserCredentialWithBinary, error) {
	return nil, nil
}
func (s *stubVersionsStore) GetUserCredentials(ctx context.Context, binaryID uuid.UUID, userID string) (*store.SecureCLIUserCredential, error) {
	return nil, nil
}
func (s *stubVersionsStore) SetUserCredentials(ctx context.Context, binaryID uuid.UUID, userID string, encryptedEnv []byte, metadata json.RawMessage) error {
	return nil
}
func (s *stubVersionsStore) DeleteUserCredentials(ctx context.Context, binaryID uuid.UUID, userID string) error {
	return nil
}
func (s *stubVersionsStore) ListUserCredentials(ctx context.Context, binaryID uuid.UUID) ([]store.SecureCLIUserCredential, error) {
	return nil, nil
}

func TestSecureCLI_HandleCliVersions_ReturnsRegistered(t *testing.T) {
	ghVer, kubectlVer := "2.71.0", "1.32.0"
	st := &stubVersionsStore{
		rows: []store.SecureCLIBinary{
			{BinaryName: "gh", Version: &ghVer, Enabled: true},
			{BinaryName: "kubectl", Version: &kubectlVer, Enabled: true},
		},
	}
	req := httptest.NewRequest(http.MethodGet, "/v1/system/cli-versions", nil)
	rec := httptest.NewRecorder()
	NewSecureCLIHandler(st, nil).handleCliVersions(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var got struct {
		Versions map[string]string `json:"versions"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Versions["gh"] != "2.71.0" || got.Versions["kubectl"] != "1.32.0" {
		t.Fatalf("versions=%+v", got.Versions)
	}
}

func TestSecureCLI_HandleCliVersions_OmitsNullVersions(t *testing.T) {
	ghVer, blank := "2.71.0", ""
	st := &stubVersionsStore{
		rows: []store.SecureCLIBinary{
			{BinaryName: "gh", Version: &ghVer, Enabled: true},
			{BinaryName: "old-cli", Version: nil, Enabled: true},
			{BinaryName: "blank-cli", Version: &blank, Enabled: true},
		},
	}
	req := httptest.NewRequest(http.MethodGet, "/v1/system/cli-versions", nil)
	rec := httptest.NewRecorder()
	NewSecureCLIHandler(st, nil).handleCliVersions(rec, req)

	var got struct {
		Versions map[string]string `json:"versions"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	if _, ok := got.Versions["old-cli"]; ok {
		t.Errorf("old-cli should be omitted (NULL version)")
	}
	if _, ok := got.Versions["blank-cli"]; ok {
		t.Errorf("blank-cli should be omitted (empty string version)")
	}
	if got.Versions["gh"] != "2.71.0" {
		t.Errorf("gh=%q want 2.71.0", got.Versions["gh"])
	}
}

func TestSecureCLI_HandleCliVersions_EmptyResult(t *testing.T) {
	st := &stubVersionsStore{rows: nil}
	req := httptest.NewRequest(http.MethodGet, "/v1/system/cli-versions", nil)
	rec := httptest.NewRecorder()
	NewSecureCLIHandler(st, nil).handleCliVersions(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d", rec.Code)
	}
	var got struct {
		Versions map[string]string `json:"versions"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	if len(got.Versions) != 0 {
		t.Fatalf("expected empty map, got %v", got.Versions)
	}
}
