package tools

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/nextlevelbuilder/goclaw/internal/skills"
)

// TestFilesystemTools_RespectSkillContext is the consistency guard that proves
// every filesystem tool routing through allowed*WithTeamWorkspace inherits the
// per-session skill allow-list. If a new filesystem tool is added without
// going through these helpers, this test (or its callers) should catch it.
//
// Layout:
//   workspace/      ← agent workspace (where standard reads are allowed)
//   skill-dir/      ← outside workspace; activated via SkillContext
//     SKILL.md
//     assets/logo.jpg
//
// All filesystem tools should accept a path under skill-dir/ once the
// SkillContext has it activated, and deny the same path when activation
// is missing.
func TestFilesystemTools_RespectSkillContext(t *testing.T) {
	root := t.TempDir()
	workspace := filepath.Join(root, "workspace")
	skillDir := filepath.Join(root, "skill")
	assetsDir := filepath.Join(skillDir, "assets")
	logoPath := filepath.Join(assetsDir, "logo.jpg")

	for _, d := range []string{workspace, assetsDir} {
		if err := os.MkdirAll(d, 0755); err != nil {
			t.Fatalf("mkdir %s: %v", d, err)
		}
	}
	if err := os.WriteFile(logoPath, []byte("fake-jpg"), 0644); err != nil {
		t.Fatalf("write logo: %v", err)
	}

	bareCtx := WithToolWorkspace(context.Background(), workspace)

	sc := skills.NewSkillContext()
	sc.Activate(&skills.ActivatedSkill{Slug: "design-test", BaseDir: skillDir})
	activatedCtx := skills.WithSkillContext(bareCtx, sc)

	// Without activation: resolvePath against the skill path should be denied.
	if _, err := resolvePathWithAllowed(logoPath, workspace, true, allowedWithTeamWorkspace(bareCtx, nil)); err == nil {
		t.Errorf("bare ctx should reject path outside workspace (got allowed): %s", logoPath)
	}

	// With activation: resolvePath against the skill path should succeed.
	resolved, err := resolvePathWithAllowed(logoPath, workspace, true, allowedWithTeamWorkspace(activatedCtx, nil))
	if err != nil {
		t.Fatalf("activated ctx should accept skill path: %v", err)
	}
	if !strings.Contains(resolved, "logo.jpg") {
		t.Errorf("resolved path: got %q, want suffix logo.jpg", resolved)
	}

	// Same test against the WRITE helper (edit, write_file, shell cwd).
	if _, err := resolvePathWithAllowed(logoPath, workspace, true, allowedWriteWithTeamWorkspace(bareCtx, nil)); err == nil {
		t.Errorf("write bare ctx should reject skill path")
	}
	if _, err := resolvePathWithAllowed(logoPath, workspace, true, allowedWriteWithTeamWorkspace(activatedCtx, nil)); err != nil {
		t.Errorf("write activated ctx should accept skill path: %v", err)
	}

	// A path outside both workspace AND any activated skill stays denied.
	outsidePath := filepath.Join(root, "other", "secret.txt")
	if _, err := resolvePathWithAllowed(outsidePath, workspace, true, allowedWithTeamWorkspace(activatedCtx, nil)); err == nil {
		t.Errorf("activated ctx should still deny non-skill paths; got allowed: %s", outsidePath)
	}
}

