package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/nextlevelbuilder/goclaw/internal/channels"
)

// fakeZaloPersonalAction records the last call so tests can assert routing.
type fakeZaloPersonalAction struct {
	createPollCall struct {
		chatID, question string
		options          []string
		settings         ZaloPollSettings
	}
	listCall struct {
		chatID      string
		page, count int
	}
	getPollID int64
	voteCall  struct {
		pollID int64
		ids    []int64
	}
	lockCall int64
	addCall  struct {
		pollID int64
		opts   []string
		voted  []int64
	}
	reactCall          struct{ chatID, msgID, cliMsgID, reaction, hint string }
	createReminderCall struct {
		threadID string
		isGroup  bool
		settings ZaloReminderSettings
	}
	removeReminderCall   struct{ reminderID, groupID string }
	createReturn         string
	createErr            error
	listReturn           ZaloPollList
	getReturn            ZaloPollState
	voteReturn           ZaloPollState
	addReturn            ZaloPollState
	reactErr             error
	createReminderReturn string
	createReminderErr    error
	removeReminderErr    error
	isGroup              bool
	isRunning            bool
}

var _ ZaloPersonalAction = (*fakeZaloPersonalAction)(nil)

func (f *fakeZaloPersonalAction) CreatePoll(_ context.Context, chatID, q string, opts []string, s ZaloPollSettings) (string, error) {
	f.createPollCall.chatID = chatID
	f.createPollCall.question = q
	f.createPollCall.options = opts
	f.createPollCall.settings = s
	return f.createReturn, f.createErr
}
func (f *fakeZaloPersonalAction) ListPolls(_ context.Context, chatID string, page, count int) (ZaloPollList, error) {
	f.listCall.chatID = chatID
	f.listCall.page = page
	f.listCall.count = count
	return f.listReturn, nil
}
func (f *fakeZaloPersonalAction) GetPoll(_ context.Context, id int64) (ZaloPollState, error) {
	f.getPollID = id
	return f.getReturn, nil
}
func (f *fakeZaloPersonalAction) VotePoll(_ context.Context, id int64, ids []int64) (ZaloPollState, error) {
	f.voteCall.pollID = id
	f.voteCall.ids = ids
	return f.voteReturn, nil
}
func (f *fakeZaloPersonalAction) LockPoll(_ context.Context, id int64) error {
	f.lockCall = id
	return nil
}
func (f *fakeZaloPersonalAction) AddPollOptions(_ context.Context, id int64, opts []string, voted []int64) (ZaloPollState, error) {
	f.addCall.pollID = id
	f.addCall.opts = opts
	f.addCall.voted = voted
	return f.addReturn, nil
}
func (f *fakeZaloPersonalAction) React(_ context.Context, chatID, msgID, cliMsgID, reaction, hint string) error {
	f.reactCall.chatID = chatID
	f.reactCall.msgID = msgID
	f.reactCall.cliMsgID = cliMsgID
	f.reactCall.reaction = reaction
	f.reactCall.hint = hint
	return f.reactErr
}
func (f *fakeZaloPersonalAction) CreateReminder(_ context.Context, threadID string, isGroup bool, s ZaloReminderSettings) (string, error) {
	f.createReminderCall.threadID = threadID
	f.createReminderCall.isGroup = isGroup
	f.createReminderCall.settings = s
	return f.createReminderReturn, f.createReminderErr
}
func (f *fakeZaloPersonalAction) RemoveReminder(_ context.Context, reminderID, groupID string) error {
	f.removeReminderCall.reminderID = reminderID
	f.removeReminderCall.groupID = groupID
	return f.removeReminderErr
}
func (f *fakeZaloPersonalAction) IsRunning() bool       { return f.isRunning }
func (f *fakeZaloPersonalAction) IsGroup(_ string) bool { return f.isGroup }

func zpFakeFn(fake *fakeZaloPersonalAction) ZaloPersonalActionFn {
	return func(_ context.Context, _ string) (ZaloPersonalAction, error) { return fake, nil }
}

func zpCtx(t *testing.T) context.Context {
	t.Helper()
	ctx := context.Background()
	ctx = WithToolChannel(ctx, "my-zalo")
	ctx = WithToolChannelType(ctx, channels.TypeZaloPersonal)
	ctx = WithToolChatID(ctx, "group-1")
	return ctx
}

