package protocol

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
)

// Message is the interface for incoming Zalo messages (DM or group).
type Message interface {
	Type() ThreadType
	ThreadID() string
	IsSelf() bool
}

// TMessage is the raw JSON message payload from Zalo WebSocket.
type TMessage struct {
	MsgID    string      `json:"msgId"`
	CliMsgID json.Number `json:"cliMsgId,omitempty"`
	UIDFrom  string      `json:"uidFrom"`
	IDTo     string      `json:"idTo"`
	DName    string      `json:"dName"`
	TS       string      `json:"ts"`
	Content  Content     `json:"content"`
	MsgType  string      `json:"msgType"`
	CMD      int         `json:"cmd"`
	ST       int         `json:"st"`
	AT       int         `json:"at"`
	// Held as raw JSON so a polymorphic / unexpected quote shape (e.g. attach
	// as object instead of string) never fails the parent TMessage unmarshal
	// and silently drops the entire message in handleUserMessages.
	Quote json.RawMessage `json:"quote,omitempty"`
}

// ParseQuote decodes the lazy Quote raw JSON into TQuote. Returns nil, nil if
// no quote is attached. Returns nil, err on parse failure — caller decides
// whether to drop quote metadata or fall back to a minimal stamp.
func (m *TMessage) ParseQuote() (*TQuote, error) {
	if len(m.Quote) == 0 || string(m.Quote) == "null" {
		return nil, nil
	}
	var q TQuote
	if err := json.Unmarshal(m.Quote, &q); err != nil {
		return nil, err
	}
	return &q, nil
}

// TQuote represents a quoted message attached to a TMessage (when a user replies
// to another message). Mirrors zca-js's TQuote shape. Pointer on TMessage so the
// common no-quote case costs nothing.
//
// Numeric ID fields use json.Number because Zalo serializes the same field as a
// string or number depending on endpoint version. PropertyExt is kept as raw
// JSON so the opaque server-side payload survives a marshal/unmarshal roundtrip
// for outbound /quote sends. MsgType (raw inbound wire string) is preferred
// over CliMsgType for outbound qmsgType — the int↔string roundtrip is lossy.
type TQuote struct {
	OwnerID     json.Number     `json:"ownerId"`
	CliMsgID    json.Number     `json:"cliMsgId"`
	GlobalMsgID json.Number     `json:"globalMsgId"`
	CliMsgType  int             `json:"cliMsgType"`
	MsgType     string          `json:"msgType,omitempty"`
	TS          json.Number     `json:"ts"`
	Msg         string          `json:"msg"`
	Attach      string          `json:"attach"`
	FromD       string          `json:"fromD"`
	TTL         int             `json:"ttl"`
	PropertyExt json.RawMessage `json:"propertyExt,omitempty"`
}

// GlobalMsgIDString returns the global message ID as a string for downstream
// metadata stamping. Defensive on nil receiver.
func (q *TQuote) GlobalMsgIDString() string {
	if q == nil {
		return ""
	}
	return q.GlobalMsgID.String()
}

// TGroupMessage extends TMessage with group-specific fields.
type TGroupMessage struct {
	TMessage
	Mentions []*TMention `json:"mentions,omitempty"`
}

// TMention represents an @mention in a group message.
type TMention struct {
	UID  string      `json:"uid"`  // user ID or "-1" for @all
	Pos  int         `json:"pos"`
	Len  int         `json:"len"`
	Type MentionType `json:"type"` // 0=individual, 1=all
}

// MentionType distinguishes individual vs @all mentions.
type MentionType int

const (
	MentionEach MentionType = 0
	MentionAll  MentionType = 1
	MentionAllUID           = "-1"
)

// Content is a union type: can be a plain string or an attachment object.
// String is set for text messages; Raw is set for non-text (images, stickers, files).
type Content struct {
	String *string
	Raw    json.RawMessage // non-nil when content is a JSON object (attachment)
}

func (c *Content) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		c.String = &s
		return nil
	}
	c.Raw = slices.Clone(data) // preserve raw attachment payload
	return nil
}

func (c Content) MarshalJSON() ([]byte, error) {
	if c.String != nil {
		return json.Marshal(c.String)
	}
	return []byte("null"), nil
}

