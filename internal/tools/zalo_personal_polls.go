package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/nextlevelbuilder/goclaw/internal/channels"
)

const (
	maxZaloPollExpirationSeconds = int64(10 * 365 * 24 * 60 * 60)
	defaultZaloPollListCount     = 20
	maxZaloPollListCount         = 20
)

func isZaloPersonalChannel(ctx context.Context) bool {
	return ToolChannelTypeFromCtx(ctx) == channels.TypeZaloPersonal
}

func resolveZaloPersonalAction(ctx context.Context, fn ZaloPersonalActionFn) (ZaloPersonalAction, *Result) {
	if fn == nil {
		return nil, ErrorResult("zalo_personal action function not wired")
	}
	if !isZaloPersonalChannel(ctx) {
		return nil, ErrorResult("only available on zalo_personal channels")
	}
	channelName := ToolChannelFromCtx(ctx)
	if channelName == "" {
		return nil, ErrorResult("missing channel name in context")
	}
	handle, err := fn(ctx, channelName)
	if err != nil {
		return nil, ErrorResult(fmt.Sprintf("resolve channel: %v", err))
	}
	return handle, nil
}

func jsonResult(payload map[string]any) *Result {
	blob, _ := json.Marshal(payload)
	return NewResult(string(blob))
}

// --- create_poll ---

type ZaloPersonalCreatePollTool struct {
	actionFn ZaloPersonalActionFn
}

func NewZaloPersonalCreatePollTool() *ZaloPersonalCreatePollTool {
	return &ZaloPersonalCreatePollTool{}
}

func (t *ZaloPersonalCreatePollTool) SetZaloPersonalActionFn(fn ZaloPersonalActionFn) {
	t.actionFn = fn
}

func (t *ZaloPersonalCreatePollTool) RequiredChannelTypes() []string {
	return []string{channels.TypeZaloPersonal}
}

func (t *ZaloPersonalCreatePollTool) Name() string { return "zalo_personal_create_poll" }

func (t *ZaloPersonalCreatePollTool) Description() string {
	return "Create a poll in the current Zalo Personal group chat. Returns the poll ID. Only works in groups, not DMs. Pass expired_time_seconds as a duration in seconds, not a Unix timestamp. Only set allow_add_new_option when the user explicitly asks voters to add choices; if Zalo returns code 114, retry the same question/options with allow_add_new_option=false before changing expiry or inventing options."
}

func (t *ZaloPersonalCreatePollTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"question": map[string]any{
				"type":        "string",
				"description": "Poll question text",
			},
			"options": map[string]any{
				"type":        "array",
				"items":       map[string]any{"type": "string"},
				"description": "Poll answer options (at least 2)",
			},
			"expired_time_seconds": map[string]any{
				"type":        "integer",
				"description": "Optional poll lifetime as a positive duration in seconds (0 or omitted = no expiration). Do not pass Unix timestamps or milliseconds.",
			},
			"allow_multi_choices":  map[string]any{"type": "boolean", "description": "Allow voters to pick multiple options"},
			"is_anonymous":         map[string]any{"type": "boolean", "description": "Hide voter identities"},
			"allow_add_new_option": map[string]any{"type": "boolean", "description": "Let voters add new options. Leave false unless the user explicitly asks for this; Zalo may reject this flag with error code 114 for otherwise valid polls."},
			"hide_vote_preview":    map[string]any{"type": "boolean", "description": "Hide results until voter votes"},
		},
		"required": []string{"question", "options"},
	}
}

func (t *ZaloPersonalCreatePollTool) Execute(ctx context.Context, args map[string]any) *Result {
	handle, errRes := resolveZaloPersonalAction(ctx, t.actionFn)
	if errRes != nil {
		return errRes
	}
	chatID := ToolChatIDFromCtx(ctx)
	if chatID == "" {
		return ErrorResult("missing chat ID in context")
	}
	question := strings.TrimSpace(argString(args, "question"))
	options := argStringSlice(args, "options")
	if question == "" {
		return ErrorResult("question is required")
	}
	if len(options) < 2 {
		return ErrorResult("at least 2 options required")
	}
	expirySeconds := argInt64(args, "expired_time_seconds")
	expireAtMillis, expiryErr := normalizeZaloPollExpireAtMillis(expirySeconds, time.Now())
	if expiryErr != nil {
		return expiryErr
	}
	settings := ZaloPollSettings{
		ExpireAtMillis:    expireAtMillis,
		AllowMultiChoices: argBool(args, "allow_multi_choices"),
		AllowAddNewOption: argBool(args, "allow_add_new_option"),
		HideVotePreview:   argBool(args, "hide_vote_preview"),
		IsAnonymous:       argBool(args, "is_anonymous"),
	}
	pollID, err := handle.CreatePoll(ctx, chatID, question, options, settings)
	if err != nil {
		return ErrorResult(formatCreatePollError(err, question, options, expirySeconds, settings))
	}
	return jsonResult(map[string]any{"poll_id": pollID, "status": "created"})
}

