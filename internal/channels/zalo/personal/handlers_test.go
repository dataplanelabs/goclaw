package personal

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/nextlevelbuilder/goclaw/internal/bus"
	"github.com/nextlevelbuilder/goclaw/internal/channels"
	"github.com/nextlevelbuilder/goclaw/internal/channels/zalo/personal/protocol"
	"github.com/nextlevelbuilder/goclaw/internal/config"
)

// _ assertion: zalo_personal Channel must implement DMQuoteChannel so the
// gateway helper stamps reply_to_message_id on outbound DM metadata.
var _ channels.DMQuoteChannel = (*Channel)(nil)

func TestChannel_QuoteInboundOnDM_True(t *testing.T) {
	t.Parallel()
	c := &Channel{}
	if !c.QuoteInboundOnDM() {
		t.Fatal("zalo_personal must opt into DMQuoteChannel (native quote bubble support)")
	}
}

func TestBuildQuoteMetadata_NilReturnsNil(t *testing.T) {
	t.Parallel()
	if got := buildQuoteMetadata(nil); got != nil {
		t.Errorf("buildQuoteMetadata(nil) = %v, want nil", got)
	}
}

// TestBuildQuoteMetadata_InvalidPropertyExtReturnsNil: when PropertyExt
// contains invalid JSON, json.Marshal fails — the helper must return nil
// (no half-stamp) so the outbound reply gracefully degrades to a plain send
// rather than carrying a stale reply_to_message_id with no payload.
func TestBuildQuoteMetadata_InvalidPropertyExtReturnsNil(t *testing.T) {
	t.Parallel()
	bad := &protocol.TQuote{
		OwnerID:     "111",
		GlobalMsgID: json.Number("9876543210"),
		PropertyExt: json.RawMessage(`not-valid-json{`),
	}
	if got := buildQuoteMetadata(bad); got != nil {
		t.Errorf("buildQuoteMetadata with invalid PropertyExt should return nil, got %v", got)
	}
}

func TestBuildQuoteMetadata_RoundTrip(t *testing.T) {
	t.Parallel()
	propertyExt := json.RawMessage(`{"color":-16777216,"size":18}`)
	original := &protocol.TQuote{
		OwnerID:     "111",
		CliMsgID:    json.Number("1709300000123"),
		GlobalMsgID: json.Number("9876543210"),
		CliMsgType:  1,
		TS:          json.Number("1709300000"),
		Msg:         "hello world",
		Attach:      `{"hdUrl":"x"}`,
		FromD:       "789",
		TTL:         0,
		PropertyExt: propertyExt,
	}

	meta := buildQuoteMetadata(original)
	if meta["reply_to_message_id"] != "9876543210" {
		t.Errorf("reply_to_message_id = %q, want 9876543210", meta["reply_to_message_id"])
	}
	if meta["reply_to_quote_payload"] == "" {
		t.Fatal("reply_to_quote_payload missing")
	}

	// Roundtrip: unmarshal the payload back into a TQuote, then check fields
	// match (including byte-equal PropertyExt).
	var back protocol.TQuote
	if err := json.Unmarshal([]byte(meta["reply_to_quote_payload"]), &back); err != nil {
		t.Fatalf("payload not valid JSON: %v", err)
	}
	if back.OwnerID != original.OwnerID || back.Msg != original.Msg ||
		back.GlobalMsgID.String() != original.GlobalMsgID.String() ||
		back.CliMsgID.String() != original.CliMsgID.String() ||
		back.Attach != original.Attach || back.FromD != original.FromD {
		t.Errorf("roundtrip mismatch:\n got=%+v\nwant=%+v", back, original)
	}
	// PropertyExt is RawMessage — re-encode to canonical form to compare without
	// being fooled by whitespace differences (none expected here but safe).
	var origExt, backExt any
	_ = json.Unmarshal(original.PropertyExt, &origExt)
	_ = json.Unmarshal(back.PropertyExt, &backExt)
	origCanon, _ := json.Marshal(origExt)
	backCanon, _ := json.Marshal(backExt)
	if string(origCanon) != string(backCanon) {
		t.Errorf("PropertyExt roundtrip mismatch:\n got=%s\nwant=%s", backCanon, origCanon)
	}
}

