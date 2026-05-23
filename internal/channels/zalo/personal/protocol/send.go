package protocol

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"
)

// Endpoint suffixes appended to the chat / group service base URL. The /quote
// variants REPLACE the /sms or /sendmsg suffix rather than appending to it —
// matches zca-js sendMessage.ts:290-302 where pathname + "/quote" is the
// alternative branch to pathname + "/sms" / "/sendmsg".
//
// VERIFY (Phase 3 preflight): no live HTTP capture is attached yet. If the
// /quote endpoint 404s in dev, the alternative is /api/message/sms/quote and
// /api/group/sendmsg/quote — try those and update the constants. The
// FamilyPayload-style fallback in Phase 4 will mask a wrong URL as a quote
// rejection, so the failure mode is "quotes silently drop" rather than crash.
const (
	apiPathDM         = "/api/message/sms"
	apiPathGroup      = "/api/group/sendmsg"
	apiPathDMQuote    = "/api/message/quote"
	apiPathGroupQuote = "/api/group/quote"
)

// ErrQuoteRejected signals that Zalo rejected the message specifically because
// of the attached quote (e.g. source deleted, expired, malformed). Callers
// (Phase 4's sendChunkWithQuoteFallback) should retry once without the quote.
//
// The current heuristic returns ErrQuoteRejected on ANY non-zero error_code
// from a /quote endpoint EXCEPT the codes listed in nonQuoteErrorCodes. This
// is intentionally permissive at the start — Zalo doesn't document error
// codes for quote rejections. As real codes are observed in the warn-log
// fallback stream, tighten the matching set here.
var ErrQuoteRejected = errors.New("zalo_personal: quote rejected by server")

// nonQuoteErrorCodes lists error_code values that must NOT be wrapped as
// ErrQuoteRejected — these are auth / rate-limit / encryption / generic
// failures that should bubble up so callers can reauth or back off rather
// than silently retrying without the quote.
//
// Starter set inferred from common Zalo error conventions and zalo-oa
// observations; tighten / extend as real codes surface via the
// zalo_personal.quote.fallback_no_quote warn-log stream.
var nonQuoteErrorCodes = map[int]bool{
	-100:  true, // session expired / auth invalid
	-114:  true, // session token invalid (zalo-oa equivalent: -216)
	-201:  true, // params invalid / encryption error (NOT a quote rejection)
	12010: true, // per-user rate limit (zalo-oa pattern)
}

// SendMessageQuote carries the fields needed to attach a quote to an outbound
// message via Zalo's /quote endpoint. Built from an inbound TQuote via
// FromInboundQuote, or deserialized from outbound metadata in Phase 4.
type SendMessageQuote struct {
	OwnerID     string          `json:"ownerId"`
	MsgID       string          `json:"msgId"`    // = original TQuote.GlobalMsgID
	CliMsgID    string          `json:"cliMsgId"`
	MsgType     string          `json:"msgType"`  // string form: chat.text, chat.photo, etc.
	Msg         string          `json:"msg"`      // quoted text body — qmsg payload field
	Attach      string          `json:"attach"`   // quoted attachment metadata as JSON string — qmsgAttach
	TS          string          `json:"ts"`
	TTL         int             `json:"ttl"`
	PropertyExt json.RawMessage `json:"propertyExt,omitempty"`
}

// FromInboundQuote builds a SendMessageQuote from the inbound TQuote received
// earlier. Maps TQuote.GlobalMsgID → MsgID; copies Msg/Attach/PropertyExt
// verbatim (Attach stays an opaque JSON string — Zalo's wire shape, we don't
// unpack it). Returns nil when the input is nil so callers can chain.
func FromInboundQuote(q *TQuote) *SendMessageQuote {
	if q == nil {
		return nil
	}
	return &SendMessageQuote{
		OwnerID:     q.OwnerID,
		MsgID:       q.GlobalMsgID.String(),
		CliMsgID:    q.CliMsgID.String(),
		MsgType:     classifyQuoteMsgType(q.CliMsgType),
		Msg:         q.Msg,
		Attach:      q.Attach,
		TS:          q.TS.String(),
		TTL:         q.TTL,
		PropertyExt: q.PropertyExt,
	}
}

