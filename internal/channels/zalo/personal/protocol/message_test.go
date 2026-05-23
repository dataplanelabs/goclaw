package protocol

import (
	"encoding/json"
	"testing"
)

func TestContent_UnmarshalJSON_String(t *testing.T) {
	var c Content
	if err := json.Unmarshal([]byte(`"hello world"`), &c); err != nil {
		t.Fatal(err)
	}
	if c.String == nil || *c.String != "hello world" {
		t.Errorf("expected 'hello world', got %v", c.String)
	}
	if c.Text() != "hello world" {
		t.Errorf("Text() = %q, want 'hello world'", c.Text())
	}
}

func TestContent_UnmarshalJSON_Object(t *testing.T) {
	// Non-string content (attachment) should preserve raw JSON
	var c Content
	if err := json.Unmarshal([]byte(`{"type":"sticker","id":123}`), &c); err != nil {
		t.Fatal(err)
	}
	if c.String != nil {
		t.Errorf("expected nil String for object content, got %q", *c.String)
	}
	if c.Text() != "" {
		t.Errorf("Text() = %q, want empty", c.Text())
	}
	if c.Raw == nil {
		t.Error("expected Raw to be preserved for object content")
	}
}

func TestContent_AttachmentText(t *testing.T) {
	t.Run("image with caption", func(t *testing.T) {
		var c Content
		raw := `{"title":"what do you see","href":"https://example.com/photo.jpg","thumb":"https://example.com/thumb.jpg"}`
		if err := json.Unmarshal([]byte(raw), &c); err != nil {
			t.Fatal(err)
		}
		got := c.AttachmentText()
		want := "[User sent an image: what do you see]"
		if got != want {
			t.Errorf("AttachmentText() = %q, want %q", got, want)
		}
	})

	t.Run("image without caption", func(t *testing.T) {
		var c Content
		raw := `{"title":"","href":"https://example.com/photo.jpg"}`
		if err := json.Unmarshal([]byte(raw), &c); err != nil {
			t.Fatal(err)
		}
		got := c.AttachmentText()
		if got != "[User sent an image]" {
			t.Errorf("AttachmentText() = %q, want '[User sent an image]'", got)
		}
	})

	t.Run("file with caption", func(t *testing.T) {
		var c Content
		raw := `{"title":"report.pdf","href":"https://example.com/report.pdf"}`
		if err := json.Unmarshal([]byte(raw), &c); err != nil {
			t.Fatal(err)
		}
		got := c.AttachmentText()
		want := "[User sent a file: report.pdf]"
		if got != want {
			t.Errorf("AttachmentText() = %q, want %q", got, want)
		}
	})

	t.Run("file without caption", func(t *testing.T) {
		var c Content
		raw := `{"title":"","href":"https://example.com/doc.docx"}`
		if err := json.Unmarshal([]byte(raw), &c); err != nil {
			t.Fatal(err)
		}
		got := c.AttachmentText()
		if got != "[User sent a file]" {
			t.Errorf("AttachmentText() = %q, want '[User sent a file]'", got)
		}
	})

	t.Run("non-image attachment", func(t *testing.T) {
		var c Content
		raw := `{"type":"sticker","id":123}`
		if err := json.Unmarshal([]byte(raw), &c); err != nil {
			t.Fatal(err)
		}
		got := c.AttachmentText()
		if got != "[User sent a non-text message]" {
			t.Errorf("AttachmentText() = %q, want '[User sent a non-text message]'", got)
		}
	})

	t.Run("text content returns empty", func(t *testing.T) {
		var c Content
		if err := json.Unmarshal([]byte(`"hello"`), &c); err != nil {
			t.Fatal(err)
		}
		if c.AttachmentText() != "" {
			t.Errorf("AttachmentText() should be empty for text content")
		}
	})
}

func TestContent_MarshalJSON(t *testing.T) {
	s := "test message"
	c := Content{String: &s}
	b, err := c.MarshalJSON()
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != `"test message"` {
		t.Errorf("got %s, want %q", b, `"test message"`)
	}

	// Nil content marshals as null
	c2 := Content{}
	b2, err := c2.MarshalJSON()
	if err != nil {
		t.Fatal(err)
	}
	if string(b2) != "null" {
		t.Errorf("got %s, want null", b2)
	}
}

