package providers

import "testing"

// Authoritative table — providers that pass reference_images to the wire body.
// Drives both the OpenAIProvider multiplex check + acts as documentation.
func TestProviderTypeEmitsImageRefs_Table(t *testing.T) {
	cases := []struct {
		ptype string
		want  bool
	}{
		// Refs-capable wire paths
		{"openai", true},          // /v1/images/edits multipart
		{"gemini_native", true},   // inline_data parts
		{"openrouter", true},      // image_url parts
		{"minimax", true},         // subject_reference
		{"minimax_native", true},  // alias
		// Refs-incapable / not yet implemented
		{"dashscope", false},      // Phase 04 deferred
		{"byteplus", false},       // Phase 04 deferred
		{"openai_compat", false},  // generic proxy — unknown capabilities
		{"chatgpt_oauth", false},  // routed via CodexProvider's own Capabilities()
		{"", false},
		{"unknown", false},
	}
	for _, c := range cases {
		if got := ProviderTypeEmitsImageRefs(c.ptype); got != c.want {
			t.Errorf("ProviderTypeEmitsImageRefs(%q) = %v, want %v", c.ptype, got, c.want)
		}
	}
}

func TestOpenAIProviderCapabilities_ImageRefs(t *testing.T) {
	cases := []struct {
		providerType string
		wantRefs     bool
	}{
		{"openai", true},
		{"gemini_native", true},
		{"openrouter", true},
		{"minimax", true},
		{"dashscope", false},
		{"openai_compat", false},
		{"", false},
	}
	for _, c := range cases {
		p := &OpenAIProvider{providerType: c.providerType}
		got := p.Capabilities().ImageRefs
		if got != c.wantRefs {
			t.Errorf("OpenAIProvider{providerType:%q}.Capabilities().ImageRefs = %v, want %v",
				c.providerType, got, c.wantRefs)
		}
	}
}

func TestCodexProviderCapabilities_ImageRefs(t *testing.T) {
	p := &CodexProvider{}
	if !p.Capabilities().ImageRefs {
		t.Error("CodexProvider must declare ImageRefs=true (Responses API emits input_image)")
	}
}
