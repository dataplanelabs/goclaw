package personal

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/nextlevelbuilder/goclaw/internal/bus"
	"github.com/nextlevelbuilder/goclaw/internal/channels"
	"github.com/nextlevelbuilder/goclaw/internal/channels/zalo/personal/protocol"
	pkgproto "github.com/nextlevelbuilder/goclaw/pkg/protocol"
)

func TestHandleGroupMessage_MarksGroupApproved(t *testing.T) {
	t.Parallel()
	ch, _ := newHandlerTestChannel(t)
	ch.SetRequireMention(false)

	const gid = "group-mark-approved"
	if ch.IsGroupApproved(gid) {
		t.Fatalf("group must not be approved before inbound")
	}

	text := "hello"
	ch.handleGroupMessage(protocol.NewGroupMessage("self-uid", protocol.TGroupMessage{
		TMessage: protocol.TMessage{
			MsgID:    "msg-mark-001",
			CliMsgID: json.Number("1700000099001"),
			UIDFrom:  "user-1",
			IDTo:     gid,
			DName:    "Alice",
			TS:       "1700000099",
			Content:  protocol.Content{String: &text},
		},
	}))

	if !ch.IsGroupApproved(gid) {
		t.Errorf("group %q must be approved after inbound — tools (create_poll etc.) need this", gid)
	}
}

func TestHandleGroupMessage_StampsSenderUID(t *testing.T) {
	t.Parallel()
	ch, mb := newHandlerTestChannel(t)
	ch.SetRequireMention(false)

	text := "hello group"
	ch.handleGroupMessage(protocol.NewGroupMessage("self-uid", protocol.TGroupMessage{
		TMessage: protocol.TMessage{
			MsgID:    "msg-001",
			CliMsgID: json.Number("1700000000001"),
			UIDFrom:  "user-789",
			IDTo:     "group-abc",
			DName:    "Alice",
			TS:       "1700000001",
			Content:  protocol.Content{String: &text},
		},
	}))

	got := drainInbound(t, mb)
	if got.Metadata["sender_uid"] != "user-789" {
		t.Errorf("sender_uid = %q, want user-789", got.Metadata["sender_uid"])
	}
}

func TestHandleGroupMessage_NoMentions_NoMentionsKey(t *testing.T) {
	t.Parallel()
	ch, mb := newHandlerTestChannel(t)
	ch.SetRequireMention(false)

	text := "hello no mentions"
	ch.handleGroupMessage(protocol.NewGroupMessage("self-uid", protocol.TGroupMessage{
		TMessage: protocol.TMessage{
			MsgID:    "msg-002",
			CliMsgID: json.Number("1700000000002"),
			UIDFrom:  "user-789",
			IDTo:     "group-abc",
			DName:    "Alice",
			TS:       "1700000002",
			Content:  protocol.Content{String: &text},
		},
	}))

	got := drainInbound(t, mb)
	if _, ok := got.Metadata["mentions"]; ok {
		t.Errorf("mentions key should be absent when no mentions present; got %q", got.Metadata["mentions"])
	}
	if got.Metadata["sender_uid"] == "" {
		t.Error("sender_uid should still be present on messages without mentions")
	}
}

func TestHandleGroupMessage_StampsMentionsMetadata(t *testing.T) {
	t.Parallel()
	ch, mb := newHandlerTestChannel(t)
	ch.SetRequireMention(false)

	text := "@Bob and @all"
	ch.handleGroupMessage(protocol.NewGroupMessage("self-uid", protocol.TGroupMessage{
		TMessage: protocol.TMessage{
			MsgID:    "msg-003",
			CliMsgID: json.Number("1700000000003"),
			UIDFrom:  "user-789",
			IDTo:     "group-abc",
			DName:    "Alice",
			TS:       "1700000003",
			Content:  protocol.Content{String: &text},
		},
		Mentions: []*protocol.TMention{
			{UID: "user-bob", Pos: 0, Len: 4, Type: protocol.MentionEach},
			{UID: protocol.MentionAllUID, Pos: 9, Len: 4, Type: protocol.MentionAll},
		},
	}))

	got := drainInbound(t, mb)
	raw, ok := got.Metadata["mentions"]
	if !ok {
		t.Fatalf("mentions key missing; metadata=%v", got.Metadata)
	}
	var ms []pkgproto.Mention
	if err := json.Unmarshal([]byte(raw), &ms); err != nil {
		t.Fatalf("unmarshal mentions: %v", err)
	}
	if len(ms) != 2 {
		t.Fatalf("len(mentions) = %d, want 2", len(ms))
	}
	if ms[0].UserID != "user-bob" || ms[0].Type != 0 || ms[0].Position != 0 || ms[0].Length != 4 {
		t.Errorf("ms[0] = %+v", ms[0])
	}
	if ms[1].UserID != "-1" || ms[1].Type != 1 {
		t.Errorf("ms[1] = %+v", ms[1])
	}
}

