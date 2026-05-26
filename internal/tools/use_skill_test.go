package tools

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/skills"
	"github.com/nextlevelbuilder/goclaw/internal/store"
)

type fakeSkillAccess struct {
	skills []store.SkillInfo
	err    error
}

func (f *fakeSkillAccess) ListAccessible(_ context.Context, _ uuid.UUID, _ string) ([]store.SkillInfo, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.skills, nil
}

func TestMain(m *testing.M) {
	os.Setenv("GOCLAW_DISABLE_PERSONAL_SKILLS", "1")
	os.Exit(m.Run())
}

func newLoaderWithSkill(t *testing.T, slug, body string, withAssets bool) *skills.Loader {
	t.Helper()
	ws := t.TempDir()
	skillDir := filepath.Join(ws, "skills", slug)
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatalf("mkdir skill: %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(body), 0644); err != nil {
		t.Fatalf("write SKILL.md: %v", err)
	}
	if withAssets {
		assetsDir := filepath.Join(skillDir, "assets")
		if err := os.MkdirAll(assetsDir, 0755); err != nil {
			t.Fatalf("mkdir assets: %v", err)
		}
		if err := os.WriteFile(filepath.Join(assetsDir, "logo.jpg"), []byte("fake-jpg"), 0644); err != nil {
			t.Fatalf("write logo: %v", err)
		}
	}
	return skills.NewLoader(ws, "", "")
}

const skillBody = "---\nname: Test Skill\nslug: test-skill\ndescription: Test\n---\n\n# Test\nBody."

func TestUseSkill_ReturnsStructuredPayload(t *testing.T) {
	loader := newLoaderWithSkill(t, "test-skill", skillBody, true)
	tool := NewUseSkillTool(loader)

	result := tool.Execute(context.Background(), map[string]any{"name": "test-skill"})
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.ForLLM)
	}

	var payload skills.ActivationPayload
	if err := json.Unmarshal([]byte(result.ForLLM), &payload); err != nil {
		t.Fatalf("unmarshal payload: %v\nraw: %s", err, result.ForLLM)
	}
	if payload.Slug != "test-skill" {
		t.Errorf("slug: got %q, want %q", payload.Slug, "test-skill")
	}
	if !strings.HasSuffix(payload.SkillMDPath, "SKILL.md") {
		t.Errorf("skill_md_path: got %q, want suffix SKILL.md", payload.SkillMDPath)
	}
	if !strings.Contains(payload.SkillMDContent, "# Test") {
		t.Errorf("skill_md_content missing body; got %q", payload.SkillMDContent)
	}
	if payload.SkillMDTruncatedReason != "" {
		t.Errorf("unexpected truncation: %q", payload.SkillMDTruncatedReason)
	}
	if len(payload.AssetPaths) != 1 || !strings.HasSuffix(payload.AssetPaths[0], "logo.jpg") {
		t.Errorf("asset_paths: got %v, want [.../logo.jpg]", payload.AssetPaths)
	}
	if payload.ActivatedAt.IsZero() {
		t.Errorf("activated_at not set")
	}
}

func TestUseSkill_MissingName(t *testing.T) {
	loader := newLoaderWithSkill(t, "test-skill", skillBody, false)
	tool := NewUseSkillTool(loader)
	result := tool.Execute(context.Background(), map[string]any{})
	if !result.IsError || !strings.Contains(result.ForLLM, "name parameter is required") {
		t.Errorf("expected name-required error, got: %+v", result)
	}
}

func TestUseSkill_SkillNotFound(t *testing.T) {
	loader := newLoaderWithSkill(t, "test-skill", skillBody, false)
	tool := NewUseSkillTool(loader)
	result := tool.Execute(context.Background(), map[string]any{"name": "nonexistent"})
	if !result.IsError || !strings.Contains(result.ForLLM, "skill_not_found") {
		t.Errorf("expected skill_not_found, got: %+v", result)
	}
}

func TestUseSkill_OversizeContent_PathsOnly(t *testing.T) {
	body := "---\nname: Big Skill\nslug: big-skill\ndescription: too big\n---\n\n" +
		strings.Repeat("x", skills.SkillMDByteBudget+10)
	loader := newLoaderWithSkill(t, "big-skill", body, false)
	tool := NewUseSkillTool(loader)

	result := tool.Execute(context.Background(), map[string]any{"name": "big-skill"})
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.ForLLM)
	}
	var payload skills.ActivationPayload
	if err := json.Unmarshal([]byte(result.ForLLM), &payload); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if payload.SkillMDContent != "" {
		t.Errorf("oversize SKILL.md should not be inlined")
	}
	if payload.SkillMDTruncatedReason != "exceeds_200kb_budget" {
		t.Errorf("truncated_reason: got %q, want exceeds_200kb_budget", payload.SkillMDTruncatedReason)
	}
	if payload.SkillMDPath == "" {
		t.Errorf("skill_md_path should still be set when oversize")
	}
}

