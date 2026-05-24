package protocol

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

type GroupMember struct {
	UID         string `json:"uid"`
	DisplayName string `json:"display_name"`
	Avatar      string `json:"avatar,omitempty"`
}

// FetchGroupMembers hits zca-js's /api/group/getmem-v2 endpoint.
func FetchGroupMembers(ctx context.Context, sess *Session, groupID string) ([]GroupMember, error) {
	if sess == nil {
		return nil, fmt.Errorf("zalo_personal: nil session")
	}
	baseURL := getServiceURL(sess, "group")
	if baseURL == "" {
		return nil, fmt.Errorf("zalo_personal: no group service URL")
	}

	payload := map[string]any{
		"grid":    groupID,
		"imei":    sess.IMEI,
		"members": []string{},
	}
	encData, err := encryptPayload(sess, payload)
	if err != nil {
		return nil, fmt.Errorf("zalo_personal: encrypt members payload: %w", err)
	}
	reqURL := makeURL(sess, baseURL+"/api/group/getmem-v2", nil, true)
	form := buildFormBody(map[string]string{"params": encData})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, form)
	if err != nil {
		return nil, err
	}
	setDefaultHeaders(req, sess)

	resp, err := sess.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("zalo_personal: fetch members: %w", err)
	}
	defer resp.Body.Close()

	var envelope Response[*string]
	if err := readJSON(resp, &envelope); err != nil {
		return nil, fmt.Errorf("zalo_personal: parse members response: %w", err)
	}
	if envelope.ErrorCode != 0 {
		return nil, fmt.Errorf("zalo_personal: members error code %d", envelope.ErrorCode)
	}
	if envelope.Data == nil {
		return nil, nil
	}
	plain, err := decryptDataField(sess, *envelope.Data)
	if err != nil {
		return nil, fmt.Errorf("zalo_personal: decrypt members: %w", err)
	}
	var result struct {
		Members []struct {
			UID         string `json:"uid"`
			DisplayName string `json:"dName"`
			Avatar      string `json:"avatar"`
		} `json:"members"`
	}
	if err := json.Unmarshal(plain, &result); err != nil {
		return nil, fmt.Errorf("zalo_personal: parse members: %w", err)
	}
	out := make([]GroupMember, 0, len(result.Members))
	for _, m := range result.Members {
		out = append(out, GroupMember{UID: m.UID, DisplayName: m.DisplayName, Avatar: m.Avatar})
	}
	return out, nil
}