func normalizeZaloPollExpireAtMillis(seconds int64, now time.Time) (int64, *Result) {
	switch {
	case seconds < 0:
		return 0, ErrorResult("expired_time_seconds must be 0 or a positive duration in seconds")
	case seconds == 0:
		return 0, nil
	case seconds > maxZaloPollExpirationSeconds:
		return 0, ErrorResult(fmt.Sprintf(
			"expired_time_seconds=%d is out of range or looks like a timestamp. Pass a duration in seconds, for example 604800 for 7 days. Maximum accepted by this tool is %d seconds.",
			seconds, maxZaloPollExpirationSeconds,
		))
	default:
		return now.Add(time.Duration(seconds) * time.Second).UnixMilli(), nil
	}
}

func formatCreatePollError(err error, question string, options []string, expirySeconds int64, settings ZaloPollSettings) string {
	msg := fmt.Sprintf("create poll: %v", err)
	if !isZaloInvalidPollParameterError(err) {
		return msg
	}
	return fmt.Sprintf(
		"%s. %s. Sent shape: question_chars=%d, options_count=%d, expired_time_seconds=%d, expire_at_ms=%d, allow_multi_choices=%t, allow_add_new_option=%t, hide_vote_preview=%t, is_anonymous=%t. Repair order: %s",
		msg,
		describeZaloInvalidPollParameterError(err),
		len([]rune(question)),
		len(options),
		expirySeconds,
		settings.ExpireAtMillis,
		settings.AllowMultiChoices,
		settings.AllowAddNewOption,
		settings.HideVotePreview,
		settings.IsAnonymous,
		strings.Join(createPollRepairHints(options, expirySeconds, settings), " "),
	)
}

func isZaloInvalidPollParameterError(err error) bool {
	if err == nil {
		return false
	}
	lower := strings.ToLower(err.Error())
	return strings.Contains(lower, "114") || strings.Contains(lower, "tham số") || strings.Contains(lower, "invalid parameter")
}

func describeZaloInvalidPollParameterError(err error) string {
	if strings.Contains(err.Error(), "114") {
		return "Zalo returned code 114 (invalid poll parameter)"
	}
	return "Zalo rejected one or more poll parameters"
}

func createPollRepairHints(options []string, expirySeconds int64, settings ZaloPollSettings) []string {
	hints := make([]string, 0, 5)
	if settings.AllowAddNewOption {
		hints = append(hints, "First retry the same user-provided question/options with allow_add_new_option=false unless the user explicitly asked voters to add choices.")
	}
	if len(options) > 6 {
		hints = append(hints, fmt.Sprintf("If that still fails, options_count=%d may be rejected by this Zalo client/account path; ask the user before reducing choices.", len(options)))
	}
	if hasBlankPollOption(options) {
		hints = append(hints, "Remove blank options.")
	}
	if hasDuplicatePollOption(options) {
		hints = append(hints, "Remove duplicate options.")
	}
	switch {
	case expirySeconds > 0:
		hints = append(hints, fmt.Sprintf("expired_time_seconds=%d was converted to future Zalo expire_at_ms=%d; if Zalo still rejects it, retry once without expiry or ask the user.", expirySeconds, settings.ExpireAtMillis))
	default:
		hints = append(hints, "expired_time_seconds=0 means no expiration and is locally valid.")
	}
	hints = append(hints, "Do not retry with placeholder options; correct the invalid field or ask the user.")
	return hints
}

func hasBlankPollOption(options []string) bool {
	for _, option := range options {
		if strings.TrimSpace(option) == "" {
			return true
		}
	}
	return false
}

func hasDuplicatePollOption(options []string) bool {
	seen := make(map[string]struct{}, len(options))
	for _, option := range options {
		normalized := strings.ToLower(strings.TrimSpace(option))
		if normalized == "" {
			continue
		}
		if _, ok := seen[normalized]; ok {
			return true
		}
		seen[normalized] = struct{}{}
	}
	return false
}

// --- list_polls ---

type ZaloPersonalListPollsTool struct{ actionFn ZaloPersonalActionFn }

