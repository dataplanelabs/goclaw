package protocol

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// encryptInnerResponse wraps an arbitrary JSON payload in Zalo's double-
// encrypted response shape: outer `{"error_code":0,"data":"<enc>"}` where the
// encrypted blob decrypts to `{"error_code":0,"data":<payload>}`.
func encryptInnerResponse(t *testing.T, payloadJSON string) string {
	t.Helper()
	innerJSON := []byte(`{"error_code":0,"data":` + payloadJSON + `}`)
	key, _ := base64.StdEncoding.DecodeString(testKeyB64)
	enc, err := EncodeAESCBC(key, string(innerJSON), false)
	if err != nil {
		t.Fatalf("encrypt inner: %v", err)
	}
	outer := map[string]any{"error_code": 0, "data": enc}
	out, _ := json.Marshal(outer)
	return string(out)
}

// pollCaptured records the method, path, raw body, and parsed query params of
// each request so tests can assert GET-vs-POST and the `params` location.
type pollCaptured struct {
	method string
	path   string
	body   []byte
	query  url.Values
}

func pollCaptureServer(t *testing.T, responseJSON string, errorCode int) (*httptest.Server, *[]pollCaptured) {
	t.Helper()
	var cap []pollCaptured
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		cap = append(cap, pollCaptured{
			method: r.Method,
			path:   r.URL.Path,
			body:   body,
			query:  r.URL.Query(),
		})
		w.Header().Set("Content-Type", "application/json")
		if errorCode != 0 {
			_, _ = w.Write([]byte(`{"error_code":` + jsonNum(errorCode) + `,"error_message":"err","data":null}`))
			return
		}
		_, _ = w.Write([]byte(encryptInnerResponse(t, responseJSON)))
	}))
	t.Cleanup(srv.Close)
	return srv, &cap
}

func jsonNum(i int) string  { return string(mustJSON(i)) }
func mustJSON(v any) []byte { b, _ := json.Marshal(v); return b }

// decryptCapturedFormParams reverses postEncryptedJSON's body shape.
func decryptCapturedFormParams(t *testing.T, body []byte) map[string]any {
	t.Helper()
	form, err := url.ParseQuery(string(body))
	if err != nil {
		t.Fatalf("parse form: %v", err)
	}
	enc := form.Get("params")
	if enc == "" {
		t.Fatalf("missing params in body: %s", body)
	}
	return decryptParams(t, enc)
}

// decryptCapturedQueryParams reverses getEncryptedJSON's URL params.
func decryptCapturedQueryParams(t *testing.T, q url.Values) map[string]any {
	t.Helper()
	enc := q.Get("params")
	if enc == "" {
		t.Fatalf("missing params in query: %v", q)
	}
	return decryptParams(t, enc)
}

func decryptParams(t *testing.T, enc string) map[string]any {
	t.Helper()
	key, _ := base64.StdEncoding.DecodeString(testKeyB64)
	plain, err := DecodeAESCBC(key, enc)
	if err != nil {
		t.Fatalf("decrypt params: %v", err)
	}
	var out map[string]any
	if err := json.Unmarshal(plain, &out); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	return out
}

// --- CreatePoll ---