func TestCreatePoll_HappyPath(t *testing.T) {
	t.Parallel()
	fake := &fakeZaloPersonalAction{createReturn: "1234"}
	tool := NewZaloPersonalCreatePollTool()
	tool.SetZaloPersonalActionFn(zpFakeFn(fake))

	res := tool.Execute(zpCtx(t), map[string]any{
		"question": "Lunch?",
		"options":  []any{"pizza", "sushi"},
	})
	if res.IsError {
		t.Fatalf("unexpected error: %s", res.ForLLM)
	}
	if fake.createPollCall.question != "Lunch?" || len(fake.createPollCall.options) != 2 {
		t.Errorf("call mismatch: %+v", fake.createPollCall)
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(res.ForLLM), &out); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if out["poll_id"] != "1234" {
		t.Errorf("poll_id=%v", out["poll_id"])
	}
}

func TestCreatePoll_ConvertsExpiryDurationToFutureUnixMillis(t *testing.T) {
	t.Parallel()
	fake := &fakeZaloPersonalAction{createReturn: "1234"}
	tool := NewZaloPersonalCreatePollTool()
	tool.SetZaloPersonalActionFn(zpFakeFn(fake))

	before := time.Now().Add(24*time.Hour - time.Second).UnixMilli()
	res := tool.Execute(zpCtx(t), map[string]any{
		"question":             "Q",
		"options":              []any{"a", "b"},
		"expired_time_seconds": float64(86400),
	})
	after := time.Now().Add(24*time.Hour + time.Second).UnixMilli()
	if res.IsError {
		t.Fatalf("unexpected error: %s", res.ForLLM)
	}
	got := fake.createPollCall.settings.ExpireAtMillis
	if got < before || got > after {
		t.Fatalf("expire_at_ms=%d, want between %d and %d", got, before, after)
	}
	if got == 86400*1000 {
		t.Fatalf("expire_at_ms must be an absolute future timestamp, got duration milliseconds")
	}
}

func TestNormalizeZaloPollExpireAtMillis(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 6, 14, 8, 22, 9, 0, time.UTC)

	got, errRes := normalizeZaloPollExpireAtMillis(86400, now)
	if errRes != nil {
		t.Fatalf("unexpected validation error: %s", errRes.ForLLM)
	}
	const want = int64(1781511729000)
	if got != want {
		t.Fatalf("expire_at_ms=%d, want %d", got, want)
	}
}

func TestCreatePoll_MissingOptions(t *testing.T) {
	t.Parallel()
	tool := NewZaloPersonalCreatePollTool()
	tool.SetZaloPersonalActionFn(zpFakeFn(&fakeZaloPersonalAction{}))
	res := tool.Execute(zpCtx(t), map[string]any{
		"question": "Q",
		"options":  []any{"only-one"},
	})
	if !res.IsError {
		t.Errorf("want error for <2 options")
	}
}

func TestCreatePoll_RejectsTimestampExpiry(t *testing.T) {
	t.Parallel()
	fake := &fakeZaloPersonalAction{}
	tool := NewZaloPersonalCreatePollTool()
	tool.SetZaloPersonalActionFn(zpFakeFn(fake))

	res := tool.Execute(zpCtx(t), map[string]any{
		"question":             "Q",
		"options":              []any{"a", "b"},
		"expired_time_seconds": float64(1770000000000),
	})
	if !res.IsError || !strings.Contains(res.ForLLM, "duration in seconds") {
		t.Fatalf("want duration validation error, got: %+v", res)
	}
	if fake.createPollCall.question != "" {
		t.Fatalf("CreatePoll should not be called after validation failure: %+v", fake.createPollCall)
	}
}

