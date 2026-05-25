package agent

import (
	"strings"
	"testing"
)

// Test 8: ChatID present → prompt contains <current_reply_target> block.
func TestSystemPromptCurrentReplyTargetInjected(t *testing.T) {
	cfg := fullTestConfig()
	cfg.Channel = "telegram"
	cfg.ChannelType = "telegram"
	cfg.ChatID = "123"
	cfg.PeerKind = "direct"

	prompt := BuildSystemPrompt(cfg)

	for _, want := range []string{
		"<current_reply_target>",
		"chat_id: 123",
		"kind: direct",
		"</current_reply_target>",
		"omit `target` to reply here",
	} {
		if !strings.Contains(prompt, want) {
			t.Errorf("prompt missing %q", want)
		}
	}
}

// Test 8b: group peer → kind: group.
func TestSystemPromptCurrentReplyTargetGroup(t *testing.T) {
	cfg := fullTestConfig()
	cfg.Channel = "telegram"
	cfg.ChannelType = "telegram"
	cfg.ChatID = "-100G"
	cfg.PeerKind = "group"

	prompt := BuildSystemPrompt(cfg)

	if !strings.Contains(prompt, "chat_id: -100G") {
		t.Error("prompt missing group chat_id")
	}
	if !strings.Contains(prompt, "kind: group") {
		t.Error("prompt missing kind: group")
	}
}

// Test 9: ChatID empty → no <current_reply_target> block.
func TestSystemPromptCurrentReplyTargetOmittedWhenNoChat(t *testing.T) {
	cfg := fullTestConfig()
	cfg.ChatID = ""
	prompt := BuildSystemPrompt(cfg)
	if strings.Contains(prompt, "<current_reply_target>") {
		t.Error("prompt should NOT include <current_reply_target> when ChatID is empty")
	}
}

// Reply-target channel renders the instance name (cfg.Channel), not the type.
// Identity narrative line uses cfg.ChannelType for readability.
func TestSystemPromptReplyTargetUsesInstanceName(t *testing.T) {
	cfg := fullTestConfig()
	cfg.Channel = "zalo-annhien"
	cfg.ChannelType = "zalo_personal"
	cfg.ChatID = "4075490771358232471"
	cfg.PeerKind = "group"

	prompt := BuildSystemPrompt(cfg)

	if !strings.Contains(prompt, "channel: zalo-annhien") {
		t.Error("prompt should render `channel: zalo-annhien` (instance name) in <current_reply_target>")
	}
	if strings.Contains(prompt, "channel: zalo_personal") {
		t.Error("prompt should NOT render channel TYPE inside <current_reply_target>")
	}
	if !strings.Contains(prompt, "running in zalo_personal") {
		t.Error("identity narrative should use channel TYPE for readability")
	}
	if !strings.Contains(prompt, "kind: group") {
		t.Error("group chat must stamp kind: group")
	}
}

// Fallback path: instance name absent → reply-target falls back to type.
func TestSystemPromptReplyTargetFallbackToType(t *testing.T) {
	cfg := fullTestConfig()
	cfg.Channel = ""
	cfg.ChannelType = "telegram"
	cfg.ChatID = "999"
	cfg.PeerKind = "direct"

	prompt := BuildSystemPrompt(cfg)

	if !strings.Contains(prompt, "channel: telegram") {
		t.Error("prompt should fall back to ChannelType when Channel is empty")
	}
}
