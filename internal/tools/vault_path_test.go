package tools

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/store"
)

func nonMasterTenantCtx(slug string) context.Context {
	ctx := store.WithTenantID(context.Background(), uuid.MustParse("019d542d-5c1f-74e9-9e67-f65044b7445c"))
	return store.WithTenantSlug(ctx, slug)
}

// Interceptor stores tenant-root-relative paths (no tenants/<slug>/ prefix) — the
// characterization deferred from Phase 1, now testable via the extracted helper.
func TestVaultInterceptorRelPath_TenantRootRelative(t *testing.T) {
	ws := t.TempDir()
	v := &VaultInterceptor{workspace: ws}

	// Non-master tenant: file under <ws>/tenants/<slug>/... → bare path.
	ctx := nonMasterTenantCtx("shtp")
	got := v.vaultRelPath(ctx, filepath.Join(ws, "tenants", "shtp", "teams", "u", "f.md"))
	if got != "teams/u/f.md" {
		t.Errorf("non-master relPath = %q, want teams/u/f.md", got)
	}

	// Master tenant: workspace root unchanged → already bare.
	mctx := store.WithTenantID(context.Background(), store.MasterTenantID)
	if got := v.vaultRelPath(mctx, filepath.Join(ws, "agents", "bot", "f.md")); got != "agents/bot/f.md" {
		t.Errorf("master relPath = %q, want agents/bot/f.md", got)
	}

	// Outside the tenant root → empty (skip registration).
	if got := v.vaultRelPath(ctx, "/etc/passwd"); got != "" {
		t.Errorf("outside-root relPath = %q, want empty", got)
	}
}

// vault_read resolves bare AND legacy-prefixed paths to the same in-tenant file,
// and a cross-tenant prefix is bounded to THIS tenant (no escape).
func TestVaultReadResolvePath_StripsLegacyPrefixWithinTenant(t *testing.T) {
	ws := t.TempDir()
	abs := filepath.Join(ws, "tenants", "shtp", "notes", "a.md")
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(abs, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	tool := NewVaultReadTool()
	tool.SetWorkspace(ws)
	ctx := nonMasterTenantCtx("shtp")

	for _, in := range []string{
		"notes/a.md",                   // bare (canonical)
		"tenants/shtp/notes/a.md",      // legacy prefixed
		"tenants/otherslug/notes/a.md", // cross-tenant prefix → stripped, bounded to shtp
	} {
		got, err := tool.resolvePath(ctx, in)
		if err != nil {
			t.Fatalf("resolvePath(%q): %v", in, err)
		}
		if filepath.Base(got) != "a.md" || !strings.Contains(filepath.ToSlash(got), "tenants/shtp/notes/a.md") {
			t.Errorf("resolvePath(%q) = %q, want under tenants/shtp/notes/a.md", in, got)
		}
	}

	if _, err := tool.resolvePath(ctx, "../../../etc/passwd"); err == nil {
		t.Error("traversal escape must be rejected")
	}
}