func TestCreatePoll_Error114AddsFieldGuidance(t *testing.T) {
	t.Parallel()
	fake := &fakeZaloPersonalAction{createErr: fmt.Errorf("zalo_personal: inner error code 114: Tham số không hợp lệ")}
	tool := NewZaloPersonalCreatePollTool()
	tool.SetZaloPersonalActionFn(zpFakeFn(fake))

	res := tool.Execute(zpCtx(t), map[string]any{
		"question":             "Q",
		"options":              []any{"a", "b"},
		"expired_time_seconds": float64(3600),
	})
	if !res.IsError {
		t.Fatal("want error")
	}
	for _, want := range []string{"code 114", "options_count=2", "expired_time_seconds=3600", "expire_at_ms=", "future Zalo expire_at_ms", "Do not retry with placeholder options"} {
		if !strings.Contains(res.ForLLM, want) {
			t.Fatalf("error missing %q: %s", want, res.ForLLM)
		}
	}
	if strings.Contains(res.ForLLM, "Check expired_time_seconds first") {
		t.Fatalf("error should not prioritize expiry after local validation: %s", res.ForLLM)
	}
}

func TestCreatePoll_Error114SuggestsDisablingAddOptionFlag(t *testing.T) {
	t.Parallel()
	fake := &fakeZaloPersonalAction{
		createErr: fmt.Errorf("zalo_personal: decrypt /api/poll/create response: zalo_personal: inner error code 114: Tham số không hợp lệ"),
	}
	tool := NewZaloPersonalCreatePollTool()
	tool.SetZaloPersonalActionFn(zpFakeFn(fake))

	res := tool.Execute(zpCtx(t), map[string]any{
		"question":             "Example poll question text",
		"options":              []any{"Option 1", "Option 2", "Option 3", "Option 4", "Option 5", "Option 6", "Option 7", "Option 8"},
		"expired_time_seconds": float64(86400),
		"allow_add_new_option": true,
	})
	if !res.IsError {
		t.Fatal("want error")
	}
	for _, want := range []string{
		"options_count=8",
		"expired_time_seconds=86400",
		"allow_add_new_option=true",
		"First retry the same user-provided question/options with allow_add_new_option=false",
		"future Zalo expire_at_ms",
		"Do not retry with placeholder options",
	} {
		if !strings.Contains(res.ForLLM, want) {
			t.Fatalf("error missing %q: %s", want, res.ForLLM)
		}
	}
	if strings.Contains(res.ForLLM, "Check expired_time_seconds first") {
		t.Fatalf("error should not prioritize expiry after local validation: %s", res.ForLLM)
	}
}

func TestCreatePoll_GenericInvalidErrorDoesNotAddParameterRepairHints(t *testing.T) {
	t.Parallel()
	fake := &fakeZaloPersonalAction{createErr: fmt.Errorf("zalo_personal: invalid session")}
	tool := NewZaloPersonalCreatePollTool()
	tool.SetZaloPersonalActionFn(zpFakeFn(fake))

	res := tool.Execute(zpCtx(t), map[string]any{
		"question": "Q",
		"options":  []any{"a", "b"},
	})
	if !res.IsError {
		t.Fatal("want error")
	}
	if strings.Contains(res.ForLLM, "Repair order") {
		t.Fatalf("generic invalid error should not receive poll parameter hints: %s", res.ForLLM)
	}
}

func TestCreatePoll_WrongChannelType(t *testing.T) {
	t.Parallel()
	tool := NewZaloPersonalCreatePollTool()
	tool.SetZaloPersonalActionFn(zpFakeFn(&fakeZaloPersonalAction{}))
	ctx := context.Background()
	ctx = WithToolChannelType(ctx, "telegram")
	ctx = WithToolChannel(ctx, "tg-bot")
	ctx = WithToolChatID(ctx, "chat")
	res := tool.Execute(ctx, map[string]any{
		"question": "Q", "options": []any{"a", "b"},
	})
	if !res.IsError || !strings.Contains(res.ForLLM, "zalo_personal") {
		t.Errorf("want channel-type error, got: %+v", res)
	}
}

func TestCreatePoll_NoActionFn(t *testing.T) {
	t.Parallel()
	tool := NewZaloPersonalCreatePollTool()
	res := tool.Execute(zpCtx(t), map[string]any{
		"question": "Q", "options": []any{"a", "b"},
	})
	if !res.IsError || !strings.Contains(res.ForLLM, "not wired") {
		t.Errorf("want not-wired error: %+v", res)
	}
}

