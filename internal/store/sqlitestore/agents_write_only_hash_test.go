//go:build sqlite || sqliteonly

package sqlitestore

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/store"
)

// TestSQLiteAgentStore_WriteOnlyHashRoundtrip exercises the gcplane
// drift-detection contract for agents: write a hash via Create, read via
// GetByID + GetByKey, overwrite via Update, confirm set-not-append.
// Mirrors the cron + provider write_only_hash tests.
func TestSQLiteAgentStore_WriteOnlyHashRoundtrip(t *testing.T) {
	db, err := OpenDB(filepath.Join(t.TempDir(), "agent_woh.db"))
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	if err := EnsureSchema(db); err != nil {
		t.Fatalf("EnsureSchema: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	tenantID := uuid.Must(uuid.NewV7())
	if _, err := db.Exec(
		`INSERT INTO tenants (id, name, slug, status) VALUES (?,?,?,'active')`,
		tenantID.String(), "woh-"+tenantID.String()[:8], "woh"+tenantID.String()[:8],
	); err != nil {
		t.Fatalf("seed tenant: %v", err)
	}

	s := NewSQLiteAgentStore(db)
	ctx := store.WithTenantID(context.Background(), tenantID)

	ag := &store.AgentData{
		AgentKey:    "agent-woh-test",
		DisplayName: "Agent WOH Test",
		TenantID:    tenantID,
		OwnerID:     "owner-1",
		AgentType:   store.AgentTypePredefined,
		Status:      store.AgentStatusActive,
		Provider:    "test",
		Model:       "test-model",
	}
	if err := s.Create(ctx, ag); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := s.GetByID(ctx, ag.ID)
	if err != nil {
		t.Fatalf("GetByID fresh: %v", err)
	}
	if got.WriteOnlyHash != "" {
		t.Fatalf("expected empty WriteOnlyHash on fresh row, got %q", got.WriteOnlyHash)
	}

	hash := "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	if err := s.Update(ctx, ag.ID, map[string]any{"write_only_hash": hash}); err != nil {
		t.Fatalf("Update write_only_hash: %v", err)
	}

	got, err = s.GetByID(ctx, ag.ID)
	if err != nil {
		t.Fatalf("GetByID after patch: %v", err)
	}
	if got.WriteOnlyHash != hash {
		t.Fatalf("GetByID WriteOnlyHash: got %q, want %q", got.WriteOnlyHash, hash)
	}

	byKey, err := s.GetByKey(ctx, ag.AgentKey)
	if err != nil {
		t.Fatalf("GetByKey: %v", err)
	}
	if byKey.WriteOnlyHash != hash {
		t.Fatalf("GetByKey WriteOnlyHash: got %q, want %q", byKey.WriteOnlyHash, hash)
	}

	newHash := "sha256:deadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeef"
	if err := s.Update(ctx, ag.ID, map[string]any{"write_only_hash": newHash}); err != nil {
		t.Fatalf("Update overwrite: %v", err)
	}
	got, _ = s.GetByID(ctx, ag.ID)
	if got.WriteOnlyHash != newHash {
		t.Fatalf("WriteOnlyHash overwrite: got %q, want %q", got.WriteOnlyHash, newHash)
	}
}
