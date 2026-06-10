package bot

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/nextlevelbuilder/goclaw/internal/bus"
)

// capturedSend records the parsed JSON body of each sendMessage POST.
type capturedSend struct {
	mu    sync.Mutex
	calls []map[string]any
}

func (cs *capturedSend) add(body map[string]any) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	cs.calls = append(cs.calls, body)
}

func (cs *capturedSend) snapshot() []map[string]any {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	return append([]map[string]any(nil), cs.calls...)
}

func newSendCaptureServer(t *testing.T, cs *capturedSend) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var body map[string]any
		_ = json.Unmarshal(raw, &body)
		cs.add(body)
		_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":"m1"}}`))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestBotSend_StripsMentionMarkers(t *testing.T) {
	cs := &capturedSend{}
	srv := newSendCaptureServer(t, cs)
	ch := newTestChannel(t, srv.URL)

	if err := ch.Send(context.Background(), bus.OutboundMessage{
		ChatID:  "user-1",
		Content: "Hello @[uid_abc] and @[all]",
	}); err != nil {
		t.Fatalf("Send: %v", err)
	}
	calls := cs.snapshot()
	if len(calls) == 0 {
		t.Fatal("no API calls captured")
	}
	got, _ := calls[0]["text"].(string)
	want := "Hello @uid_abc and @All"
	if got != want {
		t.Errorf("text = %q, want %q", got, want)
	}
}

func TestBotSend_NoMentionMarkers_PassesThrough(t *testing.T) {
	cs := &capturedSend{}
	srv := newSendCaptureServer(t, cs)
	ch := newTestChannel(t, srv.URL)

	if err := ch.Send(context.Background(), bus.OutboundMessage{
		ChatID:  "user-1",
		Content: "plain text no markers",
	}); err != nil {
		t.Fatalf("Send: %v", err)
	}
	calls := cs.snapshot()
	got, _ := calls[0]["text"].(string)
	if got != "plain text no markers" {
		t.Errorf("text = %q, want unchanged", got)
	}
}

func TestBotSend_OnlyAtAll(t *testing.T) {
	cs := &capturedSend{}
	srv := newSendCaptureServer(t, cs)
	ch := newTestChannel(t, srv.URL)

	if err := ch.Send(context.Background(), bus.OutboundMessage{
		ChatID:  "user-1",
		Content: "@[all]",
	}); err != nil {
		t.Fatalf("Send: %v", err)
	}
	calls := cs.snapshot()
	got, _ := calls[0]["text"].(string)
	if got != "@All" {
		t.Errorf("text = %q, want %q", got, "@All")
	}
}
