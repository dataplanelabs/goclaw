package cmd

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/store"
)

type fakeContactStore struct {
	store.ContactStore
	result map[string]store.ChannelContact
	err    error
}

func (f *fakeContactStore) GetContactsBySenderIDs(_ context.Context, _ []string) (map[string]store.ChannelContact, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.result, nil
}

func TestResolveCronPeerKind_LegacyGroupPrefix(t *testing.T) {
	job := &store.CronJob{UserID: "group:abc", DeliverTo: "ignored"}
	got := resolveCronPeerKind(context.Background(), job, nil)
	if got != "group" {
		t.Errorf("group: prefix → want %q, got %q", "group", got)
	}
}

func TestResolveCronPeerKind_LegacyGuildPrefix(t *testing.T) {
	job := &store.CronJob{UserID: "guild:abc"}
	got := resolveCronPeerKind(context.Background(), job, nil)
	if got != "group" {
		t.Errorf("guild: prefix → want %q, got %q", "group", got)
	}
}

func TestResolveCronPeerKind_ContactGroup(t *testing.T) {
	cs := &fakeContactStore{
		result: map[string]store.ChannelContact{
			"chat-xyz": {ContactType: "group"},
		},
	}
	job := &store.CronJob{UserID: "user-1", DeliverTo: "chat-xyz", TenantID: uuid.New()}
	got := resolveCronPeerKind(context.Background(), job, cs)
	if got != "group" {
		t.Errorf("contact_type=group → want %q, got %q", "group", got)
	}
}

func TestResolveCronPeerKind_ContactUser(t *testing.T) {
	cs := &fakeContactStore{
		result: map[string]store.ChannelContact{
			"chat-xyz": {ContactType: "user"},
		},
	}
	job := &store.CronJob{UserID: "user-1", DeliverTo: "chat-xyz"}
	got := resolveCronPeerKind(context.Background(), job, cs)
	if got != "direct" {
		t.Errorf("contact_type=user → want %q, got %q", "direct", got)
	}
}

func TestResolveCronPeerKind_NoContactFound_FailsSafeDirect(t *testing.T) {
	cs := &fakeContactStore{result: map[string]store.ChannelContact{}}
	job := &store.CronJob{UserID: "user-1", DeliverTo: "unknown-chat"}
	got := resolveCronPeerKind(context.Background(), job, cs)
	if got != "direct" {
		t.Errorf("no contact → want %q (fail-safe), got %q", "direct", got)
	}
}

func TestResolveCronPeerKind_ContactStoreError_FailsSafeDirect(t *testing.T) {
	cs := &fakeContactStore{err: errors.New("db down")}
	job := &store.CronJob{UserID: "user-1", DeliverTo: "chat-xyz"}
	got := resolveCronPeerKind(context.Background(), job, cs)
	if got != "direct" {
		t.Errorf("contact-store error → want %q (fail-safe), got %q", "direct", got)
	}
}

func TestResolveCronPeerKind_NilContactStore_FailsSafeDirect(t *testing.T) {
	job := &store.CronJob{UserID: "user-1", DeliverTo: "chat-xyz"}
	got := resolveCronPeerKind(context.Background(), job, nil)
	if got != "direct" {
		t.Errorf("nil contact-store → want %q, got %q", "direct", got)
	}
}
