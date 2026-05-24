package tools

import "context"

// ZaloPersonalActionFn resolves a Zalo Personal channel by name and returns a
// typed handle. Returns an error if the channel does not exist or isn't a
// Zalo Personal channel.
type ZaloPersonalActionFn func(ctx context.Context, channelName string) (ZaloPersonalAction, error)

// ZaloPersonalAction is the narrow surface tools call on the channel. Defined
// here to avoid a circular import between tools and channels/zalo/personal.
type ZaloPersonalAction interface {
	CreatePoll(ctx context.Context, chatID, question string, options []string, settings ZaloPollSettings) (pollID string, err error)
	GetPoll(ctx context.Context, pollID int64) (ZaloPollState, error)
	VotePoll(ctx context.Context, pollID int64, optionIDs []int64) (ZaloPollState, error)
	LockPoll(ctx context.Context, pollID int64) error
	AddPollOptions(ctx context.Context, pollID int64, newOptions []string, votedOptionIDs []int64) (ZaloPollState, error)
	CreateReminder(ctx context.Context, threadID string, isGroup bool, settings ZaloReminderSettings) (reminderID string, err error)
	RemoveReminder(ctx context.Context, reminderID, groupID string) error
	IsRunning() bool
	IsGroup(chatID string) bool
}

type ZaloPollSettings struct {
	ExpiredTimeMillis int64
	AllowMultiChoices bool
	AllowAddNewOption bool
	HideVotePreview   bool
	IsAnonymous       bool
}

type ZaloPollState struct {
	PollID   string                `json:"poll_id"`
	Question string                `json:"question,omitempty"`
	Options  []ZaloPollStateOption `json:"options"`
	Locked   bool                  `json:"locked,omitempty"`
}

// ZaloReminderSettings is the channel-neutral DTO. Repeat is a human-friendly
// string (none|daily|weekly|monthly) — channel layer maps to protocol enum.
type ZaloReminderSettings struct {
	Title     string
	StartTime int64 // Unix ms; 0 = now
	Repeat    string
	PinToTop  bool   // group only — DM ignores
	Emoji     string // "" = default ⏰
}

type ZaloPollStateOption struct {
	OptionID  int64  `json:"option_id"`
	Content   string `json:"content"`
	VoteCount int    `json:"vote_count"`
	Voted     bool   `json:"voted,omitempty"`
}

// ZaloPersonalActionAware is the DI setter implemented by every tool that
// needs the resolver.
type ZaloPersonalActionAware interface {
	SetZaloPersonalActionFn(ZaloPersonalActionFn)
}
