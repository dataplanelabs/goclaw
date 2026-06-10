//go:build sqlite || sqliteonly

package sqlitestore

import (
	"database/sql"
	"testing"
)

// TestSQLiteSchemaUpgrade_47_to_48_VaultPathNormalize verifies the v47→v48
// data migration: strip the legacy tenants/<slug>/ prefix from vault_documents
// paths, deduping prefixed-vs-bare collisions (keep latest updated_at) BEFORE
// the strip so the unique index never violates.
func TestSQLiteSchemaUpgrade_47_to_48_VaultPathNormalize(t *testing.T) {
	db := openTestDBAtVersion(t, 47)

	tenantID := "11111111-1111-1111-1111-111111111111"
	agentID := "22222222-2222-2222-2222-222222222222"
	mustExec(t, db, `INSERT INTO tenants (id, name, slug, status) VALUES (?, 'T', 'shtp', 'active')`, tenantID)
	mustExec(t, db, `INSERT INTO agents (id, agent_key, display_name, status, tenant_id, owner_id, model, provider, agent_type) VALUES (?, 'a', 'A', 'active', ?, 'o', 'm', 'p', 'predefined')`, agentID, tenantID)

	seed := func(id, path, hash, updatedAt string) {
		mustExec(t, db, `INSERT INTO vault_documents
			(id, tenant_id, agent_id, scope, path, content_hash, created_at, updated_at)
			VALUES (?, ?, ?, 'personal', ?, ?, ?, ?)`,
			id, tenantID, agentID, path, hash, updatedAt, updatedAt)
	}

	// Collision: prefixed (newer) + bare (older) strip to the same path.
	seed("doc-prefixed-new", "tenants/shtp/notes/a.md", "hash-new", "2026-02-01T00:00:00.000Z")
	seed("doc-bare-old", "notes/a.md", "hash-old", "2026-01-01T00:00:00.000Z")
	// Non-colliding prefixed, multi-segment.
	seed("doc-multi", "tenants/shtp/x/y/z.md", "hash-multi", "2026-01-15T00:00:00.000Z")
	// Already bare, no counterpart.
	seed("doc-clean", "memos/e.md", "hash-clean", "2026-01-15T00:00:00.000Z")
	// Degenerate single-segment tenants/<slug> (no file part) — excluded by the %/% gate.
	seed("doc-degenerate", "tenants/shtp", "hash-degen", "2026-01-15T00:00:00.000Z")

	if err := EnsureSchema(db); err != nil {
		t.Fatalf("EnsureSchema (v47→48) failed: %v", err)
	}

	var version int
	db.QueryRow("SELECT version FROM schema_version LIMIT 1").Scan(&version)
	if version != SchemaVersion {
		t.Errorf("schema version = %d, want %d", version, SchemaVersion)
	}

	// No tenants/<slug>/ prefix survives (the degenerate single-segment row stays).
	var prefixed int
	db.QueryRow(`SELECT count(*) FROM vault_documents WHERE path LIKE 'tenants/%/%'`).Scan(&prefixed)
	if prefixed != 0 {
		t.Errorf("rows still prefixed after migration = %d, want 0", prefixed)
	}

	// Dedupe kept the newer row (hash-new) at the stripped path; older deleted.
	assertPath(t, db, "doc-prefixed-new", "notes/a.md")
	assertGone(t, db, "doc-bare-old")
	assertPath(t, db, "doc-multi", "x/y/z.md")
	assertPath(t, db, "doc-clean", "memos/e.md")
	assertPath(t, db, "doc-degenerate", "tenants/shtp")

	// The surviving collision row is the newer one by content_hash.
	var hash string
	if err := db.QueryRow(`SELECT content_hash FROM vault_documents WHERE path = 'notes/a.md'`).Scan(&hash); err != nil {
		t.Fatalf("collision survivor query: %v", err)
	}
	if hash != "hash-new" {
		t.Errorf("collision survivor hash = %q, want hash-new (latest updated_at)", hash)
	}
}

func assertPath(t *testing.T, db *sql.DB, id, wantPath string) {
	t.Helper()
	var got string
	if err := db.QueryRow(`SELECT path FROM vault_documents WHERE id = ?`, id).Scan(&got); err != nil {
		t.Fatalf("row %s missing: %v", id, err)
	}
	if got != wantPath {
		t.Errorf("row %s path = %q, want %q", id, got, wantPath)
	}
}

func assertGone(t *testing.T, db *sql.DB, id string) {
	t.Helper()
	var n int
	db.QueryRow(`SELECT count(*) FROM vault_documents WHERE id = ?`, id).Scan(&n)
	if n != 0 {
		t.Errorf("row %s should have been deduped away, still present", id)
	}
}
