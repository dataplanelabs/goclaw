package personal

import (
	"context"
	"testing"
	"time"

	"github.com/nextlevelbuilder/goclaw/internal/bus"
	"github.com/nextlevelbuilder/goclaw/internal/channels"
	"github.com/nextlevelbuilder/goclaw/internal/config"
	"github.com/nextlevelbuilder/goclaw/internal/channels/zalo/personal/protocol"
)

// newPolicyTestChannel builds a Channel with the given allowlist + policies,
// wired enough to invoke dispatchInboundTurn without a Zalo session.
func newPolicyTestChannel(t *testing.T, allowFrom []string, dmPolicy, groupPolicy string) (*Channel, *bus.MessageBus) {
	t.Helper()
	mb := bus.New()
	base := channels.NewBaseChannel(channels.TypeZaloPersonal, mb, allowFrom)
	ch := &Channel{
		BaseChannel: base,
		config: config.ZaloPersonalConfig{
			DMPolicy:    dmPolicy,
			GroupPolicy: groupPolicy,
			AllowFrom:   allowFrom,
		},
		stopCh: make(chan struct{}),
	}
	ch.turnCoalescer = channels.NewTurnCoalescer[inboundTurn](0, mergeInboundTurns, ch.dispatchInboundTurn)
	return ch, mb
}

// #255: a turn accepted under an old policy must be dropped at dequeue when the
// sender is no longer allowed — not delivered to the agent.
func TestDispatchInboundTurn_DropsWhenSenderNoLongerAllowed(t *testing.T) {
	t.Parallel()
	// Allowlist that EXCLUDES the sender → CheckDMPolicy returns PolicyDeny.
	ch, mb := newPolicyTestChannel(t, []string{"approved-user"}, "allowlist", "allowlist")

	ch.dispatchInboundTurn(inboundTurn{
		senderID:   "blocked-user",
		threadID:   "thread-1",
		content:    "hello",
		peerKind:   "group", // group bypasses HandleMessage's DM safety-net; isolates the dequeue check
		threadType: protocol.ThreadTypeGroup,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	if msg, ok := mb.ConsumeInbound(ctx); ok {
		t.Fatalf("blocked sender's queued turn was delivered: %+v", msg)
	}
}

// Sanity: an allowed sender's turn still runs after the dequeue re-check.
func TestDispatchInboundTurn_DeliversWhenSenderStillAllowed(t *testing.T) {
	t.Parallel()
	ch, mb := newPolicyTestChannel(t, []string{"approved-user"}, "allowlist", "allowlist")

	ch.dispatchInboundTurn(inboundTurn{
		senderID:   "approved-user",
		threadID:   "thread-1",
		content:    "hello",
		peerKind:   "group",
		threadType: protocol.ThreadTypeGroup,
	})

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	msg, ok := mb.ConsumeInbound(ctx)
	if !ok {
		t.Fatal("allowed sender's turn should have been delivered")
	}
	if msg.SenderID != "approved-user" {
		t.Errorf("delivered SenderID = %q, want approved-user", msg.SenderID)
	}
}