func NewZaloPersonalListPollsTool() *ZaloPersonalListPollsTool { return &ZaloPersonalListPollsTool{} }
func (t *ZaloPersonalListPollsTool) SetZaloPersonalActionFn(fn ZaloPersonalActionFn) {
	t.actionFn = fn
}
func (t *ZaloPersonalListPollsTool) RequiredChannelTypes() []string {
	return []string{channels.TypeZaloPersonal}
}
func (t *ZaloPersonalListPollsTool) Name() string { return "zalo_personal_list_polls" }
func (t *ZaloPersonalListPollsTool) Description() string {
	return "List recent polls from the current Zalo Personal group board with vote counts. Use this before answering questions like 'show the poll result' when the poll was created outside the current context window; call zalo_personal_get_poll with a returned poll_id to refresh one poll exactly."
}
func (t *ZaloPersonalListPollsTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"page":  map[string]any{"type": "integer", "description": "Board page to read, default 1"},
			"count": map[string]any{"type": "integer", "description": "Number of board items to inspect, default 20, max 20"},
		},
		"required": []string{},
	}
}
func (t *ZaloPersonalListPollsTool) Execute(ctx context.Context, args map[string]any) *Result {
	handle, errRes := resolveZaloPersonalAction(ctx, t.actionFn)
	if errRes != nil {
		return errRes
	}
	chatID := ToolChatIDFromCtx(ctx)
	if chatID == "" {
		return ErrorResult("missing chat ID in context")
	}
	page, count := normalizedPollListPageCount(args)
	list, err := handle.ListPolls(ctx, chatID, page, count)
	if err != nil {
		return ErrorResult(fmt.Sprintf("list polls: %v", err))
	}
	blob, _ := json.Marshal(list)
	return NewResult(string(blob))
}

func normalizedPollListPageCount(args map[string]any) (int, int) {
	page := max(int(argInt64(args, "page")), 1)
	count := int(argInt64(args, "count"))
	switch {
	case count <= 0:
		count = defaultZaloPollListCount
	case count > maxZaloPollListCount:
		count = maxZaloPollListCount
	}
	return page, count
}

// --- get_poll ---

type ZaloPersonalGetPollTool struct{ actionFn ZaloPersonalActionFn }

func NewZaloPersonalGetPollTool() *ZaloPersonalGetPollTool                         { return &ZaloPersonalGetPollTool{} }
func (t *ZaloPersonalGetPollTool) SetZaloPersonalActionFn(fn ZaloPersonalActionFn) { t.actionFn = fn }
func (t *ZaloPersonalGetPollTool) RequiredChannelTypes() []string {
	return []string{channels.TypeZaloPersonal}
}
func (t *ZaloPersonalGetPollTool) Name() string { return "zalo_personal_get_poll" }
func (t *ZaloPersonalGetPollTool) Description() string {
	return "Read the current state of a Zalo Personal poll: options, vote counts, locked flag."
}
func (t *ZaloPersonalGetPollTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"poll_id": map[string]any{"type": "string", "description": "Poll ID returned by create_poll"},
		},
		"required": []string{"poll_id"},
	}
}
func (t *ZaloPersonalGetPollTool) Execute(ctx context.Context, args map[string]any) *Result {
	handle, errRes := resolveZaloPersonalAction(ctx, t.actionFn)
	if errRes != nil {
		return errRes
	}
	pollID := argInt64(args, "poll_id")
	if pollID == 0 {
		return ErrorResult("poll_id is required")
	}
	state, err := handle.GetPoll(ctx, pollID)
	if err != nil {
		return ErrorResult(fmt.Sprintf("get poll: %v", err))
	}
	blob, _ := json.Marshal(state)
	return NewResult(string(blob))
}

// --- vote_poll ---

type ZaloPersonalVotePollTool struct{ actionFn ZaloPersonalActionFn }