// classifyQuoteMsgType maps zca-js's numeric cliMsgType to the string form
// Zalo's /quote endpoint expects. Unknown types fall back to "chat.text" with
// a warn log so misclassified quotes surface in logs rather than silently
// rejecting server-side (which would just trigger the fallback path with no
// indication WHY).
func classifyQuoteMsgType(cliMsgType int) string {
	switch cliMsgType {
	case 1:
		return "chat.text"
	case 2:
		return "chat.photo"
	case 3:
		return "chat.sticker"
	case 5:
		return "chat.voice"
	case 19:
		return "chat.todo"
	default:
		slog.Warn("zalo_personal.quote.unknown_msgtype",
			"cli_msg_type", cliMsgType,
			"hint", "falling back to chat.text — quote may be rejected by server; extend classifyQuoteMsgType")
		return "chat.text"
	}
}

// SendMessage sends a text message to a user or group. When quote is non-nil
// the request routes to Zalo's /quote endpoint and carries the encrypted
// qmsg* parameters; otherwise the existing /sms or /sendmsg behavior is
// preserved.
//
// threadID: user UID (DM) or group ID (group).
func SendMessage(
	ctx context.Context,
	sess *Session,
	threadID string,
	threadType ThreadType,
	text string,
	quote *SendMessageQuote,
) (string, error) {
	if text == "" {
		return "", fmt.Errorf("zalo_personal: message text cannot be empty")
	}

	// Note: zca-js's prepareQMSG validates `group.poll` and `webchat` quote
	// types pre-send, but classifyQuoteMsgType never emits those strings from
	// any known inbound cliMsgType, so guarding here would be unreachable for
	// the only in-tree caller (FromInboundQuote). The server still rejects
	// these payloads — caught by ErrQuoteRejected + fallback retry below.

	serviceKey := "chat"
	apiPath := apiPathDM
	if threadType == ThreadTypeGroup {
		serviceKey = "group"
		apiPath = apiPathGroup
	}
	if quote != nil {
		if threadType == ThreadTypeGroup {
			apiPath = apiPathGroupQuote
		} else {
			apiPath = apiPathDMQuote
		}
	}

	baseURL := getServiceURL(sess, serviceKey)
	if baseURL == "" {
		return "", fmt.Errorf("zalo_personal: no service URL for %s", serviceKey)
	}

	// Build payload
	payload := map[string]any{
		"message":  text,
		"clientId": time.Now().UnixMilli(),
		"ttl":      0,
	}
	if threadType == ThreadTypeGroup {
		payload["grid"] = threadID
		payload["visibility"] = 0
	} else {
		payload["toid"] = threadID
		payload["imei"] = sess.IMEI
	}
	if quote != nil {
		// Field names mirror zca-js's encrypted /quote payload. VERIFY against
		// captured wire traffic — names may differ (e.g. qmsgFromUid). Fallback
		// path in Phase 4 catches misnamed-field rejections as ErrQuoteRejected.
		payload["qmsgOwner"] = quote.OwnerID
		payload["qmsgId"] = quote.MsgID
		payload["qmsgCliId"] = quote.CliMsgID
		payload["qmsgType"] = quote.MsgType
		payload["qmsg"] = quote.Msg
		payload["qmsgAttach"] = quote.Attach
		payload["qmsgTs"] = quote.TS
		payload["qmsgTtl"] = quote.TTL
	}

	// Encrypt payload with session secret key
	encData, err := encryptPayload(sess, payload)
	if err != nil {
		return "", fmt.Errorf("zalo_personal: encrypt send payload: %w", err)
	}

	// Build URL with standard params
	sendURL := makeURL(sess, baseURL+apiPath, map[string]any{"nretry": 0}, true)

	// POST form-encoded
	form := buildFormBody(map[string]string{"params": encData})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, sendURL, form)
	if err != nil {
		return "", err
	}
	setDefaultHeaders(req, sess)

	resp, err := sess.Client.Do(req)
	if err != nil {
		return "", fmt.Errorf("zalo_personal: send message: %w", err)
	}
	defer resp.Body.Close()

	// Send response is encrypted: {"error_code":0, "data":"<encrypted>"}
	var envelope Response[*string]
	if err := readJSON(resp, &envelope); err != nil {
		return "", fmt.Errorf("zalo_personal: parse send response: %w", err)
	}
	if envelope.ErrorCode != 0 {
		baseErr := fmt.Errorf("zalo_personal: send error code %d", envelope.ErrorCode)
		if quote != nil && !nonQuoteErrorCodes[envelope.ErrorCode] {
			// Wrap so Phase 4's fallback can detect via errors.Is.
			return "", fmt.Errorf("%w: %w", ErrQuoteRejected, baseErr)
		}
		return "", baseErr
	}
	if envelope.Data == nil {
		return "", nil
	}

	plain, err := decryptDataField(sess, *envelope.Data)
	if err != nil {
		return "", fmt.Errorf("zalo_personal: decrypt send response: %w", err)
	}

	var result struct {
		MsgID json.Number `json:"msgId"`
	}
	if err := json.Unmarshal(plain, &result); err != nil {
		return "", fmt.Errorf("zalo_personal: parse send result: %w", err)
	}

	return result.MsgID.String(), nil
}