func TestListPolls_HappyPath(t *testing.T) {
	t.Parallel()
	fake := &fakeZaloPersonalAction{listReturn: ZaloPollList{
		Page:  2,
		Count: 1,
		Polls: []ZaloPollState{{
			PollID:     "42",
			Question:   "Lunch?",
			TotalVotes: 3,
			Options: []ZaloPollStateOption{
				{OptionID: 1, Content: "pizza", VoteCount: 2},
				{OptionID: 2, Content: "sushi", VoteCount: 1},
			},
		}},
	}}
	tool := NewZaloPersonalListPollsTool()
	tool.SetZaloPersonalActionFn(zpFakeFn(fake))

	res := tool.Execute(zpCtx(t), map[string]any{"page": float64(2), "count": float64(99)})
	if res.IsError {
		t.Fatalf("unexpected error: %s", res.ForLLM)
	}
	if fake.listCall.chatID != "group-1" || fake.listCall.page != 2 || fake.listCall.count != maxZaloPollListCount {
		t.Fatalf("list call mismatch: %+v", fake.listCall)
	}
	var out ZaloPollList
	if err := json.Unmarshal([]byte(res.ForLLM), &out); err != nil {
		t.Fatalf("decode list result: %v", err)
	}
	if len(out.Polls) != 1 || out.Polls[0].PollID != "42" {
		t.Fatalf("unexpected list result: %+v", out)
	}
}

func TestGetPoll_StringIDParses(t *testing.T) {
	t.Parallel()
	fake := &fakeZaloPersonalAction{getReturn: ZaloPollState{PollID: "42"}}
	tool := NewZaloPersonalGetPollTool()
	tool.SetZaloPersonalActionFn(zpFakeFn(fake))

	res := tool.Execute(zpCtx(t), map[string]any{"poll_id": "42"})
	if res.IsError {
		t.Fatalf("unexpected error: %s", res.ForLLM)
	}
	if fake.getPollID != 42 {
		t.Errorf("argInt64 must accept string IDs, got %d", fake.getPollID)
	}
}

func TestVotePoll_EmptyOptionsMeansUnvote(t *testing.T) {
	t.Parallel()
	fake := &fakeZaloPersonalAction{}
	tool := NewZaloPersonalVotePollTool()
	tool.SetZaloPersonalActionFn(zpFakeFn(fake))

	res := tool.Execute(zpCtx(t), map[string]any{"poll_id": "100"})
	if res.IsError {
		t.Fatalf("unexpected error: %s", res.ForLLM)
	}
	if fake.voteCall.pollID != 100 || fake.voteCall.ids != nil {
		t.Errorf("unvote not routed correctly: %+v", fake.voteCall)
	}
}

func TestLockPoll_HappyPath(t *testing.T) {
	t.Parallel()
	fake := &fakeZaloPersonalAction{}
	tool := NewZaloPersonalLockPollTool()
	tool.SetZaloPersonalActionFn(zpFakeFn(fake))

	res := tool.Execute(zpCtx(t), map[string]any{"poll_id": int64(99)})
	if res.IsError {
		t.Fatalf("unexpected error: %s", res.ForLLM)
	}
	if fake.lockCall != 99 {
		t.Errorf("lockCall=%d, want 99", fake.lockCall)
	}
}

func TestAddPollOptions_HappyPath(t *testing.T) {
	t.Parallel()
	fake := &fakeZaloPersonalAction{}
	tool := NewZaloPersonalAddPollOptionsTool()
	tool.SetZaloPersonalActionFn(zpFakeFn(fake))

	res := tool.Execute(zpCtx(t), map[string]any{
		"poll_id":          "55",
		"new_options":      []any{"soup", "salad"},
		"voted_option_ids": []any{float64(1)},
	})
	if res.IsError {
		t.Fatalf("unexpected error: %s", res.ForLLM)
	}
	if len(fake.addCall.opts) != 2 || fake.addCall.opts[0] != "soup" {
		t.Errorf("opts=%+v", fake.addCall.opts)
	}
	if len(fake.addCall.voted) != 1 || fake.addCall.voted[0] != 1 {
		t.Errorf("voted_option_ids=%+v", fake.addCall.voted)
	}
}