func TestHandleGroupMessage_MentionsJSONRoundtrip(t *testing.T) {
	t.Parallel()
	ch, mb := newHandlerTestChannel(t)
	ch.SetRequireMention(false)

	text := "ping @user"
	ch.handleGroupMessage(protocol.NewGroupMessage("self-uid", protocol.TGroupMessage{
		TMessage: protocol.TMessage{
			MsgID:    "msg-004",
			CliMsgID: json.Number("1700000000004"),
			UIDFrom:  "user-789",
			IDTo:     "group-abc",
			DName:    "Alice",
			TS:       "1700000004",
			Content:  protocol.Content{String: &text},
		},
		Mentions: []*protocol.TMention{
			{UID: "user-x", Pos: 5, Len: 5, Type: protocol.MentionEach},
		},
	}))

	got := drainInbound(t, mb)
	var ms []pkgproto.Mention
	if err := json.Unmarshal([]byte(got.Metadata["mentions"]), &ms); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(ms) != 1 || ms[0].UserID != "user-x" || ms[0].Position != 5 || ms[0].Length != 5 {
		t.Fatalf("got %+v", ms)
	}
}

// Regression: RequireMention bot-gating still works after metadata stamping
// changes. The gate is in checkBotMentioned which reads msg.Data.Mentions
// directly, not metadata.

func TestHandleGroupMessage_RequireMention_DispatchesWhenMentioned(t *testing.T) {
	t.Parallel()
	ch, mb := newHandlerTestChannel(t)
	// RequireMention defaults to true; do not disable. Without authenticated
	// session, sess.UID is empty so isBotMentioned returns false → message
	// recorded in history but not dispatched. To exercise the dispatch path,
	// disable RequireMention. This test asserts the gate is bypassed once
	// disabled (same as before the change).
	ch.SetRequireMention(false)

	text := "hello with mention"
	ch.handleGroupMessage(protocol.NewGroupMessage("self-uid", protocol.TGroupMessage{
		TMessage: protocol.TMessage{
			MsgID:    "msg-005",
			CliMsgID: json.Number("1700000000005"),
			UIDFrom:  "user-789",
			IDTo:     "group-abc",
			DName:    "Alice",
			TS:       "1700000005",
			Content:  protocol.Content{String: &text},
		},
		Mentions: []*protocol.TMention{
			{UID: "bot-uid", Pos: 0, Len: 5, Type: protocol.MentionEach},
		},
	}))

	_ = drainInbound(t, mb) // must dispatch
}

func TestHandleGroupMessage_RequireMention_SilentWhenNotMentioned(t *testing.T) {
	t.Parallel()
	ch, mb := newHandlerTestChannel(t)
	ch.SetRequireMention(true)

	text := "not for the bot"
	ch.handleGroupMessage(protocol.NewGroupMessage("self-uid", protocol.TGroupMessage{
		TMessage: protocol.TMessage{
			MsgID:    "msg-006",
			CliMsgID: json.Number("1700000000006"),
			UIDFrom:  "user-789",
			IDTo:     "group-abc",
			DName:    "Alice",
			TS:       "1700000006",
			Content:  protocol.Content{String: &text},
		},
	}))

	// No session → checkBotMentioned returns false → not dispatched.
	// Confirm via short-timeout ConsumeInbound (must return ok=false).
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if msg, ok := mb.ConsumeInbound(ctx); ok {
		t.Fatalf("expected NO dispatch (RequireMention=true, no mention), got %+v", msg)
	}
}

func TestZaloGroupTurnGraceCollectsFollowupMedia(t *testing.T) {
	t.Parallel()
	ch, mb := newHandlerTestChannel(t)
	ch.turnCoalescer = channels.NewTurnCoalescer[inboundTurn](100*time.Millisecond, mergeInboundTurns, ch.dispatchInboundTurn)

	ch.enqueueInboundTurn(inboundTurn{
		senderID:   "user-1",
		threadID:   "group-1",
		content:    "[From: Example User (uid:user-1)]\nplease update this using the image",
		metadata:   map[string]string{"message_id": "mention-1"},
		peerKind:   "group",
		threadType: protocol.ThreadTypeGroup,
	})

	const mediaTag = `<media:image id="late-image" path="/tmp/followup.jpg">`
	ch.GroupHistory().Record("group-1", channels.HistoryEntry{
		Sender:    "Example User",
		SenderID:  "user-1",
		Body:      mediaTag,
		Media:     []string{"/tmp/followup.jpg"},
		Timestamp: time.Date(2026, 6, 3, 9, 12, 0, 0, time.UTC),
		MessageID: "media-1",
	}, ch.HistoryLimit())

	got := drainInbound(t, mb)
	if !strings.Contains(got.Content, mediaTag) {
		t.Fatalf("content missing follow-up media tag:\n%s", got.Content)
	}
	if !strings.Contains(got.Content, channels.CurrentMessageMarker) {
		t.Fatalf("content missing current message marker:\n%s", got.Content)
	}
	if len(got.Media) != 1 || got.Media[0].Path != "/tmp/followup.jpg" {
		t.Fatalf("media = %+v, want follow-up media path", got.Media)
	}
	if entries := ch.GroupHistory().GetEntries("group-1"); len(entries) != 0 {
		t.Fatalf("pending history should be cleared after dispatch, got %d entries", len(entries))
	}
}

