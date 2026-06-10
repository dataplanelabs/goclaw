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
	client      *Client
	tokenSrc    func() string // returns current OA access token (refresh handled externally)
	tokenErrSrc func() (string, error)
}

func NewOnBehalfClient(client *Client, tokenSrc func() string) *OnBehalfClient {
	return &OnBehalfClient{client: client, tokenSrc: tokenSrc}
}

func NewOnBehalfClientWithError(client *Client, tokenSrc func() (string, error)) *OnBehalfClient {
	return &OnBehalfClient{client: client, tokenErrSrc: tokenSrc}
}

// RecentChatEntry / ConversationMessage retained as aliases for the
// existing fake-store tests. Real Zalo /listrecentchat returns the same
// shape as the customer-poll `message` struct in poll.go: each row IS a
// message (not a thread summary).
type RecentChatEntry struct {
	UID         string `json:"-"` // populated from FromID/ToID at process-time
	LastMsgID   string `json:"-"`
	LastMsgTime int64  `json:"-"`
	UserName    string `json:"-"`
}

// ConversationMessage mirrors the customer poller's `message` shape:
// `from_id`, `to_id`, `message_id`, `message` (text body), `time`.
type ConversationMessage struct {
	MsgID       string `json:"message_id"`
	SrcID       string `json:"from_id"` // sender uid; OA's uid for OA→user, customer uid for user→OA
	DstID       string `json:"to_id,omitempty"`
	Type        string `json:"type,omitempty"`
	Text        string `json:"message,omitempty"`
	Time        int64  `json:"time,omitempty"`
	DisplayName string `json:"from_display_name,omitempty"`
}

// ListRecentMessages fetches the most-recent N messages across all users
// on this OA. Zalo's /v2.0/oa/listrecentchat returns the same flat
// message shape used by the customer poller.
func (c *OnBehalfClient) ListRecentMessages(ctx context.Context, offset, count int) ([]ConversationMessage, error) {
	tok, err := c.token()
	if err != nil {
		return nil, err
	}
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
		Error   int                   `json:"error"`
		Message string                `json:"message"`
		Data    []ConversationMessage `json:"data"`
	}
	if jerr := json.Unmarshal(raw, &env); jerr != nil {
		return nil, fmt.Errorf("unmarshal listrecentchat: %w", jerr)
	}
	return env.Data, nil
}

// ListRecentChat is kept for backwards-compat with existing tests. Calls
// ListRecentMessages and projects to legacy entry shape (one per uid).
func (c *OnBehalfClient) ListRecentChat(ctx context.Context, offset, count int) ([]RecentChatEntry, error) {
	msgs, err := c.ListRecentMessages(ctx, offset, count)
	if err != nil {
		return nil, err
	}
	seen := make(map[string]struct{})
	out := make([]RecentChatEntry, 0, len(msgs))
	for _, m := range msgs {
		peer := m.SrcID
		if _, ok := seen[peer]; ok {
			continue
		}
		seen[peer] = struct{}{}
		out = append(out, RecentChatEntry{UID: peer, LastMsgID: m.MsgID, LastMsgTime: m.Time, UserName: m.DisplayName})
	}
	return out, nil
}

// GetConversation pages messages for a given partner UID.
func (c *OnBehalfClient) GetConversation(ctx context.Context, uid string, offset, count int) ([]ConversationMessage, error) {
	if uid == "" {
		return nil, fmt.Errorf("oa.onbehalf: empty uid")
	}
	tok, err := c.token()
	if err != nil {
		return nil, err
	}
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

func (c *OnBehalfClient) token() (string, error) {
	if c.tokenErrSrc != nil {
		return c.tokenErrSrc()
	}
	if c.tokenSrc == nil {
		return "", nil
	}
	return c.tokenSrc(), nil
}

// wrap maps a refresh-token-dead APIError to ErrInvalidRefreshToken so the
// worker can short-circuit via errors.Is.
func (c *OnBehalfClient) wrap(err error) error {
	var apiErr *APIError
	if errors.As(err, &apiErr) && (apiErr.Code == codeInvalidGrant || apiErr.Code == codeInvalidRefreshToken) {
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