func TestNewUserMessage(t *testing.T) {
	selfUID := "12345"
	text := "hello"

	t.Run("incoming message", func(t *testing.T) {
		msg := TMessage{
			MsgID:   "m1",
			UIDFrom: "67890",
			IDTo:    selfUID,
			Content: Content{String: &text},
		}
		um := NewUserMessage(selfUID, msg)

		if um.Type() != ThreadTypeUser {
			t.Errorf("Type = %d, want %d", um.Type(), ThreadTypeUser)
		}
		if um.ThreadID() != "67890" {
			t.Errorf("ThreadID = %q, want '67890'", um.ThreadID())
		}
		if um.IsSelf() {
			t.Error("IsSelf should be false for incoming message")
		}
	})

	t.Run("self-sent message", func(t *testing.T) {
		msg := TMessage{
			MsgID:   "m2",
			UIDFrom: DefaultUIDSelf,
			IDTo:    "67890",
			Content: Content{String: &text},
		}
		um := NewUserMessage(selfUID, msg)

		if !um.IsSelf() {
			t.Error("IsSelf should be true for self-sent")
		}
		if um.ThreadID() != "67890" {
			t.Errorf("ThreadID = %q, want '67890' (should use IDTo for self-sent)", um.ThreadID())
		}
		if um.Data.UIDFrom != selfUID {
			t.Errorf("UIDFrom = %q, want %q (should resolve '0' to selfUID)", um.Data.UIDFrom, selfUID)
		}
	})

	t.Run("IDTo is self", func(t *testing.T) {
		msg := TMessage{
			MsgID:   "m3",
			UIDFrom: "67890",
			IDTo:    DefaultUIDSelf,
		}
		um := NewUserMessage(selfUID, msg)

		if um.Data.IDTo != selfUID {
			t.Errorf("IDTo = %q, want %q (should resolve '0' to selfUID)", um.Data.IDTo, selfUID)
		}
	})
}

func TestNewGroupMessage(t *testing.T) {
	selfUID := "12345"
	text := "group msg"

	t.Run("incoming group message", func(t *testing.T) {
		msg := TGroupMessage{
			TMessage: TMessage{
				MsgID:   "gm1",
				UIDFrom: "67890",
				IDTo:    "group_abc",
				Content: Content{String: &text},
			},
		}
		gm := NewGroupMessage(selfUID, msg)

		if gm.Type() != ThreadTypeGroup {
			t.Errorf("Type = %d, want %d", gm.Type(), ThreadTypeGroup)
		}
		if gm.ThreadID() != "group_abc" {
			t.Errorf("ThreadID = %q, want 'group_abc'", gm.ThreadID())
		}
		if gm.IsSelf() {
			t.Error("IsSelf should be false")
		}
	})

	t.Run("self-sent group message", func(t *testing.T) {
		msg := TGroupMessage{
			TMessage: TMessage{
				MsgID:   "gm2",
				UIDFrom: DefaultUIDSelf,
				IDTo:    "group_abc",
			},
		}
		gm := NewGroupMessage(selfUID, msg)

		if !gm.IsSelf() {
			t.Error("IsSelf should be true for self-sent")
		}
		if gm.Data.UIDFrom != selfUID {
			t.Errorf("UIDFrom = %q, want %q", gm.Data.UIDFrom, selfUID)
		}
		if gm.ThreadID() != "group_abc" {
			t.Errorf("ThreadID = %q, want 'group_abc'", gm.ThreadID())
		}
	})
}