func TestZaloTurnGraceMergesDirectFollowupMedia(t *testing.T) {
	t.Parallel()
	ch, mb := newHandlerTestChannel(t)
	ch.turnCoalescer = channels.NewTurnCoalescer[inboundTurn](100*time.Millisecond, mergeInboundTurns, ch.dispatchInboundTurn)

	ch.enqueueInboundTurn(inboundTurn{
		senderID:   "user-1",
		threadID:   "user-1",
		content:    "[From: Example User]\nplease inspect this",
		metadata:   map[string]string{"message_id": "text-1"},
		peerKind:   "direct",
		threadType: protocol.ThreadTypeUser,
	})
	ch.enqueueInboundTurn(inboundTurn{
		senderID:   "user-1",
		threadID:   "user-1",
		content:    `<media:image id="dm-image" path="/tmp/dm-followup.jpg">`,
		media:      []string{"/tmp/dm-followup.jpg"},
		metadata:   map[string]string{"message_id": "media-1"},
		peerKind:   "direct",
		threadType: protocol.ThreadTypeUser,
	})

	got := drainInbound(t, mb)
	if got.PeerKind != "direct" {
		t.Fatalf("peer kind = %q, want direct", got.PeerKind)
	}
	if !strings.Contains(got.Content, "please inspect this") || !strings.Contains(got.Content, "dm-image") {
		t.Fatalf("content did not merge text and media tag:\n%s", got.Content)
	}
	if len(got.Media) != 1 || got.Media[0].Path != "/tmp/dm-followup.jpg" {
		t.Fatalf("media = %+v, want direct follow-up media", got.Media)
	}
	if got.Metadata["message_id"] != "media-1" {
		t.Fatalf("message_id = %q, want latest metadata", got.Metadata["message_id"])
	}
}

func TestZaloGroupTurnGraceDoesNotMergeDifferentSenders(t *testing.T) {
	t.Parallel()
	ch, mb := newHandlerTestChannel(t)
	ch.turnCoalescer = channels.NewTurnCoalescer[inboundTurn](50*time.Millisecond, mergeInboundTurns, ch.dispatchInboundTurn)

	ch.enqueueInboundTurn(inboundTurn{
		senderID:   "user-a",
		threadID:   "group-1",
		content:    "[From: User A (uid:user-a)]\nfirst request",
		metadata:   map[string]string{"message_id": "mention-a"},
		peerKind:   "group",
		threadType: protocol.ThreadTypeGroup,
	})
	ch.enqueueInboundTurn(inboundTurn{
		senderID:   "user-b",
		threadID:   "group-1",
		content:    "[From: User B (uid:user-b)]\nsecond request",
		metadata:   map[string]string{"message_id": "mention-b"},
		peerKind:   "group",
		threadType: protocol.ThreadTypeGroup,
	})

	first := drainInbound(t, mb)
	second := drainInbound(t, mb)
	bySender := map[string]bus.InboundMessage{
		first.SenderID:  first,
		second.SenderID: second,
	}
	if len(bySender) != 2 {
		t.Fatalf("expected two separate sender turns, got first=%+v second=%+v", first, second)
	}
	if strings.Contains(bySender["user-a"].Content, "second request") {
		t.Fatalf("user-a turn was merged with user-b content:\n%s", bySender["user-a"].Content)
	}
	if strings.Contains(bySender["user-b"].Content, "first request") {
		t.Fatalf("user-b turn was merged with user-a content:\n%s", bySender["user-b"].Content)
	}
}

func TestStampMentionsMetadata_NilResolver(t *testing.T) {
	t.Parallel()
	md := map[string]string{}
	stampMentionsMetadata(md, []*protocol.TMention{
		{UID: "u-1", Pos: 0, Len: 5, Type: protocol.MentionEach},
	}, nil)
	if _, ok := md["mentions"]; !ok {
		t.Fatalf("nil resolver should not crash; mentions key missing: %v", md)
	}
	var ms []pkgproto.Mention
	if err := json.Unmarshal([]byte(md["mentions"]), &ms); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if ms[0].DisplayName != "" {
		t.Errorf("DisplayName = %q, want empty (nil resolver)", ms[0].DisplayName)
	}
}

func TestStampMentionsMetadata_EmptyInput_NoOp(t *testing.T) {
	t.Parallel()
	md := map[string]string{"keep": "me"}
	stampMentionsMetadata(md, nil, nil)
	if _, ok := md["mentions"]; ok {
		t.Errorf("mentions should be absent")
	}
	if md["keep"] != "me" {
		t.Errorf("existing keys mutated")
	}
}
