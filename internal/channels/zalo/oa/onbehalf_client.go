package oa

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strconv"
)

// ErrInvalidRefreshToken is the public sentinel surfaced by the polling
// worker so it can pause + alert "operator must re-consent". Wraps
// codeInvalidGrant under the hood; kept as a separate sentinel for
// caller ergonomics (errors.Is).
var ErrInvalidRefreshToken = errors.New("zalo_oa: refresh token invalid")

// OnBehalfClient is the typed wrapper over Zalo's /onbehalf/* endpoints.
// Returns full conversation regardless of whether the OA-side message was
// sent by the bot via API or typed by a human in the Manager app.
type OnBehalfClient struct {
	client   *Client
	tokenSrc func() string // returns current OA access token (refresh handled externally)
}

func NewOnBehalfClient(client *Client, tokenSrc func() string) *OnBehalfClient {
	return &OnBehalfClient{client: client, tokenSrc: tokenSrc}
}

// RecentChatEntry is one row from /onbehalf/listrecentchat.
type RecentChatEntry struct {
	UID         string `json:"uid"`
	LastMsgID   string `json:"last_msg_id"`
	LastMsgTime int64  `json:"last_msg_time"`
	UserName    string `json:"display_name,omitempty"`
}

// ConversationMessage is one row from /onbehalf/conversation. Field names
// are best-effort from SDK inspection; verify and revise after first real
// response captured in prod.
type ConversationMessage struct {
	MsgID string `json:"msg_id"`
	SrcID string `json:"src_id"`
	DstID string `json:"dst_id"`
	Type  string `json:"type"`
	Text  string `json:"message"`
	Time  int64  `json:"time"`
}

// ListRecentChat pages over recently-active chat partners.
func (c *OnBehalfClient) ListRecentChat(ctx context.Context, offset, count int) ([]RecentChatEntry, error) {
	tok := c.token()
	if tok == "" {
		return nil, ErrInvalidRefreshToken
	}
	q := url.Values{}
	q.Set("data", buildOffsetCount(offset, count))
	raw, err := c.client.apiGet(ctx, pathOnBehalfListRecentChat, q, tok)
	if err != nil {
		return nil, c.wrap(err)
	}
	var env struct {
		Error   int               `json:"error"`
		Message string            `json:"message"`
		Data    []RecentChatEntry `json:"data"`
	}
	if jerr := json.Unmarshal(raw, &env); jerr != nil {
		return nil, fmt.Errorf("unmarshal listrecentchat: %w", jerr)
	}
	return env.Data, nil
}

// GetConversation pages messages for a given partner UID.
func (c *OnBehalfClient) GetConversation(ctx context.Context, uid string, offset, count int) ([]ConversationMessage, error) {
	if uid == "" {
		return nil, fmt.Errorf("oa.onbehalf: empty uid")
	}
	tok := c.token()
	if tok == "" {
		return nil, ErrInvalidRefreshToken
	}
	q := url.Values{}
	q.Set("data", buildConversationData(uid, offset, count))
	raw, err := c.client.apiGet(ctx, pathOnBehalfConversation, q, tok)
	if err != nil {
		return nil, c.wrap(err)
	}
	var env struct {
		Error   int                   `json:"error"`
		Message string                `json:"message"`
		Data    []ConversationMessage `json:"data"`
	}
	if jerr := json.Unmarshal(raw, &env); jerr != nil {
		return nil, fmt.Errorf("unmarshal conversation: %w", jerr)
	}
	return env.Data, nil
}

func (c *OnBehalfClient) token() string {
	if c.tokenSrc == nil {
		return ""
	}
	return c.tokenSrc()
}

// wrap maps a refresh-token-dead APIError to ErrInvalidRefreshToken so the
// worker can short-circuit via errors.Is.
func (c *OnBehalfClient) wrap(err error) error {
	var apiErr *APIError
	if errors.As(err, &apiErr) && apiErr.Code == codeInvalidGrant {
		return fmt.Errorf("%w: %s", ErrInvalidRefreshToken, apiErr.Message)
	}
	return err
}

func buildOffsetCount(offset, count int) string {
	return `{"offset":` + strconv.Itoa(offset) + `,"count":` + strconv.Itoa(count) + `}`
}

func buildConversationData(uid string, offset, count int) string {
	return `{"user_id":"` + uid + `","offset":` + strconv.Itoa(offset) + `,"count":` + strconv.Itoa(count) + `}`
}