func TestParametersAreValidJSONSchema(t *testing.T) {
	t.Parallel()
	tools := []interface {
		Name() string
		Parameters() map[string]any
	}{
		NewZaloPersonalCreatePollTool(),
		NewZaloPersonalListPollsTool(),
		NewZaloPersonalGetPollTool(),
		NewZaloPersonalVotePollTool(),
		NewZaloPersonalLockPollTool(),
		NewZaloPersonalAddPollOptionsTool(),
	}
	for _, tl := range tools {
		p := tl.Parameters()
		if p["type"] != "object" {
			t.Errorf("%s: type must be object, got %v", tl.Name(), p["type"])
		}
		if _, ok := p["properties"].(map[string]any); !ok {
			t.Errorf("%s: properties missing or wrong type", tl.Name())
		}
		if _, ok := p["required"].([]string); !ok {
			t.Errorf("%s: required must be []string, got %T", tl.Name(), p["required"])
		}
		// Encode/decode roundtrip to catch any non-marshalable values.
		if _, err := json.Marshal(p); err != nil {
			t.Errorf("%s: schema not marshalable: %v", tl.Name(), err)
		}
	}
}

func TestToolNamesMatchExpected(t *testing.T) {
	t.Parallel()
	got := map[string]string{
		NewZaloPersonalCreatePollTool().Name():     "zalo_personal_create_poll",
		NewZaloPersonalListPollsTool().Name():      "zalo_personal_list_polls",
		NewZaloPersonalGetPollTool().Name():        "zalo_personal_get_poll",
		NewZaloPersonalVotePollTool().Name():       "zalo_personal_vote_poll",
		NewZaloPersonalLockPollTool().Name():       "zalo_personal_lock_poll",
		NewZaloPersonalAddPollOptionsTool().Name(): "zalo_personal_add_poll_options",
	}
	for name, want := range got {
		if name != want {
			t.Errorf("name drift: got %q want %q", name, want)
		}
	}
}

// Compile-time assert each tool implements ZaloPersonalActionAware + ChannelAware.
// Missing ChannelAware would silently leak these tools into every agent's LLM
// tool list (loop_tool_filter.go uses it to filter per-request).
var _ = func() bool {
	var _ ZaloPersonalActionAware = (*ZaloPersonalCreatePollTool)(nil)
	var _ ZaloPersonalActionAware = (*ZaloPersonalListPollsTool)(nil)
	var _ ZaloPersonalActionAware = (*ZaloPersonalGetPollTool)(nil)
	var _ ZaloPersonalActionAware = (*ZaloPersonalVotePollTool)(nil)
	var _ ZaloPersonalActionAware = (*ZaloPersonalLockPollTool)(nil)
	var _ ZaloPersonalActionAware = (*ZaloPersonalAddPollOptionsTool)(nil)
	var _ ChannelAware = (*ZaloPersonalCreatePollTool)(nil)
	var _ ChannelAware = (*ZaloPersonalListPollsTool)(nil)
	var _ ChannelAware = (*ZaloPersonalGetPollTool)(nil)
	var _ ChannelAware = (*ZaloPersonalVotePollTool)(nil)
	var _ ChannelAware = (*ZaloPersonalLockPollTool)(nil)
	var _ ChannelAware = (*ZaloPersonalAddPollOptionsTool)(nil)
	return true
}()

func TestAllToolsDeclareZaloPersonalChannelType(t *testing.T) {
	t.Parallel()
	tools := []ChannelAware{
		NewZaloPersonalCreatePollTool(),
		NewZaloPersonalListPollsTool(),
		NewZaloPersonalGetPollTool(),
		NewZaloPersonalVotePollTool(),
		NewZaloPersonalLockPollTool(),
		NewZaloPersonalAddPollOptionsTool(),
	}
	for _, tl := range tools {
		types := tl.RequiredChannelTypes()
		if len(types) != 1 || types[0] != channels.TypeZaloPersonal {
			t.Errorf("%T: RequiredChannelTypes()=%v, want [%s]", tl, types, channels.TypeZaloPersonal)
		}
	}
}

// Silence unused-import warnings if test branches go away later.
var _ = fmt.Sprintf
