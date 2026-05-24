package protocol

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
)

// testKeyB64 is a deterministic 32-byte AES key, base64-encoded, used as the
// session secret in /quote endpoint tests. Anything decodable to a valid AES
// key size (16/24/32 bytes) works; using a fixed value keeps tests reproducible.
const testKeyB64 = "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="

// newQuoteTestSession wires a Session to a httptest server, using both the
// chat and group service URL slots so DM and group tests can share scaffolding.
func newQuoteTestSession(t *testing.T, srv *httptest.Server) *Session {
	t.Helper()
	sess := NewSession()
	sess.IMEI = "test-imei"
	sess.SecretKey = testKeyB64
	sess.LoginInfo = &LoginInfo{
		UID: "self-uid",
		ZpwServiceMapV3: ZpwServiceMapV3{
			Chat:  []string{srv.URL},
			Group: []string{srv.URL},
		},
	}
	return sess
}

// decryptRequestParams reverses encryptPayload: extracts the form's "params"
// field, AES-CBC-decrypts with the test key, and decodes the JSON payload so
// tests can assert qmsg* fields are present.
func decryptRequestParams(t *testing.T, body []byte) map[string]any {
	t.Helper()
	form, err := url.ParseQuery(string(body))
	if err != nil {
		t.Fatalf("parse form body: %v", err)
	}
	enc := form.Get("params")
	if enc == "" {
		t.Fatalf("no 'params' field in form body: %s", body)
	}
	key, err := base64.StdEncoding.DecodeString(testKeyB64)
	if err != nil {
		t.Fatalf("decode test key: %v", err)
	}
	plain, err := DecodeAESCBC(key, enc)
	if err != nil {
		t.Fatalf("decrypt params: %v", err)
	}
	var out map[string]any
	if err := json.Unmarshal(plain, &out); err != nil {
		t.Fatalf("unmarshal decrypted payload: %v", err)
	}
	return out
}

// encryptSuccessResponse builds the doubly-encrypted server response shape
// SendMessage's decryptDataField expects: outer envelope contains an encrypted
// inner envelope, which itself contains {"msgId":...}.
func encryptSuccessResponse(t *testing.T, msgID string) string {
	t.Helper()
	// msgId is a json.Number on the receiver — must be raw numeric JSON, not a
	// quoted string. Pass msgID as already-numeric-string and embed verbatim.
	innerJSON := []byte(`{"error_code":0,"data":{"msgId":` + msgID + `}}`)
	key, _ := base64.StdEncoding.DecodeString(testKeyB64)
	enc, err := EncodeAESCBC(key, string(innerJSON), false)
	if err != nil {
		t.Fatalf("encode inner response: %v", err)
	}
	outer := map[string]any{
		"error_code": 0,
		"data":       enc,
	}
	out, _ := json.Marshal(outer)
	return string(out)
}

// captureServer returns a server that records the path + body of each request
// and responds with the supplied JSON. If errorCode != 0, that overrides the
// successful response.
type captured struct {
	path string
	body []byte
}

func captureServer(t *testing.T, successMsgID string, errorCode int) (*httptest.Server, *[]captured) {
	t.Helper()
	var cap []captured
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		cap = append(cap, captured{path: r.URL.Path, body: body})
		w.Header().Set("Content-Type", "application/json")
		if errorCode != 0 {
			_, _ = w.Write([]byte(`{"error_code":` + strconv.Itoa(errorCode) + `,"error_message":"x","data":null}`))
			return
		}
		_, _ = w.Write([]byte(encryptSuccessResponse(t, successMsgID)))
	}))
	t.Cleanup(srv.Close)
	return srv, &cap
}

func TestSendMessage_NoQuote_HitsSmsEndpoint(t *testing.T) {
	t.Parallel()
	srv, cap := captureServer(t, "1001", 0)
	sess := newQuoteTestSession(t, srv)

	_, err := SendMessage(context.Background(), sess, "user-1", ThreadTypeUser, "hi", nil)
	if err != nil {
		t.Fatalf("SendMessage: %v", err)
	}
	if !strings.HasSuffix((*cap)[0].path, apiPathDM) {
		t.Errorf("path = %q, want suffix %q", (*cap)[0].path, apiPathDM)
	}
}

