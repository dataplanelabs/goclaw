package tools

import (
	"context"
	"strings"
	"testing"

	"github.com/nextlevelbuilder/goclaw/internal/providers"
)

// Regression for #219: five valid references (SHTP logo + 2 runner photos +
// blue shirt + green shirt) must all resolve under the resolution cap, so the
// fifth is NOT reported as "did not resolve". Per-provider caps truncate later.
func TestResolveRefImageIDs_FiveRefsResolveUnderResolutionCap(t *testing.T) {
	dir := t.TempDir()
	names := []string{"logo.png", "runner-a.jpg", "runner-b.jpg", "club-shirt-blue.jpg", "club-shirt-green.jpg"}
	refs := make([]providers.MediaRef, 0, len(names))
	for _, n := range names {
		mime := "image/jpeg"
		if strings.HasSuffix(n, ".png") {
			mime = "image/png"
		}
		refs = append(refs, writeRef(t, dir, n, mime, []byte{0xff, 0xd8, 0x00}))
	}

	got, unresolved := resolveRefImageIDsDetailed(context.Background(), names, refs, maxResolvedRefImages)
	if len(got) != 5 {
		t.Fatalf("resolved = %d, want 5 (all valid refs)", len(got))
	}
	if len(unresolved) != 0 {
		t.Fatalf("unresolved = %v, want none (no misleading did-not-resolve for valid refs)", unresolved)
	}
}

func TestImageRefCapForProvider(t *testing.T) {
	cases := map[string]int{
		"openai":     openAIEditRefCap,
		"codex":      codexImageRefCap,
		"chatgpt":    codexImageRefCap,
		"gemini":     geminiRefCap,
		"openrouter": openRouterRefCap,
		"minimax":    minimaxRefCap,
		"dashscope":  0,
		"byteplus":   0,
		"unknown":    maxRefImages,
	}
	for provider, want := range cases {
		if got := imageRefCapForProvider(provider); got != want {
			t.Errorf("imageRefCapForProvider(%q) = %d, want %d", provider, got, want)
		}
	}
	// Case-insensitive.
	if imageRefCapForProvider("OpenAI") != openAIEditRefCap {
		t.Errorf("provider match should be case-insensitive")
	}
}

func TestFormatRefsOverProviderCapNote(t *testing.T) {
	note := formatRefsOverProviderCapNote("gemini", geminiRefCap, 5)
	for _, want := range []string{"1 of 5", "gemini", "at most 4"} {
		if !strings.Contains(note, want) {
			t.Errorf("over-cap note missing %q; got: %s", want, note)
		}
	}
	// Must NOT use the misleading "did not resolve" phrasing.
	if strings.Contains(strings.ToLower(note), "did not resolve") {
		t.Errorf("over-cap note must be distinct from the not-found note")
	}
}
