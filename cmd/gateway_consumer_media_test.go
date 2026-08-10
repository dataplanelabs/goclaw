package cmd

// Regression tests for send_file media delivery when the agent's final text is empty/silent.
// Root cause: gateway_consumer_normal.go suppressed media when content=="" because it
// returned early before calling appendMediaToOutbound (unlike the announce-queue path that
// already guarded with `!(isSilent && len(media)==0)`).

import (
	"testing"

	"github.com/nextlevelbuilder/goclaw/internal/agent"
	"github.com/nextlevelbuilder/goclaw/internal/bus"
)

// TestAppendMediaToOutbound_PopulatesURL verifies the helper sets MediaAttachment.URL
// from MediaResult.Path so channel adapters (Slack uploadFile, Telegram sendDocument, etc.)
// receive the correct local path.
func TestAppendMediaToOutbound_PopulatesURL(t *testing.T) {
	msg := bus.OutboundMessage{}
	appendMediaToOutbound(&msg, []agent.MediaResult{
		{Path: "/workspace/cats.png", ContentType: "image/png", Caption: "cat caption"},
		{Path: "/workspace/report.pdf", ContentType: "application/pdf"},
	})
	if len(msg.Media) != 2 {
		t.Fatalf("expected 2 attachments, got %d", len(msg.Media))
	}
	if msg.Media[0].URL != "/workspace/cats.png" {
		t.Errorf("Media[0].URL = %q, want /workspace/cats.png", msg.Media[0].URL)
	}
	if msg.Media[1].URL != "/workspace/report.pdf" {
		t.Errorf("Media[1].URL = %q, want /workspace/report.pdf", msg.Media[1].URL)
	}
	if msg.Media[0].ContentType != "image/png" {
		t.Errorf("Media[0].ContentType = %q, want image/png", msg.Media[0].ContentType)
	}
	if msg.Media[0].Caption != "cat caption" {
		t.Errorf("Media[0].Caption = %q, want cat caption", msg.Media[0].Caption)
	}
}

// TestAppendMediaToOutbound_VoiceAnnotation verifies that AsVoice=true stamps
// audio_as_voice metadata so Telegram sends OGG as a voice message.
func TestAppendMediaToOutbound_VoiceAnnotation(t *testing.T) {
	msg := bus.OutboundMessage{}
	appendMediaToOutbound(&msg, []agent.MediaResult{
		{Path: "/tmp/reply.ogg", ContentType: "audio/ogg", AsVoice: true},
	})
	if msg.Metadata["audio_as_voice"] != "true" {
		t.Errorf("audio_as_voice metadata = %q, want true", msg.Metadata["audio_as_voice"])
	}
}

// TestAppendMediaToOutbound_EmptyNoOp verifies that an empty media slice is a no-op.
func TestAppendMediaToOutbound_EmptyNoOp(t *testing.T) {
	msg := bus.OutboundMessage{Content: "hello"}
	appendMediaToOutbound(&msg, nil)
	if len(msg.Media) != 0 {
		t.Errorf("expected 0 attachments for nil media, got %d", len(msg.Media))
	}
	if msg.Content != "hello" {
		t.Errorf("Content must not be modified, got %q", msg.Content)
	}
}

func TestFilterAnnounceForwardedMedia_KeepsForwardedWhenNoManualMedia(t *testing.T) {
	forwarded := []bus.MediaFile{{Path: "/team/out.png", MimeType: "image/png"}}
	media := []agent.MediaResult{{Path: "/team/out.png", ContentType: "image/png"}}

	got := filterAnnounceForwardedMedia(media, forwarded)
	if len(got) != 1 || got[0].Path != "/team/out.png" {
		t.Fatalf("expected forwarded media to stay when no manual media exists, got %+v", got)
	}
}

func TestFilterAnnounceForwardedMedia_DropsForwardedWhenManualMediaExists(t *testing.T) {
	forwarded := []bus.MediaFile{{Path: "/team/out.png", MimeType: "image/png"}}
	media := []agent.MediaResult{
		{Path: "/team/out.png", ContentType: "image/png"},
		{Path: "/agent/generated/out.png", ContentType: "image/png"},
	}

	got := filterAnnounceForwardedMedia(media, forwarded)
	if len(got) != 1 {
		t.Fatalf("expected only manual media, got %+v", got)
	}
	if got[0].Path != "/agent/generated/out.png" {
		t.Fatalf("expected manual media path, got %+v", got)
	}
}