func TestSendMessage_WithQuote_HitsQuoteEndpoint_DM(t *testing.T) {
	t.Parallel()
	srv, cap := captureServer(t, "1001", 0)
	sess := newQuoteTestSession(t, srv)

	q := &SendMessageQuote{
		OwnerID: "111", MsgID: "9876543210", CliMsgID: "1709300000",
		MsgType: "chat.text", Msg: "original", TS: "1709300000",
	}
	_, err := SendMessage(context.Background(), sess, "user-1", ThreadTypeUser, "reply", q)
	if err != nil {
		t.Fatalf("SendMessage: %v", err)
	}
	if !strings.HasSuffix((*cap)[0].path, apiPathDMQuote) {
		t.Errorf("path = %q, want suffix %q", (*cap)[0].path, apiPathDMQuote)
	}
}

func TestSendMessage_WithQuote_HitsQuoteEndpoint_Group(t *testing.T) {
	t.Parallel()
	srv, cap := captureServer(t, "1001", 0)
	sess := newQuoteTestSession(t, srv)

	q := &SendMessageQuote{
		OwnerID: "111", MsgID: "9876543210", MsgType: "chat.text", Msg: "x",
	}
	_, err := SendMessage(context.Background(), sess, "group-abc", ThreadTypeGroup, "reply", q)
	if err != nil {
		t.Fatalf("SendMessage: %v", err)
	}
	if !strings.HasSuffix((*cap)[0].path, apiPathGroupQuote) {
		t.Errorf("path = %q, want suffix %q", (*cap)[0].path, apiPathGroupQuote)
	}
}

func TestSendMessage_WithQuote_IncludesQmsgParams(t *testing.T) {
	t.Parallel()
	srv, cap := captureServer(t, "1001", 0)
	sess := newQuoteTestSession(t, srv)

	q := &SendMessageQuote{
		OwnerID:  "owner-111",
		MsgID:    "9876543210",
		CliMsgID: "1709300000123",
		MsgType:  "chat.text",
		Msg:      "original text",
		Attach:   `{"hdUrl":"x"}`,
		TS:       "1709300000",
		TTL:      0,
	}
	_, err := SendMessage(context.Background(), sess, "user-1", ThreadTypeUser, "reply", q)
	if err != nil {
		t.Fatalf("SendMessage: %v", err)
	}

	payload := decryptRequestParams(t, (*cap)[0].body)
	wantPairs := map[string]any{
		"qmsgOwner": "owner-111",
		"qmsgId":    "9876543210",
		"qmsgCliId": "1709300000123",
		"qmsg":      "original text",
		"qmsgTs":    "1709300000",
	}
	for k, want := range wantPairs {
		got, ok := payload[k]
		if !ok {
			t.Errorf("missing qmsg field %q in payload %v", k, payload)
			continue
		}
		if got != want {
			t.Errorf("payload[%q] = %v, want %v", k, got, want)
		}
	}
	// qmsgType must be a NUMBER, not the "chat.text" string — Zalo /quote rejects
	// the string form with code 114. zca-js getClientMessageType returns 1 for
	// text / unknown.
	if v, ok := payload["qmsgType"]; !ok || v.(float64) != 1 {
		t.Errorf("qmsgType = %v (%T), want 1 (number)", v, v)
	}
	// qmsgTTL must be uppercase TTL — the lowercase variant was a port typo.
	if v, ok := payload["qmsgTTL"]; !ok || v.(float64) != 0 {
		t.Errorf("qmsgTTL = %v, want 0", v)
	}
	if _, ok := payload["qmsgTtl"]; ok {
		t.Errorf("legacy lowercase qmsgTtl must not appear in payload")
	}
	// qmsgAttach must be OMITTED for TEXT quotes on DM — observed to trigger
	// server code 114. Non-text quotes (file/photo/video) DO include qmsgAttach
	// on DM per shouldSendQmsgAttach — covered by the *_DM_*IncludesAttach tests.
	if _, ok := payload["qmsgAttach"]; ok {
		t.Errorf("qmsgAttach must not appear in DM text payload, got %v", payload["qmsgAttach"])
	}
	// Regular send fields still present.
	if payload["message"] != "reply" {
		t.Errorf("message = %v, want 'reply'", payload["message"])
	}
	if payload["toid"] != "user-1" {
		t.Errorf("toid = %v, want 'user-1'", payload["toid"])
	}
}

