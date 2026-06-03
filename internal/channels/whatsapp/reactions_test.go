package whatsapp

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.mau.fi/whatsmeow/proto/waCommon"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"

	"github.com/nextlevelbuilder/goclaw/internal/channels"
	"github.com/nextlevelbuilder/goclaw/internal/config"
	"github.com/nextlevelbuilder/goclaw/internal/store"
)

func TestExtractWhatsAppReaction(t *testing.T) {
	t.Parallel()
	reactionAt := time.Date(2026, 6, 3, 9, 53, 0, 0, time.UTC)
	evt := whatsappReactionEvent("react-msg-1", "Alice", reactionAt)
	reaction := whatsappReactionMessage("target-1", "chat-1@g.us", "user-a@s.whatsapp.net", true, "❤️", reactionAt)

	got, ok := extractWhatsAppReaction(evt, reaction, "user-a@s.whatsapp.net", "chat-1@g.us", "group")
	if !ok {
		t.Fatal("extractWhatsAppReaction returned false")
	}
	if got.TargetMsgID != "target-1" {
		t.Errorf("target msg id=%q, want target-1", got.TargetMsgID)
	}
	if got.TargetParticipant != "user-a@s.whatsapp.net" {
		t.Errorf("target participant=%q", got.TargetParticipant)
	}
	if !got.TargetFromMe {
		t.Error("target from_me should be true")
	}
	if got.ReactorName != "Alice" {
		t.Errorf("reactor name=%q, want Alice", got.ReactorName)
	}
	if got.Emoji != "❤️" || got.Removed {
		t.Errorf("emoji/removed=%q/%v", got.Emoji, got.Removed)
	}
	if !got.Timestamp.Equal(reactionAt) {
		t.Errorf("timestamp=%v, want %v", got.Timestamp, reactionAt)
	}
	if got.Sentiment != "positive" {
		t.Errorf("sentiment=%q, want positive", got.Sentiment)
	}
}

func TestExtractWhatsAppReactionRemoved(t *testing.T) {
	t.Parallel()
	reactionAt := time.Date(2026, 6, 3, 9, 54, 0, 0, time.UTC)
	evt := whatsappReactionEvent("react-msg-2", "", reactionAt)
	reaction := whatsappReactionMessage("target-2", "chat-1@g.us", "", false, "", reactionAt)

	got, ok := extractWhatsAppReaction(evt, reaction, "user-b@s.whatsapp.net", "chat-1@g.us", "group")
	if !ok {
		t.Fatal("extractWhatsAppReaction returned false")
	}
	if !got.Removed {
		t.Fatal("removed reaction should be marked removed")
	}
	if got.ReactorName != "user-b@s.whatsapp.net" {
		t.Errorf("empty push name should fall back to sender id, got %q", got.ReactorName)
	}
	if got.Sentiment != "unknown" {
		t.Errorf("sentiment=%q, want unknown", got.Sentiment)
	}
}

func TestHandleReactionMessagePersistsFeedback(t *testing.T) {
	t.Parallel()
	reactionAt := time.Date(2026, 6, 3, 9, 53, 0, 0, time.UTC)
	fake := &fakeWhatsAppEpisodicStore{
		previews: map[string]string{
			whatsappMessageSourceID("target-file"): "example_inventory.xlsx",
		},
	}
	ch := &Channel{
		BaseChannel:   channels.NewBaseChannel(channels.TypeWhatsApp, nil, nil),
		config:        config.WhatsAppConfig{},
		episodicStore: fake,
	}
	ch.SetAgentID("agent-a")
	ch.SetAgentUUID(uuid.New())

	evt := whatsappReactionEvent("react-msg-3", "Nguyen Van A", reactionAt)
	evt.Message = &waE2E.Message{
		ReactionMessage: whatsappReactionMessage(
			"target-file",
			"group-1@g.us",
			"",
			true,
			"👍",
			reactionAt,
		),
	}
	handled := ch.handleReactionMessage(context.Background(), evt, "user-a@s.whatsapp.net", "group-1@g.us", "group")
	if !handled {
		t.Fatal("reaction event was not handled")
	}

	if len(fake.created) != 1 {
		t.Fatalf("created=%d, want 1", len(fake.created))
	}
	ep := fake.created[0]
	if ep.SourceType != "reaction_feedback" {
		t.Errorf("source_type=%q, want reaction_feedback", ep.SourceType)
	}
	if ep.UserID != "group:whatsapp:group-1@g.us" {
		t.Errorf("user_id=%q", ep.UserID)
	}
	if !ep.CreatedAt.Equal(reactionAt) {
		t.Errorf("created_at=%v, want reaction timestamp %v", ep.CreatedAt, reactionAt)
	}
	for _, want := range []string{"Nguyen Van A", "👍", "positive", "example_inventory.xlsx", "target-file"} {
		if !strings.Contains(ep.Summary, want) {
			t.Errorf("summary missing %q: %s", want, ep.Summary)
		}
	}
	if !strings.HasPrefix(ep.SourceID, "react:target-file:user-a@s.whatsapp.net:react-msg-3") {
		t.Errorf("source_id=%q", ep.SourceID)
	}
}

func whatsappReactionEvent(id, pushName string, at time.Time) *events.Message {
	return &events.Message{
		Info: types.MessageInfo{
			ID:        types.MessageID(id),
			PushName:  pushName,
			Timestamp: at,
		},
	}
}

func whatsappReactionMessage(targetID, remoteJID, participant string, fromMe bool, emoji string, at time.Time) *waE2E.ReactionMessage {
	return &waE2E.ReactionMessage{
		Key: &waCommon.MessageKey{
			ID:          new(targetID),
			RemoteJID:   new(remoteJID),
			Participant: new(participant),
			FromMe:      new(fromMe),
		},
		Text:              new(emoji),
		SenderTimestampMS: new(at.UnixMilli()),
	}
}

type fakeWhatsAppEpisodicStore struct {
	store.EpisodicStore
	previews map[string]string
	created  []*store.EpisodicSummary
	mu       sync.Mutex
}

func (f *fakeWhatsAppEpisodicStore) GetBySourceID(_ context.Context, _, _, sourceID string) (*store.EpisodicSummary, error) {
	if preview := f.previews[sourceID]; preview != "" {
		return &store.EpisodicSummary{Summary: preview}, nil
	}
	return nil, nil
}

func (f *fakeWhatsAppEpisodicStore) Create(_ context.Context, ep *store.EpisodicSummary) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.created = append(f.created, ep)
	return nil
}
