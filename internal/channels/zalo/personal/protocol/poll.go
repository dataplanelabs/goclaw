package protocol

// Poll endpoints. All five live under the "group" service map entry —
// "group_poll" is reserved for /api/group/getlg/v4 (see contacts.go).

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

const (
	apiPathPollCreate     = "/api/poll/create"
	apiPathPollDetail     = "/api/poll/detail"
	apiPathPollVote       = "/api/poll/vote"
	apiPathPollEnd        = "/api/poll/end"
	apiPathPollOptionAdd  = "/api/poll/option/add"
	apiPathBoardList      = "/api/board/list"
	pollServiceKey        = "group"
	boardServiceKey       = "group_board"
	boardTypePoll         = 3
	defaultBoardListPage  = 1
	defaultBoardListCount = 20
	maxBoardListCount     = 20
)

// CreatePoll creates a new poll inside a Zalo group. Returns the parsed
// PollDetail as Zalo echoes it back in the response.
func CreatePoll(ctx context.Context, sess *Session, groupID string, opts CreatePollOptions) (*PollDetail, error) {
	if groupID == "" {
		return nil, fmt.Errorf("zalo_personal: createPoll requires groupID")
	}
	if strings.TrimSpace(opts.Question) == "" {
		return nil, fmt.Errorf("zalo_personal: createPoll requires question")
	}
	if len(opts.Options) < 2 {
		return nil, fmt.Errorf("zalo_personal: createPoll requires at least 2 options")
	}
	payload := map[string]any{
		"group_id":             groupID,
		"question":             opts.Question,
		"options":              opts.Options,
		"expired_time":         opts.ExpiredTime,
		"pinAct":               false,
		"allow_multi_choices":  opts.AllowMultiChoices,
		"allow_add_new_option": opts.AllowAddNewOption,
		"is_hide_vote_preview": opts.HideVotePreview,
		"is_anonymous":         opts.IsAnonymous,
		"poll_type":            0,
		"src":                  1,
		"imei":                 sess.IMEI,
	}
	var out PollDetail
	if err := postEncryptedJSON(ctx, sess, apiPathPollCreate, payload, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetPollDetail reads the current poll state by ID.
func GetPollDetail(ctx context.Context, sess *Session, pollID int64) (*PollDetail, error) {
	if pollID == 0 {
		return nil, fmt.Errorf("zalo_personal: getPollDetail requires pollID")
	}
	payload := map[string]any{
		"poll_id": pollID,
		"imei":    sess.IMEI,
	}
	var out PollDetail
	if err := postEncryptedJSON(ctx, sess, apiPathPollDetail, payload, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

type ListPollsOptions struct {
	Page  int
	Count int
}

type PollList struct {
	Polls []PollDetail
	Count int
}

type boardListResponse struct {
	Items []boardListItem `json:"items"`
	Count int             `json:"count"`
}

type boardListItem struct {
	BoardType int             `json:"boardType"`
	Data      json.RawMessage `json:"data"`
}

// ListPolls reads group board items and returns only poll board entries. Zalo
// does not expose a dedicated poll-list endpoint; the web client gets this
// through /api/board/list on the group_board service.
func ListPolls(ctx context.Context, sess *Session, groupID string, opts ListPollsOptions) (*PollList, error) {
	if groupID == "" {
		return nil, fmt.Errorf("zalo_personal: listPolls requires groupID")
	}
	page := opts.Page
	if page < 1 {
		page = defaultBoardListPage
	}
	count := opts.Count
	switch {
	case count <= 0:
		count = defaultBoardListCount
	case count > maxBoardListCount:
		count = maxBoardListCount
	}
	payload := map[string]any{
		"group_id":   groupID,
		"board_type": 0,
		"page":       page,
		"count":      count,
		"last_id":    0,
		"last_type":  0,
		"imei":       sess.IMEI,
	}
	var out boardListResponse
	if err := getEncryptedJSONForService(ctx, sess, boardServiceKey, apiPathBoardList, payload, &out); err != nil {
		return nil, err
	}
	polls := make([]PollDetail, 0, len(out.Items))
	for _, item := range out.Items {
		if item.BoardType != boardTypePoll {
			continue
		}
		var poll PollDetail
		if err := json.Unmarshal(item.Data, &poll); err != nil {
			return nil, fmt.Errorf("zalo_personal: parse poll board item: %w", err)
		}
		polls = append(polls, poll)
	}
	return &PollList{Polls: polls, Count: out.Count}, nil
}

// votePollResponse and addPollOptionsResponse share the {"options": [...]}
// envelope Zalo returns on GET endpoints. Defined here so each endpoint
// function stays focused on payload shape.
type pollOptionsResponse struct {
	Options []PollOption `json:"options"`
}

// VotePoll submits a vote (or unvotes when optionIDs is empty). HTTP GET
// with the encrypted payload carried in the `params` query string.
func VotePoll(ctx context.Context, sess *Session, pollID int64, optionIDs []int64) ([]PollOption, error) {
	if pollID == 0 {
		return nil, fmt.Errorf("zalo_personal: votePoll requires pollID")
	}
	if optionIDs == nil {
		optionIDs = []int64{}
	}
	payload := map[string]any{
		"poll_id":    pollID,
		"option_ids": optionIDs,
		"imei":       sess.IMEI,
	}
	var out pollOptionsResponse
	if err := getEncryptedJSON(ctx, sess, apiPathPollVote, payload, &out); err != nil {
		return nil, err
	}
	return out.Options, nil
}

// LockPoll ends a poll. Zalo returns an empty data field on success.
func LockPoll(ctx context.Context, sess *Session, pollID int64) error {
	if pollID == 0 {
		return fmt.Errorf("zalo_personal: lockPoll requires pollID")
	}
	payload := map[string]any{
		"poll_id": pollID,
		"imei":    sess.IMEI,
	}
	// Discard the data field; only the error_code in the outer envelope matters.
	var discard json.RawMessage
	return postEncryptedJSON(ctx, sess, apiPathPollEnd, payload, &discard)
}

// AddPollOptions extends an existing poll. HTTP GET; `new_options` is sent
// as a JSON-stringified array (not nested array) inside the encrypted payload.
func AddPollOptions(ctx context.Context, sess *Session, pollID int64, newOpts []AddPollOptionsItem, votedOptionIDs []int64) ([]PollOption, error) {
	if pollID == 0 {
		return nil, fmt.Errorf("zalo_personal: addPollOptions requires pollID")
	}
	if newOpts == nil {
		newOpts = []AddPollOptionsItem{}
	}
	if votedOptionIDs == nil {
		votedOptionIDs = []int64{}
	}
	newOptsJSON, err := json.Marshal(newOpts)
	if err != nil {
		return nil, fmt.Errorf("zalo_personal: marshal new_options: %w", err)
	}
	payload := map[string]any{
		"poll_id":          pollID,
		"new_options":      string(newOptsJSON),
		"voted_option_ids": votedOptionIDs,
		"imei":             sess.IMEI,
	}
	var out pollOptionsResponse
	if err := getEncryptedJSON(ctx, sess, apiPathPollOptionAdd, payload, &out); err != nil {
		return nil, err
	}
	return out.Options, nil
}

// postEncryptedJSON encrypts payload, POSTs form-body `params=<encrypted>` to
// the group service base + apiPath, parses the outer Response envelope, then
// decrypts the inner data field into `out`. `out` may be a struct, slice, or
// json.RawMessage.
func postEncryptedJSON(ctx context.Context, sess *Session, apiPath string, payload map[string]any, out any) error {
	baseURL := getServiceURL(sess, pollServiceKey)
	if baseURL == "" {
		return fmt.Errorf("zalo_personal: no service URL for %s", pollServiceKey)
	}
	encData, err := encryptPayload(sess, payload)
	if err != nil {
		return fmt.Errorf("zalo_personal: encrypt %s: %w", apiPath, err)
	}
	sendURL := makeURL(sess, baseURL+apiPath, nil, true)
	form := buildFormBody(map[string]string{"params": encData})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, sendURL, form)
	if err != nil {
		return err
	}
	setDefaultHeaders(req, sess)
	return doDecodedRequest(sess, req, apiPath, out)
}

// getEncryptedJSON is the GET variant — payload encrypted into the `params`
// query string rather than the body. Used by VotePoll + AddPollOptions.
func getEncryptedJSON(ctx context.Context, sess *Session, apiPath string, payload map[string]any, out any) error {
	return getEncryptedJSONForService(ctx, sess, pollServiceKey, apiPath, payload, out)
}

func getEncryptedJSONForService(ctx context.Context, sess *Session, serviceKey, apiPath string, payload map[string]any, out any) error {
	baseURL := getServiceURL(sess, serviceKey)
	if baseURL == "" {
		return fmt.Errorf("zalo_personal: no service URL for %s", serviceKey)
	}
	encData, err := encryptPayload(sess, payload)
	if err != nil {
		return fmt.Errorf("zalo_personal: encrypt %s: %w", apiPath, err)
	}
	sendURL := makeURL(sess, baseURL+apiPath, map[string]any{"params": encData}, true)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, sendURL, nil)
	if err != nil {
		return err
	}
	setDefaultHeaders(req, sess)
	return doDecodedRequest(sess, req, apiPath, out)
}

// doDecodedRequest runs the request, parses the outer Response[*string]
// envelope, decrypts the data field, and unmarshals into `out`. When the data
// field is nil (void endpoints like lockPoll) and `out` is a *json.RawMessage,
// it stays nil and success is returned.
func doDecodedRequest(sess *Session, req *http.Request, apiPath string, out any) error {
	resp, err := sess.Client.Do(req)
	if err != nil {
		return fmt.Errorf("zalo_personal: %s request: %w", apiPath, err)
	}
	defer resp.Body.Close()

	var envelope Response[*string]
	if err := readJSON(resp, &envelope); err != nil {
		return fmt.Errorf("zalo_personal: parse %s response: %w", apiPath, err)
	}
	if envelope.ErrorCode != 0 {
		return fmt.Errorf("zalo_personal: %s error code %d: %s", apiPath, envelope.ErrorCode, envelope.ErrorMessage)
	}
	if envelope.Data == nil || *envelope.Data == "" {
		return nil
	}
	plain, err := decryptDataField(sess, *envelope.Data)
	if err != nil {
		return fmt.Errorf("zalo_personal: decrypt %s response: %w", apiPath, err)
	}
	if len(plain) == 0 {
		return nil
	}
	if err := json.Unmarshal(plain, out); err != nil {
		return fmt.Errorf("zalo_personal: parse %s payload: %w", apiPath, err)
	}
	return nil
}
