package tools

import (
	"bytes"
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nextlevelbuilder/goclaw/internal/providers"
)

// TestExecute_WithRefs_ErrorNormalization proves the end-to-end Execute()
// path threads reference_image_ids through ctx lookup → resolveRefImageIDs →
// chain (empty here) → refs-aware error normalization (Phase 05).
//
// Wire-format coverage is in:
//   - create_image_gemini_refs_test.go (callGeminiNativeImageGen direct)
//   - create_image_openrouter_refs_test.go (callImageGenAPI direct)
//   - create_image_minimax_refs_test.go (callMinimaxImageGen direct)
//   - create_image_openai_edit_test.go (callOpenAIImageEdit direct)
//
// The seam those don't cover — args["reference_image_ids"] flowing all the
// way through Execute() — lands here.
func TestExecute_WithRefs_ErrorNormalization(t *testing.T) {
	dir := t.TempDir()
	jpegPath := filepath.Join(dir, "photo.jpg")
	jpegBytes := []byte{0xff, 0xd8, 0xff, 0xe0, 0x00, 0x10, 0x4a, 0x46, 0x49, 0x46}
	if err := os.WriteFile(jpegPath, jpegBytes, 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	// Empty registry → ResolveMediaProviderChain returns nothing → chain fails.
	reg := providers.NewRegistry(nil)
	tool := NewCreateImageTool(reg)

	ctx := WithMediaImageRefs(context.Background(), []providers.MediaRef{
		{ID: "photo-1", MimeType: "image/jpeg", Kind: "image", Path: jpegPath},
	})

	result := tool.Execute(ctx, map[string]any{
		"prompt":              "put me in a tuxedo",
		"reference_image_ids": []any{"photo-1"},
	})

	if result == nil {
		t.Fatal("Execute returned nil")
	}
	if !result.IsError {
		t.Fatalf("expected error result, got success: %s", result.ForLLM)
	}
	// Refs-aware error message — proves Phase 05 normalization fired,
	// which requires refImages to be populated upstream.
	if !strings.Contains(result.ForLLM, "reference images") {
		t.Errorf("error message should mention 'reference images', got: %s", result.ForLLM)
	}
}

// TestExecute_NoRefs_GenericError verifies the non-refs error path is
// preserved (no refs → no refs-specific phrasing).
func TestExecute_NoRefs_GenericError(t *testing.T) {
	reg := providers.NewRegistry(nil)
	tool := NewCreateImageTool(reg)

	result := tool.Execute(context.Background(), map[string]any{
		"prompt": "a sunset",
	})

	if result == nil || !result.IsError {
		t.Fatalf("expected error result, got: %+v", result)
	}
	if strings.Contains(result.ForLLM, "reference images") {
		t.Errorf("non-refs error should not mention 'reference images', got: %s", result.ForLLM)
	}
	if !strings.Contains(result.ForLLM, "image generation failed") {
		t.Errorf("expected generic 'image generation failed', got: %s", result.ForLLM)
	}
}

// TestWarnIfRefsDropped_LogsWhenRefsPresent verifies that providers without
// refs support log a warn (so operators see the gap in production logs).
func TestWarnIfRefsDropped_LogsWhenRefsPresent(t *testing.T) {
	var buf bytes.Buffer
	old := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn})))
	t.Cleanup(func() { slog.SetDefault(old) })

	refs := []providers.ImageContent{{MimeType: "image/jpeg", Data: "AAA="}}
	warnIfRefsDropped(map[string]any{"reference_images": refs}, "dashscope", "test reason")

	log := buf.String()
	if !strings.Contains(log, "reference images dropped") {
		t.Errorf("expected warn log, got: %s", log)
	}
	if !strings.Contains(log, "provider=dashscope") {
		t.Errorf("log should include provider name, got: %s", log)
	}
	if !strings.Contains(log, "count=1") {
		t.Errorf("log should include ref count, got: %s", log)
	}
}

// TestWarnIfRefsDropped_SilentWhenNoRefs verifies no log fires when refs absent.
func TestWarnIfRefsDropped_SilentWhenNoRefs(t *testing.T) {
	var buf bytes.Buffer
	old := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn})))
	t.Cleanup(func() { slog.SetDefault(old) })

	warnIfRefsDropped(map[string]any{}, "dashscope", "test")
	if got := buf.String(); got != "" {
		t.Errorf("expected no log, got: %s", got)
	}
}

// TestExecute_PartialRefs_StillTriggersRefsError verifies that when ≥1 ref
// resolves (even if others fail), the refs-aware error path fires — proving
// the gate is `len(refImages) > 0` after resolveRefImageIDs, not "all IDs
// resolved".
func TestExecute_PartialRefs_StillTriggersRefsError(t *testing.T) {
	dir := t.TempDir()
	jpegPath := filepath.Join(dir, "photo.jpg")
	if err := os.WriteFile(jpegPath, []byte{0xff, 0xd8}, 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	reg := providers.NewRegistry(nil)
	tool := NewCreateImageTool(reg)

	ctx := WithMediaImageRefs(context.Background(), []providers.MediaRef{
		{ID: "real", MimeType: "image/jpeg", Kind: "image", Path: jpegPath},
	})
	result := tool.Execute(ctx, map[string]any{
		"prompt":              "a portrait",
		"reference_image_ids": []any{"missing", "real"},
	})

	if result == nil || !result.IsError {
		t.Fatalf("expected error result, got: %+v", result)
	}
	if !strings.Contains(result.ForLLM, "reference images") {
		t.Errorf("partial resolution (1/2) must still trigger refs-aware error, got: %s", result.ForLLM)
	}
}

// TestExecute_MissingRefID_FallsThroughCleanly verifies that a non-resolvable
// ref ID does NOT crash Execute — resolveRefImageIDs logs and returns zero
// refs; the call proceeds as if refs were absent (refImages slice is nil).
func TestExecute_MissingRefID_FallsThroughCleanly(t *testing.T) {
	reg := providers.NewRegistry(nil)
	tool := NewCreateImageTool(reg)

	// ctx has refs, but the requested ID doesn't match.
	ctx := WithMediaImageRefs(context.Background(), []providers.MediaRef{
		{ID: "other-id", MimeType: "image/jpeg", Kind: "image", Path: "/nonexistent"},
	})
	result := tool.Execute(ctx, map[string]any{
		"prompt":              "a portrait",
		"reference_image_ids": []any{"missing-id"},
	})

	if result == nil || !result.IsError {
		t.Fatalf("expected error result (empty chain), got: %+v", result)
	}
	// Since 0 refs resolved, the refs-specific path should NOT fire.
	if strings.Contains(result.ForLLM, "reference images") {
		t.Errorf("zero-resolved-refs should fall through to generic error, got: %s", result.ForLLM)
	}
}