// Group photo quotes include qmsgAttach. Pre-fix this was the only branch that
// sent qmsgAttach at all; post-fix DM non-text also includes it (see
// TestSendMessage_WithQuote_DM_FileIncludesAttach).
func TestSendMessage_WithQuote_Group_IncludesAttach(t *testing.T) {
	t.Parallel()
	srv, cap := captureServer(t, "1001", 0)
	sess := newQuoteTestSession(t, srv)

	q := &SendMessageQuote{
		OwnerID: "owner-111", MsgID: "9876543210", CliMsgID: "1709300000123",
		MsgType: "chat.photo", Msg: "", Attach: `{"hdUrl":"x"}`, TS: "1709300000",
	}
	_, err := SendMessage(context.Background(), sess, "group-1", ThreadTypeGroup, "reply", q)
	if err != nil {
		t.Fatalf("SendMessage: %v", err)
	}
	payload := decryptRequestParams(t, (*cap)[0].body)
	if v, ok := payload["qmsgAttach"]; !ok || v != `{"hdUrl":"x"}` {
		t.Errorf("group qmsgAttach = %v, want raw JSON string", v)
	}
	// chat.photo → 32 per zca-js getClientMessageType.
	if v, ok := payload["qmsgType"]; !ok || v.(float64) != 32 {
		t.Errorf("qmsgType = %v, want 32 (chat.photo)", v)
	}
}

// Reproduces the "[Tin nhắn chưa hỗ trợ ở phiên bản hiện tại]" bug on the
// receiver side when a bot quotes a file on DM. Pre-fix: qmsgType collapsed to
// 1 (text) and qmsgAttach was stripped, leaving the receiver no signal to
// render a file-quote bubble. Post-fix: qmsgType=46 (share.file) and Attach
// rides through verbatim so the client can resolve the source.
func TestSendMessage_WithQuote_DM_FileIncludesAttach(t *testing.T) {
	t.Parallel()
	srv, cap := captureServer(t, "1001", 0)
	sess := newQuoteTestSession(t, srv)

	attachJSON := `{"title":"BaoCao_DonHang_20260523.xlsx","href":"https://files.zalo.me/xy","fileExt":"xlsx","totalSize":6021,"checksum":"abc"}`
	q := &SendMessageQuote{
		OwnerID: "owner-111", MsgID: "9876543210", CliMsgID: "1709300000123",
		MsgType: "share.file", Msg: "", Attach: attachJSON, TS: "1709300000",
	}
	_, err := SendMessage(context.Background(), sess, "user-1", ThreadTypeUser, "reply", q)
	if err != nil {
		t.Fatalf("SendMessage: %v", err)
	}
	payload := decryptRequestParams(t, (*cap)[0].body)
	if v, ok := payload["qmsgType"]; !ok || v.(float64) != 46 {
		t.Errorf("qmsgType = %v, want 46 (share.file)", v)
	}
	if v, ok := payload["qmsgAttach"]; !ok || v != attachJSON {
		t.Errorf("DM qmsgAttach = %v, want raw JSON string (file metadata)", v)
	}
	if v, ok := payload["qmsg"]; !ok || v != "" {
		t.Errorf("qmsg = %v, want empty string for non-text quote", v)
	}
}

