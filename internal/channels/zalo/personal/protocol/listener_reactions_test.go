package protocol

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

// makeFrameBody marshals the inner envelope (the JSON the reaction handler
// receives AFTER handleFrame extracts envelope.Data + decryptEventData runs
// on it with encType=0). The handler treats `data` as the decrypted payload
// directly, not as the outer WS frame envelope.
func makeFrameBody(t *testing.T, inner any) string {
	t.Helper()
	payload, err := json.Marshal(inner)
	if err != nil {
		t.Fatalf("marshal inner: %v", err)
	}
	return string(payload)
}

// drainReactions reads up to `want` events from the reaction channel within
// `timeout`. Returns whatever it collected so tests can assert counts and
// fields without flakiness.
func drainReactions(t *testing.T, ch <-chan ReactionEvent, want int, timeout time.Duration) []ReactionEvent {
	t.Helper()
	deadline := time.After(timeout)
	out := make([]ReactionEvent, 0, want)
	for len(out) < want {
		select {
		case ev := <-ch:
			out = append(out, ev)
		case <-deadline:
			return out
		}
	}
	return out
}

func newListenerForReactions(t *testing.T) *Listener {
	t.Helper()
	return &Listener{
		sess:       &Session{UID: "self-uid"},
		reactionCh: make(chan ReactionEvent, msgBufferSize),
		errorCh:    make(chan error, 4),
	}
}

func TestHandleReactionEvents_DMOnly(t *testing.T) {
	t.Parallel()
	ln := newListenerForReactions(t)
	inner := `{"rMsg":[{"gMsgID":100,"cMsgID":200,"msgType":1}],"rIcon":"/-heart","rType":5,"source":6}`
	envelope := map[string]any{
		"data": map[string]any{
			"reacts": []map[string]any{
				{
					"msgId": "100", "cliMsgId": "200",
					"uidFrom": "user-1", "idTo": "self-uid",
					"content": inner, "ts": "1700000000",
				},
			},
		},
	}
	ctx := context.Background()
	body := makeFrameBody(t, envelope)
	ln.handleReactionEvents(ctx, body, 0)
	events := drainReactions(t, ln.reactionCh, 1, 500*time.Millisecond)
	if len(events) != 1 {
		t.Fatalf("got %d events, want 1", len(events))
	}
	if events[0].Code != ReactionHeart || events[0].ThreadID != "user-1" {
		t.Errorf("event=%+v", events[0])
	}
	if events[0].ThreadType != ThreadTypeUser {
		t.Errorf("threadType=%v, want user", events[0].ThreadType)
	}
}

func TestHandleReactionEvents_GroupOnly(t *testing.T) {
	t.Parallel()
	ln := newListenerForReactions(t)
	inner := `{"rMsg":[{"gMsgID":100,"cMsgID":200,"msgType":1}],"rIcon":"/-strong","rType":3,"source":6}`
	envelope := map[string]any{
		"data": map[string]any{
			"reactGroups": []map[string]any{
				{
					"msgId": "100", "cliMsgId": "200",
					"uidFrom": "user-2", "idTo": "group-7",
					"content": inner, "ts": "1700000001",
				},
			},
		},
	}
	body := makeFrameBody(t, envelope)
	ln.handleReactionEvents(context.Background(), body, 0)
	events := drainReactions(t, ln.reactionCh, 1, 500*time.Millisecond)
	if len(events) != 1 {
		t.Fatalf("got %d events, want 1", len(events))
	}
	if events[0].ThreadID != "group-7" || events[0].ThreadType != ThreadTypeGroup {
		t.Errorf("event=%+v", events[0])
	}
}

func TestHandleReactionEvents_SelfReactionFlag(t *testing.T) {
	t.Parallel()
	ln := newListenerForReactions(t)
	inner := `{"rMsg":[{"gMsgID":1,"cMsgID":2,"msgType":1}],"rIcon":"/-heart","rType":5,"source":6}`
	envelope := map[string]any{
		"data": map[string]any{
			"reacts": []map[string]any{
				{
					"msgId": "1", "cliMsgId": "2",
					"uidFrom": "self-uid", "idTo": "user-x",
					"content": inner, "ts": "1700000000",
				},
			},
		},
	}
	ln.handleReactionEvents(context.Background(), makeFrameBody(t, envelope), 0)
	events := drainReactions(t, ln.reactionCh, 1, 500*time.Millisecond)
	if len(events) != 1 || !events[0].IsSelf {
		t.Errorf("expected IsSelf=true, got %+v", events)
	}
	// uidFrom=="self-uid" (not "0") on a DM → threadID resolves to uidFrom.
	// Only uidFrom=="0" OR group threads route to idTo.
	if events[0].ThreadID != "self-uid" {
		t.Errorf("threadID=%s, want self-uid (uidFrom path)", events[0].ThreadID)
	}
}

