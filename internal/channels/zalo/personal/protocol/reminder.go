package protocol

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

const (
	apiPathReminderGroupCreate = "/api/board/topic/createv2"
	apiPathReminderDMCreate    = "/api/board/oneone/create"
	apiPathReminderRemove      = "/api/board/topic/remove"
	reminderServiceKey         = "group_board"
	reminderDefaultColor       = -16245706
	reminderDefaultEmoji       = "⏰"
)

// RepeatMode mirrors zca-js ReminderRepeatMode wire values.
type RepeatMode int

const (
	RepeatNone    RepeatMode = 0
	RepeatDaily   RepeatMode = 1
	RepeatWeekly  RepeatMode = 2
	RepeatMonthly RepeatMode = 3
)

// CreateReminderOptions configures a new reminder.
type CreateReminderOptions struct {
	Title     string     // required
	Emoji     string     // default "⏰"
	StartTime int64      // Unix ms; 0 → now
	Repeat    RepeatMode // default RepeatNone
	PinToTop  bool       // group only — DM ignores
}

// CreateReminderInGroup posts to /api/board/topic/createv2 (group_board service).
// Wire shape mirrors zca-js createReminder.ts group branch.
func CreateReminderInGroup(ctx context.Context, sess *Session, groupID string, opts CreateReminderOptions) (string, error) {
	if groupID == "" {
		return "", fmt.Errorf("zalo_personal: createReminder requires groupID")
	}
	if strings.TrimSpace(opts.Title) == "" {
		return "", fmt.Errorf("zalo_personal: createReminder requires title")
	}
	emoji := opts.Emoji
	if emoji == "" {
		emoji = reminderDefaultEmoji
	}
	startTime := opts.StartTime
	if startTime == 0 {
		startTime = time.Now().UnixMilli()
	}
	innerParams, err := json.Marshal(map[string]any{"title": opts.Title})
	if err != nil {
		return "", fmt.Errorf("zalo_personal: marshal reminder inner params: %w", err)
	}
	pinAct := 0
	if opts.PinToTop {
		pinAct = 1
	}
	payload := map[string]any{
		"grid":      groupID,
		"type":      0,
		"color":     reminderDefaultColor,
		"emoji":     emoji,
		"startTime": startTime,
		"duration":  -1,
		"params":    string(innerParams),
		"repeat":    int(opts.Repeat),
		"src":       1,
		"imei":      sess.IMEI,
		"pinAct":    pinAct,
	}
	var out struct {
		ID string `json:"id"`
	}
	if err := postEncryptedReminder(ctx, sess, apiPathReminderGroupCreate, payload, &out); err != nil {
		return "", err
	}
	return out.ID, nil
}

// CreateReminderInDM posts to /api/board/oneone/create (group_board service).
// DM form wraps the reminder fields under `objectData` per zca-js DM branch.
func CreateReminderInDM(ctx context.Context, sess *Session, toUID string, opts CreateReminderOptions) (string, error) {
	if toUID == "" {
		return "", fmt.Errorf("zalo_personal: createReminder requires toUID")
	}
	if strings.TrimSpace(opts.Title) == "" {
		return "", fmt.Errorf("zalo_personal: createReminder requires title")
	}
	emoji := opts.Emoji
	if emoji == "" {
		emoji = reminderDefaultEmoji
	}
	startTime := opts.StartTime
	if startTime == 0 {
		startTime = time.Now().UnixMilli()
	}
	if sess.LoginInfo == nil || sess.LoginInfo.UID == "" {
		return "", fmt.Errorf("zalo_personal: createReminder DM requires logged-in session uid")
	}
	creatorUID := sess.LoginInfo.UID
	// DM `params` is a NESTED OBJECT (not stringified) per zca-js.
	objectData, err := json.Marshal(map[string]any{
		"toUid":      toUID,
		"type":       0,
		"color":      reminderDefaultColor,
		"emoji":      emoji,
		"startTime":  startTime,
		"duration":   -1,
		"params":     map[string]any{"title": opts.Title},
		"needPin":    false,
		"repeat":     int(opts.Repeat),
		"creatorUid": creatorUID,
		"src":        1,
	})
	if err != nil {
		return "", fmt.Errorf("zalo_personal: marshal reminder objectData: %w", err)
	}
	payload := map[string]any{
		"objectData": string(objectData),
		"imei":       sess.IMEI,
	}
	var out struct {
		ReminderID string `json:"reminderId"`
		ID         string `json:"id"`
	}
	if err := postEncryptedReminder(ctx, sess, apiPathReminderDMCreate, payload, &out); err != nil {
		return "", err
	}
	if out.ReminderID != "" {
		return out.ReminderID, nil
	}
	return out.ID, nil
}

// RemoveReminder removes a reminder by ID. zca-js uses the same /topic/remove
// endpoint for both group and DM reminders; groupID may be empty for DM (Zalo
// accepts a missing grid when the topicId resolves to a DM reminder).
func RemoveReminder(ctx context.Context, sess *Session, reminderID, groupID string) error {
	if reminderID == "" {
		return fmt.Errorf("zalo_personal: removeReminder requires reminderID")
	}
	payload := map[string]any{
		"topicId": reminderID,
		"imei":    sess.IMEI,
	}
	if groupID != "" {
		payload["grid"] = groupID
	}
	var discard json.RawMessage
	return postEncryptedReminder(ctx, sess, apiPathReminderRemove, payload, &discard)
}

// postEncryptedReminder mirrors poll.go::postEncryptedJSON but routes through
// the group_board service (poll.go is hardcoded to "group").
func postEncryptedReminder(ctx context.Context, sess *Session, apiPath string, payload map[string]any, out any) error {
	baseURL := getServiceURL(sess, reminderServiceKey)
	if baseURL == "" {
		return fmt.Errorf("zalo_personal: no service URL for %s", reminderServiceKey)
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