// Text returns the plain text content, or empty string for non-text.
func (c Content) Text() string {
	if c.String != nil {
		return *c.String
	}
	return ""
}

// Attachment holds parsed fields from a non-text content object.
type Attachment struct {
	Title string `json:"title"`
	Href  string `json:"href"`
}

// ParseAttachment extracts attachment metadata from non-text content.
// Returns nil if content is plain text or unrecognized.
func (c Content) ParseAttachment() *Attachment {
	if c.Raw == nil {
		return nil
	}
	var att Attachment
	if json.Unmarshal(c.Raw, &att) != nil {
		return &Attachment{} // unrecognized but non-text
	}
	return &att
}

// imageExts lists file extensions recognized as images by the agent's vision pipeline.
// `.jxl` is included so Zalo HD photos (decoded to JPEG via agent.SanitizeImage)
// surface correct media-kind labels in placeholders.
var imageExts = map[string]bool{
	".jpg": true, ".jpeg": true, ".png": true, ".gif": true, ".webp": true, ".jxl": true,
}

// IsImage reports whether the attachment href points to an image file.
// Checks both file extension and Zalo CDN path patterns (e.g. /jpg/, /png/, /jxl/).
func (a *Attachment) IsImage() bool {
	if a == nil || a.Href == "" {
		return false
	}
	path := strings.SplitN(a.Href, "?", 2)[0]
	if imageExts[strings.ToLower(filepath.Ext(path))] {
		return true
	}
	// Zalo CDN paths like https://f20-zpc.zdn.vn/jpg/...
	lower := strings.ToLower(path)
	return strings.Contains(lower, "/jpg/") || strings.Contains(lower, "/png/") ||
		strings.Contains(lower, "/gif/") || strings.Contains(lower, "/webp/") ||
		strings.Contains(lower, "/jxl/")
}

// AttachmentText returns a human-readable placeholder for non-text content.
func (c Content) AttachmentText() string {
	att := c.ParseAttachment()
	if att == nil {
		return ""
	}
	if att.IsImage() {
		if att.Title != "" {
			return fmt.Sprintf("[User sent an image: %s]", att.Title)
		}
		return "[User sent an image]"
	}
	if att.Href != "" {
		if att.Title != "" {
			return fmt.Sprintf("[User sent a file: %s]", att.Title)
		}
		return "[User sent a file]"
	}
	return "[User sent a non-text message]"
}

// UserMessage represents a DM (type=0).
type UserMessage struct {
	Data     TMessage
	threadID string
	isSelf   bool
}

// NewUserMessage creates a UserMessage, resolving self-sent messages.
func NewUserMessage(selfUID string, data TMessage) UserMessage {
	msg := UserMessage{Data: data, threadID: data.UIDFrom}
	msg.isSelf = data.UIDFrom == DefaultUIDSelf

	if data.UIDFrom == DefaultUIDSelf {
		msg.threadID = data.IDTo
		msg.Data.UIDFrom = selfUID
	}
	if data.IDTo == DefaultUIDSelf {
		msg.Data.IDTo = selfUID
	}
	return msg
}

func (m UserMessage) Type() ThreadType { return ThreadTypeUser }
func (m UserMessage) ThreadID() string { return m.threadID }
func (m UserMessage) IsSelf() bool     { return m.isSelf }

// GroupMessage represents a group message (type=1).
type GroupMessage struct {
	Data     TGroupMessage
	threadID string
	isSelf   bool
}

// NewGroupMessage creates a GroupMessage, resolving self-sent messages.
func NewGroupMessage(selfUID string, data TGroupMessage) GroupMessage {
	g := GroupMessage{Data: data, threadID: data.IDTo}
	g.isSelf = data.UIDFrom == DefaultUIDSelf
	if data.UIDFrom == DefaultUIDSelf {
		g.Data.UIDFrom = selfUID
	}
	return g
}

func (m GroupMessage) Type() ThreadType { return ThreadTypeGroup }
func (m GroupMessage) ThreadID() string { return m.threadID }
func (m GroupMessage) IsSelf() bool     { return m.isSelf }
