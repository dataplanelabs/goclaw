package personal

import (
	"context"
	"fmt"
	"strings"

	"github.com/nextlevelbuilder/goclaw/internal/channels/zalo/personal/protocol"
	"github.com/nextlevelbuilder/goclaw/internal/tools"
)

// Compile-time guard: drift between Channel methods and the ZaloPersonalAction
// interface fails the build instead of surfacing at runtime.
var _ tools.ZaloPersonalAction = (*Channel)(nil)

// CreatePoll creates a poll in the given group. Errors if polls are disabled,
// the channel is offline, or the chat is not a group.
func (c *Channel) CreatePoll(ctx context.Context, chatID, question string, options []string, settings tools.ZaloPollSettings) (string, error) {
	sess, err := c.guardPoll()
	if err != nil {
		return "", err
	}
	if !c.IsGroupApproved(chatID) {
		return "", fmt.Errorf("zalo_personal: polls only work in group chats")
	}
	detail, err := protocol.CreatePoll(ctx, sess, chatID, protocol.CreatePollOptions{
		Question:          question,
		Options:           options,
		ExpiredTime:       settings.ExpireAtMillis,
		AllowMultiChoices: settings.AllowMultiChoices,
		AllowAddNewOption: settings.AllowAddNewOption,
		HideVotePreview:   settings.HideVotePreview,
		IsAnonymous:       settings.IsAnonymous,
	})
	if err != nil {
		return "", err
	}
	return detail.PollID.String(), nil
}

func (c *Channel) ListPolls(ctx context.Context, chatID string, page, count int) (tools.ZaloPollList, error) {
	sess, err := c.guardPoll()
	if err != nil {
		return tools.ZaloPollList{}, err
	}
	if !c.IsGroupApproved(chatID) {
		return tools.ZaloPollList{}, fmt.Errorf("zalo_personal: polls only work in group chats")
	}
	list, err := protocol.ListPolls(ctx, sess, chatID, protocol.ListPollsOptions{
		Page:  page,
		Count: count,
	})
	if err != nil {
		return tools.ZaloPollList{}, err
	}
	out := tools.ZaloPollList{
		Polls: make([]tools.ZaloPollState, 0, len(list.Polls)),
		Count: list.Count,
		Page:  page,
	}
	for _, p := range list.Polls {
		out.Polls = append(out.Polls, c.pollDetailToState(ctx, &p, chatID))
	}
	return out, nil
}

func (c *Channel) GetPoll(ctx context.Context, pollID int64) (tools.ZaloPollState, error) {
	sess, err := c.guardPoll()
	if err != nil {
		return tools.ZaloPollState{}, err
	}
	d, err := protocol.GetPollDetail(ctx, sess, pollID)
	if err != nil {
		return tools.ZaloPollState{}, err
	}
	return c.pollDetailToState(ctx, d, tools.ToolChatIDFromCtx(ctx)), nil
}

func (c *Channel) VotePoll(ctx context.Context, pollID int64, optionIDs []int64) (tools.ZaloPollState, error) {
	sess, err := c.guardPoll()
	if err != nil {
		return tools.ZaloPollState{}, err
	}
	opts, err := protocol.VotePoll(ctx, sess, pollID, optionIDs)
	if err != nil {
		return tools.ZaloPollState{}, err
	}
	return tools.ZaloPollState{
		PollID:  fmt.Sprintf("%d", pollID),
		Options: optionsToState(opts, nil),
	}, nil
}

func (c *Channel) LockPoll(ctx context.Context, pollID int64) error {
	sess, err := c.guardPoll()
	if err != nil {
		return err
	}
	return protocol.LockPoll(ctx, sess, pollID)
}

func (c *Channel) AddPollOptions(ctx context.Context, pollID int64, newOptions []string, votedOptionIDs []int64) (tools.ZaloPollState, error) {
	sess, err := c.guardPoll()
	if err != nil {
		return tools.ZaloPollState{}, err
	}
	items := make([]protocol.AddPollOptionsItem, 0, len(newOptions))
	for _, s := range newOptions {
		items = append(items, protocol.AddPollOptionsItem{Content: s})
	}
	opts, err := protocol.AddPollOptions(ctx, sess, pollID, items, votedOptionIDs)
	if err != nil {
		return tools.ZaloPollState{}, err
	}
	return tools.ZaloPollState{
		PollID:  fmt.Sprintf("%d", pollID),
		Options: optionsToState(opts, nil),
	}, nil
}

// IsGroup mirrors the ZaloPersonalAction surface.
func (c *Channel) IsGroup(chatID string) bool { return c.IsGroupApproved(chatID) }

