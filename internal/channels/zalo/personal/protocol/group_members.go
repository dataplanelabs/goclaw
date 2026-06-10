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

// FetchGroupMembers retrieves member profiles via the 2-step flow zca-js uses:
//  1. /api/group/getmg-v2 — returns gridInfoMap[groupId].memVerList (member IDs)
//  2. /api/social/group/members — returns profiles for those IDs (50-batched)
//
// Member IDs sent to step 2 are suffixed "_0" per zca-js friend_pversion_map.
func FetchGroupMembers(ctx context.Context, sess *Session, groupID string) ([]GroupMember, error) {
	if sess == nil {
		return nil, fmt.Errorf("zalo_personal: nil session")
	}
	memberIDs, err := fetchGroupMemberIDs(ctx, sess, groupID)
	if err != nil {
		return nil, err
	}
	if len(memberIDs) == 0 {
		return nil, nil
	}
	return fetchMemberProfiles(ctx, sess, memberIDs)
}

func fetchGroupMemberIDs(ctx context.Context, sess *Session, groupID string) ([]string, error) {
	baseURL := getServiceURL(sess, "group")
	if baseURL == "" {
		return nil, fmt.Errorf("zalo_personal: no group service URL")
	}
	gridVerJSON, err := json.Marshal(map[string]int{groupID: 0})
	if err != nil {
		return nil, err
	}
	payload := map[string]any{"gridVerMap": string(gridVerJSON)}
	encData, err := encryptPayload(sess, payload)
	if err != nil {
		return nil, fmt.Errorf("zalo_personal: encrypt group info payload: %w", err)
	}
	reqURL := makeURL(sess, baseURL+"/api/group/getmg-v2", nil, true)
	form := buildFormBody(map[string]string{"params": encData})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, form)
	if err != nil {
		return nil, err
	}
	setDefaultHeaders(req, sess)
	resp, err := sess.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("zalo_personal: fetch group info: %w", err)
	}
	defer resp.Body.Close()
	var envelope Response[*string]
	if err := readJSON(resp, &envelope); err != nil {
		return nil, fmt.Errorf("zalo_personal: parse group info response: %w", err)
	}
	if envelope.ErrorCode != 0 || envelope.Data == nil {
		return nil, fmt.Errorf("zalo_personal: group info error code %d", envelope.ErrorCode)
	}
	plain, err := decryptDataField(sess, *envelope.Data)
	if err != nil {
		return nil, fmt.Errorf("zalo_personal: decrypt group info: %w", err)
	}
	var result struct {
		GridInfoMap map[string]struct {
			MemVerList []string `json:"memVerList"`
		} `json:"gridInfoMap"`
	}
	if err := json.Unmarshal(plain, &result); err != nil {
		return nil, fmt.Errorf("zalo_personal: parse group info: %w", err)
	}
	info, ok := result.GridInfoMap[groupID]
	if !ok {
		return nil, nil
	}
	out := make([]string, 0, len(info.MemVerList))
	for _, raw := range info.MemVerList {
		out = append(out, stripVersionSuffix(raw))
	}
	return out, nil
}

// stripVersionSuffix removes a trailing "_N" version suffix from a Zalo member
// ID ("<uid>_<ver>" or bare "<uid>").
func stripVersionSuffix(s string) string {
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == '_' {
			return s[:i]
		}
		if s[i] < '0' || s[i] > '9' {
			break
		}
	}
	return s
}

func fetchMemberProfiles(ctx context.Context, sess *Session, memberIDs []string) ([]GroupMember, error) {
	baseURL := getServiceURL(sess, "profile")
	if baseURL == "" {
		return nil, fmt.Errorf("zalo_personal: no profile service URL")
	}
	const batchSize = 50
	out := make([]GroupMember, 0, len(memberIDs))
	for i := 0; i < len(memberIDs); i += batchSize {
		end := min(i+batchSize, len(memberIDs))
		batch := memberIDs[i:end]
		versioned := make([]string, len(batch))
		for j, id := range batch {
			versioned[j] = id + "_0"
		}
		members, err := fetchProfilesBatch(ctx, sess, baseURL, versioned)
		if err != nil {
			return out, err
		}
		out = append(out, members...)
	}
	return out, nil
}

func fetchProfilesBatch(ctx context.Context, sess *Session, baseURL string, versionedIDs []string) ([]GroupMember, error) {
	payload := map[string]any{"friend_pversion_map": versionedIDs}
	encData, err := encryptPayload(sess, payload)
	if err != nil {
		return nil, fmt.Errorf("zalo_personal: encrypt members payload: %w", err)
	}
	reqURL := makeURL(sess, baseURL+"/api/social/group/members",
		map[string]any{"params": encData}, true)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, err
	}
	setDefaultHeaders(req, sess)
	resp, err := sess.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("zalo_personal: fetch member profiles: %w", err)
	}
	defer resp.Body.Close()
	var envelope Response[*string]
	if err := readJSON(resp, &envelope); err != nil {
		return nil, fmt.Errorf("zalo_personal: parse profiles response: %w", err)
	}
	if envelope.ErrorCode != 0 || envelope.Data == nil {
		return nil, fmt.Errorf("zalo_personal: profiles error code %d", envelope.ErrorCode)
	}
	plain, err := decryptDataField(sess, *envelope.Data)
	if err != nil {
		return nil, fmt.Errorf("zalo_personal: decrypt profiles: %w", err)
	}
	var result struct {
		Profiles map[string]struct {
			DisplayName string `json:"displayName"`
			ZaloName    string `json:"zaloName"`
			Avatar      string `json:"avatar"`
		} `json:"profiles"`
	}
	if err := json.Unmarshal(plain, &result); err != nil {
		return nil, fmt.Errorf("zalo_personal: parse profiles: %w", err)
	}
	out := make([]GroupMember, 0, len(result.Profiles))
	for memberID, prof := range result.Profiles {
		dn := prof.DisplayName
		if dn == "" {
			dn = prof.ZaloName
		}
		out = append(out, GroupMember{
			UID:         stripVersionSuffix(memberID),
			DisplayName: dn,
			Avatar:      prof.Avatar,
		})
	}
	return out, nil
}
