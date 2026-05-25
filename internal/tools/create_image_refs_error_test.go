package tools

import (
	"strings"
	"testing"

	"github.com/nextlevelbuilder/goclaw/internal/providers"
)

func TestFormatRefResolveError_NoAvailable(t *testing.T) {
	msg := formatRefResolveError([]string{"/tmp/foo.png"}, nil)
	for _, want := range []string{"could not be resolved", "no user-uploaded images", "/tmp/foo.png", "Do NOT pass sandbox paths"} {
		if !strings.Contains(msg, want) {
			t.Errorf("missing %q in error:\n%s", want, msg)
		}
	}
}

func TestFormatRefResolveError_WithAvailable_ListsThem(t *testing.T) {
	refs := []providers.MediaRef{
		{ID: "abc123", Path: "/app/workspace/tenants/t1/uploads/logo.png", MimeType: "image/png"},
		{ID: "def456", Path: "/app/workspace/tenants/t1/uploads/photo.jpg", MimeType: "image/jpeg"},
	}
	msg := formatRefResolveError([]string{"/tmp/wrong.png"}, refs)
	for _, want := range []string{`id="abc123"`, `basename="logo.png"`, `id="def456"`, `basename="photo.jpg"`, `mime=image/png`} {
		if !strings.Contains(msg, want) {
			t.Errorf("missing %q in error:\n%s", want, msg)
		}
	}
	if strings.Contains(msg, "no user-uploaded images") {
		t.Error("with-available message must not say 'no user-uploaded images'")
	}
}

func TestFormatRefPartialResolveNote_Empty(t *testing.T) {
	if got := formatRefPartialResolveNote(nil, nil); got != "" {
		t.Errorf("empty unresolved should produce empty note, got %q", got)
	}
}

func TestFormatRefPartialResolveNote_Lists(t *testing.T) {
	refs := []providers.MediaRef{
		{ID: "abc123", Path: "/uploads/logo.png", MimeType: "image/png"},
	}
	note := formatRefPartialResolveNote([]string{"/tmp/bad.png"}, refs)
	for _, want := range []string{"did not resolve", "/tmp/bad.png", `id="abc123"`, `basename="logo.png"`} {
		if !strings.Contains(note, want) {
			t.Errorf("missing %q in note:\n%s", want, note)
		}
	}
}

func TestResolveRefImageIDsDetailed_ReturnsUnresolvedKeys(t *testing.T) {
	refs := []providers.MediaRef{
		{ID: "good", Path: "/nonexistent/x.png", MimeType: "image/png"},
	}
	_, unresolved := resolveRefImageIDsDetailed(nil, []string{"good", "missing"}, refs, 4)
	hasMissing := false
	for _, u := range unresolved {
		if u == "missing" {
			hasMissing = true
		}
	}
	if !hasMissing {
		t.Errorf("expected 'missing' in unresolved, got %v", unresolved)
	}
}
