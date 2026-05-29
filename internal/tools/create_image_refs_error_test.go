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

func TestFormatRefTrimmedNote_Empty(t *testing.T) {
	if got := formatRefTrimmedNote(nil); got != "" {
		t.Errorf("empty trimmed should produce empty note, got %q", got)
	}
}

func TestFormatRefTrimmedNote_Lists(t *testing.T) {
	note := formatRefTrimmedNote([]string{"x", "y"})
	for _, want := range []string{"not sent", "Trimmed: [x y]"} {
		if !strings.Contains(note, want) {
			t.Errorf("missing %q in note:\n%s", want, note)
		}
	}
	// Trimmed (over budget) is NOT the not-found case.
	if strings.Contains(strings.ToLower(note), "could not be resolved") {
		t.Errorf("trimmed note must be distinct from the not-found error")
	}
}

func TestFormatRefPartialResolveError_ListsAvailableAndGuidance(t *testing.T) {
	refs := []providers.MediaRef{
		{ID: "abc123", Path: "/uploads/logo.png", MimeType: "image/png"},
	}
	msg := formatRefPartialResolveError([]string{"/tmp/bad.png"}, refs)
	for _, want := range []string{"could not be resolved", "/tmp/bad.png", `id="abc123"`, `basename="logo.png"`, "resend"} {
		if !strings.Contains(msg, want) {
			t.Errorf("missing %q in error:\n%s", want, msg)
		}
	}
}

func TestFormatRefPartialResolveError_NoAvailableSuggestsResend(t *testing.T) {
	msg := formatRefPartialResolveError([]string{"missing.png"}, nil)
	if !strings.Contains(msg, "resend") {
		t.Errorf("with no available refs the error must tell the LLM to ask the user to resend:\n%s", msg)
	}
}

func TestResolveRefImageIDsDetailed_ReturnsMissingKeys(t *testing.T) {
	refs := []providers.MediaRef{
		{ID: "good", Path: "/nonexistent/x.png", MimeType: "image/png"},
	}
	// Both are missing: "good" stat-fails (/nonexistent), "missing" not in refs.
	_, missing, _, trimmed := resolveRefImageIDsDetailed([]string{"good", "missing"}, refs, 4)
	hasMissing := false
	for _, u := range missing {
		if u == "missing" {
			hasMissing = true
		}
	}
	if !hasMissing {
		t.Errorf("expected 'missing' in missing keys, got %v", missing)
	}
	if len(trimmed) != 0 {
		t.Errorf("expected no trimmed keys, got %v", trimmed)
	}
}