func TestAttachment_IsImage(t *testing.T) {
	tests := []struct {
		name string
		att  *Attachment
		want bool
	}{
		{"nil attachment", nil, false},
		{"no href", &Attachment{Title: "test"}, false},
		{"jpg", &Attachment{Href: "https://cdn.example.com/photo.jpg"}, true},
		{"png with query", &Attachment{Href: "https://cdn.example.com/img.png?w=100"}, true},
		{"webp", &Attachment{Href: "https://cdn.example.com/sticker.webp"}, true},
		{"pdf", &Attachment{Href: "https://cdn.example.com/doc.pdf"}, false},
		{"docx", &Attachment{Href: "https://cdn.example.com/report.docx"}, false},
		{"zalo cdn jpg path", &Attachment{Href: "https://f20-zpc.zdn.vn/jpg/abc123.jpg"}, true},
		{"zalo cdn path no ext", &Attachment{Href: "https://f20-zpc.zdn.vn/jpg/abc123"}, true},
		{"no extension", &Attachment{Href: "https://cdn.example.com/file"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.att.IsImage(); got != tt.want {
				t.Errorf("IsImage() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTMessage_UnmarshalJSON(t *testing.T) {
	raw := `{
		"msgId": "123",
		"uidFrom": "456",
		"idTo": "789",
		"dName": "Test User",
		"ts": "1709300000",
		"content": "hello",
		"msgType": "chat.message",
		"cmd": 501,
		"st": 1,
		"at": 0
	}`

	var msg TMessage
	if err := json.Unmarshal([]byte(raw), &msg); err != nil {
		t.Fatal(err)
	}
	if msg.MsgID != "123" {
		t.Errorf("MsgID = %q", msg.MsgID)
	}
	if msg.Content.Text() != "hello" {
		t.Errorf("Content = %q", msg.Content.Text())
	}
	if msg.CMD != 501 {
		t.Errorf("CMD = %d", msg.CMD)
	}
}

func TestTMessage_UnmarshalQuote_Text(t *testing.T) {
	raw := `{
		"msgId": "999",
		"uidFrom": "456",
		"idTo": "789",
		"dName": "Replier",
		"ts": "1709300100",
		"content": "thanks!",
		"msgType": "chat.message",
		"cmd": 501,
		"st": 1,
		"at": 0,
		"quote": {
			"ownerId": "111",
			"cliMsgId": "1709300000123",
			"globalMsgId": 9876543210,
			"cliMsgType": 1,
			"ts": "1709300000",
			"msg": "hello world",
			"attach": "",
			"fromD": "789",
			"ttl": 0,
			"propertyExt": {"color":-16777216,"size":18,"type":0,"subType":0}
		}
	}`

	var msg TMessage
	if err := json.Unmarshal([]byte(raw), &msg); err != nil {
		t.Fatal(err)
	}
	q, err := msg.ParseQuote()
	if err != nil {
		t.Fatalf("ParseQuote: %v", err)
	}
	if q == nil {
		t.Fatal("Quote should not be nil")
	}
	if q.OwnerID != "111" {
		t.Errorf("OwnerID = %q, want 111", q.OwnerID)
	}
	if q.CliMsgID.String() != "1709300000123" {
		t.Errorf("CliMsgID = %q, want 1709300000123", q.CliMsgID.String())
	}
	if q.GlobalMsgID.String() != "9876543210" {
		t.Errorf("GlobalMsgID = %q, want 9876543210", q.GlobalMsgID.String())
	}
	if q.CliMsgType != 1 {
		t.Errorf("CliMsgType = %d, want 1", q.CliMsgType)
	}
	if q.TS.String() != "1709300000" {
		t.Errorf("TS = %q, want 1709300000", q.TS.String())
	}
	if q.Msg != "hello world" {
		t.Errorf("Msg = %q, want 'hello world'", q.Msg)
	}
	if q.FromD != "789" {
		t.Errorf("FromD = %q, want 789", q.FromD)
	}
	if q.TTL != 0 {
		t.Errorf("TTL = %d, want 0", q.TTL)
	}
	if len(q.PropertyExt) == 0 {
		t.Error("PropertyExt should be preserved")
	}
	// Re-marshal PropertyExt to ensure it survived byte-equal.
	var ext map[string]any
	if err := json.Unmarshal(q.PropertyExt, &ext); err != nil {
		t.Errorf("PropertyExt is not valid JSON: %v", err)
	}
	if ext["size"].(float64) != 18 {
		t.Errorf("PropertyExt.size = %v, want 18", ext["size"])
	}
}

func TestTMessage_UnmarshalQuote_Media(t *testing.T) {
	// Media quote: msg is empty placeholder, attach carries opaque JSON string.
	raw := `{
		"msgId": "1000",
		"uidFrom": "456",
		"idTo": "789",
		"ts": "1709300200",
		"content": "see this",
		"msgType": "chat.message",
		"cmd": 501,
		"st": 1,
		"at": 0,
		"quote": {
			"ownerId": "111",
			"cliMsgId": "1709300050000",
			"globalMsgId": "9876543299",
			"cliMsgType": 2,
			"ts": "1709300050",
			"msg": "",
			"attach": "{\"hdUrl\":\"https://example.com/photo.jpg\",\"width\":1080,\"height\":720}",
			"fromD": "789",
			"ttl": 0
		}
	}`

	var msg TMessage
	if err := json.Unmarshal([]byte(raw), &msg); err != nil {
		t.Fatal(err)
	}
	q, err := msg.ParseQuote()
	if err != nil {
		t.Fatalf("ParseQuote: %v", err)
	}
	if q == nil {
		t.Fatal("Quote should not be nil")
	}
	if q.CliMsgType != 2 {
		t.Errorf("CliMsgType = %d, want 2 (image)", q.CliMsgType)
	}
	wantAttach := `{"hdUrl":"https://example.com/photo.jpg","width":1080,"height":720}`
	if q.Attach != wantAttach {
		t.Errorf("Attach = %q\nwant %q", q.Attach, wantAttach)
	}
	if q.PropertyExt != nil {
		t.Errorf("PropertyExt should be nil when absent, got %s", q.PropertyExt)
	}
	if q.GlobalMsgID.String() != "9876543299" {
		t.Errorf("GlobalMsgID = %q, want 9876543299", q.GlobalMsgID.String())
	}
}

func TestTMessage_UnmarshalQuote_TolerantToShapeVariance(t *testing.T) {
	// Regression for the silent-drop bug: when Zalo sends a quote with a
	// shape we don't anticipate (e.g. attach as object), TMessage must STILL
	// parse so the user's text reaches the agent. Quote becomes nil via
	// ParseQuote — outbound loses the quote, inbound text survives.
	raw := `{
		"msgId": "9999",
		"uidFrom": "456",
		"idTo": "789",
		"ts": "1709300300",
		"content": "the user typed this",
		"msgType": "chat.message",
		"cmd": 501,
		"st": 1,
		"at": 0,
		"quote": {
			"ownerId": "111",
			"attach": {"weird": "object instead of string"}
		}
	}`
	var msg TMessage
	if err := json.Unmarshal([]byte(raw), &msg); err != nil {
		t.Fatalf("TMessage unmarshal must NOT fail on polymorphic quote: %v", err)
	}
	if msg.Content.Text() != "the user typed this" {
		t.Errorf("user text lost: %q", msg.Content.Text())
	}
	if _, err := msg.ParseQuote(); err == nil {
		t.Error("ParseQuote should surface the schema mismatch error so callers can warn-log")
	}
}

func TestTMessage_UnmarshalNoQuote(t *testing.T) {
	raw := `{
		"msgId": "1",
		"uidFrom": "2",
		"idTo": "3",
		"ts": "1709300000",
		"content": "no quote here",
		"msgType": "chat.message",
		"cmd": 501,
		"st": 1,
		"at": 0
	}`
	var msg TMessage
	if err := json.Unmarshal([]byte(raw), &msg); err != nil {
		t.Fatal(err)
	}
	if len(msg.Quote) != 0 {
		t.Errorf("Quote should be empty for non-quoted message, got %s", msg.Quote)
	}
	q, err := msg.ParseQuote()
	if err != nil {
		t.Errorf("ParseQuote on empty: %v", err)
	}
	if q != nil {
		t.Errorf("ParseQuote should return nil for non-quoted, got %+v", q)
	}
}

func TestTQuote_GlobalMsgIDString_Nil(t *testing.T) {
	var q *TQuote
	if got := q.GlobalMsgIDString(); got != "" {
		t.Errorf("GlobalMsgIDString() on nil = %q, want empty string", got)
	}
}

func TestTGroupMessage_WithMentions(t *testing.T) {
	raw := `{
		"msgId": "gm1",
		"uidFrom": "111",
		"idTo": "group1",
		"content": "@all hello",
		"msgType": "chat.message",
		"cmd": 521,
		"st": 1,
		"at": 0,
		"mentions": [
			{"uid": "-1", "pos": 0, "len": 4, "type": 1}
		]
	}`

	var msg TGroupMessage
	if err := json.Unmarshal([]byte(raw), &msg); err != nil {
		t.Fatal(err)
	}
	if len(msg.Mentions) != 1 {
		t.Fatalf("expected 1 mention, got %d", len(msg.Mentions))
	}
	m := msg.Mentions[0]
	if m.UID != MentionAllUID {
		t.Errorf("mention UID = %q, want %q", m.UID, MentionAllUID)
	}
	if m.Type != MentionAll {
		t.Errorf("mention Type = %d, want %d", m.Type, MentionAll)
	}
}
