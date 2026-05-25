package channels

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/bus"
)

type tenantStubChannel struct {
	*BaseChannel
	tenantID uuid.UUID
	sentCh   chan bus.OutboundMessage
}

func (c *tenantStubChannel) TenantID() uuid.UUID { return c.tenantID }
func (c *tenantStubChannel) Send(_ context.Context, msg bus.OutboundMessage) error {
	select {
	case c.sentCh <- msg:
	default:
	}
	return nil
}
func (c *tenantStubChannel) Start(context.Context) error { c.SetRunning(true); return nil }
func (c *tenantStubChannel) Stop(context.Context) error  { c.SetRunning(false); return nil }

func newTenantStubChannel(name string, tenantID uuid.UUID) *tenantStubChannel {
	return &tenantStubChannel{
		BaseChannel: NewBaseChannel(name, bus.New(), nil),
		tenantID:    tenantID,
		sentCh:      make(chan bus.OutboundMessage, 1),
	}
}

func captureLogs(t *testing.T) (*bytes.Buffer, func()) {
	t.Helper()
	buf := &bytes.Buffer{}
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	return buf, func() { slog.SetDefault(prev) }
}

func runDispatcher(t *testing.T, mgr *Manager) (context.CancelFunc, chan struct{}) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		mgr.dispatchOutbound(ctx)
		close(done)
	}()
	return cancel, done
}

func TestDispatchTenantMismatch_LogsWarning(t *testing.T) {
	logs, restore := captureLogs(t)
	defer restore()

	msgBus := bus.New()
	mgr := NewManager(msgBus)
	channelTenant := uuid.New()
	ch := newTenantStubChannel("zalo-annhien", channelTenant)
	mgr.RegisterChannel("zalo-annhien", ch)

	cancel, done := runDispatcher(t, mgr)
	defer func() {
		cancel()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
		}
	}()

	otherTenant := uuid.New()
	msgBus.PublishOutbound(bus.OutboundMessage{
		Channel:  "zalo-annhien",
		ChatID:   "chat-1",
		Content:  "leaked?",
		TenantID: otherTenant,
	})

	deadline := time.After(2 * time.Second)
	for {
		if strings.Contains(logs.String(), "security.dispatch.tenant_mismatch") {
			return
		}
		select {
		case <-deadline:
			t.Fatalf("expected security.dispatch.tenant_mismatch in logs, got: %s", logs.String())
		case <-time.After(20 * time.Millisecond):
		}
	}
}

func TestDispatchTenantMatch_NoWarning(t *testing.T) {
	logs, restore := captureLogs(t)
	defer restore()

	msgBus := bus.New()
	mgr := NewManager(msgBus)
	tenant := uuid.New()
	ch := newTenantStubChannel("zalo-annhien", tenant)
	mgr.RegisterChannel("zalo-annhien", ch)

	cancel, done := runDispatcher(t, mgr)
	defer func() {
		cancel()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
		}
	}()

	msgBus.PublishOutbound(bus.OutboundMessage{
		Channel:  "zalo-annhien",
		ChatID:   "chat-1",
		Content:  "ok",
		TenantID: tenant,
	})

	select {
	case <-ch.sentCh:
	case <-time.After(2 * time.Second):
		t.Fatal("channel.Send was never called")
	}
	if strings.Contains(logs.String(), "security.dispatch.tenant_mismatch") {
		t.Errorf("matching tenants should NOT log security warning; got: %s", logs.String())
	}
}

func TestDispatchNoTenantID_SkipsMismatchCheck(t *testing.T) {
	logs, restore := captureLogs(t)
	defer restore()

	msgBus := bus.New()
	mgr := NewManager(msgBus)
	ch := newTenantStubChannel("zalo-annhien", uuid.New())
	mgr.RegisterChannel("zalo-annhien", ch)

	cancel, done := runDispatcher(t, mgr)
	defer func() {
		cancel()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
		}
	}()

	msgBus.PublishOutbound(bus.OutboundMessage{
		Channel: "zalo-annhien",
		ChatID:  "chat-1",
		Content: "legacy caller without tenant",
	})

	select {
	case <-ch.sentCh:
	case <-time.After(2 * time.Second):
		t.Fatal("channel.Send was never called")
	}
	if strings.Contains(logs.String(), "security.dispatch.tenant_mismatch") {
		t.Errorf("nil msg.TenantID should skip the mismatch check; got: %s", logs.String())
	}
}
