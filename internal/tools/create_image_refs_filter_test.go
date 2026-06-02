package tools

import (
	"context"
	"testing"

	"github.com/nextlevelbuilder/goclaw/internal/providers"
)

// fakeRefsCapableProvider implements just enough of providers.Provider +
// providers.CapabilitiesAware to drive filterChainForRefs in unit tests.
type fakeRefsCapableProvider struct {
	name      string
	imageRefs bool
}

func (f *fakeRefsCapableProvider) Name() string         { return f.name }
func (f *fakeRefsCapableProvider) DefaultModel() string { return "" }
func (f *fakeRefsCapableProvider) Chat(context.Context, providers.ChatRequest) (*providers.ChatResponse, error) {
	return nil, nil
}
func (f *fakeRefsCapableProvider) ChatStream(context.Context, providers.ChatRequest, func(providers.StreamChunk)) (*providers.ChatResponse, error) {
	return nil, nil
}
func (f *fakeRefsCapableProvider) Capabilities() providers.ProviderCapabilities {
	return providers.ProviderCapabilities{ImageRefs: f.imageRefs}
}

func buildFakeRegistry(t *testing.T, entries map[string]bool) *providers.Registry {
	t.Helper()
	r := providers.NewRegistry(nil)
	for name, refs := range entries {
		r.Register(&fakeRefsCapableProvider{name: name, imageRefs: refs})
	}
	return r
}

func TestProviderTypeEmitsImageRefs(t *testing.T) {
	cases := []struct {
		ptype string
		want  bool
	}{
		{"openai", true},
		{"gemini_native", true},
		{"openrouter", true},
		{"minimax", true},
		{"minimax_native", true},
		{"dashscope", false},
		{"byteplus", false},
		{"openai_compat", false},
		{"chatgpt_oauth", false},
		{"", false},
	}
	for _, c := range cases {
		if got := providers.ProviderTypeEmitsImageRefs(c.ptype); got != c.want {
			t.Errorf("ProviderTypeEmitsImageRefs(%q) = %v, want %v", c.ptype, got, c.want)
		}
	}
}

func TestFilterChainForRefs_PreservesOperatorOrder(t *testing.T) {
	reg := buildFakeRegistry(t, map[string]bool{
		"dashscope": false,
		"gemini":    true,
		"byteplus":  false,
		"openai":    true,
	})
	chain := []MediaProviderEntry{
		{Provider: "dashscope"},
		{Provider: "gemini"},
		{Provider: "byteplus"},
		{Provider: "openai"},
	}
	got := filterChainForRefs(context.Background(), chain, reg)
	if len(got) != 2 {
		t.Fatalf("expected 2 refs-capable entries, got %d: %+v", len(got), got)
	}
	if got[0].Provider != "gemini" || got[1].Provider != "openai" {
		t.Errorf("operator order must be preserved; got: %+v", got)
	}
}

func TestFilterChainForRefs_AllDropped(t *testing.T) {
	reg := buildFakeRegistry(t, map[string]bool{
		"dashscope": false,
		"byteplus":  false,
	})
	chain := []MediaProviderEntry{
		{Provider: "dashscope"},
		{Provider: "byteplus"},
	}
	got := filterChainForRefs(context.Background(), chain, reg)
	if len(got) != 0 {
		t.Errorf("expected empty filtered chain, got %d entries: %+v", len(got), got)
	}
}

func TestFilterChainForRefs_UnregisteredEntrySkipped(t *testing.T) {
	reg := buildFakeRegistry(t, map[string]bool{
		"gemini": true,
	})
	chain := []MediaProviderEntry{
		{Provider: "ghost-provider"},
		{Provider: "gemini"},
	}
	got := filterChainForRefs(context.Background(), chain, reg)
	if len(got) != 1 || got[0].Provider != "gemini" {
		t.Errorf("unregistered entry should be skipped silently; got: %+v", got)
	}
}