// Symmetry guard: group file quote must still ship qmsgAttach post-fix (it
// did pre-fix unconditionally for groups; ensure the new gating preserves it).
func TestSendMessage_WithQuote_Group_FileIncludesAttach(t *testing.T) {
	t.Parallel()
	srv, cap := captureServer(t, "1001", 0)
	sess := newQuoteTestSession(t, srv)

	attachJSON := `{"title":"BaoCao.xlsx","href":"https://files.zalo.me/x","fileExt":"xlsx"}`
	q := &SendMessageQuote{
		OwnerID: "owner-111", MsgID: "9876543210", CliMsgID: "1709300000123",
		MsgType: "share.file", Msg: "", Attach: attachJSON, TS: "1709300000",
	}
	_, err := SendMessage(context.Background(), sess, "group-abc", ThreadTypeGroup, "reply", q)
	if err != nil {
		t.Fatalf("SendMessage: %v", err)
	}
	payload := decryptRequestParams(t, (*cap)[0].body)
	if v, ok := payload["qmsgType"]; !ok || v.(float64) != 46 {
		t.Errorf("qmsgType = %v, want 46 (share.file)", v)
	}
	if v, ok := payload["qmsgAttach"]; !ok || v != attachJSON {
		t.Errorf("group qmsgAttach = %v, want raw JSON string", v)
	}
}

// Reproduces zca-js upstream issue #143 "Trả lời hình ảnh" on the DM path.
// Photo quote on DM must include qmsgAttach + qmsgType=32 so the receiver
// renders the image-quote bubble instead of "unsupported version".
func TestSendMessage_WithQuote_DM_PhotoIncludesAttach(t *testing.T) {
	t.Parallel()
	srv, cap := captureServer(t, "1001", 0)
	sess := newQuoteTestSession(t, srv)

	attachJSON := `{"hdUrl":"https://photos.zalo.me/abc.jpg","width":1080,"height":720,"title":"IMG_001.jpg"}`
	q := &SendMessageQuote{
		OwnerID: "owner-111", MsgID: "9876543210", CliMsgID: "1709300000123",
		MsgType: "chat.photo", Msg: "", Attach: attachJSON, TS: "1709300000",
	}
	_, err := SendMessage(context.Background(), sess, "user-1", ThreadTypeUser, "reply", q)
	if err != nil {
		t.Fatalf("SendMessage: %v", err)
	}
	payload := decryptRequestParams(t, (*cap)[0].body)
	if v, ok := payload["qmsgType"]; !ok || v.(float64) != 32 {
		t.Errorf("qmsgType = %v, want 32 (chat.photo)", v)
	}
	if v, ok := payload["qmsgAttach"]; !ok || v != attachJSON {
		t.Errorf("DM qmsgAttach = %v, want raw JSON string (photo metadata)", v)
	}
}

// FromInboundQuote must prefer the raw inbound MsgType string when populated
// (set by handlers.buildSelfQuoteMetadata) over the classifyQuoteMsgType(int)
// fallback. Without this preference, share.file collapses to chat.text and
// qmsgType ships as 1 — the original failure mode.
func TestFromInboundQuote_PrefersMsgTypeString(t *testing.T) {
	t.Parallel()
	q := &TQuote{
		OwnerID:     json.Number("111"),
		GlobalMsgID: json.Number("9876543210"),
		CliMsgID:    json.Number("1709300000123"),
		CliMsgType:  1, // legacy fallback would say "chat.text"
		MsgType:     "share.file",
		Msg:         "",
		Attach:      `{"title":"x.pdf"}`,
		TS:          json.Number("1709300000"),
	}
	smq := FromInboundQuote(q)
	if smq == nil {
		t.Fatal("FromInboundQuote returned nil for non-nil input")
	}
	if smq.MsgType != "share.file" {
		t.Errorf("MsgType = %q, want %q (string preference over CliMsgType=1 fallback)", smq.MsgType, "share.file")
	}
	// Empty MsgType still falls back via classifyQuoteMsgType.
	q.MsgType = ""
	q.CliMsgType = 2
	smq = FromInboundQuote(q)
	if smq.MsgType != "chat.photo" {
		t.Errorf("fallback MsgType = %q, want chat.photo from CliMsgType=2", smq.MsgType)
	}
}

