package codexreauth

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/nextlevelbuilder/goclaw/internal/store"
)

func TestParseDeviceAuth_BothPresent(t *testing.T) {
	output := `
Checking for updates...
Please visit the following URL on your device:
https://auth.openai.com/activate?user_code=ABCD-1234
User code: ABCD-1234
Enter code: ABCD-1234
Waiting for approval...
`
	info := parseDeviceAuth(output)
	if info == nil {
		t.Fatal("expected non-nil DeviceAuthInfo")
	}
	if info.VerificationURL == "" {
		t.Error("expected VerificationURL to be set")
	}
	if info.UserCode != "ABCD-1234" {
		t.Errorf("expected UserCode 'ABCD-1234', got %q", info.UserCode)
	}
}

func TestParseDeviceAuth_MissingCode(t *testing.T) {
	output := `
Visit: https://auth.openai.com/activate
Waiting...
`
	info := parseDeviceAuth(output)
	if info != nil {
		t.Error("expected nil when user code is missing")
	}
}

func TestParseDeviceAuth_MissingURL(t *testing.T) {
	output := `
User code: XYZW-9876
Enter code: XYZW-9876
`
	info := parseDeviceAuth(output)
	if info != nil {
		t.Error("expected nil when URL is missing")
	}
}

func TestParseDeviceAuth_Empty(t *testing.T) {
	if parseDeviceAuth("") != nil {
		t.Error("expected nil for empty output")
	}
}

func TestParseDeviceAuth_URLExclusion(t *testing.T) {
	// URLs that don't contain openai.com or auth should be excluded
	output := `
Visit: https://example.com/check-updates
User code: ABCD-1234
`
	info := parseDeviceAuth(output)
	// example.com URL should be excluded, so info should be nil (no valid URL)
	if info != nil {
		t.Error("expected nil when URL doesn't match openai/auth pattern")
	}
}

func TestParseDeviceAuth_CodeFormats(t *testing.T) {
	cases := []struct {
		name     string
		line     string
		wantCode string
	}{
		{"enter_code", "Enter code: ABCD-1234", "ABCD-1234"},
		{"user_code", "User code: WXYZ-5678", "WXYZ-5678"},
		{"case_insensitive", "USER CODE: AAAA-BBBB", "AAAA-BBBB"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			output := "https://auth.openai.com/device\n" + tc.line
			info := parseDeviceAuth(output)
			if info == nil {
				t.Fatal("expected non-nil")
			}
			if info.UserCode != tc.wantCode {
				t.Errorf("got %q, want %q", info.UserCode, tc.wantCode)
			}
		})
	}
}

// fakeWsStore returns its workstation only when the ctx tenant matches wantTenant.
type fakeWsStore struct {
	wantTenant uuid.UUID
	ws         *store.Workstation
}

func (f *fakeWsStore) GetByKey(ctx context.Context, _ string) (*store.Workstation, error) {
	if store.TenantIDFromContext(ctx) == f.wantTenant {
		return f.ws, nil
	}
	return nil, sql.ErrNoRows
}
func (f *fakeWsStore) Create(context.Context, *store.Workstation) error { return nil }
func (f *fakeWsStore) GetByID(context.Context, uuid.UUID) (*store.Workstation, error) {
	return nil, sql.ErrNoRows
}
func (f *fakeWsStore) List(context.Context) ([]store.Workstation, error)       { return nil, nil }
func (f *fakeWsStore) Update(context.Context, uuid.UUID, map[string]any) error { return nil }
func (f *fakeWsStore) SetActive(context.Context, uuid.UUID, bool) error        { return nil }
func (f *fakeWsStore) Delete(context.Context, uuid.UUID) error                 { return nil }

func TestResolveWorkstation_MasterFallback(t *testing.T) {
	ws := &store.Workstation{}
	tenant := uuid.MustParse("11111111-1111-1111-1111-111111111111")

	// Workstation lives under master; a non-master caller still resolves it via fallback.
	if got, err := resolveWorkstation(context.Background(), &fakeWsStore{wantTenant: store.MasterTenantID, ws: ws}, tenant, "coding-agent"); err != nil || got != ws {
		t.Fatalf("master fallback: got (%v, %v), want (ws, nil)", got, err)
	}
	// Workstation under the caller's own tenant: no fallback needed.
	if got, err := resolveWorkstation(context.Background(), &fakeWsStore{wantTenant: tenant, ws: ws}, tenant, "coding-agent"); err != nil || got != ws {
		t.Fatalf("own tenant: got (%v, %v), want (ws, nil)", got, err)
	}
	// Found under neither caller nor master: ErrNoRows surfaces.
	if _, err := resolveWorkstation(context.Background(), &fakeWsStore{wantTenant: uuid.New()}, tenant, "coding-agent"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("not found: want sql.ErrNoRows, got %v", err)
	}
}
