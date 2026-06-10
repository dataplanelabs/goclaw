package personal

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nextlevelbuilder/goclaw/internal/channels"
	"github.com/nextlevelbuilder/goclaw/internal/channels/zalo/personal/protocol"
)

// makeChannelWithFetcher returns a Channel with a stubbed memberFetcher and a
// real session so LookupGroupMember's slow path can fire without requiring a
// network call. fetcher returns the supplied members + err.
func makeChannelWithFetcher(t *testing.T, fetcher func(ctx context.Context, sess *protocol.Session, groupID string) ([]protocol.GroupMember, error)) *Channel {
	t.Helper()
	ch, _ := newHandlerTestChannel(t)
	ch.memberFetcher = fetcher
	// Stub session so LookupGroupMember doesn't bail at the nil-session check.
	ch.mu.Lock()
	ch.sess = protocol.NewSession()
	ch.mu.Unlock()
	return ch
}

func TestLookupGroupMember_FastPathRecentPoster(t *testing.T) {
	t.Parallel()
	ch, _ := newHandlerTestChannel(t)
	// Seed GroupHistory so groupNameResolver finds the UID.
	ch.GroupHistory().Record("group-1", channels.HistoryEntry{
		SenderID:  "u_recent",
		Sender:    "RecentPoster",
		Timestamp: time.Now(),
	}, 10)

	name, ok := ch.LookupGroupMember(context.Background(), "group-1", "u_recent")
	if !ok || name != "RecentPoster" {
		t.Fatalf("name=%q ok=%v, want RecentPoster, true", name, ok)
	}
}

func TestLookupGroupMember_CacheHit(t *testing.T) {
	t.Parallel()
	ch, _ := newHandlerTestChannel(t)
	ch.memberCache.Set("group-1", "u_cached", "Cached")

	name, ok := ch.LookupGroupMember(context.Background(), "group-1", "u_cached")
	if !ok || name != "Cached" {
		t.Fatalf("name=%q ok=%v", name, ok)
	}
}

func TestLookupGroupMember_SlowPathFetchAndCache(t *testing.T) {
	t.Parallel()
	var fetchCalls int32
	ch := makeChannelWithFetcher(t, func(ctx context.Context, sess *protocol.Session, gid string) ([]protocol.GroupMember, error) {
		atomic.AddInt32(&fetchCalls, 1)
		return []protocol.GroupMember{
			{UID: "u_remote", DisplayName: "Remote"},
			{UID: "u_other", DisplayName: "Other"},
		}, nil
	})

	name, ok := ch.LookupGroupMember(context.Background(), "group-1", "u_remote")
	if !ok || name != "Remote" {
		t.Fatalf("first lookup: name=%q ok=%v", name, ok)
	}
	if got := atomic.LoadInt32(&fetchCalls); got != 1 {
		t.Errorf("fetchCalls = %d, want 1", got)
	}

	// Second lookup for a different cached uid should NOT trigger another fetch.
	name2, ok2 := ch.LookupGroupMember(context.Background(), "group-1", "u_other")
	if !ok2 || name2 != "Other" {
		t.Fatalf("second lookup: name=%q ok=%v", name2, ok2)
	}
	if got := atomic.LoadInt32(&fetchCalls); got != 1 {
		t.Errorf("after cached lookup fetchCalls = %d, want 1", got)
	}
}

func TestLookupGroupMember_RateLimitedOnRepeatedMiss(t *testing.T) {
	t.Parallel()
	var fetchCalls int32
	ch := makeChannelWithFetcher(t, func(ctx context.Context, sess *protocol.Session, gid string) ([]protocol.GroupMember, error) {
		atomic.AddInt32(&fetchCalls, 1)
		return nil, nil // empty → never cached → next call hits limiter
	})

	// First miss triggers fetch.
	if _, ok := ch.LookupGroupMember(context.Background(), "group-1", "u_missing"); ok {
		t.Fatalf("expected miss")
	}
	// Second miss within 60s window must NOT fetch again.
	if _, ok := ch.LookupGroupMember(context.Background(), "group-1", "u_missing"); ok {
		t.Fatalf("expected miss")
	}
	if got := atomic.LoadInt32(&fetchCalls); got != 1 {
		t.Errorf("fetchCalls = %d, want 1 (rate-limited)", got)
	}
}

func TestLookupGroupMember_FetcherErrorReturnsFalse(t *testing.T) {
	t.Parallel()
	ch := makeChannelWithFetcher(t, func(ctx context.Context, sess *protocol.Session, gid string) ([]protocol.GroupMember, error) {
		return nil, errors.New("boom")
	})
	if _, ok := ch.LookupGroupMember(context.Background(), "group-1", "u_x"); ok {
		t.Fatal("expected ok=false on fetcher error")
	}
}

func TestLookupGroupMember_NilSessionReturnsFalse(t *testing.T) {
	t.Parallel()
	ch, _ := newHandlerTestChannel(t)
	// No session set — slow path must bail.
	if _, ok := ch.LookupGroupMember(context.Background(), "group-1", "u_x"); ok {
		t.Fatal("expected ok=false with no session")
	}
}

func TestMemberCache_LookupSet(t *testing.T) {
	t.Parallel()
	mc := NewMemberCache()
	if _, ok := mc.Lookup("g", "u"); ok {
		t.Fatal("empty cache should miss")
	}
	mc.Set("g", "u", "Alice")
	name, ok := mc.Lookup("g", "u")
	if !ok || name != "Alice" {
		t.Fatalf("got %q ok=%v", name, ok)
	}
}

func TestMemberCache_Set_IgnoresEmpty(t *testing.T) {
	t.Parallel()
	mc := NewMemberCache()
	mc.Set("g", "", "Alice")
	mc.Set("g", "u", "")
	if _, ok := mc.Lookup("g", ""); ok {
		t.Errorf("empty UID should not be stored")
	}
	if _, ok := mc.Lookup("g", "u"); ok {
		t.Errorf("empty display name should not be stored")
	}
}

func TestMemberFetchLimiter_Allows_AfterWindow(t *testing.T) {
	t.Parallel()
	lim := NewMemberFetchLimiter(10 * time.Millisecond)
	if !lim.Allow("g") {
		t.Fatal("first Allow should succeed")
	}
	if lim.Allow("g") {
		t.Fatal("second Allow within window should fail")
	}
	time.Sleep(15 * time.Millisecond)
	if !lim.Allow("g") {
		t.Fatal("Allow after window should succeed")
	}
}

func TestHandleGroupMessage_OpportunisticallyCachesSender(t *testing.T) {
	t.Parallel()
	ch, mb := newHandlerTestChannel(t)
	ch.SetRequireMention(false)

	text := "hi"
	ch.handleGroupMessage(protocol.NewGroupMessage("self-uid", protocol.TGroupMessage{
		TMessage: protocol.TMessage{
			MsgID: "m1", UIDFrom: "u_sender", IDTo: "group-1", DName: "Sender",
			Content: protocol.Content{String: &text},
		},
	}))
	_ = drainInbound(t, mb)

	name, ok := ch.memberCache.Lookup("group-1", "u_sender")
	if !ok || name != "Sender" {
		t.Fatalf("opportunistic cache miss: name=%q ok=%v", name, ok)
	}
}
