package tools

import (
	"context"
	"testing"

	"github.com/nextlevelbuilder/goclaw/internal/providers"
)

// singleAttemptCapturingProvider records whether ctx carried WithSingleAttempt
// when GenerateImage was invoked. The flag is exercised end-to-end by RetryDo
// in the real CodexProvider; here we assert callProvider sets it so the chain
// stays the single retry authority (#254 — collapse 3×N retry amplification).
type singleAttemptCapturingProvider struct {
	name         string
	model        string
	returnData   []byte
	sawSingleCtx bool
}

func (p *singleAttemptCapturingProvider) Name() string         { return p.name }
func (p *singleAttemptCapturingProvider) DefaultModel() string { return p.model }
func (p *singleAttemptCapturingProvider) Chat(_ context.Context, _ providers.ChatRequest) (*providers.ChatResponse, error) {
	return &providers.ChatResponse{}, nil
}
func (p *singleAttemptCapturingProvider) ChatStream(_ context.Context, _ providers.ChatRequest, _ func(providers.StreamChunk)) (*providers.ChatResponse, error) {
	return &providers.ChatResponse{}, nil
}
func (p *singleAttemptCapturingProvider) GenerateImage(ctx context.Context, _ providers.NativeImageRequest) (*providers.NativeImageResult, error) {
	// RetryDo collapses to one attempt iff this is set — assert callProvider applied it.
	p.sawSingleCtx = providers.RetryAttemptsForCtx(ctx, 3) == 1
	return &providers.NativeImageResult{MimeType: "image/png", Data: p.returnData}, nil
}

func TestCreateImageTool_NativePath_CollapsesProviderRetryToSingleAttempt(t *testing.T) {
	pngMagic := []byte{
		0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a,
		0x00, 0x00, 0x00, 0x00, 0x49, 0x45, 0x4e, 0x44, 0xae, 0x42, 0x60, 0x82,
	}
	fake := &singleAttemptCapturingProvider{name: "openai-codex", model: "gpt-image-2", returnData: pngMagic}

	reg := providers.NewRegistry(nil)
	reg.Register(fake)

	chain := []MediaProviderEntry{
		{Provider: "openai-codex", Model: "gpt-image-2", Enabled: true, Timeout: 30, MaxRetries: 1},
	}
	ctx := WithToolWorkspace(context.Background(), t.TempDir())
	tool := NewCreateImageTool(reg)

	if _, err := ExecuteWithChain(ctx, chain, reg, tool.callProvider); err != nil {
		t.Fatalf("ExecuteWithChain error: %v", err)
	}
	if !fake.sawSingleCtx {
		t.Error("native provider was NOT invoked with single-attempt ctx — inner retry not collapsed (3×N amplification persists)")
	}
}
