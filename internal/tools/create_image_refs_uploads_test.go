package tools

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/nextlevelbuilder/goclaw/internal/providers"
)

// mkUpload writes an image into <workspace>/.uploads/ and returns the path.
func mkUpload(t *testing.T, workspace, name string) string {
	t.Helper()
	dir := filepath.Join(workspace, ".uploads")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir .uploads: %v", err)
	}
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte{0xff, 0xd8, 0xff, 0xe0}, 0o644); err != nil {
		t.Fatalf("write upload: %v", err)
	}
	return p
}

func TestUploadsImageRefs_ListsImagesOnly(t *testing.T) {
	ws := t.TempDir()
	mkUpload(t, ws, "goclaw-zca-1532281175-c401fb7f.jpg")
	mkUpload(t, ws, "logo.png")
	mkUpload(t, ws, "notes.txt") // non-image dropped

	refs := uploadsImageRefs(ws)
	if len(refs) != 2 {
		t.Fatalf("len(refs) = %d, want 2 (two images, txt dropped): %+v", len(refs), refs)
	}
	for _, r := range refs {
		if r.Kind != "image" || r.Path == "" {
			t.Errorf("bad ref: %+v", r)
		}
	}
}

func TestUploadsImageRefs_RejectsSymlinkEscape(t *testing.T) {
	ws := t.TempDir()
	mkUpload(t, ws, "real.jpg")
	secret := filepath.Join(t.TempDir(), "secret.jpg")
	if err := os.WriteFile(secret, []byte{0xff, 0xd8}, 0o644); err != nil {
		t.Fatalf("write secret: %v", err)
	}
	// A symlink in .uploads/ pointing outside must not be listed.
	if err := os.Symlink(secret, filepath.Join(ws, ".uploads", "evil.jpg")); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}
	for _, r := range uploadsImageRefs(ws) {
		if filepath.Base(r.Path) == "secret.jpg" || r.ID == "evil.jpg" {
			t.Fatalf("symlink escape leaked: %+v", r)
		}
	}
}

func TestUploadsImageRefs_RejectsHardlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("hardlink nlink check is unix-only")
	}
	ws := t.TempDir()
	outside := filepath.Join(ws, "outside.jpg") // same fs, outside .uploads
	if err := os.WriteFile(outside, []byte{0xff, 0xd8}, 0o644); err != nil {
		t.Fatalf("write outside: %v", err)
	}
	_ = os.MkdirAll(filepath.Join(ws, ".uploads"), 0o755)
	if err := os.Link(outside, filepath.Join(ws, ".uploads", "hard.jpg")); err != nil {
		t.Skipf("hardlink unsupported: %v", err)
	}
	for _, r := range uploadsImageRefs(ws) {
		if r.ID == "hard.jpg" {
			t.Fatalf("hardlinked file must be rejected: %+v", r)
		}
	}
}

func TestUploadsImageRefs_EmptyOrMissing(t *testing.T) {
	if refs := uploadsImageRefs(""); refs != nil {
		t.Errorf("empty workspace should return nil, got %+v", refs)
	}
	if refs := uploadsImageRefs(t.TempDir()); refs != nil {
		t.Errorf("workspace without .uploads should return nil, got %+v", refs)
	}
}

// Regression for trace 019e7256 / 019e728d: an upload that aged out of the
// conversation window (not in the in-context ref set) must still be reachable
// because availableImageRefs enumerates the .uploads/ folder.
func TestAvailableImageRefs_IncludesAgedOutUploads(t *testing.T) {
	ws := t.TempDir()
	face := mkUpload(t, ws, "goclaw-zca-3641626789-33f6006b.jpg")
	ctx := WithToolWorkspace(WithMediaImageRefs(t.Context(), nil), ws)

	refs := availableImageRefs(ctx)
	found := false
	for _, r := range refs {
		if r.Path == face || filepath.Base(r.Path) == filepath.Base(face) {
			found = true
		}
	}
	if !found {
		t.Fatalf("availableImageRefs must include the aged-out .uploads file; got %+v", refs)
	}

	// And it resolves by the relative path / basename the LLM would pass.
	for _, id := range []string{".uploads/" + filepath.Base(face), filepath.Base(face)} {
		out, missing, _, _ := resolveRefImageIDsDetailed([]string{id}, refs, maxResolvedRefImages)
		if len(out) != 1 || len(missing) != 0 {
			t.Fatalf("id %q: out=%d missing=%v, want 1 resolved", id, len(out), missing)
		}
	}
}