func TestShouldSendQmsgAttach(t *testing.T) {
	t.Parallel()
	cases := []struct {
		msgType    string
		threadType ThreadType
		want       bool
	}{
		{"chat.text", ThreadTypeUser, false},
		{"chat.text", ThreadTypeGroup, true},
		{"webchat", ThreadTypeUser, false},
		{"", ThreadTypeUser, false},
		{"share.file", ThreadTypeUser, true},
		{"share.file", ThreadTypeGroup, true},
		{"chat.photo", ThreadTypeUser, true},
		{"chat.video.msg", ThreadTypeUser, true},
		{"chat.voice", ThreadTypeUser, true},
		{"chat.gif", ThreadTypeUser, true},
		{"chat.location.new", ThreadTypeUser, true},
	}
	for _, c := range cases {
		if got := shouldSendQmsgAttach(c.msgType, c.threadType); got != c.want {
			t.Errorf("shouldSendQmsgAttach(%q, %v) = %v, want %v", c.msgType, c.threadType, got, c.want)
		}
	}
}

func TestSendMessage_QuoteServerError_WrapsErrQuoteRejected(t *testing.T) {
	t.Parallel()
	// Non-zero error_code at the OUTER envelope (no nested encryption needed).
	srv, _ := captureServer(t, "", 999)
	sess := newQuoteTestSession(t, srv)

	q := &SendMessageQuote{OwnerID: "x", MsgID: "y", MsgType: "chat.text", Msg: "z"}
	_, err := SendMessage(context.Background(), sess, "user-1", ThreadTypeUser, "reply", q)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, ErrQuoteRejected) {
		t.Errorf("err = %v, want errors.Is(err, ErrQuoteRejected)", err)
	}
}

// TestSendMessage_AuthErrorDoesNotWrapErrQuoteRejected: -100 (session expired)
// must NOT be wrapped as ErrQuoteRejected — otherwise Phase 4's fallback
// would silently retry with the same expired session instead of surfacing
// the reauth requirement.
func TestSendMessage_AuthErrorDoesNotWrapErrQuoteRejected(t *testing.T) {
	t.Parallel()
	srv, _ := captureServer(t, "", -100)
	sess := newQuoteTestSession(t, srv)

	q := &SendMessageQuote{OwnerID: "x", MsgID: "y", MsgType: "chat.text", Msg: "z"}
	_, err := SendMessage(context.Background(), sess, "user-1", ThreadTypeUser, "reply", q)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if errors.Is(err, ErrQuoteRejected) {
		t.Errorf("auth error must NOT wrap ErrQuoteRejected; got %v", err)
	}
}

// Inner-envelope error 114 ("Tham số không hợp lệ") rides under a clean
// outer envelope (error_code=0). Without typed-error wrapping in
// decryptDataField the channel layer's silent retry never fired and the
// reply silently failed — verified live on v3.18.3.
func TestSendMessage_InnerEnvelopeError_WrapsErrQuoteRejected(t *testing.T) {
	t.Parallel()
	srv, _ := innerErrorServer(t, 114, "Tham số không hợp lệ")
	sess := newQuoteTestSession(t, srv)

	q := &SendMessageQuote{OwnerID: "x", MsgID: "y", MsgType: "chat.text", Msg: "z"}
	_, err := SendMessage(context.Background(), sess, "user-1", ThreadTypeUser, "reply", q)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, ErrQuoteRejected) {
		t.Errorf("err = %v, want errors.Is(err, ErrQuoteRejected)", err)
	}
	var innerErr *InnerEnvelopeError
	if !errors.As(err, &innerErr) || innerErr.Code != 114 {
		t.Errorf("err must carry InnerEnvelopeError{Code:114}, got %v", err)
	}
}