// SendTypingEvent sends a typing indicator to a user or group.
// Zalo typing lasts ~5s server-side; callers should re-send every 3-4s.
func SendTypingEvent(ctx context.Context, sess *Session, threadID string, threadType ThreadType) error {
	serviceKey := "chat"
	apiPath := "/api/message/typing"
	if threadType == ThreadTypeGroup {
		serviceKey = "group"
		apiPath = "/api/group/typing"
	}

	baseURL := getServiceURL(sess, serviceKey)
	if baseURL == "" {
		return fmt.Errorf("zalo_personal: no service URL for %s", serviceKey)
	}

	payload := map[string]any{"imei": sess.IMEI}
	if threadType == ThreadTypeGroup {
		payload["grid"] = threadID
	} else {
		payload["toid"] = threadID
		payload["destType"] = 3 // DestTypeUser
	}

	encData, err := encryptPayload(sess, payload)
	if err != nil {
		return fmt.Errorf("zalo_personal: encrypt typing payload: %w", err)
	}

	typingURL := makeURL(sess, baseURL+apiPath, nil, true)
	form := buildFormBody(map[string]string{"params": encData})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, typingURL, form)
	if err != nil {
		return err
	}
	setDefaultHeaders(req, sess)

	resp, err := sess.Client.Do(req)
	if err != nil {
		return fmt.Errorf("zalo_personal: send typing: %w", err)
	}
	defer resp.Body.Close()

	var envelope Response[any]
	if err := readJSON(resp, &envelope); err != nil {
		return fmt.Errorf("zalo_personal: parse typing response: %w", err)
	}
	if envelope.ErrorCode != 0 {
		return fmt.Errorf("zalo_personal: typing error code %d", envelope.ErrorCode)
	}
	return nil
}

// getServiceURL extracts a service base URL from LoginInfo.
func getServiceURL(sess *Session, service string) string {
	if sess.LoginInfo == nil {
		return ""
	}
	var urls []string
	switch service {
	case "chat":
		urls = sess.LoginInfo.ZpwServiceMapV3.Chat
	case "group":
		urls = sess.LoginInfo.ZpwServiceMapV3.Group
	case "file":
		urls = sess.LoginInfo.ZpwServiceMapV3.File
	case "profile":
		urls = sess.LoginInfo.ZpwServiceMapV3.Profile
	case "group_poll":
		urls = sess.LoginInfo.ZpwServiceMapV3.GroupPoll
	case "reaction":
		urls = sess.LoginInfo.ZpwServiceMapV3.Reaction
	}
	if len(urls) == 0 {
		return ""
	}
	return urls[0]
}

// encryptPayload encrypts a JSON payload with the session's secret key via AES-CBC.
func encryptPayload(sess *Session, payload map[string]any) (string, error) {
	blob, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	key, err := base64.StdEncoding.DecodeString(sess.SecretKey)
	if err != nil {
		return "", fmt.Errorf("decode secret key: %w", err)
	}
	return EncodeAESCBC(key, string(blob), false)
}
