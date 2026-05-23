package protocol

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// newReactionTestSession wires both the Reaction service slot and Chat/Group
// so getServiceURL("reaction") resolves to the httptest server.
func newReactionTestSession(t *testing.T, srv *httptest.Server) *Session {
	t.Helper()
	sess := NewSession()
	sess.IMEI = "test-imei"
	sess.SecretKey = testKeyB64
	sess.UID = "self-uid"
	sess.LoginInfo = &LoginInfo{
		UID: "self-uid",
		ZpwServiceMapV3: ZpwServiceMapV3{
			Chat:     []string{srv.URL},
			Group:    []string{srv.URL},
			Reaction: []string{srv.URL},
		},
	}
	return sess
}

// reactionResponseEnvelope returns Zalo's outer envelope wrapping an encrypted
// inner `{"error_code":0,"data":{"msgIds":<msgIDsJSON>}}` payload. msgIDsJSON
// can be either a JSON array (`[1,2]`) or a JSON string (`"[1,2]"`) so tests
// can exercise both shapes.
func reactionResponseEnvelope(t *testing.T, msgIDsJSON string) string {
	t.Helper()
	innerJSON := []byte(`{"error_code":0,"data":{"msgIds":` + msgIDsJSON + `}}`)
	key, _ := base64.StdEncoding.DecodeString(testKeyB64)
	enc, err := EncodeAESCBC(key, string(innerJSON), false)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	out, _ := json.Marshal(map[string]any{"error_code": 0, "data": enc})
	return string(out)
}

func TestAddReaction_DMHappyPath(t *testing.T) {
	t.Parallel()
	srv, cap := pollCaptureServer(t, `{"msgIds":[1001]}`, 0)
	sess := newReactionTestSession(t, srv)

	_, err := AddReaction(context.Background(), sess, ReactionDest{
		MsgID: "100", CliMsgID: "200", ThreadID: "user-1", Type: ThreadTypeUser,
	}, ReactionHeart)
	if err != nil {
		t.Fatalf("AddReaction: %v", err)
	}
	if !strings.HasSuffix((*cap)[0].path, apiPathReactionDM) {
		t.Errorf("path=%s, want suffix %s", (*cap)[0].path, apiPathReactionDM)
	}
	payload := decryptCapturedFormParams(t, (*cap)[0].body)
	if payload["toid"] != "user-1" {
		t.Errorf("toid=%v, want user-1", payload["toid"])
	}
	if _, present := payload["grid"]; present {
		t.Errorf("grid should not be present on DM, got %+v", payload)
	}
}

func TestAddReaction_GroupHappyPath(t *testing.T) {
	t.Parallel()
	srv, cap := pollCaptureServer(t, `{"msgIds":[1001]}`, 0)
	sess := newReactionTestSession(t, srv)

	_, err := AddReaction(context.Background(), sess, ReactionDest{
		MsgID: "100", CliMsgID: "200", ThreadID: "group-1", Type: ThreadTypeGroup,
	}, ReactionLike)
	if err != nil {
		t.Fatalf("AddReaction: %v", err)
	}
	if !strings.HasSuffix((*cap)[0].path, apiPathReactionGroup) {
		t.Errorf("path=%s, want suffix %s", (*cap)[0].path, apiPathReactionGroup)
	}
	payload := decryptCapturedFormParams(t, (*cap)[0].body)
	if payload["grid"] != "group-1" {
		t.Errorf("grid=%v, want group-1", payload["grid"])
	}
	if payload["imei"] != "test-imei" {
		t.Errorf("imei missing on group reaction: %+v", payload)
	}
	if _, present := payload["toid"]; present {
		t.Errorf("toid should not be present on group reaction")
	}
}

func TestAddReaction_InnerJSONShape(t *testing.T) {
	t.Parallel()
	srv, cap := pollCaptureServer(t, `{"msgIds":[1001]}`, 0)
	sess := newReactionTestSession(t, srv)

	_, err := AddReaction(context.Background(), sess, ReactionDest{
		MsgID: "9876543210", CliMsgID: "1709300000123",
		ThreadID: "user-1", Type: ThreadTypeUser,
	}, ReactionHeart)
	if err != nil {
		t.Fatalf("AddReaction: %v", err)
	}
	payload := decryptCapturedFormParams(t, (*cap)[0].body)

	rl, ok := payload["react_list"].([]any)
	if !ok || len(rl) != 1 {
		t.Fatalf("react_list shape: %+v", payload["react_list"])
	}
	rlItem := rl[0].(map[string]any)
	messageStr, ok := rlItem["message"].(string)
	if !ok {
		t.Fatalf("react_list[0].message must be a string, got %T", rlItem["message"])
	}
	var inner map[string]any
	if err := json.Unmarshal([]byte(messageStr), &inner); err != nil {
		t.Fatalf("decode inner message: %v", err)
	}
	if inner["rIcon"] != ReactionHeart {
		t.Errorf("rIcon=%v, want %s", inner["rIcon"], ReactionHeart)
	}
	heartMeta := LookupReactionMeta(ReactionHeart)
	if int(inner["rType"].(float64)) != heartMeta.RType {
		t.Errorf("rType=%v, want %d", inner["rType"], heartMeta.RType)
	}
	if int(inner["source"].(float64)) != heartMeta.Source {
		t.Errorf("source=%v, want %d", inner["source"], heartMeta.Source)
	}
	rMsg := inner["rMsg"].([]any)
	rMsgItem := rMsg[0].(map[string]any)
	if int64(rMsgItem["gMsgID"].(float64)) != 9876543210 {
		t.Errorf("gMsgID=%v, want 9876543210", rMsgItem["gMsgID"])
	}
}

