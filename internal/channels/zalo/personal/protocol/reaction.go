package protocol

// Reaction outbound + shared types. Inbound decoding lives in
// listener_reactions.go alongside the cmd=612/610/611 switch handlers.

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"
)

const (
	apiPathReactionDM    = "/api/message/reaction"
	apiPathReactionGroup = "/api/group/reaction"
	reactionServiceKey   = "reaction"
)

// ReactionDest is the target of an AddReaction call.
type ReactionDest struct {
	MsgID    string     // global message ID (TMessage.MsgID)
	CliMsgID string     // client message ID (TMessage.CliMsgID)
	ThreadID string     // user UID (DM) or group ID
	Type     ThreadType // ThreadTypeUser or ThreadTypeGroup
}

// ReactionEvent is the parsed inbound reaction emitted by the listener for
// cmd=612 (real-time) and cmd=610/611 (historical replay).
type ReactionEvent struct {
	MsgID      string
	CliMsgID   string
	ThreadID   string
	ThreadType ThreadType
	UIDFrom    string
	DName      string
	Code       string // rIcon — Zalo reaction code; "" = removed
	RType      int
	Source     int
	Timestamp  int64 // server ts in ms
	IsSelf     bool  // UIDFrom matches the session's own UID
	IsHistoric bool  // true for cmd=610/611 (replay on reconnect)
}

// AddReaction adds (or removes via empty code) a reaction. code must be a
// valid catalog string from reactions_catalog.go — callers translate
// unicode/English via ResolveReactionCode first. Returns the server-issued
// message ID list (often the same target msgID).
func AddReaction(ctx context.Context, sess *Session, dest ReactionDest, code string) ([]int64, error) {
	if dest.MsgID == "" || dest.CliMsgID == "" {
		return nil, fmt.Errorf("zalo_personal: react requires msgId and cliMsgId")
	}
	if dest.ThreadID == "" {
		return nil, fmt.Errorf("zalo_personal: react requires threadId")
	}

	meta := LookupReactionMeta(code)

	// Inner uses numeric IDs (matches how Zalo echoes them in captures).
	// ParseInt errors fall through to 0; the outer string IDs still carry truth.
	gMsgID, _ := strconv.ParseInt(dest.MsgID, 10, 64)
	cMsgID, _ := strconv.ParseInt(dest.CliMsgID, 10, 64)
	inner := map[string]any{
		"rMsg": []map[string]any{
			{"gMsgID": gMsgID, "cMsgID": cMsgID, "msgType": 1},
		},
		"rIcon":  code,
		"rType":  meta.RType,
		"source": meta.Source,
	}
	innerJSON, err := json.Marshal(inner)
	if err != nil {
		return nil, fmt.Errorf("zalo_personal: marshal reaction inner: %w", err)
	}

	params := map[string]any{
		"react_list": []map[string]any{
			{
				"message":  string(innerJSON),
				"clientId": time.Now().UnixMilli(),
			},
		},
	}

	var apiPath string
	if dest.Type == ThreadTypeUser {
		apiPath = apiPathReactionDM
		params["toid"] = dest.ThreadID
	} else {
		apiPath = apiPathReactionGroup
		params["grid"] = dest.ThreadID
		params["imei"] = sess.IMEI
	}

	baseURL := getServiceURL(sess, reactionServiceKey)
	if baseURL == "" {
		return nil, fmt.Errorf("zalo_personal: no service URL for %s", reactionServiceKey)
	}
	encData, err := encryptPayload(sess, params)
	if err != nil {
		return nil, fmt.Errorf("zalo_personal: encrypt reaction: %w", err)
	}

	sendURL := makeURL(sess, baseURL+apiPath, nil, true)
	form := buildFormBody(map[string]string{"params": encData})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, sendURL, form)
	if err != nil {
		return nil, err
	}
	setDefaultHeaders(req, sess)

	resp, err := sess.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("zalo_personal: react request: %w", err)
	}
	defer resp.Body.Close()

	var envelope Response[*string]
	if err := readJSON(resp, &envelope); err != nil {
		return nil, fmt.Errorf("zalo_personal: parse react response: %w", err)
	}
	if envelope.ErrorCode != 0 {
		return nil, fmt.Errorf("zalo_personal: react error code %d: %s", envelope.ErrorCode, envelope.ErrorMessage)
	}
	if envelope.Data == nil || *envelope.Data == "" {
		return nil, nil
	}

	plain, err := decryptDataField(sess, *envelope.Data)
	if err != nil {
		return nil, fmt.Errorf("zalo_personal: decrypt react response: %w", err)
	}
	if len(plain) == 0 {
		return nil, nil
	}
	var raw struct {
		MsgIDs json.RawMessage `json:"msgIds"`
	}
	if err := json.Unmarshal(plain, &raw); err != nil {
		return nil, fmt.Errorf("zalo_personal: parse react result: %w", err)
	}
	return parseMsgIDs(raw.MsgIDs)
}

// parseMsgIDs handles Zalo's quirk: msgIds may be a JSON array or a JSON
// string containing the array. Both forms return the same []int64.
func parseMsgIDs(raw json.RawMessage) ([]int64, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var arr []int64
	if err := json.Unmarshal(raw, &arr); err == nil {
		return arr, nil
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return nil, err
	}
	if s == "" {
		return nil, nil
	}
	if err := json.Unmarshal([]byte(s), &arr); err != nil {
		return nil, err
	}
	return arr, nil
}
