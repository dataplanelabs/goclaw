package personal

import (
	"context"
	"strings"
	"testing"

	pkgproto "github.com/nextlevelbuilder/goclaw/pkg/protocol"
)

// TestParseOutboundMentions_DM_RewritesTextNoMentionsWire verifies that the
// parser still rewrites markers to readable text on DM threads, while the
// Mentions slice is dropped at the wire layer (handled by
// SendMessageWithOptions). This is the channel-layer half of the DM degrade
// path; the wire-layer half is covered by
// TestSendMessageWithOptions_DM_DropsMentions in the protocol package.
func TestParseOutboundMentions_DM_RewritesTextNoMentionsWire(t *testing.T) {
	t.Parallel()
	ch, _ := newHandlerTestChannel(t)
	ch.memberCache.Set("u_partner", "u_a", "Alice")

	rendered, ms := ch.parseOutboundMentions(context.Background(), "u_partner", 0 /* user */, "Cảm ơn @[u_a]!")
	if !strings.Contains(rendered, "@Alice") {
		t.Fatalf("DM rendered = %q, expected @Alice in text", rendered)
	}
	if len(ms) != 1 {
		t.Fatalf("parser still surfaces mentions slice for channel-layer; DM wire-drop is at protocol layer. got %+v", ms)
	}
	// Sanity: even though the parser surfaces a mention here, the wire drop
	// occurs in protocol.SendMessageWithOptions for threadType=User. See
	// TestSendMessageWithOptions_DM_DropsMentions in the protocol package.
	if ms[0].UserID != "u_a" {
		t.Fatalf("unexpected mention: %+v", ms[0])
	}
}

// TestParseOutboundMentions_NoMarkers_ReturnsNilMentions confirms the cheap
// pre-check path that skips parsing entirely for plain text.
func TestParseOutboundMentions_NoMarkers_ReturnsNilMentions(t *testing.T) {
	t.Parallel()
	ch, _ := newHandlerTestChannel(t)
	rendered, ms := ch.parseOutboundMentions(context.Background(), "u_partner", 0, "plain text no markers")
	if rendered != "plain text no markers" {
		t.Fatalf("text mutated: %q", rendered)
	}
	if ms != nil {
		t.Fatalf("mentions = %+v, want nil", ms)
	}
	_ = pkgproto.Mention{} // keep import
}
