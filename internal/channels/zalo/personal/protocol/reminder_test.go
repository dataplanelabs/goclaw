package protocol

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestCreateReminderInGroup_WireShape(t *testing.T) {
	t.Parallel()
	srv, cap := captureServer(t, "100", 0)
	sess := newQuoteTestSession(t, srv)

	_, err := CreateReminderInGroup(context.Background(), sess, "group-abc", CreateReminderOptions{
		Title:     "test reminder",
		Emoji:     "📌",
		StartTime: 1700000000000,
		Repeat:    RepeatDaily,
		PinToTop:  true,
	})
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	if !strings.HasSuffix((*cap)[0].path, apiPathReminderGroupCreate) {
		t.Errorf("path = %q, want suffix %q", (*cap)[0].path, apiPathReminderGroupCreate)
	}
	payload := decryptRequestParams(t, (*cap)[0].body)

	cases := map[string]any{
		"grid":      "group-abc",
		"type":      float64(0),
		"color":     float64(reminderDefaultColor),
		"emoji":     "📌",
		"startTime": float64(1700000000000),
		"duration":  float64(-1),
		"repeat":    float64(1),
		"src":       float64(1),
		"imei":      "test-imei",
		"pinAct":    float64(1),
	}
	for k, want := range cases {
		if got := payload[k]; got != want {
			t.Errorf("payload[%q] = %#v, want %#v", k, got, want)
		}
	}
	paramsStr, _ := payload["params"].(string)
	if paramsStr != `{"title":"test reminder"}` {
		t.Errorf("params = %q, want stringified inner JSON", paramsStr)
	}
}

func TestCreateReminderInGroup_DefaultEmoji_NoPin(t *testing.T) {
	t.Parallel()
	srv, cap := captureServer(t, "200", 0)
	sess := newQuoteTestSession(t, srv)
	_, err := CreateReminderInGroup(context.Background(), sess, "g1", CreateReminderOptions{
		Title:  "x",
		Repeat: RepeatNone,
	})
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	payload := decryptRequestParams(t, (*cap)[0].body)
	if payload["emoji"] != reminderDefaultEmoji {
		t.Errorf("emoji = %v, want default %q", payload["emoji"], reminderDefaultEmoji)
	}
	if payload["pinAct"] != float64(0) {
		t.Errorf("pinAct = %v, want 0", payload["pinAct"])
	}
	// startTime should be approx now() since opts.StartTime was 0.
	now := time.Now().UnixMilli()
	st, _ := payload["startTime"].(float64)
	if delta := now - int64(st); delta < 0 || delta > 5000 {
		t.Errorf("startTime delta %d ms from now — want within 5s", delta)
	}
}

func TestCreateReminderInGroup_RepeatModes(t *testing.T) {
	t.Parallel()
	for mode, want := range map[RepeatMode]float64{RepeatNone: 0, RepeatDaily: 1, RepeatWeekly: 2, RepeatMonthly: 3} {
		srv, cap := captureServer(t, "300", 0)
		sess := newQuoteTestSession(t, srv)
		_, err := CreateReminderInGroup(context.Background(), sess, "g1", CreateReminderOptions{
			Title:  "r",
			Repeat: mode,
		})
		if err != nil {
			t.Fatalf("mode %d send: %v", mode, err)
		}
		payload := decryptRequestParams(t, (*cap)[0].body)
		if got := payload["repeat"]; got != want {
			t.Errorf("repeat for mode %d = %v, want %v", mode, got, want)
		}
	}
}

