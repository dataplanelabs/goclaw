package tools

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// A document that aged out of the conversation refs is still resolvable by media_id from
// the session .uploads/ folder, with the MIME sniffed from content (.bin has no extension).
// Regression for trace 019e79ef (read_document "not found in conversation" on an on-disk PDF).
func TestResolveUploadedDoc_FromDisk(t *testing.T) {
	ws := t.TempDir()
	uploads := filepath.Join(ws, ".uploads")
	if err := os.MkdirAll(uploads, 0o755); err != nil {
		t.Fatal(err)
	}
	const mediaID = "20260530-172931_van-duc_document_380bb86d"
	if err := os.WriteFile(filepath.Join(uploads, mediaID+".bin"), []byte("%PDF-1.3\n%abc"), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx := WithToolWorkspace(context.Background(), ws)

	path, mime, ok := resolveUploadedDoc(ctx, mediaID)
	if !ok {
		t.Fatal("expected to resolve the uploaded .bin by media_id stem")
	}
	if filepath.Base(path) != mediaID+".bin" {
		t.Errorf("path = %q, want the .uploads/%s.bin file", path, mediaID)
	}
	if mime != "application/pdf" {
		t.Errorf("mime = %q, want application/pdf (sniffed from %%PDF magic)", mime)
	}
}

// media_id must not escape the .uploads/ folder.
func TestResolveUploadedDoc_RejectsTraversal(t *testing.T) {
	ws := t.TempDir()
	_ = os.MkdirAll(filepath.Join(ws, ".uploads"), 0o755)
	ctx := WithToolWorkspace(context.Background(), ws)
	for _, bad := range []string{"../secret", "a/b", `..\win`, ".."} {
		if _, _, ok := resolveUploadedDoc(ctx, bad); ok {
			t.Errorf("media_id %q must be rejected", bad)
		}
	}
}

// A real extension on the upload is honored without sniffing.
func TestResolveUploadedDoc_KeepsExtMIME(t *testing.T) {
	ws := t.TempDir()
	_ = os.MkdirAll(filepath.Join(ws, ".uploads"), 0o755)
	if err := os.WriteFile(filepath.Join(ws, ".uploads", "report.csv"), []byte("a,b\n1,2"), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx := WithToolWorkspace(context.Background(), ws)
	if _, mime, ok := resolveUploadedDoc(ctx, "report"); !ok || mime != "text/csv" {
		t.Errorf("ok=%v mime=%q, want true text/csv", ok, mime)
	}
}