func TestHandleReactionEvents_DefaultUIDSelfRoutesToIDTo(t *testing.T) {
	t.Parallel()
	// uidFrom=="0" (DefaultUIDSelf) routes the threadID to idTo (the chat
	// partner) rather than uidFrom.
	ln := newListenerForReactions(t)
	inner := `{"rMsg":[{"gMsgID":1,"cMsgID":2,"msgType":1}],"rIcon":"/-heart","rType":5,"source":6}`
	envelope := map[string]any{
		"data": map[string]any{
			"reacts": []map[string]any{
				{
					"msgId": "1", "cliMsgId": "2",
					"uidFrom": "0", "idTo": "chat-partner",
					"content": inner, "ts": "1700000000",
				},
			},
		},
	}
	ln.handleReactionEvents(context.Background(), makeFrameBody(t, envelope), 0)
	events := drainReactions(t, ln.reactionCh, 1, 500*time.Millisecond)
	if len(events) != 1 || events[0].ThreadID != "chat-partner" {
		t.Errorf("threadID=%v, want chat-partner; events=%+v", events[0].ThreadID, events)
	}
	if !events[0].IsSelf {
		t.Errorf("uidFrom==DefaultUIDSelf must set IsSelf=true")
	}
}

func TestHandleOldReactions_GroupThread(t *testing.T) {
	t.Parallel()
	ln := newListenerForReactions(t)
	inner := `{"rMsg":[{"gMsgID":1,"cMsgID":2,"msgType":1}],"rIcon":"/-strong","rType":3,"source":6}`
	envelope := map[string]any{
		"data": map[string]any{
			"reactGroups": []map[string]any{
				{
					"msgId": "1", "cliMsgId": "2",
					"uidFrom": "user-x", "idTo": "group-z",
					"content": inner, "ts": "1700000000",
				},
			},
		},
	}
	ln.handleOldReactions(context.Background(), makeFrameBody(t, envelope), 0, true)
	events := drainReactions(t, ln.reactionCh, 1, 500*time.Millisecond)
	if len(events) != 1 {
		t.Fatalf("got %d events, want 1", len(events))
	}
	if !events[0].IsHistoric {
		t.Errorf("expected IsHistoric=true for cmd=611")
	}
	if events[0].ThreadID != "group-z" {
		t.Errorf("threadID=%s, want group-z", events[0].ThreadID)
	}
}

func TestHandleOldReactions_UserThread(t *testing.T) {
	t.Parallel()
	ln := newListenerForReactions(t)
	inner := `{"rMsg":[{"gMsgID":1,"cMsgID":2,"msgType":1}],"rIcon":"/-heart","rType":5,"source":6}`
	envelope := map[string]any{
		"data": map[string]any{
			"reacts": []map[string]any{
				{
					"msgId": "1", "cliMsgId": "2",
					"uidFrom": "user-x", "idTo": "self-uid",
					"content": inner, "ts": "1700000000",
				},
			},
		},
	}
	ln.handleOldReactions(context.Background(), makeFrameBody(t, envelope), 0, false)
	events := drainReactions(t, ln.reactionCh, 1, 500*time.Millisecond)
	if len(events) != 1 {
		t.Fatalf("got %d events, want 1", len(events))
	}
	if !events[0].IsHistoric || events[0].ThreadType != ThreadTypeUser {
		t.Errorf("event mismatch: %+v", events[0])
	}
}

func TestHandleReactionEvents_MalformedInnerContent(t *testing.T) {
	t.Parallel()
	ln := newListenerForReactions(t)
	envelope := map[string]any{
		"data": map[string]any{
			"reacts": []map[string]any{
				{
					"msgId": "1", "cliMsgId": "2",
					"uidFrom": "u", "idTo": "self-uid",
					"content": "not-valid-json", "ts": "1",
				},
			},
		},
	}
	ln.handleReactionEvents(context.Background(), makeFrameBody(t, envelope), 0)
	events := drainReactions(t, ln.reactionCh, 1, 200*time.Millisecond)
	if len(events) != 0 {
		t.Errorf("malformed inner should be dropped silently, got %+v", events)
	}
}

func TestDecodeReaction_PrefersOuterIDs(t *testing.T) {
	t.Parallel()
	r := TReaction{
		MsgID:    "100",
		CliMsgID: "200",
		UIDFrom:  "u",
		IDTo:     "g",
		Content:  `{"rMsg":[{"gMsgID":999,"cMsgID":888,"msgType":1}],"rIcon":"/-heart","rType":5,"source":6}`,
	}
	ev, ok := decodeReaction("self", r, ThreadTypeUser, false)
	if !ok {
		t.Fatalf("decode failed")
	}
	if ev.MsgID != "100" || ev.CliMsgID != "200" {
		t.Errorf("outer IDs should win, got msgID=%s cliMsgID=%s", ev.MsgID, ev.CliMsgID)
	}
}

func TestDecodeReaction_FallsBackToInnerIDs(t *testing.T) {
	t.Parallel()
	r := TReaction{
		MsgID:    "0",
		CliMsgID: "0",
		UIDFrom:  "u",
		IDTo:     "g",
		Content:  `{"rMsg":[{"gMsgID":999,"cMsgID":888,"msgType":1}],"rIcon":"/-heart","rType":5,"source":6}`,
	}
	ev, ok := decodeReaction("self", r, ThreadTypeUser, false)
	if !ok {
		t.Fatalf("decode failed")
	}
	if ev.MsgID != "999" || ev.CliMsgID != "888" {
		t.Errorf("inner IDs should win when outer is 0, got msgID=%s cliMsgID=%s", ev.MsgID, ev.CliMsgID)
	}
}