func TestAddReaction_RemoveViaNone(t *testing.T) {
	t.Parallel()
	srv, cap := pollCaptureServer(t, `{"msgIds":[1001]}`, 0)
	sess := newReactionTestSession(t, srv)

	_, err := AddReaction(context.Background(), sess, ReactionDest{
		MsgID: "100", CliMsgID: "200", ThreadID: "user-1", Type: ThreadTypeUser,
	}, ReactionNone)
	if err != nil {
		t.Fatalf("AddReaction: %v", err)
	}
	payload := decryptCapturedFormParams(t, (*cap)[0].body)
	rlItem := payload["react_list"].([]any)[0].(map[string]any)
	var inner map[string]any
	_ = json.Unmarshal([]byte(rlItem["message"].(string)), &inner)
	if inner["rIcon"] != "" {
		t.Errorf("rIcon should be empty for NONE, got %v", inner["rIcon"])
	}
	if int(inner["rType"].(float64)) != -1 {
		t.Errorf("rType=%v, want -1 for NONE", inner["rType"])
	}
}

func TestAddReaction_ValidationErrors(t *testing.T) {
	t.Parallel()
	srv, _ := pollCaptureServer(t, `{}`, 0)
	sess := newReactionTestSession(t, srv)
	cases := []ReactionDest{
		{CliMsgID: "2", ThreadID: "t"},                       // missing MsgID
		{MsgID: "1", ThreadID: "t"},                           // missing CliMsgID
		{MsgID: "1", CliMsgID: "2"},                           // missing ThreadID
	}
	for i, d := range cases {
		_, err := AddReaction(context.Background(), sess, d, ReactionHeart)
		if err == nil {
			t.Errorf("case %d: want validation error", i)
		}
	}
}

func TestAddReaction_NoServiceURL(t *testing.T) {
	t.Parallel()
	sess := NewSession()
	sess.SecretKey = testKeyB64
	sess.LoginInfo = &LoginInfo{} // empty service map
	_, err := AddReaction(context.Background(), sess, ReactionDest{
		MsgID: "1", CliMsgID: "2", ThreadID: "t", Type: ThreadTypeUser,
	}, ReactionHeart)
	if err == nil {
		t.Errorf("want error for missing service URL")
	}
}

func TestAddReaction_ResponseStringMsgIds(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(reactionResponseEnvelope(t, `"[1,2,3]"`)))
	}))
	t.Cleanup(srv.Close)
	sess := newReactionTestSession(t, srv)

	ids, err := AddReaction(context.Background(), sess, ReactionDest{
		MsgID: "1", CliMsgID: "2", ThreadID: "t", Type: ThreadTypeUser,
	}, ReactionHeart)
	if err != nil {
		t.Fatalf("AddReaction: %v", err)
	}
	if len(ids) != 3 || ids[0] != 1 || ids[2] != 3 {
		t.Errorf("ids=%v, want [1 2 3]", ids)
	}
}

func TestAddReaction_ResponseArrayMsgIds(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(reactionResponseEnvelope(t, `[10,20]`)))
	}))
	t.Cleanup(srv.Close)
	sess := newReactionTestSession(t, srv)

	ids, err := AddReaction(context.Background(), sess, ReactionDest{
		MsgID: "1", CliMsgID: "2", ThreadID: "t", Type: ThreadTypeUser,
	}, ReactionHeart)
	if err != nil {
		t.Fatalf("AddReaction: %v", err)
	}
	if len(ids) != 2 || ids[0] != 10 {
		t.Errorf("ids=%v, want [10 20]", ids)
	}
}

func TestAddReaction_ServerError(t *testing.T) {
	t.Parallel()
	srv, _ := pollCaptureServer(t, "", 217)
	sess := newReactionTestSession(t, srv)
	_, err := AddReaction(context.Background(), sess, ReactionDest{
		MsgID: "1", CliMsgID: "2", ThreadID: "t", Type: ThreadTypeUser,
	}, ReactionHeart)
	if err == nil || !strings.Contains(err.Error(), "217") {
		t.Errorf("want error with code 217, got %v", err)
	}
}

func TestParseMsgIDs_EmptyAndShapes(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in   string
		want []int64
	}{
		{`[1,2,3]`, []int64{1, 2, 3}},
		{`"[4,5]"`, []int64{4, 5}},
		{`""`, nil},
	}
	for _, tc := range cases {
		got, err := parseMsgIDs(json.RawMessage(tc.in))
		if err != nil {
			t.Errorf("parseMsgIDs(%s): %v", tc.in, err)
		}
		if len(got) != len(tc.want) {
			t.Errorf("parseMsgIDs(%s) = %v, want %v", tc.in, got, tc.want)
		}
	}
}
