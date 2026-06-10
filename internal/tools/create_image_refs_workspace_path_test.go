package tools

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/nextlevelbuilder/goclaw/internal/providers"
)

func writeWorkspaceImage(t *testing.T, ws, rel string) string {
	t.Helper()
	p := filepath.Join(ws, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(p, []byte{0xff, 0xd8, 0xff, 0xe0}, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	return p
}

// Regression for trace 019e728d: the LLM copies an upload into portraits/ and
// references it by absolute (and relative) path — create_image must resolve it,
// the same way read_image loads any in-workspace image.
func TestAppendWorkspaceImageRefs_ResolvesPortraitsPath(t *testing.T) {
	ws := t.TempDir()
	abs := writeWorkspaceImage(t, ws, "portraits/linhdragon.jpg")
	ctx := WithToolWorkspace(t.Context(), ws)
	tool := &CreateImageTool{}

	for _, id := range []string{"portraits/linhdragon.jpg", abs} {
		refs := tool.appendWorkspaceImageRefs(ctx, nil, []string{id})
		out, missing, _, _ := resolveRefImageIDsDetailed([]string{id}, refs, maxResolvedRefImages)
		if len(out) != 1 || len(missing) != 0 {
			t.Fatalf("id %q: out=%d missing=%v, want 1 resolved", id, len(out), missing)
		}
	}
}

func TestAppendWorkspaceImageRefs_RejectsOutsideAndTraversal(t *testing.T) {
	ws := t.TempDir()
	writeWorkspaceImage(t, ws, ".uploads/real.jpg")
	outside := filepath.Join(t.TempDir(), "secret.jpg")
	if err := os.WriteFile(outside, []byte{0xff, 0xd8}, 0o644); err != nil {
		t.Fatalf("write outside: %v", err)
	}
	ctx := WithToolWorkspace(t.Context(), ws)
	tool := &CreateImageTool{}

	for _, id := range []string{outside, "../escape.jpg", "../../etc/hosts.jpg"} {
		refs := tool.appendWorkspaceImageRefs(ctx, nil, []string{id})
		if len(refs) != 0 {
			t.Errorf("id %q: expected no ref (outside workspace), got %+v", id, refs)
		}
	}
}

func TestAppendWorkspaceImageRefs_RespectsDenyPaths(t *testing.T) {
	ws := t.TempDir()
	writeWorkspaceImage(t, ws, "memory/secret.png") // image ext, but denied location
	ctx := WithToolWorkspace(t.Context(), ws)
	tool := &CreateImageTool{}
	tool.DenyPaths("memory/")

	refs := tool.appendWorkspaceImageRefs(ctx, nil, []string{"memory/secret.png"})
	if len(refs) != 0 {
		t.Errorf("denied path must not resolve, got %+v", refs)
	}
}

func TestAppendWorkspaceImageRefs_SkipsBareBasenameAndNonImage(t *testing.T) {
	ws := t.TempDir()
	writeWorkspaceImage(t, ws, "portraits/x.jpg")
	ctx := WithToolWorkspace(t.Context(), ws)
	tool := &CreateImageTool{}

	// Bare basename (no separator) is left to the in-context lookup, not path-resolved.
	if refs := tool.appendWorkspaceImageRefs(ctx, nil, []string{"x.jpg"}); len(refs) != 0 {
		t.Errorf("bare basename should be skipped, got %+v", refs)
	}
	// Non-image extension never resolves.
	writeWorkspaceImage(t, ws, "portraits/notes.txt")
	if refs := tool.appendWorkspaceImageRefs(ctx, nil, []string{"portraits/notes.txt"}); len(refs) != 0 {
		t.Errorf("non-image ext should be skipped, got %+v", refs)
	}
}

func TestAppendWorkspaceImageRefs_NoWorkspaceNoop(t *testing.T) {
	tool := &CreateImageTool{}
	in := []providers.MediaRef{{ID: "a", Path: "/x/a.jpg", MimeType: "image/jpeg"}}
	got := tool.appendWorkspaceImageRefs(t.Context(), in, []string{"portraits/b.jpg"})
	if len(got) != 1 {
		t.Errorf("no workspace in ctx → must return refs unchanged, got %+v", got)
	}
}
