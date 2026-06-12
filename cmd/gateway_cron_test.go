package cmd

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/providers"
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

func TestBuildCronTargetHistoryContext_FormatsRecentMessages(t *testing.T) {
	history := []providers.Message{
		{Role: "user", Content: "[From: Alice (uid:1)] morning"},
		{Role: "assistant", Content: "chào Alice"},
		{Role: "user", Content: "[From: Bob (uid:2)] đã xong report"},
	}
	got := buildCronTargetHistoryContext(history, cronTargetHistoryDefaultLimit)
	if !strings.Contains(got, "READ-ONLY") {
		t.Fatalf("block missing READ-ONLY label: %q", got)
	}
	for _, want := range []string{"morning", "chào Alice", "đã xong report"} {
		if !strings.Contains(got, want) {
			t.Errorf("block missing %q; got %q", want, got)
		}
	}
	// Chronological order preserved (oldest first).
	if strings.Index(got, "morning") > strings.Index(got, "đã xong report") {
		t.Errorf("messages out of order: %q", got)
	}
}

func TestBuildCronTargetHistoryContext_EmptyReturnsNothing(t *testing.T) {
	if got := buildCronTargetHistoryContext(nil, cronTargetHistoryDefaultLimit); got != "" {
		t.Errorf("nil history → want empty, got %q", got)
	}
	blank := []providers.Message{{Role: "user", Content: "   "}}
	if got := buildCronTargetHistoryContext(blank, cronTargetHistoryDefaultLimit); got != "" {
		t.Errorf("blank-only history → want empty, got %q", got)
	}
}

func TestBuildCronTargetHistoryContext_LimitsToLastN(t *testing.T) {
	var history []providers.Message
	for i := 0; i < cronTargetHistoryDefaultLimit+10; i++ {
		marker := "msg-keep"
		if i < 10 {
			marker = "msg-old-dropped"
		}
		history = append(history, providers.Message{Role: "user", Content: marker})
	}
	got := buildCronTargetHistoryContext(history, cronTargetHistoryDefaultLimit)
	if strings.Contains(got, "msg-old-dropped") {
		t.Errorf("expected oldest messages beyond limit to be dropped; got %q", got)
	}
	if strings.Count(got, "msg-keep") != cronTargetHistoryDefaultLimit {
		t.Errorf("expected %d kept lines, got %d", cronTargetHistoryDefaultLimit, strings.Count(got, "msg-keep"))
	}
}

func TestBuildCronTargetHistoryContext_CustomLimitHonored(t *testing.T) {
	var history []providers.Message
	for i := 0; i < 20; i++ {
		marker := "msg-keep"
		if i < 15 {
			marker = "msg-old-dropped"
		}
		history = append(history, providers.Message{Role: "user", Content: marker})
	}
	got := buildCronTargetHistoryContext(history, 5)
	if strings.Contains(got, "msg-old-dropped") {
		t.Errorf("limit=5: expected messages beyond limit to be dropped; got %q", got)
	}
	if n := strings.Count(got, "msg-keep"); n != 5 {
		t.Errorf("limit=5: expected 5 kept lines, got %d", n)
	}
}

func TestBuildCronTargetHistoryContext_NonPositiveLimitFallsBackToDefault(t *testing.T) {
	var history []providers.Message
	for i := 0; i < cronTargetHistoryDefaultLimit+10; i++ {
		history = append(history, providers.Message{Role: "user", Content: "m"})
	}
	got := buildCronTargetHistoryContext(history, 0)
	if n := strings.Count(got, "user: m"); n != cronTargetHistoryDefaultLimit {
		t.Errorf("limit=0: expected default %d lines, got %d", cronTargetHistoryDefaultLimit, n)
	}
}

func TestBuildCronTargetHistoryContext_CharCapDropsOldest(t *testing.T) {
	// Many moderate messages (each under the per-message clamp) whose total
	// exceeds the char cap → newest kept, oldest dropped.
	body := strings.Repeat("y", 500)
	var history []providers.Message
	for i := 0; i < 20; i++ {
		tag := "oldest-"
		if i == 19 {
			tag = "newest-"
		} else if i > 0 {
			tag = fmt.Sprintf("msg%02d-", i)
		}
		history = append(history, providers.Message{Role: "user", Content: tag + body})
	}
	got := buildCronTargetHistoryContext(history, cronTargetHistoryDefaultLimit)
	if len(got) > cronTargetHistoryCharsCap+700 {
		t.Errorf("output exceeds char cap budget: %d", len(got))
	}
	if !strings.Contains(got, "newest-") {
		t.Errorf("newest message must always be kept; got len %d", len(got))
	}
	if strings.Contains(got, "oldest-") {
		t.Errorf("oldest message should be dropped by char cap; got len %d", len(got))
	}
}

func TestBuildCronTargetHistoryContext_ClampsHugeMessage(t *testing.T) {
	// A single message far larger than the per-message clamp must be truncated
	// (rune-safe) so it can't blow the context budget.
	huge := strings.Repeat("ô", 5000) // multi-byte runes — exercises rune-safe clamp
	got := buildCronTargetHistoryContext([]providers.Message{{Role: "user", Content: huge}}, cronTargetHistoryDefaultLimit)
	if !utf8.ValidString(got) {
		t.Fatalf("output is not valid UTF-8 (clamp cut mid-rune)")
	}
	if !strings.Contains(got, "…") {
		t.Errorf("huge message should be truncated with an ellipsis")
	}
	if n := utf8.RuneCountInString(got); n > cronTargetHistoryMsgRunes+200 {
		t.Errorf("clamped output too large: %d runes", n)
	}
}
