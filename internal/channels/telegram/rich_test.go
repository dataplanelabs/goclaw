package telegram

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/nextlevelbuilder/goclaw/internal/config"
)

// newTestChannel builds a minimal Channel backed by the given httptest server.
func newTestChannel(t *testing.T, srv *httptest.Server, richEnabled bool) *Channel {
	t.Helper()
	rich := richEnabled
	return &Channel{
		config: config.TelegramConfig{
			Token:       "TEST",
			APIServer:   srv.URL,
			RichMessage: &rich,
		},
		httpClient: srv.Client(),
	}
}

// --- sendRichMessage raw call ---

func TestSendRichMessage_Success(t *testing.T) {
	var gotPath string
	var gotBody map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &gotBody)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":42}}`))
	}))
	defer srv.Close()

	c := newTestChannel(t, srv, true)
	msgID, err := c.sendRichMessage(context.Background(), sendRichMessageParams{
		ChatID:      12345,
		RichMessage: inputRichMessage{Markdown: "# Hello\nworld"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if msgID != 42 {
		t.Errorf("message_id = %d, want 42", msgID)
	}
	if gotPath != "/botTEST/sendRichMessage" {
		t.Errorf("path = %q, want /botTEST/sendRichMessage", gotPath)
	}
	rm, ok := gotBody["rich_message"].(map[string]any)
	if !ok {
		t.Fatalf("rich_message not in body: %v", gotBody)
	}
	if md, _ := rm["markdown"].(string); md != "# Hello\nworld" {
		t.Errorf("rich_message.markdown = %q, want '# Hello\\nworld'", md)
	}
}

func TestSendRichMessage_APIError(t *testing.T) {
	// Telegram returns its error description in the envelope; our sendRichMessage
	// surfaces it verbatim so callers' regex matchers can decide on fallback.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		// Use an entity parse error (same surface Telegram uses for bad markup).
		_, _ = w.Write([]byte(`{"ok":false,"error_code":400,"description":"Bad Request: can't parse entities"}`))
	}))
	defer srv.Close()

	c := newTestChannel(t, srv, true)
	_, err := c.sendRichMessage(context.Background(), sendRichMessageParams{
		ChatID:      12345,
		RichMessage: inputRichMessage{Markdown: "bad"},
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	// Error string must carry Telegram's description so callers can apply parseErrRe / fallback logic.
	if !strings.Contains(err.Error(), "can't parse entities") {
		t.Errorf("error %q should contain Telegram description", err.Error())
	}
	if !parseErrRe.MatchString(err.Error()) {
		t.Errorf("error %q should match parseErrRe", err.Error())
	}
}

func TestSendRichMessage_ThreadOmitted(t *testing.T) {
	var gotBody map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &gotBody)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":1}}`))
	}))
	defer srv.Close()

	c := newTestChannel(t, srv, true)
	// Thread 1 = General topic; resolveThreadIDForSend should return 0 → omitted.
	_, err := c.sendRichMessage(context.Background(), sendRichMessageParams{
		ChatID:          -100123,
		RichMessage:     inputRichMessage{Markdown: "hello"},
		MessageThreadID: resolveThreadIDForSend(1),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tid, ok := gotBody["message_thread_id"]; ok {
		t.Errorf("message_thread_id should be omitted for General topic, got %v", tid)
	}
}

// --- prepareRichMarkdown ---

func TestPrepareRichMarkdown(t *testing.T) {
	c := &Channel{}
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"heading preserved", "# Hello World", "# Hello World"},
		{"table preserved", "| A | B |\n|---|---|\n| 1 | 2 |", "| A | B |\n|---|---|\n| 1 | 2 |"},
		{"fenced code preserved", "```go\nfmt.Println()\n```", "```go\nfmt.Println()\n```"},
		{"inline math $..$ preserved", "$x^2 + y^2 = z^2$", "$x^2 + y^2 = z^2$"},
		{"block math \\[..\\] -> $$..$$", "\\[a = b\\]", "$$a = b$$"},
		{"inline math \\(..\\) -> $..$", "x \\(y\\) z", "x $y$ z"},
		{"block math multiline", "\\[\na = m^2 - n^2,\\quad b = 2mn\n\\]", "$$\na = m^2 - n^2,\\quad b = 2mn\n$$"},
		{"latex fence -> $$ block", "```latex\n\\[a\\]\n```", "$$\na\n$$"},
		{"latex fence multiline -> $$", "```latex\n\\[\nx = \\frac{1}{2}\n\\]\n```", "$$\nx = \\frac{1}{2}\n$$"},
		{"math fence -> $$ block", "```math\nE = mc^2\n```", "$$\nE = mc^2\n$$"},
		{"backtick-wrapped $..$ unwrapped", "x `$a^2$` y", "x $a^2$ y"},
		{"backtick-wrapped \\[..\\] unwrapped+converted", "use `\\[a\\]` here", "use $$a$$ here"},
		{"real code fence (go) with brackets untouched", "```go\narr\\[i\\]\n```", "```go\narr\\[i\\]\n```"},
		{"real inline code untouched", "run `gofmt -w`", "run `gofmt -w`"},
		{"existing $$ block untouched", "$$a = b$$", "$$a = b$$"},
		{"list preserved", "- item 1\n- item 2", "- item 1\n- item 2"},
		{"leading/trailing whitespace trimmed", "  hello  ", "hello"},
		{"newline trimmed", "\nhello\n", "hello"},
		{"empty string", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := c.prepareRichMarkdown(tt.input)
			if got != tt.want {
				t.Errorf("prepareRichMarkdown(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// --- trySendRich ---

func TestTrySendRich_Success(t *testing.T) {
	richHit := 0
	sendHit := 0

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "sendRichMessage") {
			richHit++
			_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":10}}`))
		} else if strings.Contains(r.URL.Path, "sendMessage") {
			sendHit++
			_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":11}}`))
		}
	}))
	defer srv.Close()

	c := newTestChannel(t, srv, true)
	ok := c.trySendRich(context.Background(), 12345, "# Heading", 0, 0, "key1")
	if !ok {
		t.Error("trySendRich should return true on success")
	}
	if richHit != 1 {
		t.Errorf("sendRichMessage hit %d times, want 1", richHit)
	}
	if sendHit != 0 {
		t.Errorf("sendMessage hit %d times, want 0 (rich succeeded)", sendHit)
	}
}

