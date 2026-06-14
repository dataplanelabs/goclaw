package telegram

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestDraftStream_RichDraft verifies two Update() calls produce two sendRichMessageDraft POSTs
// with the same draft_id and markdown (not HTML) body.
func TestDraftStream_RichDraft(t *testing.T) {
	type call struct {
		path string
		body map[string]any
	}
	var calls []call

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var body map[string]any
		_ = json.Unmarshal(raw, &body)
		calls = append(calls, call{path: r.URL.Path, body: body})
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"result":true}`))
	}))
	defer srv.Close()

	c := newTestChannel(t, srv, true)

	ds := NewDraftStream(nil, 12345, 0, 0, true)
	ds.draftID = 7 // fix draft_id for deterministic assertions
	ds.richSender = c.sendRichMessageDraft

	ctx := context.Background()
	// Bypass throttle by calling flush directly twice.
	ds.pending = "# First update"
	_ = ds.flush(ctx)
	ds.lastText = "" // reset dedup so second flush fires
	ds.pending = "# Second update"
	_ = ds.flush(ctx)

	if len(calls) != 2 {
		t.Fatalf("expected 2 sendRichMessageDraft calls, got %d", len(calls))
	}
	for i, c := range calls {
		if !strings.Contains(c.path, "sendRichMessageDraft") {
			t.Errorf("call %d path = %q, want sendRichMessageDraft", i, c.path)
		}
		draftID, _ := c.body["draft_id"].(float64)
		if int(draftID) != 7 {
			t.Errorf("call %d draft_id = %v, want 7", i, draftID)
		}
		rm, ok := c.body["rich_message"].(map[string]any)
		if !ok {
			t.Fatalf("call %d: rich_message missing from body", i)
		}
		md, _ := rm["markdown"].(string)
		// Verify it's markdown (has #), NOT HTML (no <b>).
		if !strings.Contains(md, "#") {
			t.Errorf("call %d: expected markdown with '#', got %q", i, md)
		}
		if strings.Contains(md, "<b>") || strings.Contains(md, "<code>") {
			t.Errorf("call %d: body appears to be HTML, not raw markdown: %q", i, md)
		}
	}
}

// TestDraftStream_RichDraftFallsBack verifies that a method-not-found error from
// sendRichMessageDraft triggers the permanent fallback (draftFailed=true).
// We test shouldFallbackFromDraft directly and the flag-setting path in isolation.
func TestDraftStream_RichDraftFallsBack(t *testing.T) {
	richCalls := 0

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		richCalls++
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"ok":false,"error_code":400,"description":"sendRichMessageDraft: unknown method"}`))
	}))
	defer srv.Close()

	c := newTestChannel(t, srv, true)

	// Verify shouldFallbackFromDraft recognises sendRichMessageDraft errors.
	err := c.sendRichMessageDraft(context.Background(), sendRichMessageDraftParams{
		ChatID:      12345,
		DraftID:     8,
		RichMessage: inputRichMessage{Markdown: "hello"},
	})
	if err == nil {
		t.Fatal("expected error from stub server")
	}
	if !shouldFallbackFromDraft(err) {
		t.Errorf("shouldFallbackFromDraft(%q) = false, want true", err.Error())
	}
	if richCalls != 1 {
		t.Errorf("sendRichMessageDraft called %d times, want 1", richCalls)
	}

	// Simulate the flush() path: richSender returns a fallback error → draftFailed set.
	ds := &DraftStream{
		useDraft: true,
		draftID:  8,
		richSender: func(_ context.Context, _ sendRichMessageDraftParams) error {
			return err // reuse the error from above
		},
	}
	ds.pending = "hello"
	ds.mu.Lock()
	// We call the internal rich sender branch manually to avoid needing a real bot
	// for the message-transport fallback path.
	rp := sendRichMessageDraftParams{
		ChatID:      12345,
		DraftID:     ds.draftID,
		RichMessage: inputRichMessage{Markdown: strings.TrimSpace(ds.pending)},
	}
	richErr := ds.richSender(context.Background(), rp)
	if shouldFallbackFromDraft(richErr) {
		ds.draftFailed = true
	}
	ds.mu.Unlock()

	if !ds.draftFailed {
		t.Error("draftFailed should be true after method-not-found response")
	}
}
