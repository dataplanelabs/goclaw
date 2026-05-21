//go:build sqlite || sqliteonly

package sqlitestore

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/nextlevelbuilder/goclaw/internal/store"
)

// TestSQLiteProviderStore_WriteOnlyHashRoundtrip exercises the gcplane
// drift-detection contract for Provider.apiKey (same pattern as CronJob
// added in migration 000059, extended to providers in migration 000060):
// gcplane writes an opaque hash on every create/update; goclaw must echo
// it back via Get/List so gcplane can detect drift on rotated keys
// without exposing the underlying value.
func TestSQLiteProviderStore_WriteOnlyHashRoundtrip(t *testing.T) {
	db, err := OpenDB(filepath.Join(t.TempDir(), "providers.db"))
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := EnsureSchema(db); err != nil {
		t.Fatalf("EnsureSchema: %v", err)
	}
	// pkgSqlxDB is normally initialized by NewSQLiteStores; bare-DB tests
	// must initialize it explicitly so Get/Select paths don't nil-panic.
	initSqlx(db)

	ctx := store.WithCrossTenant(store.WithTenantID(context.Background(), store.MasterTenantID))
	ps := NewSQLiteProviderStore(db, "")

	// Create with empty hash (default for legacy callers / UI).
	p := &store.LLMProviderData{
		Name:         "test-zai",
		DisplayName:  "Test Zai",
		ProviderType: store.ProviderZaiCoding,
		APIBase:      "https://api.z.ai/api/coding/paas/v4",
		APIKey:       "sk-test-initial",
		Enabled:      true,
	}
	if err := ps.CreateProvider(ctx, p); err != nil {
		t.Fatalf("CreateProvider: %v", err)
	}
	got, err := ps.GetProviderByName(ctx, "test-zai")
	if err != nil {
		t.Fatalf("GetProviderByName: %v", err)
	}
	if got.WriteOnlyHash != "" {
		t.Fatalf("fresh provider: got hash=%q, want empty", got.WriteOnlyHash)
	}

	// Create with hash (the gcplane path).
	p2 := &store.LLMProviderData{
		Name:          "test-openai",
		DisplayName:   "Test OpenAI",
		ProviderType:  store.ProviderOpenAICompat,
		APIBase:       "https://api.openai.com/v1",
		APIKey:        "sk-test-with-hash",
		Enabled:       true,
		WriteOnlyHash: "sha256:abc123",
	}
	if err := ps.CreateProvider(ctx, p2); err != nil {
		t.Fatalf("CreateProvider with hash: %v", err)
	}
	got2, err := ps.GetProviderByName(ctx, "test-openai")
	if err != nil {
		t.Fatalf("GetProviderByName: %v", err)
	}
	if got2.WriteOnlyHash != "sha256:abc123" {
		t.Fatalf("create-with-hash: got %q, want %q", got2.WriteOnlyHash, "sha256:abc123")
	}

	// Update via map (handleUpdateProvider path — JSON body → allowlist →
	// updates map → execMapUpdateWhereTenant uses keys as column names).
	if err := ps.UpdateProvider(ctx, got.ID, map[string]any{
		"write_only_hash": "sha256:rotated",
	}); err != nil {
		t.Fatalf("UpdateProvider hash: %v", err)
	}
	got, err = ps.GetProviderByName(ctx, "test-zai")
	if err != nil {
		t.Fatalf("GetProviderByName after update: %v", err)
	}
	if got.WriteOnlyHash != "sha256:rotated" {
		t.Fatalf("after update: got %q, want %q", got.WriteOnlyHash, "sha256:rotated")
	}

	// List echoes hash for every row — gcplane's observe path.
	listed, err := ps.ListProviders(ctx)
	if err != nil {
		t.Fatalf("ListProviders: %v", err)
	}
	byName := map[string]string{}
	for _, lp := range listed {
		byName[lp.Name] = lp.WriteOnlyHash
	}
	if byName["test-zai"] != "sha256:rotated" {
		t.Fatalf("list test-zai: got %q, want sha256:rotated", byName["test-zai"])
	}
	if byName["test-openai"] != "sha256:abc123" {
		t.Fatalf("list test-openai: got %q, want sha256:abc123", byName["test-openai"])
	}

	// ListAllProviders (cross-tenant, server-internal) must also echo.
	allCtx := store.WithCrossTenant(context.Background())
	all, err := ps.ListAllProviders(allCtx)
	if err != nil {
		t.Fatalf("ListAllProviders: %v", err)
	}
	if len(all) < 2 {
		t.Fatalf("ListAllProviders: got %d, want >=2", len(all))
	}
}
