package tools

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nextlevelbuilder/goclaw/internal/providers"
	"github.com/nextlevelbuilder/goclaw/internal/skills"
)

func contextWithActivatedSkillAsset(t *testing.T) (context.Context, string) {
	t.Helper()
	dir := t.TempDir()
	assetDir := filepath.Join(dir, "assets")
	if err := os.MkdirAll(assetDir, 0o755); err != nil {
		t.Fatalf("mkdir assets: %v", err)
	}
	logoPath := filepath.Join(assetDir, "logo.jpg")
	if err := os.WriteFile(logoPath, []byte{0xff, 0xd8, 0xff, 0xe0}, 0o644); err != nil {
		t.Fatalf("write logo: %v", err)
	}

	sc := skills.NewSkillContext()
	sc.Activate(&skills.ActivatedSkill{
		Slug:       "design-annhien",
		BaseDir:    dir,
		AssetPaths: []string{logoPath},
	})
	return skills.WithSkillContext(context.Background(), sc), logoPath
}

func TestExecute_SkillAssetPathResolvesAsReferenceImage(t *testing.T) {
	ctx, logoPath := contextWithActivatedSkillAsset(t)
	tool := NewCreateImageTool(providers.NewRegistry(nil))

	result := tool.Execute(ctx, map[string]any{
		"prompt":              "Create a poster with the exact logo from the reference image.",
		"reference_image_ids": []any{logoPath},
	})

	if result == nil || !result.IsError {
		t.Fatalf("expected empty-registry error result, got: %+v", result)
	}
	if !strings.Contains(result.ForLLM, "reference images") {
		t.Fatalf("expected refs-aware error proving skill asset resolved, got: %s", result.ForLLM)
	}
	if strings.Contains(result.ForLLM, "could not be resolved") {
		t.Fatalf("skill asset path should resolve, got: %s", result.ForLLM)
	}
}

func TestExecute_LogoPromptWithoutRefsRequiresSkillAssetRef(t *testing.T) {
	ctx, logoPath := contextWithActivatedSkillAsset(t)
	tool := NewCreateImageTool(providers.NewRegistry(nil))

	result := tool.Execute(ctx, map[string]any{
		"prompt": "LOGO — reproduce the An Nhiên Safety logo exactly at the top of the poster.",
	})

	if result == nil || !result.IsError {
		t.Fatalf("expected missing-ref error result, got: %+v", result)
	}
	if !strings.Contains(result.ForLLM, "reference_image_ids is required") {
		t.Fatalf("expected explicit reference_image_ids guidance, got: %s", result.ForLLM)
	}
	if !strings.Contains(result.ForLLM, logoPath) {
		t.Fatalf("expected result to list skill asset path %q, got: %s", logoPath, result.ForLLM)
	}
}

func TestExecute_ExplicitNoLogoDoesNotRequireSkillAssetRef(t *testing.T) {
	ctx, _ := contextWithActivatedSkillAsset(t)
	tool := NewCreateImageTool(providers.NewRegistry(nil))

	result := tool.Execute(ctx, map[string]any{
		"prompt":              "Create an abstract background without logo.",
		"reference_image_ids": []any{},
	})

	if result == nil || !result.IsError {
		t.Fatalf("expected empty-registry error result, got: %+v", result)
	}
	if strings.Contains(result.ForLLM, "reference_image_ids is required") {
		t.Fatalf("explicit no-logo flow should not require skill refs, got: %s", result.ForLLM)
	}
}
