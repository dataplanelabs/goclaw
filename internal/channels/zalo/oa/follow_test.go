package oa

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/cache"
	"github.com/nextlevelbuilder/goclaw/internal/channels"
	"github.com/nextlevelbuilder/goclaw/internal/eventbus"
	"github.com/nextlevelbuilder/goclaw/internal/store"
)

type stubFollowContactStore struct {
	mu      sync.Mutex
	upserts []stubFollowUpsert
}

type stubFollowUpsert struct {
	senderID, displayName, peerKind, contactType string
}

func (s *stubFollowContactStore) UpsertContact(_ context.Context, _, _, senderID, _, displayName, _, peerKind, contactType, _, _ string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.upserts = append(s.upserts, stubFollowUpsert{senderID, displayName, peerKind, contactType})
	return nil
}
func (s *stubFollowContactStore) ListContacts(context.Context, store.ContactListOpts) ([]store.ChannelContact, error) {
	return nil, nil
}
func (s *stubFollowContactStore) CountContacts(context.Context, store.ContactListOpts) (int, error) {
	return 0, nil
}
func (s *stubFollowContactStore) GetContactsBySenderIDs(context.Context, []string) (map[string]store.ChannelContact, error) {
	return nil, nil
}
func (s *stubFollowContactStore) GetContactByID(context.Context, uuid.UUID) (*store.ChannelContact, error) {
	return nil, nil
}
func (s *stubFollowContactStore) GetSenderIDsByContactIDs(context.Context, []uuid.UUID) ([]string, error) {
	return nil, nil
}
func (s *stubFollowContactStore) MergeContacts(context.Context, []uuid.UUID, uuid.UUID) error {
	return nil
}
func (s *stubFollowContactStore) UnmergeContacts(context.Context, []uuid.UUID) error { return nil }
func (s *stubFollowContactStore) GetContactsByMergedID(context.Context, uuid.UUID) ([]store.ChannelContact, error) {
	return nil, nil
}
func (s *stubFollowContactStore) ResolveTenantUserID(context.Context, string, string) (string, error) {
	return "", nil
}

type stubFollowBus struct {
	mu        sync.Mutex
	published []eventbus.DomainEvent
}

func (b *stubFollowBus) Publish(e eventbus.DomainEvent) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.published = append(b.published, e)
}
func (b *stubFollowBus) Subscribe(eventbus.EventType, eventbus.DomainEventHandler) func() {
	return func() {}
}
func (b *stubFollowBus) Close()                       {}
func (b *stubFollowBus) Start(context.Context)        {}
func (b *stubFollowBus) Drain(time.Duration) error    { return nil }

func newFollowTestChannel(t *testing.T) (*Channel, *stubFollowContactStore, *stubFollowBus) {
	t.Helper()
	cs := &stubFollowContactStore{}
	bus := &stubFollowBus{}
	c := &Channel{
		BaseChannel:  channels.NewBaseChannel("zalo-oa-test", nil, nil),
		instanceID:   uuid.New(),
		teamReplyBus: bus,
	}
	c.SetContactCollector(store.NewContactCollector(cs, cache.NewInMemoryCache[bool]()))
	c.SetTenantID(uuid.New())
	return c, cs, bus
}

func TestHandleUserFollow_UpsertsContactAndPublishes(t *testing.T) {
	c, cs, bus := newFollowTestChannel(t)
	ev := &oaInboundEvent{}
	ev.Sender.ID = "user-123"
	ev.Sender.DisplayName = "Alice Nguyen"
	c.handleUserFollow(ev)
	if len(cs.upserts) != 1 {
		t.Fatalf("upserts=%d want 1", len(cs.upserts))
	}
	if cs.upserts[0].peerKind != "direct" || cs.upserts[0].contactType != "follower" {
		t.Fatalf("unexpected upsert: %+v", cs.upserts[0])
	}
	if cs.upserts[0].displayName != "Alice Nguyen" {
		t.Fatalf("displayName=%q", cs.upserts[0].displayName)
	}
	if len(bus.published) != 1 || bus.published[0].Type != eventbus.EventContactFollowed {
		t.Fatalf("expected 1 EventContactFollowed; got %d events", len(bus.published))
	}
	payload, ok := bus.published[0].Payload.(eventbus.ContactLifecyclePayload)
	if !ok || payload.SenderID != "user-123" {
		t.Fatalf("payload wrong: %+v", bus.published[0].Payload)
	}
}

func TestHandleUserUnfollow_PublishesEventNoUpsert(t *testing.T) {
	c, cs, bus := newFollowTestChannel(t)
	ev := &oaInboundEvent{}
	ev.Sender.ID = "user-456"
	c.handleUserUnfollow(ev)
	if len(cs.upserts) != 0 {
		t.Fatalf("unfollow must not upsert; got %d", len(cs.upserts))
	}
	if len(bus.published) != 1 || bus.published[0].Type != eventbus.EventContactUnfollowed {
		t.Fatalf("expected 1 EventContactUnfollowed; got %d events", len(bus.published))
	}
}

func TestHandleUserFollow_EmptySenderID_Noop(t *testing.T) {
	c, cs, bus := newFollowTestChannel(t)
	ev := &oaInboundEvent{}
	c.handleUserFollow(ev)
	if len(cs.upserts) != 0 || len(bus.published) != 0 {
		t.Fatalf("empty sender must be noop; upserts=%d events=%d", len(cs.upserts), len(bus.published))
	}
}

func TestHandleUserFollow_NoBus_StillUpserts(t *testing.T) {
	cs := &stubFollowContactStore{}
	c := &Channel{
		BaseChannel: channels.NewBaseChannel("zalo-oa-test", nil, nil),
		instanceID:  uuid.New(),
	}
	c.SetContactCollector(store.NewContactCollector(cs, cache.NewInMemoryCache[bool]()))
	c.SetTenantID(uuid.New())
	ev := &oaInboundEvent{}
	ev.Sender.ID = "user-789"
	ev.Sender.DisplayName = "Bob"
	c.handleUserFollow(ev)
	if len(cs.upserts) != 1 {
		t.Fatalf("upsert should still happen even without bus; got %d", len(cs.upserts))
	}
}
