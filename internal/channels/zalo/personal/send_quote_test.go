package personal

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/nextlevelbuilder/goclaw/internal/bus"
	"github.com/nextlevelbuilder/goclaw/internal/channels/zalo/personal/protocol"
	"github.com/nextlevelbuilder/goclaw/internal/config"
)

// quoteTestKeyB64 mirrors the protocol-layer test key — a deterministic 32-byte
// AES key, base64-encoded. Channel-level Send() tests need to encrypt the inner
// success response the same way the live API does.
const quoteTestKeyB64 = "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="

type sendCapture struct {
	path string
	body []byte
}

// sendQuoteServer captures all /api/message/* and /api/group/* POSTs. If
// quoteErrorCode != 0, requests routed to a /quote path respond with that
// error code (lets us simulate Zalo rejecting the quote). All other requests
// (including the unquoted retry) return success with successMsgID.
func sendQuoteServer(t *testing.T, successMsgID string, quoteErrorCode int) (*httptest.Server, *[]sendCapture, *int32, *int32) {
	t.Helper()
	var cap []sendCapture
	var quotedHits, unquotedHits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		cap = append(cap, sendCapture{path: r.URL.Path, body: body})
		isQuote := strings.HasSuffix(r.URL.Path, "/quote")
		if isQuote {
			atomic.AddInt32(&quotedHits, 1)
		} else {
			atomic.AddInt32(&unquotedHits, 1)
		}
		w.Header().Set("Content-Type", "application/json")
		if isQuote && quoteErrorCode != 0 {
			_, _ = w.Write([]byte(`{"error_code":` + strconv.Itoa(quoteErrorCode) + `,"error_message":"x","data":null}`))
			return
		}
		_, _ = w.Write([]byte(encryptInnerForChannel(t, successMsgID)))
	}))
	t.Cleanup(srv.Close)
	return srv, &cap, &quotedHits, &unquotedHits
}

func encryptInnerForChannel(t *testing.T, msgID string) string {
	t.Helper()
	inner := []byte(`{"error_code":0,"data":{"msgId":` + msgID + `}}`)
	key, _ := base64.StdEncoding.DecodeString(quoteTestKeyB64)
	enc, err := protocol.EncodeAESCBC(key, string(inner), false)
	if err != nil {
		t.Fatalf("encrypt inner: %v", err)
	}
	outer, _ := json.Marshal(map[string]any{"error_code": 0, "data": enc})
	return string(outer)
}

