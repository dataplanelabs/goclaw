package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nextlevelbuilder/goclaw/internal/skills"
)

// TestE2E_UseSkillActivationToFilesystemRead proves the full Phase 1→2→3
// plumbing works in one shot: activate a skill, then read its bundled asset
// through the filesystem path resolver. No LLM, no provider — just the
// data flow that traces 019e62e5 / 019e62f3 / 019e62ff exercised.
//
// Before this PR the same flow needed `use_skill` → `skill_search` (often
// empty) → guess path → `read_file` (wrong path). After: `use_skill` returns
// SKILL.md inline + asset paths, AND filesystem reads under those paths just
// work because SkillContext flows through ctx.
func TestE2E_UseSkillActivationToFilesystemRead(t *testing.T) {
	root := t.TempDir()
	workspace := filepath.Join(root, "workspace")
	managedRoot := filepath.Join(root, "skills-store")
	skillVer := filepath.Join(managedRoot, "design-annhien", "1")
	assetsDir := filepath.Join(skillVer, "assets")
	logoPath := filepath.Join(assetsDir, "logo.jpg")

	skillBody := "---\nname: Thiết kế An Nhiên\nslug: design-annhien\ndescription: Tạo poster cho An Nhiên Safety\n---\n\n# Body"

	for _, d := range []string{workspace, assetsDir} {
		if err := os.MkdirAll(d, 0755); err != nil {
			t.Fatalf("mkdir %s: %v", d, err)
		}
	}
	if err := os.WriteFile(filepath.Join(skillVer, "SKILL.md"), []byte(skillBody), 0644); err != nil {
		t.Fatalf("write SKILL.md: %v", err)
	}
	if err := os.WriteFile(logoPath, []byte("fake-jpeg-bytes"), 0644); err != nil {
		t.Fatalf("write logo: %v", err)
	}

	loader := skills.NewLoader(workspace, "", "")
	loader.SetManagedDir(managedRoot)

	useSkill := NewUseSkillTool(loader)
	sc := skills.NewSkillContext()
	ctx := skills.WithSkillContext(WithToolWorkspace(context.Background(), workspace), sc)

	// Step 1 — agent activates the skill (no skill_search detour needed).
	result := useSkill.Execute(ctx, map[string]any{"name": "design-annhien"})
	if result.IsError {
		t.Fatalf("use_skill failed: %s", result.ForLLM)
	}

	// Step 2 — Phase 1 contract: response includes SKILL.md content + asset
	// paths. The LLM doesn't need to chain a read_file on SKILL.md anymore.
	var payload skills.ActivationPayload
	if err := json.Unmarshal([]byte(result.ForLLM), &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if !strings.Contains(payload.SkillMDContent, "# Body") {
		t.Errorf("payload should inline SKILL.md content; got %q", payload.SkillMDContent)
	}
	if len(payload.AssetPaths) != 1 || payload.AssetPaths[0] != logoPath {
		t.Errorf("asset_paths: got %v, want [%s]", payload.AssetPaths, logoPath)
	}

	// Step 3 — Phase 2 contract: SkillContext now has the activation.
	if !sc.IsActivated("design-annhien") {
		t.Errorf("SkillContext should record activation")
	}
	prefixes := sc.AllowedPrefixes()
	if len(prefixes) != 1 || prefixes[0] != skillVer {
		t.Errorf("AllowedPrefixes: got %v, want [%s]", prefixes, skillVer)
	}

	// Step 4 — Phase 3 contract: filesystem resolver accepts the bundled
	// asset path *because* the session ctx merges the skill's BaseDir into
	// the allow-list. This is the path that produced "access denied: path
	// outside workspace" in trace 019e62f3.
	resolved, err := resolvePathWithAllowed(logoPath, workspace, true, allowedWithTeamWorkspace(ctx, nil))
	if err != nil {
		t.Fatalf("filesystem resolver should accept activated-skill asset path; got: %v", err)
	}
	if resolved == "" {
		t.Errorf("resolver returned empty path")
	}

	// Step 5 — Negative: a bare ctx (no activation) still denies the asset.
	bareCtx := WithToolWorkspace(context.Background(), workspace)
	if _, err := resolvePathWithAllowed(logoPath, workspace, true, allowedWithTeamWorkspace(bareCtx, nil)); err == nil {
		t.Errorf("bare ctx must deny asset path; got allowed")
	}
}

// TestE2E_RepeatActivation_IdempotentCached proves Phase 1's idempotent
// activation works — re-calling use_skill in the same session is cheap and
// reports cached=true via the SkillContext.
func TestE2E_RepeatActivation_IdempotentCached(t *testing.T) {
	root := t.TempDir()
	workspace := filepath.Join(root, "workspace")
	if err := os.MkdirAll(workspace, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	skillsDir := filepath.Join(workspace, "skills", "test-skill")
	if err := os.MkdirAll(skillsDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillsDir, "SKILL.md"),
		[]byte("---\nname: Test\nslug: test-skill\ndescription: t\n---\n\n# x"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	loader := skills.NewLoader(workspace, "", "")
	useSkill := NewUseSkillTool(loader)
	sc := skills.NewSkillContext()
	ctx := skills.WithSkillContext(WithToolWorkspace(context.Background(), workspace), sc)

	first := useSkill.Execute(ctx, map[string]any{"name": "test-skill"})
	if first.IsError {
		t.Fatalf("first activation failed: %s", first.ForLLM)
	}
	second := useSkill.Execute(ctx, map[string]any{"name": "test-skill"})
	if second.IsError {
		t.Fatalf("second activation failed: %s", second.ForLLM)
	}

	// Idempotency is verified at the SkillContext level — re-activating
	// the same slug doesn't double the prefix list.
	if got := len(sc.AllowedPrefixes()); got != 1 {
		t.Errorf("re-activation should not duplicate prefix; got %d", got)
	}
}