// Same wire shape as the 114 case but with -100 (session expired). Auth /
// session-expired codes in the inner envelope must still bubble unwrapped so
// the channel surfaces reauth instead of silently retrying with a dead
// session.
func TestSendMessage_InnerEnvelopeAuthError_DoesNotWrap(t *testing.T) {
	t.Parallel()
	srv, _ := innerErrorServer(t, -100, "session expired")
	sess := newQuoteTestSession(t, srv)

	q := &SendMessageQuote{OwnerID: "x", MsgID: "y", MsgType: "chat.text", Msg: "z"}
	_, err := SendMessage(context.Background(), sess, "user-1", ThreadTypeUser, "reply", q)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if errors.Is(err, ErrQuoteRejected) {
		t.Errorf("inner auth error must NOT wrap ErrQuoteRejected; got %v", err)
	}
}

// Inner-envelope errors on plain (no-quote) sends must NOT be wrapped as
// ErrQuoteRejected — the retry path is meaningless without a quote.
func TestSendMessage_InnerEnvelopeError_NoQuote_DoesNotWrap(t *testing.T) {
	t.Parallel()
	srv, _ := innerErrorServer(t, 114, "Tham số không hợp lệ")
	sess := newQuoteTestSession(t, srv)

	_, err := SendMessage(context.Background(), sess, "user-1", ThreadTypeUser, "hi", nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if errors.Is(err, ErrQuoteRejected) {
		t.Errorf("plain send must not wrap ErrQuoteRejected; got %v", err)
	}
}

// innerErrorServer returns a test server whose responses pass the OUTER
// envelope check (error_code:0) but encrypt an INNER envelope carrying the
// given non-zero code. Mirrors how Zalo /quote rejected requests in trace
// captured on 2026-05-23.
func innerErrorServer(t *testing.T, innerCode int, innerMsg string) (*httptest.Server, *[]captured) {
	t.Helper()
	innerJSON := []byte(`{"error_code":` + strconv.Itoa(innerCode) + `,"error_message":"` + innerMsg + `","data":null}`)
	key, _ := base64.StdEncoding.DecodeString(testKeyB64)
	enc, err := EncodeAESCBC(key, string(innerJSON), false)
	if err != nil {
		t.Fatalf("encode inner: %v", err)
	}
	outer, _ := json.Marshal(map[string]any{"error_code": 0, "data": enc})

	var cap []captured
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		cap = append(cap, captured{path: r.URL.Path, body: body})
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(outer)
	}))
	t.Cleanup(srv.Close)
	return srv, &cap
}