// sizedRef creates a sparse fixture of the given on-disk size (Stat-reported)
// without allocating/writing the full bytes — for exercising byte caps cheaply.
func sizedRef(t *testing.T, dir, name string, size int64) providers.MediaRef {
	t.Helper()
	p := filepath.Join(dir, name)
	f, err := os.Create(p)
	if err != nil {
		t.Fatalf("create %s: %v", name, err)
	}
	_, _ = f.Write([]byte{0xff, 0xd8, 0xff, 0xe0})
	if err := f.Truncate(size); err != nil {
		t.Fatalf("truncate %s: %v", name, err)
	}
	_ = f.Close()
	return providers.MediaRef{ID: name, MimeType: "image/jpeg", Kind: "image", Path: p}
}

// Loop invariant: every distinct id lands in exactly one bucket, including the
// aggregate-byte path (regression: a `break` used to silently drop ids after the
// cap-tripping one, defeating the caller's fail-fast).
func TestResolveRefImageIDsDetailed_AggregateCapCategorizesAllIDs(t *testing.T) {
	dir := t.TempDir()
	var refs []providers.MediaRef
	var ids []string
	for _, n := range []string{"a", "b", "c", "d"} { // 10MB each → 2 fill the 20MB aggregate
		refs = append(refs, sizedRef(t, dir, n+".jpg", maxRefImageBytes))
		ids = append(ids, n+".jpg")
	}

	out, missing, unusable, trimmed := resolveRefImageIDsDetailed(ids, refs, maxResolvedRefImages)
	if got := len(out) + len(missing) + len(unusable) + len(trimmed); got != len(ids) {
		t.Fatalf("categorized %d ids (out=%d missing=%d unusable=%d trimmed=%d), want %d — ids leaked",
			got, len(out), len(missing), len(unusable), len(trimmed), len(ids))
	}
	if len(missing) != 0 || len(unusable) != 0 {
		t.Fatalf("at-limit refs over the aggregate budget must be trimmed: missing=%v unusable=%v", missing, unusable)
	}
}

// An over-the-per-image-cap ref is unusable (present, recompress) — never missing
// (the contradictory "resend" guidance the round-2 review flagged).
func TestResolveRefImageIDsDetailed_OversizedIsUnusableNotMissing(t *testing.T) {
	dir := t.TempDir()
	huge := sizedRef(t, dir, "huge.jpg", maxRefImageBytes+1)
	ok := writeRef(t, dir, "ok.jpg", "image/jpeg", []byte{0xff, 0xd8})

	out, missing, unusable, _ := resolveRefImageIDsDetailed(
		[]string{"huge.jpg", "ok.jpg"}, []providers.MediaRef{huge, ok}, maxResolvedRefImages)
	if len(out) != 1 {
		t.Fatalf("out = %d, want 1 (ok.jpg)", len(out))
	}
	if len(missing) != 0 {
		t.Fatalf("oversized must NOT be missing: %v", missing)
	}
	if len(unusable) != 1 || unusable[0] != "huge.jpg" {
		t.Fatalf("unusable = %v, want [huge.jpg]", unusable)
	}
	if msg := formatRefUnusableError(unusable); !strings.Contains(msg, "do NOT ask the user to resend") {
		t.Errorf("unusable error must not tell the user to resend a present file:\n%s", msg)
	}
}

// Same physical file addressed by two id forms must load once, not twice.
func TestResolveRefImageIDsDetailed_DedupesSameFileByPath(t *testing.T) {
	dir := t.TempDir()
	r := writeRef(t, dir, "photo.jpg", "image/jpeg", []byte{0xff, 0xd8})

	out, missing, _, _ := resolveRefImageIDsDetailed([]string{r.Path, "photo.jpg"}, []providers.MediaRef{r}, maxResolvedRefImages)
	if len(out) != 1 {
		t.Fatalf("len(out) = %d, want 1 (same file via abs path + basename)", len(out))
	}
	if len(missing) != 0 {
		t.Fatalf("missing = %v, want none", missing)
	}
}