func NewZaloPersonalVotePollTool() *ZaloPersonalVotePollTool                        { return &ZaloPersonalVotePollTool{} }
func (t *ZaloPersonalVotePollTool) SetZaloPersonalActionFn(fn ZaloPersonalActionFn) { t.actionFn = fn }
func (t *ZaloPersonalVotePollTool) RequiredChannelTypes() []string {
	return []string{channels.TypeZaloPersonal}
}
func (t *ZaloPersonalVotePollTool) Name() string { return "zalo_personal_vote_poll" }
func (t *ZaloPersonalVotePollTool) Description() string {
	return "Vote on a Zalo Personal poll. Pass empty option_ids to unvote."
}
func (t *ZaloPersonalVotePollTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"poll_id":    map[string]any{"type": "string", "description": "Poll ID"},
			"option_ids": map[string]any{"type": "array", "items": map[string]any{"type": "integer"}, "description": "Option IDs to vote for (empty = unvote)"},
		},
		"required": []string{"poll_id"},
	}
}
func (t *ZaloPersonalVotePollTool) Execute(ctx context.Context, args map[string]any) *Result {
	handle, errRes := resolveZaloPersonalAction(ctx, t.actionFn)
	if errRes != nil {
		return errRes
	}
	pollID := argInt64(args, "poll_id")
	if pollID == 0 {
		return ErrorResult("poll_id is required")
	}
	state, err := handle.VotePoll(ctx, pollID, argInt64Slice(args, "option_ids"))
	if err != nil {
		return ErrorResult(fmt.Sprintf("vote poll: %v", err))
	}
	blob, _ := json.Marshal(state)
	return NewResult(string(blob))
}

// --- lock_poll ---

type ZaloPersonalLockPollTool struct{ actionFn ZaloPersonalActionFn }

func NewZaloPersonalLockPollTool() *ZaloPersonalLockPollTool                        { return &ZaloPersonalLockPollTool{} }
func (t *ZaloPersonalLockPollTool) SetZaloPersonalActionFn(fn ZaloPersonalActionFn) { t.actionFn = fn }
func (t *ZaloPersonalLockPollTool) RequiredChannelTypes() []string {
	return []string{channels.TypeZaloPersonal}
}
func (t *ZaloPersonalLockPollTool) Name() string { return "zalo_personal_lock_poll" }
func (t *ZaloPersonalLockPollTool) Description() string {
	return "Close a Zalo Personal poll so no more votes are accepted."
}
func (t *ZaloPersonalLockPollTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"poll_id": map[string]any{"type": "string", "description": "Poll ID"},
		},
		"required": []string{"poll_id"},
	}
}
func (t *ZaloPersonalLockPollTool) Execute(ctx context.Context, args map[string]any) *Result {
	handle, errRes := resolveZaloPersonalAction(ctx, t.actionFn)
	if errRes != nil {
		return errRes
	}
	pollID := argInt64(args, "poll_id")
	if pollID == 0 {
		return ErrorResult("poll_id is required")
	}
	if err := handle.LockPoll(ctx, pollID); err != nil {
		return ErrorResult(fmt.Sprintf("lock poll: %v", err))
	}
	return jsonResult(map[string]any{"status": "locked", "poll_id": pollID})
}

// --- add_poll_options ---

type ZaloPersonalAddPollOptionsTool struct{ actionFn ZaloPersonalActionFn }

func NewZaloPersonalAddPollOptionsTool() *ZaloPersonalAddPollOptionsTool {
	return &ZaloPersonalAddPollOptionsTool{}
}
func (t *ZaloPersonalAddPollOptionsTool) SetZaloPersonalActionFn(fn ZaloPersonalActionFn) {
	t.actionFn = fn
}
func (t *ZaloPersonalAddPollOptionsTool) RequiredChannelTypes() []string {
	return []string{channels.TypeZaloPersonal}
}
func (t *ZaloPersonalAddPollOptionsTool) Name() string {
	return "zalo_personal_add_poll_options"
}
func (t *ZaloPersonalAddPollOptionsTool) Description() string {
	return "Append new options to an existing Zalo Personal poll."
}
func (t *ZaloPersonalAddPollOptionsTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"poll_id":          map[string]any{"type": "string", "description": "Poll ID"},
			"new_options":      map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "New option texts"},
			"voted_option_ids": map[string]any{"type": "array", "items": map[string]any{"type": "integer"}, "description": "Caller's currently-voted option IDs (optional)"},
		},
		"required": []string{"poll_id", "new_options"},
	}
}
func (t *ZaloPersonalAddPollOptionsTool) Execute(ctx context.Context, args map[string]any) *Result {
	handle, errRes := resolveZaloPersonalAction(ctx, t.actionFn)
	if errRes != nil {
		return errRes
	}
	pollID := argInt64(args, "poll_id")
	if pollID == 0 {
		return ErrorResult("poll_id is required")
	}
	newOpts := argStringSlice(args, "new_options")
	if len(newOpts) == 0 {
		return ErrorResult("new_options must contain at least one entry")
	}
	state, err := handle.AddPollOptions(ctx, pollID, newOpts, argInt64Slice(args, "voted_option_ids"))
	if err != nil {
		return ErrorResult(fmt.Sprintf("add poll options: %v", err))
	}
	blob, _ := json.Marshal(state)
	return NewResult(string(blob))
}
