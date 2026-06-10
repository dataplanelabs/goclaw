package tools

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStatImagePathWithSiblingFallback_RecoversHyphenUnderscoreTimestamp(t *testing.T) {
	dir := t.TempDir()
	actual := filepath.Join(dir, "poster_20260527-054140_657616.png")
	if err := os.WriteFile(actual, []byte{0x89, 0x50, 0x4e, 0x47}, 0o644); err != nil {
		t.Fatalf("write actual image: %v", err)
	}

	requested := filepath.Join(dir, "poster_20260527_054140_657616.png")
	got, fi, err := statImagePathWithSiblingFallback(requested)
	if err != nil {
		t.Fatalf("stat with fallback: %v", err)
	}
	if got != actual {
		t.Fatalf("resolved path = %q, want %q", got, actual)
	}
	if fi == nil || fi.Size() == 0 {
		t.Fatalf("expected file info for actual image, got %+v", fi)
	}
}

func TestStatImagePathWithSiblingFallback_DoesNotGuessAmbiguousSibling(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{
		"poster_20260527-054140_657616.png",
		"poster-20260527_054140_657616.png",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte{0x89, 0x50}, 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	requested := filepath.Join(dir, "poster_20260527_054140_657616.png")
	if _, _, err := statImagePathWithSiblingFallback(requested); err == nil {
		t.Fatal("expected original stat error when normalized sibling match is ambiguous")
	}
}