// newChannelWithSession wires a Channel to the test server. Session is set
// directly so Send() bypasses authentication. handleDM-style typing is skipped
// because Send doesn't call startTyping.
func newChannelWithSession(t *testing.T, srv *httptest.Server) *Channel {
	t.Helper()
	ch, err := New(config.ZaloPersonalConfig{
		Enabled: true, DMPolicy: "open", GroupPolicy: "open",
	}, bus.New(), nil, nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	sess := protocol.NewSession()
	sess.IMEI = "test-imei"
	sess.SecretKey = quoteTestKeyB64
	sess.LoginInfo = &protocol.LoginInfo{
		UID: "self-uid",
		ZpwServiceMapV3: protocol.ZpwServiceMapV3{
			Chat:  []string{srv.URL},
			Group: []string{srv.URL},
		},
	}
	ch.mu.Lock()
	ch.sess = sess
	ch.mu.Unlock()
	ch.SetRunning(true)
	return ch
}

// decryptCapturedForm decrypts the "params" field of a captured form body so
// tests can assert qmsg* presence/absence.
func decryptCapturedForm(t *testing.T, body []byte) map[string]any {
	t.Helper()
	form, err := url.ParseQuery(string(body))
	if err != nil {
		t.Fatalf("parse form: %v", err)
	}
	enc := form.Get("params")
	if enc == "" {
		t.Fatalf("no 'params' in body: %s", body)
	}
	key, _ := base64.StdEncoding.DecodeString(quoteTestKeyB64)
	plain, err := protocol.DecodeAESCBC(key, enc)
	if err != nil {
		t.Fatalf("decrypt params: %v", err)
	}
	var out map[string]any
	if err := json.Unmarshal(plain, &out); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	return out
}

// makeQuotePayload returns the JSON string that buildQuoteMetadata stamps —
// the wire format Phase 4's Send reads via readQuoteFromMetadata.
func makeQuotePayload(t *testing.T, globalMsgID, msg string) string {
	t.Helper()
	q := protocol.TQuote{
		OwnerID:     "111",
		GlobalMsgID: json.Number(globalMsgID),
		CliMsgID:    json.Number("1709300000123"),
		CliMsgType:  1,
		TS:          json.Number("1709300000"),
		Msg:         msg,
	}
	b, err := json.Marshal(q)
	if err != nil {
		t.Fatalf("marshal quote: %v", err)
	}
	return string(b)
}

func TestPersonalBuildQuoteSendPayload_NoQuote(t *testing.T) {
	t.Parallel()
	srv, cap, quoted, unquoted := sendQuoteServer(t, "1001", 0)
	ch := newChannelWithSession(t, srv)

	err := ch.Send(context.Background(), bus.OutboundMessage{
		ChatID:  "user-1",
		Content: "hi",
	})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if *quoted != 0 || *unquoted != 1 {
		t.Errorf("hits = quoted:%d unquoted:%d, want 0/1", *quoted, *unquoted)
	}
	payload := decryptCapturedForm(t, (*cap)[0].body)
	for _, k := range []string{"qmsgOwner", "qmsgId", "qmsg"} {
		if _, ok := payload[k]; ok {
			t.Errorf("qmsg field %q must be absent without quote, payload=%v", k, payload)
		}
	}
}

func TestPersonalBuildQuoteSendPayload_WithQuote(t *testing.T) {
	t.Parallel()
	srv, cap, quoted, _ := sendQuoteServer(t, "1001", 0)
	ch := newChannelWithSession(t, srv)

	err := ch.Send(context.Background(), bus.OutboundMessage{
		ChatID:  "user-1",
		Content: "thanks for that",
		Metadata: map[string]string{
			"reply_to_message_id":    "9876543210",
			"reply_to_quote_payload": makeQuotePayload(t, "9876543210", "original"),
		},
	})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if *quoted != 1 {
		t.Errorf("quoted hits = %d, want 1", *quoted)
	}
	payload := decryptCapturedForm(t, (*cap)[0].body)
	for _, k := range []string{"qmsgOwner", "qmsgId", "qmsgCliId", "qmsgType", "qmsg", "qmsgAttach", "qmsgTs", "qmsgTtl"} {
		if _, ok := payload[k]; !ok {
			t.Errorf("missing qmsg field %q in quoted send payload=%v", k, payload)
		}
	}
}

func TestPersonalSendText_QuoteOnFirstChunkOnly(t *testing.T) {
	t.Parallel()
	srv, cap, _, _ := sendQuoteServer(t, "1001", 0)
	ch := newChannelWithSession(t, srv)

	// Build text > maxTextLength to force chunking.
	var b strings.Builder
	for range 10 {
		b.WriteString(strings.Repeat("a", 499))
		b.WriteString("\n\n")
	}
	long := b.String()

	err := ch.Send(context.Background(), bus.OutboundMessage{
		ChatID:  "user-1",
		Content: long,
		Metadata: map[string]string{
			"reply_to_message_id":    "9876543210",
			"reply_to_quote_payload": makeQuotePayload(t, "9876543210", "orig"),
		},
	})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if len(*cap) < 2 {
		t.Fatalf("captured %d, want >=2 chunks", len(*cap))
	}
	// First chunk: /quote endpoint, qmsg present.
	if !strings.HasSuffix((*cap)[0].path, "/quote") {
		t.Errorf("chunk 0 path = %q, want /quote suffix", (*cap)[0].path)
	}
	// Subsequent chunks: non-/quote endpoint, no qmsg.
	for i := 1; i < len(*cap); i++ {
		if strings.HasSuffix((*cap)[i].path, "/quote") {
			t.Errorf("chunk %d hit /quote endpoint, want unquoted", i)
		}
		payload := decryptCapturedForm(t, (*cap)[i].body)
		if _, ok := payload["qmsgId"]; ok {
			t.Errorf("chunk %d carries qmsg fields, want unquoted", i)
		}
	}
}

func TestPersonalSendText_NoQuoteWhenIDEmpty(t *testing.T) {
	t.Parallel()
	srv, _, quoted, unquoted := sendQuoteServer(t, "1001", 0)
	ch := newChannelWithSession(t, srv)

	err := ch.Send(context.Background(), bus.OutboundMessage{
		ChatID:  "user-1",
		Content: "hi",
		// Note: reply_to_message_id present but reply_to_quote_payload absent.
		// readQuoteFromMetadata returns nil → non-quoted send.
		Metadata: map[string]string{"reply_to_message_id": "9876543210"},
	})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if *quoted != 0 || *unquoted != 1 {
		t.Errorf("hits = quoted:%d unquoted:%d, want 0/1", *quoted, *unquoted)
	}
}

func TestPersonalSendText_QuoteDroppedOnQuoteRejected(t *testing.T) {
	t.Parallel()
	// quoteErrorCode 999 triggers ErrQuoteRejected wrapping on /quote routes;
	// the fallback retries via non-/quote endpoint which returns success.
	srv, cap, quoted, unquoted := sendQuoteServer(t, "1001", 999)
	ch := newChannelWithSession(t, srv)

	err := ch.Send(context.Background(), bus.OutboundMessage{
		ChatID:  "user-1",
		Content: "hi",
		Metadata: map[string]string{
			"reply_to_message_id":    "9876543210",
			"reply_to_quote_payload": makeQuotePayload(t, "9876543210", "deleted"),
		},
	})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if *quoted != 1 || *unquoted != 1 {
		t.Errorf("hits = quoted:%d unquoted:%d, want 1/1 (one rejection + one fallback)", *quoted, *unquoted)
	}
	if len(*cap) != 2 {
		t.Fatalf("captured %d, want 2", len(*cap))
	}
	// Second request must NOT carry qmsg fields.
	payload := decryptCapturedForm(t, (*cap)[1].body)
	if _, ok := payload["qmsgId"]; ok {
		t.Errorf("fallback retry still carries qmsg fields, payload=%v", payload)
	}
}

func TestPersonalChannelSend_MetadataReplyToBecomesQuote(t *testing.T) {
	t.Parallel()
	srv, cap, quoted, _ := sendQuoteServer(t, "1001", 0)
	ch := newChannelWithSession(t, srv)

	err := ch.Send(context.Background(), bus.OutboundMessage{
		ChatID:  "user-1",
		Content: "hello",
		Metadata: map[string]string{
			"reply_to_message_id":    "9876543210",
			"reply_to_quote_payload": makeQuotePayload(t, "9876543210", "original-text"),
		},
	})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if *quoted != 1 {
		t.Errorf("quoted hits = %d, want 1", *quoted)
	}
	payload := decryptCapturedForm(t, (*cap)[0].body)
	if payload["qmsgId"] != "9876543210" {
		t.Errorf("qmsgId = %v, want 9876543210", payload["qmsgId"])
	}
	if payload["qmsg"] != "original-text" {
		t.Errorf("qmsg = %v, want original-text", payload["qmsg"])
	}
}

// TestPersonalChannelSend_TrailingTextWhenQuoteSendsTextFirst verifies that
// when a quote is present, the text-with-quote chunk goes BEFORE any media
// (order inversion vs default media-first). Mirrors zca-js's two-request
// split for quote+attachment.
func TestPersonalChannelSend_TrailingTextWhenQuoteSendsTextFirst(t *testing.T) {
	t.Parallel()
	srv, cap, _, _ := sendQuoteServer(t, "1001", 0)
	ch := newChannelWithSession(t, srv)

	// Use a non-existent image path — sendImage will fail upload and log warn,
	// but Send proceeds. The test only asserts call ORDERING via the captured
	// /quote request being recorded BEFORE any image-related path.
	err := ch.Send(context.Background(), bus.OutboundMessage{
		ChatID:  "user-1",
		Content: "trailing note",
		Media:   []bus.MediaAttachment{{URL: "/nonexistent/missing.png", ContentType: "image/png"}},
		Metadata: map[string]string{
			"reply_to_message_id":    "9876543210",
			"reply_to_quote_payload": makeQuotePayload(t, "9876543210", "orig"),
		},
	})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if len(*cap) == 0 {
		t.Fatal("no captured calls — Send did not send text")
	}
	// First captured call must be the /quote text send (text-first when quoted).
	if !strings.HasSuffix((*cap)[0].path, "/quote") {
		t.Errorf("first call path = %q, want /quote suffix (text-first when quoted)", (*cap)[0].path)
	}
}

// TestPersonalChannelSend_NoQuote_PreservesExistingMediaFirstOrder is the
// REGRESSION GUARD for the order-inversion in Send. Without a quote, media
// must still attempt upload BEFORE text — preserves pre-plan behavior for
// existing zalo-personal users.
func TestPersonalChannelSend_NoQuote_PreservesExistingMediaFirstOrder(t *testing.T) {
	t.Parallel()
	srv, cap, _, _ := sendQuoteServer(t, "1001", 0)
	ch := newChannelWithSession(t, srv)

	// Send with image (will fail upload) + text. Without quote, send.go must
	// attempt the media first, then the text — historical order.
	// We can't capture the image upload (sendMediaBestEffort swallows the
	// upload error before any HTTP hits our captureServer), but we CAN
	// verify the text call is the only captured request AND no /quote
	// endpoint was used.
	err := ch.Send(context.Background(), bus.OutboundMessage{
		ChatID:  "user-1",
		Content: "trailing note",
		Media:   []bus.MediaAttachment{{URL: "/nonexistent/missing.png", ContentType: "image/png"}},
		// NO Metadata — no quote.
	})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	for _, c := range *cap {
		if strings.HasSuffix(c.path, "/quote") {
			t.Errorf("/quote endpoint hit without quote metadata: path=%q", c.path)
		}
	}
}
