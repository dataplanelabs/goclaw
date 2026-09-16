package channels

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/channels/schedule"
)

func TestInStandby_FailOpenWithoutResolver(t *testing.T) {
	c := NewBaseChannel("tg", nil, nil)
	c.SetTenantID(uuid.New())
	if c.InStandby("direct", "chat-1") {
		t.Fatal("nil resolver must not report standby")
	}
}

func TestInStandby_UsesThreadKey(t *testing.T) {
	c := NewBaseChannel("tg", nil, nil)
	tid := uuid.New()
	c.SetTenantID(tid)
	var gotTenant, gotChannel, gotKey string
	c.SetStandbyResolver(func(_ context.Context, tenantID, channelName, threadKey string, _ time.Time) schedule.Mode {
		gotTenant, gotChannel, gotKey = tenantID, channelName, threadKey
		return schedule.ModeStandby
	})
	if !c.InStandby("group", "42") {
		t.Fatal("want standby")
	}
	if gotTenant != tid.String() || gotChannel != "tg" || gotKey != "group:42" {
		t.Fatalf("resolver args tenant=%s channel=%s key=%s", gotTenant, gotChannel, gotKey)
	}
}

func TestInStandby_EmptyPeerKindDefaultsDirect(t *testing.T) {
	c := NewBaseChannel("tg", nil, nil)
	c.SetTenantID(uuid.New())
	var gotKey string
	c.SetStandbyResolver(func(_ context.Context, _, _, threadKey string, _ time.Time) schedule.Mode {
		gotKey = threadKey
		return schedule.ModeActive
	})
	if c.InStandby("", "peer") {
		t.Fatal("active must not report standby")
	}
	if gotKey != "direct:peer" {
		t.Fatalf("key=%s want direct:peer", gotKey)
	}
}