func TestUseSkill_NoAssetsDir_EmptyList(t *testing.T) {
	loader := newLoaderWithSkill(t, "test-skill", skillBody, false)
	tool := NewUseSkillTool(loader)
	result := tool.Execute(context.Background(), map[string]any{"name": "test-skill"})
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.ForLLM)
	}
	var payload skills.ActivationPayload
	if err := json.Unmarshal([]byte(result.ForLLM), &payload); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(payload.AssetPaths) != 0 {
		t.Errorf("expected no asset_paths, got %v", payload.AssetPaths)
	}
}

func TestUseSkill_NameTrimAndTraversal(t *testing.T) {
	loader := newLoaderWithSkill(t, "test-skill", skillBody, false)
	tool := NewUseSkillTool(loader)
	cases := []struct {
		name   string
		input  string
		errSub string
	}{
		{"whitespace_only", "   ", "name parameter is required"},
		{"slash_traversal", "../etc/passwd", "invalid skill name"},
		{"backslash", "skills\\foo", "invalid skill name"},
		{"dot_prefix", ".hidden", "invalid skill name"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result := tool.Execute(context.Background(), map[string]any{"name": tc.input})
			if !result.IsError || !strings.Contains(result.ForLLM, tc.errSub) {
				t.Errorf("input %q: expected error containing %q, got: %+v", tc.input, tc.errSub, result)
			}
		})
	}
}

// makeManagedSkill creates a skill in a managed-dir so Loader tags it Source="managed".
// Layout: <root>/<slug>/<version>/SKILL.md — matches gcplane upload + tenant skills-store.
func makeManagedSkill(t *testing.T, slug, body string) *skills.Loader {
	t.Helper()
	ws := t.TempDir()
	managedRoot := filepath.Join(ws, "managed")
	versionDir := filepath.Join(managedRoot, slug, "1")
	if err := os.MkdirAll(versionDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(versionDir, "SKILL.md"), []byte(body), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}
	l := skills.NewLoader(ws, "", "")
	l.SetManagedDir(managedRoot)
	return l
}

func TestUseSkill_ManagedSkill_Granted(t *testing.T) {
	loader := makeManagedSkill(t, "design-annhien", skillBody)
	tool := NewUseSkillTool(loader)
	tool.SetSkillAccessStore(&fakeSkillAccess{
		skills: []store.SkillInfo{{Slug: "design-annhien"}},
	})
	// Inject a non-nil agentID so the grant code path runs.
	ctx := store.WithAgentID(context.Background(), uuid.New())
	result := tool.Execute(ctx, map[string]any{"name": "design-annhien"})
	if result.IsError {
		t.Fatalf("granted skill should activate: %s", result.ForLLM)
	}
}

func TestUseSkill_ManagedSkill_NotGranted(t *testing.T) {
	loader := makeManagedSkill(t, "design-annhien", skillBody)
	tool := NewUseSkillTool(loader)
	tool.SetSkillAccessStore(&fakeSkillAccess{skills: nil})
	ctx := store.WithAgentID(context.Background(), uuid.New())
	result := tool.Execute(ctx, map[string]any{"name": "design-annhien"})
	if !result.IsError || !strings.Contains(result.ForLLM, "skill_not_granted") {
		t.Errorf("expected skill_not_granted, got: %+v", result)
	}
}

func TestUseSkill_ManagedSkill_NoAccessStore_Allowed(t *testing.T) {
	loader := makeManagedSkill(t, "design-annhien", skillBody)
	tool := NewUseSkillTool(loader)
	// No SetSkillAccessStore — grant check skipped.
	ctx := store.WithAgentID(context.Background(), uuid.New())
	result := tool.Execute(ctx, map[string]any{"name": "design-annhien"})
	if result.IsError {
		t.Fatalf("without skillAccess wired, activation should pass: %s", result.ForLLM)
	}
}

func TestUseSkill_ManagedSkill_GrantCheckDBError_FailsClosed(t *testing.T) {
	loader := makeManagedSkill(t, "design-annhien", skillBody)
	tool := NewUseSkillTool(loader)
	tool.SetSkillAccessStore(&fakeSkillAccess{err: errors.New("pg conn refused")})
	ctx := store.WithAgentID(context.Background(), uuid.New())
	result := tool.Execute(ctx, map[string]any{"name": "design-annhien"})
	if !result.IsError || !strings.Contains(result.ForLLM, "skill_grant_check_failed") {
		t.Errorf("DB error should fail closed with skill_grant_check_failed, got: %+v", result)
	}
}