// newHandlerTestChannel builds a Channel suitable for invoking handleDM/
// handleGroupMessage without an authenticated Zalo session. The typing
// indicator path runs in a goroutine and silently logs when no session is
// present — acceptable for handler-level metadata tests.
func newHandlerTestChannel(t *testing.T) (*Channel, *bus.MessageBus) {
	t.Helper()
	mb := bus.New()
	ch, err := New(config.ZaloPersonalConfig{
		Enabled:     true,
		DMPolicy:    "open",
		GroupPolicy: "open",
	}, mb, nil, nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return ch, mb
}

// drainInbound waits up to 1s for a message to land on the bus.
func drainInbound(t *testing.T, mb *bus.MessageBus) bus.InboundMessage {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	msg, ok := mb.ConsumeInbound(ctx)
	if !ok {
		t.Fatal("expected inbound message on bus, got none")
	}
	return msg
}

func TestHandleDM_StampsQuoteMetadata(t *testing.T) {
	t.Parallel()
	ch, mb := newHandlerTestChannel(t)
	text := "thanks!"
	quote := &protocol.TQuote{
		OwnerID:     "111",
		GlobalMsgID: json.Number("9876543210"),
		CliMsgID:    json.Number("1709300000"),
		Msg:         "hello there",
		FromD:       "789",
	}

	ch.handleDM(protocol.NewUserMessage("self-uid", protocol.TMessage{
		MsgID:   "current-msg",
		UIDFrom: "456",
		IDTo:    "self-uid",
		DName:   "Replier",
		Content: protocol.Content{String: &text},
		Quote:   quote,
	}))

	got := drainInbound(t, mb)
	if got.Metadata["reply_to_message_id"] != "9876543210" {
		t.Errorf("reply_to_message_id = %q, want 9876543210", got.Metadata["reply_to_message_id"])
	}
	if got.Metadata["reply_to_quote_payload"] == "" {
		t.Error("reply_to_quote_payload should be stamped")
	}
	// Sanity: existing keys still present.
	if got.Metadata["message_id"] != "current-msg" {
		t.Errorf("message_id = %q, want current-msg", got.Metadata["message_id"])
	}
}

func TestHandleDM_NoQuoteStampsNothing(t *testing.T) {
	t.Parallel()
	ch, mb := newHandlerTestChannel(t)
	text := "plain hi"

	ch.handleDM(protocol.NewUserMessage("self-uid", protocol.TMessage{
		MsgID:   "plain-msg",
		UIDFrom: "456",
		IDTo:    "self-uid",
		Content: protocol.Content{String: &text},
		// Quote: nil
	}))

	got := drainInbound(t, mb)
	if _, ok := got.Metadata["reply_to_message_id"]; ok {
		t.Errorf("reply_to_message_id must not be stamped without Quote, got %v", got.Metadata)
	}
	if _, ok := got.Metadata["reply_to_quote_payload"]; ok {
		t.Errorf("reply_to_quote_payload must not be stamped without Quote, got %v", got.Metadata)
	}
}

func TestHandleGroupMessage_StampsQuoteMetadata(t *testing.T) {
	t.Parallel()
	ch, mb := newHandlerTestChannel(t)
	// Group policy = open; require_mention defaults to true so we must mention
	// the bot for the message to reach the agent. Disable mention gating to
	// keep the test focused on quote-metadata propagation.
	ch.SetRequireMention(false)

	text := "group reply"
	quote := &protocol.TQuote{
		OwnerID:     "111",
		GlobalMsgID: json.Number("9876543299"),
		Msg:         "original group msg",
	}

	ch.handleGroupMessage(protocol.NewGroupMessage("self-uid", protocol.TGroupMessage{
		TMessage: protocol.TMessage{
			MsgID:   "g-current",
			UIDFrom: "789",
			IDTo:    "group-abc",
			DName:   "GroupReplier",
			Content: protocol.Content{String: &text},
			Quote:   quote,
		},
	}))

	got := drainInbound(t, mb)
	if got.Metadata["reply_to_message_id"] != "9876543299" {
		t.Errorf("group reply_to_message_id = %q, want 9876543299", got.Metadata["reply_to_message_id"])
	}
	if got.Metadata["reply_to_quote_payload"] == "" {
		t.Error("group reply_to_quote_payload should be stamped")
	}
	if got.Metadata["group_id"] != "group-abc" {
		t.Errorf("group_id = %q, want group-abc", got.Metadata["group_id"])
	}
}