// TestFilesystemTools_SkillPathTraversalDenied confirms that path-traversal
// arguments under an activated skill don't escape the skill dir.
func TestFilesystemTools_SkillPathTraversalDenied(t *testing.T) {
	root := t.TempDir()
	workspace := filepath.Join(root, "workspace")
	skillDir := filepath.Join(root, "skill")
	secretDir := filepath.Join(root, "secret")
	for _, d := range []string{workspace, skillDir, secretDir} {
		if err := os.MkdirAll(d, 0755); err != nil {
			t.Fatalf("mkdir %s: %v", d, err)
		}
	}
	if err := os.WriteFile(filepath.Join(secretDir, "leak.txt"), []byte("x"), 0644); err != nil {
		t.Fatalf("write secret: %v", err)
	}

	sc := skills.NewSkillContext()
	sc.Activate(&skills.ActivatedSkill{Slug: "design-test", BaseDir: skillDir})
	ctx := skills.WithSkillContext(WithToolWorkspace(context.Background(), workspace), sc)

	traversal := filepath.Join(skillDir, "..", "secret", "leak.txt")
	if _, err := resolvePathWithAllowed(traversal, workspace, true, allowedWithTeamWorkspace(ctx, nil)); err == nil {
		t.Errorf("traversal out of skill dir must be denied; got allowed for %q", traversal)
	}
}

// TestFilesystemTools_SkillSymlinkEscapeDenied asserts that a symlink inside a
// skill dir pointing outside of it is denied by the EvalSymlinks resolver.
func TestFilesystemTools_SkillSymlinkEscapeDenied(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink semantics differ on Windows")
	}
	root := t.TempDir()
	workspace := filepath.Join(root, "workspace")
	skillDir := filepath.Join(root, "skill")
	secretDir := filepath.Join(root, "secret")
	for _, d := range []string{workspace, skillDir, secretDir} {
		if err := os.MkdirAll(d, 0755); err != nil {
			t.Fatalf("mkdir %s: %v", d, err)
		}
	}
	if err := os.WriteFile(filepath.Join(secretDir, "leak.txt"), []byte("x"), 0644); err != nil {
		t.Fatalf("write secret: %v", err)
	}
	link := filepath.Join(skillDir, "escape")
	if err := os.Symlink(secretDir, link); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	sc := skills.NewSkillContext()
	sc.Activate(&skills.ActivatedSkill{Slug: "design-test", BaseDir: skillDir})
	ctx := skills.WithSkillContext(WithToolWorkspace(context.Background(), workspace), sc)

	target := filepath.Join(link, "leak.txt")
	if _, err := resolvePathWithAllowed(target, workspace, true, allowedWithTeamWorkspace(ctx, nil)); err == nil {
		t.Errorf("symlink escape from skill dir must be denied; got allowed for %q", target)
	}
}

// TestSkillContext_NotActivatedSkillDenied locks in the security property: if
// the agent has multiple skills granted but only activated one, the others'
// asset paths remain denied. (Activation, not grant, opens the allow-list.)
func TestSkillContext_NotActivatedSkillDenied(t *testing.T) {
	root := t.TempDir()
	workspace := filepath.Join(root, "workspace")
	if err := os.MkdirAll(workspace, 0755); err != nil {
		t.Fatalf("mkdir ws: %v", err)
	}
	skillA := filepath.Join(root, "skill-a")
	skillB := filepath.Join(root, "skill-b")
	for _, d := range []string{skillA, skillB} {
		if err := os.MkdirAll(d, 0755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(filepath.Join(d, "marker.txt"), []byte("x"), 0644); err != nil {
			t.Fatalf("write: %v", err)
		}
	}

	sc := skills.NewSkillContext()
	sc.Activate(&skills.ActivatedSkill{Slug: "a", BaseDir: skillA})
	// b granted but NOT activated.
	ctx := skills.WithSkillContext(WithToolWorkspace(context.Background(), workspace), sc)

	if _, err := resolvePathWithAllowed(filepath.Join(skillA, "marker.txt"), workspace, true, allowedWithTeamWorkspace(ctx, nil)); err != nil {
		t.Errorf("activated skill A should allow read: %v", err)
	}
	if _, err := resolvePathWithAllowed(filepath.Join(skillB, "marker.txt"), workspace, true, allowedWithTeamWorkspace(ctx, nil)); err == nil {
		t.Errorf("non-activated skill B must remain denied")
	}
}