func TestSendMessage_NoQuote_ErrorBubblesUnwrapped(t *testing.T) {
	t.Parallel()
	srv, _ := captureServer(t, "", 999)
	sess := newQuoteTestSession(t, srv)

	_, err := SendMessage(context.Background(), sess, "user-1", ThreadTypeUser, "hi", nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if errors.Is(err, ErrQuoteRejected) {
		t.Errorf("err must not wrap ErrQuoteRejected when no quote sent; got %v", err)
	}
}

func TestFromInboundQuote_RoundTrip(t *testing.T) {
	t.Parallel()
	propExt := json.RawMessage(`{"size":18}`)
	original := &TQuote{
		OwnerID:     "111",
		CliMsgID:    json.Number("1709300000123"),
		GlobalMsgID: json.Number("9876543210"),
		CliMsgType:  1,
		TS:          json.Number("1709300000"),
		Msg:         "hello",
		Attach:      `{"x":"y"}`,
		FromD:       "789",
		TTL:         0,
		PropertyExt: propExt,
	}
	q := FromInboundQuote(original)
	if q == nil {
		t.Fatal("FromInboundQuote returned nil for non-nil input")
	}
	if q.OwnerID != "111" || q.MsgID != "9876543210" || q.CliMsgID != "1709300000123" ||
		q.MsgType != "chat.text" || q.Msg != "hello" || q.Attach != `{"x":"y"}` ||
		q.TS != "1709300000" || q.TTL != 0 {
		t.Errorf("mapping mismatch: %+v", q)
	}
	// PropertyExt roundtrip: marshal → unmarshal → compare canonical form.
	blob, err := json.Marshal(q)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var back SendMessageQuote
	if err := json.Unmarshal(blob, &back); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !bytes.Equal(q.PropertyExt, back.PropertyExt) {
		t.Errorf("PropertyExt drift: got=%s want=%s", back.PropertyExt, q.PropertyExt)
	}
}

func TestFromInboundQuote_NilReturnsNil(t *testing.T) {
	t.Parallel()
	if got := FromInboundQuote(nil); got != nil {
		t.Errorf("FromInboundQuote(nil) = %v, want nil", got)
	}
}

func TestClassifyQuoteMsgType(t *testing.T) {
	t.Parallel()
	cases := map[int]string{
		1:   "chat.text",
		2:   "chat.photo",
		3:   "chat.sticker",
		5:   "chat.voice",
		19:  "chat.todo",
		999: "chat.text", // unknown falls back
	}
	for in, want := range cases {
		if got := classifyQuoteMsgType(in); got != want {
			t.Errorf("classifyQuoteMsgType(%d) = %q, want %q", in, got, want)
		}
	}
}

func TestImageDimensions(t *testing.T) {
	tests := []struct {
		name  string
		data  []byte
		wantW int
		wantH int
	}{
		{
			name:  "valid PNG 200x100",
			data:  makePNG(200, 100),
			wantW: 200,
			wantH: 100,
		},
		{
			name:  "valid PNG 1x1",
			data:  makePNG(1, 1),
			wantW: 1,
			wantH: 1,
		},
		{
			name:  "valid JPEG 50x30",
			data:  makeJPEG(50, 30),
			wantW: 50,
			wantH: 30,
		},
		{
			name:  "not an image",
			data:  []byte("hello world"),
			wantW: 0,
			wantH: 0,
		},
		{
			name:  "empty",
			data:  nil,
			wantW: 0,
			wantH: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w, h := imageDimensions(tt.data)
			if w != tt.wantW || h != tt.wantH {
				t.Errorf("imageDimensions() = (%d, %d), want (%d, %d)", w, h, tt.wantW, tt.wantH)
			}
		})
	}
}

func makePNG(width, height int) []byte {
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	img.Set(0, 0, color.RGBA{R: 255})
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return buf.Bytes()
}

func makeJPEG(width, height int) []byte {
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	img.Set(0, 0, color.RGBA{R: 255})
	var buf bytes.Buffer
	_ = jpeg.Encode(&buf, img, nil)
	return buf.Bytes()
}

func TestIsImageFile(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{"photo.jpg", true},
		{"photo.JPG", true},
		{"photo.jpeg", true},
		{"photo.png", true},
		{"photo.webp", true},
		{"photo.PNG", true},
		{"file.md", false},
		{"file.pdf", false},
		{"file.gif", false},
		{"file.mp4", false},
		{"file.txt", false},
		{"noext", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			if got := IsImageFile(tt.path); got != tt.want {
				t.Errorf("IsImageFile(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

func TestBuildMultipartBody(t *testing.T) {
	data := []byte("hello world")
	reader, contentType, err := buildMultipartBody("chunkContent", "test.png", data)
	if err != nil {
		t.Fatalf("buildMultipartBody() error: %v", err)
	}

	if contentType == "" {
		t.Fatal("content type is empty")
	}
	if !bytes.Contains([]byte(contentType), []byte("multipart/form-data")) {
		t.Errorf("content type %q does not contain multipart/form-data", contentType)
	}

	body, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("read body error: %v", err)
	}

	if !bytes.Contains(body, data) {
		t.Error("body does not contain file data")
	}
	if !bytes.Contains(body, []byte("chunkContent")) {
		t.Error("body does not contain field name")
	}
	if !bytes.Contains(body, []byte("test.png")) {
		t.Error("body does not contain filename")
	}
}