func TestCreatePoll_HappyPath(t *testing.T) {
	t.Parallel()
	srv, cap := pollCaptureServer(t,
		`{"poll_id":42,"question":"q","options":[{"option_id":1,"content":"a","vote_count":0},{"option_id":2,"content":"b","vote_count":0}],"group_id":"g","creator_id":"self-uid","created_time":1700000000}`,
		0)
	sess := newQuoteTestSession(t, srv)

	detail, err := CreatePoll(context.Background(), sess, "g", CreatePollOptions{
		Question: "q", Options: []string{"a", "b"},
	})
	if err != nil {
		t.Fatalf("CreatePoll: %v", err)
	}
	if got := (*cap)[0].method; got != http.MethodPost {
		t.Errorf("method=%s, want POST", got)
	}
	if !strings.HasSuffix((*cap)[0].path, apiPathPollCreate) {
		t.Errorf("path=%s, want suffix %s", (*cap)[0].path, apiPathPollCreate)
	}
	payload := decryptCapturedFormParams(t, (*cap)[0].body)
	if payload["group_id"] != "g" || payload["question"] != "q" {
		t.Errorf("payload mismatch: %+v", payload)
	}
	if payload["pinAct"] != false {
		t.Errorf("pinAct should be false, got %v", payload["pinAct"])
	}
	if payload["poll_type"].(float64) != 0 {
		t.Errorf("poll_type should be 0, got %v", payload["poll_type"])
	}
	if detail.PollID.String() != "42" {
		t.Errorf("pollID=%s, want 42", detail.PollID.String())
	}
	if len(detail.Options) != 2 {
		t.Errorf("options=%d, want 2", len(detail.Options))
	}
}

func TestCreatePoll_SendsAbsoluteExpiryMillis(t *testing.T) {
	t.Parallel()
	srv, cap := pollCaptureServer(t,
		`{"poll_id":42,"question":"q","options":[{"option_id":1,"content":"a","vote_count":0},{"option_id":2,"content":"b","vote_count":0}],"group_id":"g","creator_id":"self-uid","created_time":1700000000}`,
		0)
	sess := newQuoteTestSession(t, srv)

	const expireAtMillis = int64(1781511729000)
	_, err := CreatePoll(context.Background(), sess, "g", CreatePollOptions{
		Question: "q", Options: []string{"a", "b"}, ExpiredTime: expireAtMillis,
	})
	if err != nil {
		t.Fatalf("CreatePoll: %v", err)
	}
	payload := decryptCapturedFormParams(t, (*cap)[0].body)
	if got := int64(payload["expired_time"].(float64)); got != expireAtMillis {
		t.Fatalf("expired_time=%d, want %d", got, expireAtMillis)
	}
}

func TestCreatePoll_Validation(t *testing.T) {
	t.Parallel()
	srv, _ := pollCaptureServer(t, "{}", 0)
	sess := newQuoteTestSession(t, srv)

	cases := []struct {
		name string
		gid  string
		opts CreatePollOptions
	}{
		{"empty groupID", "", CreatePollOptions{Question: "q", Options: []string{"a", "b"}}},
		{"empty question", "g", CreatePollOptions{Options: []string{"a", "b"}}},
		{"one option", "g", CreatePollOptions{Question: "q", Options: []string{"only"}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := CreatePoll(context.Background(), sess, tc.gid, tc.opts)
			if err == nil {
				t.Errorf("want error for %s", tc.name)
			}
		})
	}
}

func TestCreatePoll_ServerError(t *testing.T) {
	t.Parallel()
	srv, _ := pollCaptureServer(t, "", 114)
	sess := newQuoteTestSession(t, srv)
	_, err := CreatePoll(context.Background(), sess, "g", CreatePollOptions{
		Question: "q", Options: []string{"a", "b"},
	})
	if err == nil || !strings.Contains(err.Error(), "114") {
		t.Errorf("want error with code 114, got %v", err)
	}
}

// --- GetPollDetail ---

func TestGetPollDetail_HappyPath(t *testing.T) {
	t.Parallel()
	srv, cap := pollCaptureServer(t,
		`{"poll_id":42,"question":"q","options":[],"creator_id":"self"}`, 0)
	sess := newQuoteTestSession(t, srv)

	d, err := GetPollDetail(context.Background(), sess, 42)
	if err != nil {
		t.Fatalf("GetPollDetail: %v", err)
	}
	if (*cap)[0].method != http.MethodPost {
		t.Errorf("method should be POST")
	}
	if !strings.HasSuffix((*cap)[0].path, apiPathPollDetail) {
		t.Errorf("path=%s, want suffix %s", (*cap)[0].path, apiPathPollDetail)
	}
	if d.PollID.String() != "42" {
		t.Errorf("pollID=%s, want 42", d.PollID.String())
	}
}