func (c *Channel) CreateReminder(ctx context.Context, threadID string, isGroup bool, settings tools.ZaloReminderSettings) (string, error) {
	sess := c.session()
	if !c.IsRunning() || sess == nil {
		return "", fmt.Errorf("zalo_personal: channel not running")
	}
	repeat, err := parseRepeatMode(settings.Repeat)
	if err != nil {
		return "", err
	}
	opts := protocol.CreateReminderOptions{
		Title:     settings.Title,
		Emoji:     settings.Emoji,
		StartTime: settings.StartTime,
		Repeat:    repeat,
		PinToTop:  settings.PinToTop,
	}
	if isGroup {
		return protocol.CreateReminderInGroup(ctx, sess, threadID, opts)
	}
	return protocol.CreateReminderInDM(ctx, sess, threadID, opts)
}

func (c *Channel) RemoveReminder(ctx context.Context, reminderID, groupID string) error {
	sess := c.session()
	if !c.IsRunning() || sess == nil {
		return fmt.Errorf("zalo_personal: channel not running")
	}
	return protocol.RemoveReminder(ctx, sess, reminderID, groupID)
}

func parseRepeatMode(s string) (protocol.RepeatMode, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "none":
		return protocol.RepeatNone, nil
	case "daily":
		return protocol.RepeatDaily, nil
	case "weekly":
		return protocol.RepeatWeekly, nil
	case "monthly":
		return protocol.RepeatMonthly, nil
	default:
		return 0, fmt.Errorf("zalo_personal: unknown repeat mode %q (use none|daily|weekly|monthly)", s)
	}
}

func (c *Channel) guardPoll() (*protocol.Session, error) {
	sess := c.session()
	if !c.IsRunning() || sess == nil {
		return nil, fmt.Errorf("zalo_personal: channel not running")
	}
	if c.config.DisablePolls {
		return nil, fmt.Errorf("zalo_personal: polls disabled by config")
	}
	return sess, nil
}

func (c *Channel) pollDetailToState(ctx context.Context, d *protocol.PollDetail, fallbackGroupID string) tools.ZaloPollState {
	groupID := d.GroupID
	if groupID == "" {
		groupID = fallbackGroupID
	}
	var resolve pollVoterNameResolver
	if groupID != "" || c.ContactCollector() != nil {
		resolve = func(uid string) (string, bool) {
			if groupID != "" {
				if displayName, ok := c.LookupGroupMember(ctx, groupID, uid); ok {
					return displayName, true
				}
			}
			return c.lookupContactDisplayName(ctx, uid)
		}
	}
	return tools.ZaloPollState{
		PollID:      d.PollID.String(),
		Question:    d.Question,
		Locked:      d.Locked,
		Closed:      d.Closed,
		GroupID:     groupID,
		CreatedTime: d.CreatedTime,
		UpdatedTime: d.UpdatedTime,
		ExpiredTime: d.ExpiredTime,
		TotalVotes:  d.TotalVotes,
		Options:     optionsToState(d.Options, resolve),
	}
}

type pollVoterNameResolver func(uid string) (displayName string, ok bool)

func optionsToState(opts []protocol.PollOption, resolve pollVoterNameResolver) []tools.ZaloPollStateOption {
	out := make([]tools.ZaloPollStateOption, 0, len(opts))
	for _, o := range opts {
		voterIDs := pollOptionVoterIDs(o)
		voters := make([]tools.ZaloPollVoter, 0, len(voterIDs))
		for _, id := range voterIDs {
			voter := tools.ZaloPollVoter{UserID: id}
			if resolve != nil {
				if displayName, ok := resolve(id); ok {
					voter.DisplayName = displayName
				}
			}
			voters = append(voters, voter)
		}
		out = append(out, tools.ZaloPollStateOption{
			OptionID:  o.OptionID,
			Content:   o.Content,
			VoteCount: o.CountVotes(),
			Voted:     o.Voted,
			VoterIDs:  voterIDs,
			Voters:    voters,
		})
	}
	return out
}

func pollOptionVoterIDs(o protocol.PollOption) []string {
	ids := make([]string, 0, len(o.Voters)+len(o.VotedUsers))
	seen := make(map[string]struct{}, len(o.Voters)+len(o.VotedUsers))
	for _, src := range [][]string{o.Voters, o.VotedUsers} {
		for _, id := range src {
			if id == "" {
				continue
			}
			if _, ok := seen[id]; ok {
				continue
			}
			seen[id] = struct{}{}
			ids = append(ids, id)
		}
	}
	return ids
}
