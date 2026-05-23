package agent

import (
	"strings"
	"testing"

	"github.com/nextlevelbuilder/goclaw/internal/providers"
)

// Reproduces trace 019e5666: user quotes their own image-gen request, prior
// bot reply produced runner PNGs in /generated/. Hint must surface those paths.
func TestAppendSelfQuoteGenerationHint_SurfacesBotReplyImages(t *testing.T) {
	t.Parallel()
	messages := []providers.Message{
		{
			Role:    "user",
			Content: "tạo ảnh buồn kiểu chạy\n<media:image>",
			MediaRefs: []providers.MediaRef{
				{ID: "u-1", Kind: "image", Path: "/ws/.uploads/selfie.jpg"},
			},
		},
		{
			Role:    "assistant",
			Content: "đây ảnh anh",
			MediaRefs: []providers.MediaRef{
				{ID: "g-1", Kind: "image", Path: "/ws/generated/runner-v1.png"},
				{ID: "g-2", Kind: "image", Path: "/ws/generated/runner-v2.png"},
			},
		},
		{
			Role:    "user",
			Content: "[Replying to their own image]\n[Quoted caption: \"tạo ảnh buồn\"]\nanh nay thi sao\n<media:image>",
		},
	}

	appendSelfQuoteGenerationHint(messages)

	got := messages[len(messages)-1].Content
	if !strings.Contains(got, "/ws/generated/runner-v1.png") || !strings.Contains(got, "/ws/generated/runner-v2.png") {
		t.Errorf("expected hint to include both generated paths, got: %s", got)
	}
	if !strings.Contains(got, "read_image(path=") {
		t.Errorf("expected hint to mention read_image(path=...), got: %s", got)
	}
}

func TestAppendSelfQuoteGenerationHint_NoOpWithoutMarker(t *testing.T) {
	t.Parallel()
	messages := []providers.Message{
		{Role: "user", Content: "earlier", MediaRefs: []providers.MediaRef{{ID: "u-1", Kind: "image", Path: "/ws/x.jpg"}}},
		{Role: "assistant", Content: "ok", MediaRefs: []providers.MediaRef{{ID: "g-1", Kind: "image", Path: "/ws/gen.png"}}},
		{Role: "user", Content: "plain follow-up, no quote"},
	}
	before := messages[len(messages)-1].Content
	appendSelfQuoteGenerationHint(messages)
	if got := messages[len(messages)-1].Content; got != before {
		t.Errorf("non-quote message should not be mutated, got: %s", got)
	}
}

func TestAppendSelfQuoteGenerationHint_NoOpWhenBotReplyHasNoImage(t *testing.T) {
	t.Parallel()
	messages := []providers.Message{
		{Role: "user", Content: "earlier", MediaRefs: []providers.MediaRef{{ID: "u-1", Kind: "image", Path: "/ws/x.jpg"}}},
		{Role: "assistant", Content: "text-only reply"},
		{Role: "user", Content: "[Replying to their own image]\nfollow-up"},
	}
	before := messages[len(messages)-1].Content
	appendSelfQuoteGenerationHint(messages)
	if got := messages[len(messages)-1].Content; got != before {
		t.Errorf("no bot images means no hint, got: %s", got)
	}
}

func TestAppendSelfQuoteGenerationHint_NoOpWhenPriorUserHasNoImage(t *testing.T) {
	t.Parallel()
	messages := []providers.Message{
		{Role: "user", Content: "text only earlier"},
		{Role: "assistant", Content: "reply", MediaRefs: []providers.MediaRef{{ID: "g-1", Kind: "image", Path: "/ws/gen.png"}}},
		{Role: "user", Content: "[Replying to their own image]\nfollow-up"},
	}
	before := messages[len(messages)-1].Content
	appendSelfQuoteGenerationHint(messages)
	if got := messages[len(messages)-1].Content; got != before {
		t.Errorf("no prior user image means no hint, got: %s", got)
	}
}

func TestAppendSelfQuoteGenerationHint_PicksMostRecentPriorUserImage(t *testing.T) {
	t.Parallel()
	messages := []providers.Message{
		{Role: "user", Content: "old", MediaRefs: []providers.MediaRef{{ID: "u-old", Kind: "image", Path: "/ws/old.jpg"}}},
		{Role: "assistant", Content: "reply-old", MediaRefs: []providers.MediaRef{{ID: "g-old", Kind: "image", Path: "/ws/old-gen.png"}}},
		{Role: "user", Content: "recent", MediaRefs: []providers.MediaRef{{ID: "u-new", Kind: "image", Path: "/ws/new.jpg"}}},
		{Role: "assistant", Content: "reply-new", MediaRefs: []providers.MediaRef{{ID: "g-new", Kind: "image", Path: "/ws/new-gen.png"}}},
		{Role: "user", Content: "[Replying to their own image]\nfollow-up"},
	}
	appendSelfQuoteGenerationHint(messages)
	got := messages[len(messages)-1].Content
	if !strings.Contains(got, "/ws/new-gen.png") {
		t.Errorf("expected most-recent bot reply image, got: %s", got)
	}
	if strings.Contains(got, "/ws/old-gen.png") {
		t.Errorf("should not include older bot reply image, got: %s", got)
	}
}