func TestGetPollDetail_ZeroID(t *testing.T) {
	t.Parallel()
	srv, _ := pollCaptureServer(t, "{}", 0)
	sess := newQuoteTestSession(t, srv)
	_, err := GetPollDetail(context.Background(), sess, 0)
	if err == nil {
		t.Errorf("want error for zero pollID")
	}
}

// --- ListPolls ---

func TestListPolls_FiltersPollBoardItems(t *testing.T) {
	t.Parallel()
	srv, cap := pollCaptureServer(t,
		`{"items":[`+
			`{"boardType":1,"data":{"id":"note-1","params":"{}"}},`+
			`{"boardType":3,"data":{"poll_id":42,"question":"q","options":[{"option_id":1,"content":"a","votes":2},{"option_id":2,"content":"b","votes":1}],"group_id":"g","created_time":1700000000,"updated_time":1700000100,"expired_time":1700600000,"closed":false,"num_vote":3}}`+
			`],"count":2}`, 0)
	sess := newQuoteTestSession(t, srv)

	list, err := ListPolls(context.Background(), sess, "g", ListPollsOptions{Page: 2, Count: 99})
	if err != nil {
		t.Fatalf("ListPolls: %v", err)
	}
	if (*cap)[0].method != http.MethodGet {
		t.Errorf("method=%s, want GET", (*cap)[0].method)
	}
	if !strings.HasSuffix((*cap)[0].path, apiPathBoardList) {
		t.Errorf("path=%s, want suffix %s", (*cap)[0].path, apiPathBoardList)
	}
	payload := decryptCapturedQueryParams(t, (*cap)[0].query)
	if payload["group_id"] != "g" || payload["page"].(float64) != 2 {
		t.Fatalf("payload mismatch: %+v", payload)
	}
	if payload["count"].(float64) != maxBoardListCount {
		t.Fatalf("count should be clamped to %d, got %+v", maxBoardListCount, payload["count"])
	}
	if len(list.Polls) != 1 {
		t.Fatalf("polls=%d, want 1: %+v", len(list.Polls), list)
	}
	poll := list.Polls[0]
	if poll.PollID.String() != "42" || poll.Question != "q" || poll.TotalVotes != 3 {
		t.Fatalf("unexpected poll: %+v", poll)
	}
	if poll.Options[0].CountVotes() != 2 {
		t.Fatalf("votes not parsed from board payload: %+v", poll.Options[0])
	}
}

// --- VotePoll ---

func TestVotePoll_GetMethodAndQueryParams(t *testing.T) {
	t.Parallel()
	srv, cap := pollCaptureServer(t, `{"options":[]}`, 0)
	sess := newQuoteTestSession(t, srv)

	_, err := VotePoll(context.Background(), sess, 42, []int64{1, 2})
	if err != nil {
		t.Fatalf("VotePoll: %v", err)
	}
	if (*cap)[0].method != http.MethodGet {
		t.Errorf("method=%s, want GET", (*cap)[0].method)
	}
	if !strings.HasSuffix((*cap)[0].path, apiPathPollVote) {
		t.Errorf("path=%s", (*cap)[0].path)
	}
	if len((*cap)[0].body) != 0 {
		t.Errorf("body should be empty for GET, got %d bytes", len((*cap)[0].body))
	}
	payload := decryptCapturedQueryParams(t, (*cap)[0].query)
	if payload["poll_id"].(float64) != 42 {
		t.Errorf("poll_id=%v, want 42", payload["poll_id"])
	}
	ids, ok := payload["option_ids"].([]any)
	if !ok || len(ids) != 2 {
		t.Errorf("option_ids missing or wrong shape: %+v", payload["option_ids"])
	}
}