func TestCreateReminderInDM_WireShape(t *testing.T) {
	t.Parallel()
	srv, cap := captureServer(t, "400", 0)
	sess := newQuoteTestSession(t, srv)

	_, err := CreateReminderInDM(context.Background(), sess, "user-target-uid", CreateReminderOptions{
		Title:     "dm reminder",
		StartTime: 1700000000000,
		Repeat:    RepeatWeekly,
		PinToTop:  true, // ignored on DM
	})
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	if !strings.HasSuffix((*cap)[0].path, apiPathReminderDMCreate) {
		t.Errorf("path = %q, want suffix %q", (*cap)[0].path, apiPathReminderDMCreate)
	}
	payload := decryptRequestParams(t, (*cap)[0].body)
	if payload["imei"] != "test-imei" {
		t.Errorf("imei missing or wrong: %v", payload["imei"])
	}
	objectDataStr, ok := payload["objectData"].(string)
	if !ok {
		t.Fatalf("objectData missing or wrong type: %v", payload)
	}
	var inner map[string]any
	if err := json.Unmarshal([]byte(objectDataStr), &inner); err != nil {
		t.Fatalf("inner JSON: %v", err)
	}
	if inner["toUid"] != "user-target-uid" {
		t.Errorf("toUid = %v", inner["toUid"])
	}
	if inner["creatorUid"] != "self-uid" {
		t.Errorf("creatorUid = %v, want self-uid", inner["creatorUid"])
	}
	if inner["needPin"] != false {
		t.Errorf("needPin = %v, want false literal", inner["needPin"])
	}
	if inner["repeat"] != float64(2) {
		t.Errorf("repeat = %v, want 2 (Weekly)", inner["repeat"])
	}
	// CRITICAL: params is a NESTED OBJECT on DM, not a stringified JSON.
	innerParams, ok := inner["params"].(map[string]any)
	if !ok {
		t.Fatalf("DM inner params must be nested object, got %T = %v", inner["params"], inner["params"])
	}
	if innerParams["title"] != "dm reminder" {
		t.Errorf("inner params title = %v", innerParams["title"])
	}
}

func TestRemoveReminder_WireShape(t *testing.T) {
	t.Parallel()
	srv, cap := captureServer(t, "500", 0)
	sess := newQuoteTestSession(t, srv)

	err := RemoveReminder(context.Background(), sess, "topic-123", "group-abc")
	if err != nil {
		t.Fatalf("remove: %v", err)
	}
	if !strings.HasSuffix((*cap)[0].path, apiPathReminderRemove) {
		t.Errorf("path = %q, want suffix %q", (*cap)[0].path, apiPathReminderRemove)
	}
	payload := decryptRequestParams(t, (*cap)[0].body)
	if payload["topicId"] != "topic-123" {
		t.Errorf("topicId = %v", payload["topicId"])
	}
	if payload["grid"] != "group-abc" {
		t.Errorf("grid = %v", payload["grid"])
	}
}

func TestRemoveReminder_OmitsGridWhenEmpty(t *testing.T) {
	t.Parallel()
	srv, cap := captureServer(t, "500", 0)
	sess := newQuoteTestSession(t, srv)
	err := RemoveReminder(context.Background(), sess, "topic-dm-123", "")
	if err != nil {
		t.Fatalf("remove: %v", err)
	}
	payload := decryptRequestParams(t, (*cap)[0].body)
	if _, present := payload["grid"]; present {
		t.Errorf("grid should be absent for DM remove; payload=%v", payload)
	}
}

func TestCreateReminder_EmptyTitle_Errors(t *testing.T) {
	t.Parallel()
	sess := NewSession()
	if _, err := CreateReminderInGroup(context.Background(), sess, "g1", CreateReminderOptions{Title: "   "}); err == nil {
		t.Fatal("expected error for empty title")
	}
	if _, err := CreateReminderInDM(context.Background(), sess, "u1", CreateReminderOptions{Title: ""}); err == nil {
		t.Fatal("expected error for empty title")
	}
}

func TestCreateReminder_NoServiceURL_Errors(t *testing.T) {
	t.Parallel()
	sess := NewSession()
	sess.IMEI = "test-imei"
	// LoginInfo nil → getServiceURL returns "" → expect error.
	_, err := CreateReminderInGroup(context.Background(), sess, "g1", CreateReminderOptions{Title: "x"})
	if err == nil || !strings.Contains(err.Error(), "group_board") {
		t.Fatalf("expected group_board service URL error; got %v", err)
	}
}
