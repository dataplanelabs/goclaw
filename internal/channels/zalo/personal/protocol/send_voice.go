package protocol

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"
)

// SendVoice uploads the audio file via the existing chunked file-upload path
// then sends a voice message referencing the resulting CDN URL. The endpoint
// is /api/message/forward (user) or /api/group/forward (group); msgType=3
// signals voice on the outbound API (distinct from inbound clientMsgType=31).
//
// ttlMs of 0 = no TTL. The audio should be raw ADTS AAC (AAC-LC, mono, 44.1kHz)
// to match Zalo's own voice messages — that format plays on both mobile and
// desktop. Callers should normalize via media.NormalizeAudio(…, "aac") first.
func SendVoice(ctx context.Context, sess *Session, ln *Listener, threadID string, threadType ThreadType, filePath string, ttlMs int) (string, error) {
	upload, err := UploadFile(ctx, sess, ln, threadID, threadType, filePath)
	if err != nil {
		return "", fmt.Errorf("zalo_personal: voice upload: %w", err)
	}
	if upload.FileURL == "" {
		return "", fmt.Errorf("zalo_personal: voice upload returned empty file URL")
	}

	fileServiceURL := getServiceURL(sess, "file")
	if fileServiceURL == "" {
		return "", fmt.Errorf("zalo_personal: no file service URL")
	}

	msgInfo := map[string]any{
		"voiceUrl": upload.FileURL,
		"m4aUrl":   upload.FileURL,
		"fileSize": upload.TotalSize,
	}
	msgInfoBytes, err := json.Marshal(msgInfo)
	if err != nil {
		return "", fmt.Errorf("zalo_personal: encode voice msgInfo: %w", err)
	}

	params := map[string]any{
		"ttl":      ttlMs,
		"zsource":  -1,
		"msgType":  3,
		"clientId": strconv.FormatInt(time.Now().UnixMilli(), 10),
		"imei":     sess.IMEI,
		"msgInfo":  string(msgInfoBytes),
	}
	pathPrefix := "/api/message/"
	if threadType == ThreadTypeGroup {
		params["grid"] = threadID
		params["visibility"] = 0
		pathPrefix = "/api/group/"
	} else {
		params["toId"] = threadID
	}

	encParams, err := encryptPayload(sess, params)
	if err != nil {
		return "", fmt.Errorf("zalo_personal: encrypt voice send params: %w", err)
	}

	sendURL := makeURL(sess, fileServiceURL+pathPrefix+"forward", map[string]any{"nretry": 0}, true)
	form := buildFormBody(map[string]string{"params": encParams})

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, sendURL, form)
	if err != nil {
		return "", err
	}
	setDefaultHeaders(req, sess)

	resp, err := sess.Client.Do(req)
	if err != nil {
		return "", fmt.Errorf("zalo_personal: send voice: %w", err)
	}
	defer resp.Body.Close()

	var envelope Response[*string]
	if err := readJSON(resp, &envelope); err != nil {
		return "", fmt.Errorf("zalo_personal: parse voice send response: %w", err)
	}
	if envelope.ErrorCode != 0 {
		return "", fmt.Errorf("zalo_personal: voice send error code %d", envelope.ErrorCode)
	}
	if envelope.Data == nil {
		return "", nil
	}
	plain, err := decryptDataField(sess, *envelope.Data)
	if err != nil {
		return "", fmt.Errorf("zalo_personal: decrypt voice send response: %w", err)
	}

	var result struct {
		MsgID json.Number `json:"msgId"`
	}
	if err := json.Unmarshal(plain, &result); err != nil {
		return "", fmt.Errorf("zalo_personal: parse voice send result: %w", err)
	}
	return result.MsgID.String(), nil
}