func TestVotePoll_Unvote(t *testing.T) {
	t.Parallel()
	srv, cap := pollCaptureServer(t, `{"options":[]}`, 0)
	sess := newQuoteTestSession(t, srv)

	_, err := VotePoll(context.Background(), sess, 42, nil)
	if err != nil {
		t.Fatalf("VotePoll: %v", err)
	}
	payload := decryptCapturedQueryParams(t, (*cap)[0].query)
	ids, ok := payload["option_ids"].([]any)
	if !ok {
		t.Fatalf("option_ids missing: %+v", payload)
	}
	if len(ids) != 0 {
		t.Errorf("option_ids should be empty array, got %v", ids)
	}
}

// --- LockPoll ---

func TestLockPoll_HappyPath(t *testing.T) {
	t.Parallel()
	// Lock returns no data; server sends an envelope with no encrypted data.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"error_code":0,"data":null}`))
	}))
	t.Cleanup(srv.Close)
	sess := newQuoteTestSession(t, srv)

	if err := LockPoll(context.Background(), sess, 42); err != nil {
		t.Errorf("LockPoll: %v", err)
	}
}

func TestLockPoll_ServerError(t *testing.T) {
	t.Parallel()
	srv, _ := pollCaptureServer(t, "", 113)
	sess := newQuoteTestSession(t, srv)
	err := LockPoll(context.Background(), sess, 42)
	if err == nil || !strings.Contains(err.Error(), "113") {
		t.Errorf("want server error 113, got %v", err)
	}
}

// --- AddPollOptions ---

func TestAddPollOptions_StringifiedNewOpts_GetMethod(t *testing.T) {
	t.Parallel()
	srv, cap := pollCaptureServer(t, `{"options":[]}`, 0)
	sess := newQuoteTestSession(t, srv)

	_, err := AddPollOptions(context.Background(), sess, 42,
		[]AddPollOptionsItem{{Content: "x"}, {Content: "y", Voted: true}},
		[]int64{1, 2},
	)
	if err != nil {
		t.Fatalf("AddPollOptions: %v", err)
	}
	if (*cap)[0].method != http.MethodGet {
		t.Errorf("method=%s, want GET", (*cap)[0].method)
	}
	if !strings.HasSuffix((*cap)[0].path, apiPathPollOptionAdd) {
		t.Errorf("path=%s", (*cap)[0].path)
	}
	payload := decryptCapturedQueryParams(t, (*cap)[0].query)
	// new_options must be a string (JSON-stringified array), not a nested array.
	s, ok := payload["new_options"].(string)
	if !ok {
		t.Fatalf("new_options must be a string, got %T (%v)", payload["new_options"], payload["new_options"])
	}
	// Validate that the string is itself a JSON array of items.
	var inner []AddPollOptionsItem
	if err := json.Unmarshal([]byte(s), &inner); err != nil {
		t.Errorf("new_options not valid JSON array: %v", err)
	}
	if len(inner) != 2 || inner[0].Content != "x" || !inner[1].Voted {
		t.Errorf("new_options shape unexpected: %+v", inner)
	}
}

func TestAddPollOptions_EmptyNewOpts(t *testing.T) {
	t.Parallel()
	srv, cap := pollCaptureServer(t, `{"options":[]}`, 0)
	sess := newQuoteTestSession(t, srv)

	_, err := AddPollOptions(context.Background(), sess, 42, nil, nil)
	if err != nil {
		t.Fatalf("AddPollOptions: %v", err)
	}
	payload := decryptCapturedQueryParams(t, (*cap)[0].query)
	s, _ := payload["new_options"].(string)
	if s != "[]" {
		t.Errorf("new_options=%q, want \"[]\"", s)
	}
}

// --- Service URL resolution ---

func TestPollEndpoints_NoServiceURL(t *testing.T) {
	t.Parallel()
	sess := NewSession()
	sess.SecretKey = testKeyB64
	sess.LoginInfo = &LoginInfo{} // no Group URLs
	if _, err := CreatePoll(context.Background(), sess, "g", CreatePollOptions{
		Question: "q", Options: []string{"a", "b"},
	}); err == nil {
		t.Errorf("want error when no group service URL")
	}
}
