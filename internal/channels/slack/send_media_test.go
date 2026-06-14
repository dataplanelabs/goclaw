package slack

// Regression test: send_file media must reach Slack even when the final LLM text is
// empty (agent sends file then produces no additional text → Content="" in RunResult).
//
// Before the fix, Send() returned at the placeholder-delete early-exit when content=="",
// so the msg.Media loop was never reached and uploadFile was never called.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"testing"

	slackapi "github.com/slack-go/slack"

	"github.com/nextlevelbuilder/goclaw/internal/bus"
	"github.com/nextlevelbuilder/goclaw/internal/channels"
)

// stubUploadServer stubs the 3-step Slack upload sequence used by UploadFileContext
// in slack-go v0.19+:
//
//  1. POST /api/files.getUploadURLExternal → { ok, upload_url, file_id }
//  2. POST /upload                         → 200 OK  (counts as "uploaded")
//  3. POST /api/files.completeUploadExternal → { ok }
//
// All other /api/* endpoints return { ok:true, ts:"1234.5678" } so placeholder
// operations (chat.delete, chat.postMessage) don't error out.
func stubUploadServer(t *testing.T) (srv *httptest.Server, uploadHits *atomic.Int32) {
	t.Helper()
	var hits atomic.Int32

	mux := http.NewServeMux()

	mux.HandleFunc("/api/files.getUploadURLExternal", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// The upload_url must point back at this same server.
		host := "http://" + r.Host
		json.NewEncoder(w).Encode(map[string]any{ //nolint:errcheck
			"ok":         true,
			"upload_url": host + "/upload",
			"file_id":    "F_TEST_001",
		})
	})

	mux.HandleFunc("/upload", func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusOK)
	})

	mux.HandleFunc("/api/files.completeUploadExternal", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{ //nolint:errcheck
			"ok":    true,
			"files": []map[string]any{{"id": "F_TEST_001"}},
		})
	})

	// Fallback for chat.delete, chat.postMessage, chat.update, auth.test, etc.
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{ //nolint:errcheck
			"ok": true,
			"ts": "1234.5678",
		})
	})

	srv = httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, &hits
}

// newRunningChannel creates a minimal Channel whose BaseChannel is set to running.
func newRunningChannel(t *testing.T, api *slackapi.Client) *Channel {
	t.Helper()
	base := channels.NewBaseChannel("slack-test", nil, nil)
	base.SetRunning(true)
	ch := &Channel{BaseChannel: base, api: api}
	return ch
}

// TestSend_MediaDeliveredWhenContentEmpty is the primary regression test.
// It asserts that when Content=="" but Media is non-empty, Send uploads the file.
func TestSend_MediaDeliveredWhenContentEmpty(t *testing.T) {
	// Create a real file so uploadFile's os.ReadFile succeeds.
	f, err := os.CreateTemp(t.TempDir(), "cats-*.png")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write(make([]byte, 1024)); err != nil {
		t.Fatal(err)
	}
	f.Close()

	srv, uploadHits := stubUploadServer(t)
	api := slackapi.New("xoxb-test-token", slackapi.OptionAPIURL(srv.URL+"/api/"))
	ch := newRunningChannel(t, api)

	msg := bus.OutboundMessage{
		ChatID:  "C_CHANNEL_1",
		Content: "", // agent's final text was empty — send_file ForLLM was tool result
		Media: []bus.MediaAttachment{
			{URL: f.Name(), ContentType: "image/png"},
		},
	}

	if err := ch.Send(context.Background(), msg); err != nil {
		t.Fatalf("Send: unexpected error: %v", err)
	}

	if uploadHits.Load() == 0 {
		t.Error("regression: uploadFile was never called — media not delivered to Slack when content is empty")
	}
}

// TestSend_EmptyContentNoMediaReturnsEarly verifies that when both content and
// media are absent, Send performs placeholder cleanup and returns without uploading.
func TestSend_EmptyContentNoMediaReturnsEarly(t *testing.T) {
	srv, uploadHits := stubUploadServer(t)
	api := slackapi.New("xoxb-test-token", slackapi.OptionAPIURL(srv.URL+"/api/"))
	ch := newRunningChannel(t, api)

	msg := bus.OutboundMessage{
		ChatID:  "C_CHANNEL_1",
		Content: "",
		Media:   nil,
	}

	if err := ch.Send(context.Background(), msg); err != nil {
		t.Fatalf("Send: unexpected error for empty message: %v", err)
	}
	if uploadHits.Load() > 0 {
		t.Error("upload must not be called when both content and media are empty")
	}
}
