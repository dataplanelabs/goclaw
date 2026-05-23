package tools

import (
	"context"
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nextlevelbuilder/goclaw/internal/providers"
)

// writeRef persists `data` to a temp file and returns a MediaRef pointing to it.
func writeRef(t *testing.T, dir, name, mime string, data []byte) providers.MediaRef {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write fixture %s: %v", name, err)
	}
	return providers.MediaRef{ID: name, MimeType: mime, Kind: "image", Path: path}
}

func TestResolveRefImageIDs_HappyPath(t *testing.T) {
	dir := t.TempDir()
	jpegBytes := []byte{0xff, 0xd8, 0xff, 0xe0, 0x00, 0x10, 0x4a, 0x46, 0x49, 0x46}
	pngBytes := []byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a}
	r1 := writeRef(t, dir, "a", "image/jpeg", jpegBytes)
	r2 := writeRef(t, dir, "b", "image/png", pngBytes)

	got := resolveRefImageIDs(context.Background(),
		[]string{"a", "b"},
		[]providers.MediaRef{r1, r2},
		maxRefImages,
	)

	if len(got) != 2 {
		t.Fatalf("len(got) = %d, want 2", len(got))
	}
	if got[0].MimeType != "image/jpeg" {
		t.Errorf("got[0].MimeType = %q, want image/jpeg", got[0].MimeType)
	}
	wantJPEG := base64.StdEncoding.EncodeToString(jpegBytes)
	if got[0].Data != wantJPEG {
		t.Errorf("got[0].Data mismatch")
	}
	wantPNG := base64.StdEncoding.EncodeToString(pngBytes)
	if got[1].Data != wantPNG {
		t.Errorf("got[1].Data mismatch")
	}
}

func TestResolveRefImageIDs_EmptyIDsReturnsNil(t *testing.T) {
	got := resolveRefImageIDs(context.Background(), nil, []providers.MediaRef{}, maxRefImages)
	if len(got) != 0 {
		t.Fatalf("len(got) = %d, want 0", len(got))
	}
}

func TestResolveRefImageIDs_IDNotInCtxDropped(t *testing.T) {
	dir := t.TempDir()
	r := writeRef(t, dir, "real", "image/jpeg", []byte{0xff, 0xd8})

	got := resolveRefImageIDs(context.Background(),
		[]string{"missing", "real"},
		[]providers.MediaRef{r},
		maxRefImages,
	)
	if len(got) != 1 {
		t.Fatalf("len(got) = %d, want 1 (missing dropped, real kept)", len(got))
	}
	if got[0].MimeType != "image/jpeg" {
		t.Errorf("MimeType = %q", got[0].MimeType)
	}
}

func TestResolveRefImageIDs_UnsupportedMIMEFiltered(t *testing.T) {
	dir := t.TempDir()
	svg := writeRef(t, dir, "vec", "image/svg+xml", []byte("<svg/>"))
	gif := writeRef(t, dir, "anim", "image/gif", []byte{0x47, 0x49, 0x46, 0x38})
	jpg := writeRef(t, dir, "good", "image/jpeg", []byte{0xff, 0xd8})

	got := resolveRefImageIDs(context.Background(),
		[]string{"vec", "anim", "good"},
		[]providers.MediaRef{svg, gif, jpg},
		maxRefImages,
	)
	if len(got) != 1 {
		t.Fatalf("len(got) = %d, want 1 (svg+gif filtered)", len(got))
	}
	if got[0].MimeType != "image/jpeg" {
		t.Errorf("MimeType = %q", got[0].MimeType)
	}
}

func TestResolveRefImageIDs_DedupePreservesOrder(t *testing.T) {
	dir := t.TempDir()
	r1 := writeRef(t, dir, "a", "image/jpeg", []byte{0xff, 0xd8})
	r2 := writeRef(t, dir, "b", "image/png", []byte{0x89, 0x50, 0x4e, 0x47})

	got := resolveRefImageIDs(context.Background(),
		[]string{"a", "a", "b", "a"},
		[]providers.MediaRef{r1, r2},
		maxRefImages,
	)
	if len(got) != 2 {
		t.Fatalf("len(got) = %d, want 2 after dedup", len(got))
	}
	if got[0].MimeType != "image/jpeg" || got[1].MimeType != "image/png" {
		t.Errorf("order broken: got[0]=%q got[1]=%q", got[0].MimeType, got[1].MimeType)
	}
}

func TestResolveRefImageIDs_CapTruncates(t *testing.T) {
	dir := t.TempDir()
	refs := make([]providers.MediaRef, 0, 6)
	ids := make([]string, 0, 6)
	for i := range 6 {
		name := string(rune('a' + i))
		refs = append(refs, writeRef(t, dir, name, "image/jpeg", []byte{0xff, 0xd8, byte(i)}))
		ids = append(ids, name)
	}

	got := resolveRefImageIDs(context.Background(), ids, refs, 4)
	if len(got) != 4 {
		t.Fatalf("len(got) = %d, want 4 (cap)", len(got))
	}
}

func TestResolveRefImageIDs_OversizedDropped(t *testing.T) {
	dir := t.TempDir()
	huge := make([]byte, maxRefImageBytes+1)
	r := writeRef(t, dir, "big", "image/jpeg", huge)
	small := writeRef(t, dir, "ok", "image/jpeg", []byte{0xff, 0xd8})

	got := resolveRefImageIDs(context.Background(),
		[]string{"big", "ok"},
		[]providers.MediaRef{r, small},
		maxRefImages,
	)
	if len(got) != 1 {
		t.Fatalf("len(got) = %d, want 1 (oversized dropped)", len(got))
	}
}

func TestToStringSlice(t *testing.T) {
	tests := []struct {
		name string
		in   any
		want []string
	}{
		{"nil", nil, nil},
		{"string slice", []string{"a", "", "b"}, []string{"a", "b"}},
		{"any slice", []any{"a", 1, "b", ""}, []string{"a", "b"}},
		{"empty", []any{}, []string{}},
		{"wrong type", 42, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := toStringSlice(tt.in)
			if !equalStringSlices(got, tt.want) {
				t.Errorf("toStringSlice(%v) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

// TestCreateImageTool_ParametersIncludesRefIDs verifies the tool schema exposes
// reference_image_ids to the LLM.
func TestCreateImageTool_ParametersIncludesRefIDs(t *testing.T) {
	tool := NewCreateImageTool(nil)
	params := tool.Parameters()
	props, _ := params["properties"].(map[string]any)
	ref, ok := props["reference_image_ids"]
	if !ok {
		t.Fatalf("reference_image_ids missing from Parameters()")
	}
	m := ref.(map[string]any)
	if m["type"] != "array" {
		t.Errorf("reference_image_ids.type = %v, want array", m["type"])
	}
	desc, _ := m["description"].(string)
	if !strings.Contains(desc, "<media:image") {
		t.Errorf("description should mention <media:image tag, got %q", desc)
	}
}

// TestCreateImageTool_Description_MentionsRefSupport ensures the LLM-facing
// description tells the model that reference photos are supported.
func TestCreateImageTool_Description_MentionsRefSupport(t *testing.T) {
	tool := NewCreateImageTool(nil)
	d := tool.Description()
	if !strings.Contains(d, "reference_image_ids") {
		t.Errorf("description should mention reference_image_ids, got %q", d)
	}
}

func equalStringSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
