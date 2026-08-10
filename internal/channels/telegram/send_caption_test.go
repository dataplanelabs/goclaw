package telegram

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nextlevelbuilder/goclaw/internal/bus"
	"github.com/nextlevelbuilder/goclaw/internal/config"
)

func TestSendMediaMessagePreservesDistinctContent(t *testing.T) {
	var paths []string
	var text string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		if strings.HasSuffix(r.URL.Path, "/sendMessage") {
			body, _ := io.ReadAll(r.Body)
			var payload map[string]any
			_ = json.Unmarshal(body, &payload)
			text, _ = payload["text"].(string)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":1,"date":0,"chat":{"id":1,"type":"private"}}}`))
	}))
	defer srv.Close()

	path := filepath.Join(t.TempDir(), "report.pdf")
	if err := os.WriteFile(path, []byte("report"), 0o600); err != nil {
		t.Fatalf("write report: %v", err)
	}
	token := "123456789:" + strings.Repeat("A", 35)
	channel, err := New(config.TelegramConfig{Token: token, APIServer: srv.URL}, bus.New(), nil, nil)
	if err != nil {
		t.Fatalf("new channel: %v", err)
	}
	err = channel.sendMediaMessage(context.Background(), 1, bus.OutboundMessage{
		Content: "distinct response",
		Media: []bus.MediaAttachment{{
			URL:         path,
			ContentType: "application/pdf",
			Caption:     "file caption",
		}},
	}, 0, 0)
	if err != nil {
		t.Fatalf("send media: %v", err)
	}
	if len(paths) != 2 || !strings.HasSuffix(paths[0], "/sendDocument") || !strings.HasSuffix(paths[1], "/sendMessage") {
		t.Fatalf("request paths = %v, want sendDocument then sendMessage", paths)
	}
	if text != "distinct response" {
		t.Fatalf("text = %q, want distinct response", text)
	}
}

func TestRemainingContentAfterTelegramMediaCaptions(t *testing.T) {
	tests := []struct {
		name    string
		content string
		media   []bus.MediaAttachment
		want    string
	}{
		{
			name:    "matching first caption",
			content: "response",
			media:   []bus.MediaAttachment{{Caption: "response"}},
			want:    "",
		},
		{
			name:    "matching later caption",
			content: "response",
			media:   []bus.MediaAttachment{{}, {Caption: " response "}},
			want:    "",
		},
		{
			name:    "distinct caption",
			content: "response",
			media:   []bus.MediaAttachment{{Caption: "file caption"}},
			want:    "response",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := remainingContentAfterTelegramMediaCaptions(tt.content, tt.media); got != tt.want {
				t.Fatalf("remainingContentAfterTelegramMediaCaptions() = %q, want %q", got, tt.want)
			}
		})
	}
}
