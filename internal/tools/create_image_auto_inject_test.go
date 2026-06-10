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

// TestExecute_AutoInjectsCurrentTurnUserImage proves the LLM-forgot-refs path:
// LLM calls create_image WITHOUT reference_image_ids but the user uploaded a
// photo in the current turn. The tool should auto-inject those refs so the
// generated image preserves the user's subject (face/composition).
//
// Verified via:
//   - INFO log "auto-injected user current-turn images as references"
//   - refs-aware error message (Phase 05 normalization fires only when
//     refImages is populated upstream — same gate the explicit-refs test uses).
func TestExecute_AutoInjectsCurrentTurnUserImage(t *testing.T) {
	dir := t.TempDir()
	jpegPath := filepath.Join(dir, "selfie.jpg")
	jpegBytes := []byte{0xff, 0xd8, 0xff, 0xe0, 0x00, 0x10, 0x4a, 0x46, 0x49, 0x46}
	if err := os.WriteFile(jpegPath, jpegBytes, 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	var buf bytes.Buffer
	old := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo})))
	t.Cleanup(func() { slog.SetDefault(old) })

	reg := providers.NewRegistry(nil)
	tool := NewCreateImageTool(reg)

	ctx := WithCurrentTurnUserImageRefs(context.Background(), []providers.MediaRef{
		{ID: "selfie-current", MimeType: "image/jpeg", Kind: "image", Path: jpegPath},
	})

	// LLM omits reference_image_ids entirely — the common glm-5-turbo / weaker-
	// tool-using failure mode that drops the user's subject.
	result := tool.Execute(ctx, map[string]any{
		"prompt": "make her the hero in the center, dramatic spotlight",
	})

	if result == nil || !result.IsError {
		t.Fatalf("expected error result (empty registry), got: %+v", result)
	}
	// Refs-aware error → proves auto-inject populated refImages BEFORE chain.
	if !strings.Contains(result.ForLLM, "reference images") {
		t.Errorf("expected refs-aware error (proves auto-inject fired), got: %s", result.ForLLM)
	}
	// Log signal so operators can confirm in prod.
	log := buf.String()
	if !strings.Contains(log, "auto-injected user current-turn images") {
		t.Errorf("expected auto-inject INFO log, got: %s", log)
	}
}

// TestExecute_NoAutoInject_WhenLLMPassedRefs verifies the auto-inject does NOT
// override an LLM that already supplied reference_image_ids — even with
// current-turn refs in ctx, the LLM's explicit choice wins.
func TestExecute_NoAutoInject_WhenLLMPassedRefs(t *testing.T) {
	dir := t.TempDir()
	jpegPath := filepath.Join(dir, "selfie.jpg")
	jpegBytes := []byte{0xff, 0xd8, 0xff, 0xe0, 0x00, 0x10, 0x4a, 0x46, 0x49, 0x46}
	if err := os.WriteFile(jpegPath, jpegBytes, 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	var buf bytes.Buffer
	old := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo})))
	t.Cleanup(func() { slog.SetDefault(old) })

	reg := providers.NewRegistry(nil)
	tool := NewCreateImageTool(reg)

	ctx := WithMediaImageRefs(context.Background(), []providers.MediaRef{
		{ID: "explicit-ref", MimeType: "image/jpeg", Kind: "image", Path: jpegPath},
	})
	ctx = WithCurrentTurnUserImageRefs(ctx, []providers.MediaRef{
		{ID: "current-turn", MimeType: "image/jpeg", Kind: "image", Path: jpegPath},
	})

	tool.Execute(ctx, map[string]any{
		"prompt":              "scene description",
		"reference_image_ids": []any{"explicit-ref"},
	})

	log := buf.String()
	if strings.Contains(log, "auto-injected user current-turn images") {
		t.Errorf("auto-inject should NOT fire when LLM passed explicit refs, got log: %s", log)
	}
}

// TestExecute_NoAutoInject_WhenNoCurrentTurnRefs verifies the auto-inject is
// skipped when the user did not upload an image this turn (no false positives
// for text-only generation requests).
func TestExecute_NoAutoInject_WhenNoCurrentTurnRefs(t *testing.T) {
	var buf bytes.Buffer
	old := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo})))
	t.Cleanup(func() { slog.SetDefault(old) })

	reg := providers.NewRegistry(nil)
	tool := NewCreateImageTool(reg)

	// No CurrentTurnUserImageRefs in ctx — text-only generation.
	result := tool.Execute(context.Background(), map[string]any{
		"prompt": "a sunset",
	})

	if result == nil || !result.IsError {
		t.Fatalf("expected error from empty registry, got: %+v", result)
	}
	// Should be the GENERIC error, not the refs-aware one.
	if strings.Contains(result.ForLLM, "reference images") {
		t.Errorf("text-only request should NOT trigger refs-aware error, got: %s", result.ForLLM)
	}
	log := buf.String()
	if strings.Contains(log, "auto-injected user current-turn images") {
		t.Errorf("auto-inject should NOT fire without current-turn refs, got log: %s", log)
	}
}
