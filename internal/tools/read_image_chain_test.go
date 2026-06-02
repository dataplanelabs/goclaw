package tools

import (
	"context"
	"testing"

	"github.com/nextlevelbuilder/goclaw/internal/providers"
)

type captureVisionProvider struct {
	name          string
	defaultModel  string
	capturedModel string
}

func (p *captureVisionProvider) Name() string         { return p.name }
func (p *captureVisionProvider) DefaultModel() string { return p.defaultModel }
func (p *captureVisionProvider) Chat(_ context.Context, req providers.ChatRequest) (*providers.ChatResponse, error) {
	p.capturedModel = req.Model
	return &providers.ChatResponse{Content: "vision ok"}, nil
}
func (p *captureVisionProvider) ChatStream(_ context.Context, _ providers.ChatRequest, _ func(providers.StreamChunk)) (*providers.ChatResponse, error) {
	return &providers.ChatResponse{}, nil
}

func TestReadImageTool_UsesChainProviderAndNormalizesCodexModel(t *testing.T) {
	fake := &captureVisionProvider{name: "codex-cnb", defaultModel: "gpt-5.5"}
	tool := NewReadImageTool(providers.NewRegistry(nil))

	out, _, err := tool.callProvider(context.Background(), nil, "codex-cnb", "gpt-5.3-codex", map[string]any{
		"_native_provider": fake,
		"prompt":           "describe",
		"images": []providers.ImageContent{{
			MimeType: "image/jpeg",
			Data:     "/9j/",
		}},
	})
	if err != nil {
		t.Fatalf("callProvider() error = %v", err)
	}
	if string(out) != "vision ok" {
		t.Fatalf("callProvider() output = %q, want vision ok", string(out))
	}
	if fake.capturedModel != "gpt-5.5" {
		t.Fatalf("captured model = %q, want gpt-5.5", fake.capturedModel)
	}
}