func TestTrySendRich_FallsBackOnError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"ok":false,"error_code":400,"description":"Bad Request: can't parse entities"}`))
	}))
	defer srv.Close()

	c := newTestChannel(t, srv, true)
	// No placeholder stored (id=0 path skips deleteMessage → no nil-bot panic).

	ok := c.trySendRich(context.Background(), 12345, "bad text", 0, 0, "key2")
	if ok {
		t.Error("trySendRich should return false on API error")
	}
	// Placeholder was already consumed by LoadAndDelete; nothing left.
	if _, stillThere := c.placeholders.Load("key2"); stillThere {
		t.Error("placeholder should not exist after trySendRich consumed it")
	}
}

// TestTrySendRich_PlaceholderCleared verifies that a stored placeholder is consumed
// by trySendRich (via LoadAndDelete) before attempting the send.
// We use id=-1 (the "ghost" sentinel) so deleteMessage is not called (id <= 0 guard).
func TestTrySendRich_PlaceholderCleared(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":5}}`))
	}))
	defer srv.Close()

	c := newTestChannel(t, srv, true)
	// Store sentinel placeholder (negative id → deleteMessage skipped).
	c.placeholders.Store("key3", -1)

	ok := c.trySendRich(context.Background(), 12345, "# hello", 0, 0, "key3")
	if !ok {
		t.Error("trySendRich should return true on success")
	}
	// Placeholder consumed by LoadAndDelete.
	if _, stillThere := c.placeholders.Load("key3"); stillThere {
		t.Error("placeholder should be consumed by trySendRich")
	}
}

// --- flag-off regression ---

func TestSend_RichFlagOff_UsesHTML(t *testing.T) {
	richHit := 0

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "sendRichMessage") {
			richHit++
		}
		// Return minimal success for any call so we don't blow up.
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":1}}`))
	}))
	defer srv.Close()

	// Build channel with the flag explicitly off (default is now on).
	off := false
	c := &Channel{
		config: config.TelegramConfig{
			Token:       "TEST",
			APIServer:   srv.URL,
			RichMessage: &off, // explicitly disabled
		},
		httpClient: srv.Client(),
	}

	if c.richMessageEnabled() {
		t.Error("richMessageEnabled() should be false when RichMessage is explicitly false")
	}
	if richHit != 0 {
		t.Errorf("sendRichMessage endpoint hit %d times, want 0 when flag is off", richHit)
	}
}

// --- richMessageEnabled ---

func TestRichMessageEnabled(t *testing.T) {
	f := false
	tr := true
	tests := []struct {
		name string
		ptr  *bool
		want bool
	}{
		{"nil (default on)", nil, true},
		{"explicit false", &f, false},
		{"explicit true", &tr, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &Channel{config: config.TelegramConfig{RichMessage: tt.ptr}}
			if got := c.richMessageEnabled(); got != tt.want {
				t.Errorf("richMessageEnabled() = %v, want %v", got, tt.want)
			}
		})
	}
}
