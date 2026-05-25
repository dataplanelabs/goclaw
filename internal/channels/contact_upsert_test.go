package channels

import (
	"context"
	"sync"
	"testing"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/cache"
	"github.com/nextlevelbuilder/goclaw/internal/store"
)

type recordingContactStore struct {
	mu      sync.Mutex
	upserts []upsertCall
}

type upsertCall struct {
	channelType, channelInstance, senderID, displayName, peerKind string
}

func (s *recordingContactStore) UpsertContact(_ context.Context, channelType, channelInstance, senderID, _, displayName, _, peerKind, _, _, _ string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.upserts = append(s.upserts, upsertCall{channelType, channelInstance, senderID, displayName, peerKind})
	return nil
}
func (s *recordingContactStore) ListContacts(context.Context, store.ContactListOpts) ([]store.ChannelContact, error) {
	return nil, nil
}
func (s *recordingContactStore) CountContacts(context.Context, store.ContactListOpts) (int, error) {
	return 0, nil
}
func (s *recordingContactStore) GetContactsBySenderIDs(context.Context, []string) (map[string]store.ChannelContact, error) {
	return nil, nil
}
func (s *recordingContactStore) GetContactByID(context.Context, uuid.UUID) (*store.ChannelContact, error) {
	return nil, nil
}
func (s *recordingContactStore) GetSenderIDsByContactIDs(context.Context, []uuid.UUID) ([]string, error) {
	return nil, nil
}
func (s *recordingContactStore) MergeContacts(context.Context, []uuid.UUID, uuid.UUID) error { return nil }
func (s *recordingContactStore) UnmergeContacts(context.Context, []uuid.UUID) error          { return nil }
func (s *recordingContactStore) GetContactsByMergedID(context.Context, uuid.UUID) ([]store.ChannelContact, error) {
	return nil, nil
}
func (s *recordingContactStore) ResolveTenantUserID(context.Context, string, string) (string, error) {
	return "", nil
}

func newTestChannelWithCollector(t *testing.T) (*BaseChannel, *recordingContactStore) {
	t.Helper()
	cs := &recordingContactStore{}
	c := NewBaseChannel("test-channel", nil, nil)
	c.SetContactCollector(store.NewContactCollector(cs, cache.NewInMemoryCache[bool]()))
	return c, cs
}

func TestUpsertSenderContactFromMetadata_DM(t *testing.T) {
	c, cs := newTestChannelWithCollector(t)
	c.upsertSenderContactFromMetadata("user-1", "direct", map[string]string{"sender_display_name": "Alice"})
	if len(cs.upserts) != 1 {
		t.Fatalf("upserts=%d want 1", len(cs.upserts))
	}
	if cs.upserts[0].displayName != "Alice" || cs.upserts[0].senderID != "user-1" {
		t.Fatalf("unexpected upsert: %+v", cs.upserts[0])
	}
}

func TestUpsertSenderContactFromMetadata_PrefersDisplayName(t *testing.T) {
	c, cs := newTestChannelWithCollector(t)
	c.upsertSenderContactFromMetadata("user-2", "direct", map[string]string{
		"display_name":        "Preferred",
		"sender_display_name": "Fallback",
	})
	if cs.upserts[0].displayName != "Preferred" {
		t.Fatalf("expected Preferred, got %q", cs.upserts[0].displayName)
	}
}

func TestUpsertSenderContactFromMetadata_SkipsGroup(t *testing.T) {
	c, cs := newTestChannelWithCollector(t)
	c.upsertSenderContactFromMetadata("member-1", "group", map[string]string{"sender_display_name": "Bob"})
	if len(cs.upserts) != 0 {
		t.Fatalf("group peerKind must not upsert; got %d", len(cs.upserts))
	}
}

func TestUpsertSenderContactFromMetadata_NoCollector(t *testing.T) {
	c := NewBaseChannel("test-channel", nil, nil)
	c.upsertSenderContactFromMetadata("user-3", "direct", map[string]string{"display_name": "X"})
}

func TestUpsertSenderContactFromMetadata_EmptySenderID(t *testing.T) {
	c, cs := newTestChannelWithCollector(t)
	c.upsertSenderContactFromMetadata("", "direct", map[string]string{"display_name": "X"})
	if len(cs.upserts) != 0 {
		t.Fatalf("empty senderID must not upsert; got %d", len(cs.upserts))
	}
}

func TestUpsertSenderContactFromMetadata_EmptyDisplayName(t *testing.T) {
	c, cs := newTestChannelWithCollector(t)
	c.upsertSenderContactFromMetadata("user-4", "direct", map[string]string{})
	if len(cs.upserts) != 1 || cs.upserts[0].displayName != "" {
		t.Fatalf("should upsert with empty name (lets DB track sender even if name unknown); got %+v", cs.upserts)
	}
}
