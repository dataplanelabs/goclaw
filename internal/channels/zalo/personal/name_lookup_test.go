package personal

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/nextlevelbuilder/goclaw/internal/channels/zalo/personal/protocol"
)

func TestNormalizeName(t *testing.T) {
	t.Parallel()
	cases := []struct{ in, want string }{
		{"Alice", "alice"},
		{"  Đức  ", "đức"},
		{"NGỌC TRÂN", "ngọc trân"},
		{"", ""},
		{"\t Bob \n", "bob"},
	}
	for _, tc := range cases {
		if got := normalizeName(tc.in); got != tc.want {
			t.Errorf("normalizeName(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestMemberCache_FindByName_UniqueMatch(t *testing.T) {
	t.Parallel()
	mc := NewMemberCache()
	mc.Set("g", "u_a", "Alice")
	mc.Set("g", "u_b", "Đức")
	uid, dn, ok := mc.FindByName("g", normalizeName("Đức"))
	if !ok || uid != "u_b" || dn != "Đức" {
		t.Fatalf("got uid=%q dn=%q ok=%v", uid, dn, ok)
	}
}

func TestMemberCache_FindByName_RefuseAmbiguous(t *testing.T) {
	t.Parallel()
	mc := NewMemberCache()
	mc.Set("g", "u_1", "Hà Lan")
	mc.Set("g", "u_2", "hà lan") // different UID, same normalized name
	_, _, ok := mc.FindByName("g", normalizeName("Hà Lan"))
	if ok {
		t.Fatal("expected ambiguity refusal")
	}
}

func TestMemberCache_FindByName_Miss(t *testing.T) {
	t.Parallel()
	mc := NewMemberCache()
	mc.Set("g", "u_a", "Alice")
	if _, _, ok := mc.FindByName("g", normalizeName("Bob")); ok {
		t.Fatal("expected miss")
	}
}

func TestLookupGroupMemberByName_CacheHit(t *testing.T) {
	t.Parallel()
	ch, _ := newHandlerTestChannel(t)
	ch.memberCache.Set("g1", "u_x", "Ngoc Tran")

	uid, dn, ok := ch.LookupGroupMemberByName(context.Background(), "g1", "ngoc tran")
	if !ok || uid != "u_x" || dn != "Ngoc Tran" {
		t.Fatalf("got uid=%q dn=%q ok=%v", uid, dn, ok)
	}
}

func TestLookupGroupMemberByName_SlowPathFetchOnMiss(t *testing.T) {
	t.Parallel()
	var fetchCalls int32
	ch := makeChannelWithFetcher(t, func(ctx context.Context, sess *protocol.Session, gid string) ([]protocol.GroupMember, error) {
		atomic.AddInt32(&fetchCalls, 1)
		return []protocol.GroupMember{
			{UID: "u_remote", DisplayName: "Đức"},
		}, nil
	})

	uid, dn, ok := ch.LookupGroupMemberByName(context.Background(), "g1", "Đức")
	if !ok || uid != "u_remote" || dn != "Đức" {
		t.Fatalf("got uid=%q dn=%q ok=%v", uid, dn, ok)
	}
	if atomic.LoadInt32(&fetchCalls) != 1 {
		t.Errorf("expected exactly 1 fetch, got %d", fetchCalls)
	}
}

func TestLookupGroupMemberByName_EmptyReturnsFalse(t *testing.T) {
	t.Parallel()
	ch, _ := newHandlerTestChannel(t)
	if _, _, ok := ch.LookupGroupMemberByName(context.Background(), "g1", ""); ok {
		t.Fatal("empty name must not resolve")
	}
}

func TestLookupGroupMemberByName_NFCNormalization(t *testing.T) {
	t.Parallel()
	ch, _ := newHandlerTestChannel(t)
	// Cache the NFC composed form, search with decomposed input.
	ch.memberCache.Set("g1", "u_d", "Đức")          // U+0110 U+1EE9 U+0063 (NFC)
	uid, _, ok := ch.LookupGroupMemberByName(context.Background(), "g1", "Đức")
	if !ok || uid != "u_d" {
		t.Fatalf("NFC normalize broken: ok=%v uid=%q", ok, uid)
	}
}
