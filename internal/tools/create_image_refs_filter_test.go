package tools

import "testing"

func TestProviderSupportsRefs(t *testing.T) {
	cases := []struct {
		ptype string
		want  bool
	}{
		{"gemini", true},
		{"openrouter", true},
		{"openai", true},
		{"minimax", true},
		{"codex", true},
		{"chatgpt_oauth_router", true},
		{"dashscope", false},
		{"byteplus", false},
		{"openai_compat", false},
		{"unknown", false},
		{"", false},
	}
	for _, c := range cases {
		if got := providerSupportsRefs(c.ptype); got != c.want {
			t.Errorf("providerSupportsRefs(%q) = %v, want %v", c.ptype, got, c.want)
		}
	}
}

func TestFilterChainForRefs(t *testing.T) {
	chain := []MediaProviderEntry{
		{Provider: "dashscope", Model: "wan2.6-image"},
		{Provider: "gemini", Model: "gemini-2.5-flash-image"},
		{Provider: "byteplus", Model: "seedream-5-0"},
		{Provider: "openai", Model: "gpt-image-1.5"},
	}
	got := filterChainForRefs(chain)
	if len(got) != 2 {
		t.Fatalf("expected 2 refs-capable entries, got %d: %+v", len(got), got)
	}
	if got[0].Provider != "gemini" || got[1].Provider != "openai" {
		t.Errorf("unexpected filtered chain order: %+v", got)
	}
}

func TestFilterChainForRefs_AllDropped(t *testing.T) {
	chain := []MediaProviderEntry{
		{Provider: "dashscope", Model: "wan2.6-image"},
		{Provider: "byteplus", Model: "seedream-5-0"},
	}
	got := filterChainForRefs(chain)
	if len(got) != 0 {
		t.Errorf("expected empty filtered chain, got %d entries: %+v", len(got), got)
	}
}

func TestFormatRefsDroppedNote(t *testing.T) {
	ids := []string{"abc-123"}

	got := formatRefsDroppedNote("refs_failed", ids)
	if got == "" || !contains(got, "could not be applied") || !contains(got, "abc-123") {
		t.Errorf("refs_failed note malformed: %q", got)
	}

	got = formatRefsDroppedNote("no_refs_capable_provider", ids)
	if got == "" || !contains(got, "no configured image provider") || !contains(got, "Gemini or OpenAI") {
		t.Errorf("no_refs_capable_provider note malformed: %q", got)
	}

	got = formatRefsDroppedNote("", ids)
	if got != "" {
		t.Errorf("empty reason must return empty note, got %q", got)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
