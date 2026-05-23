package personal

import (
	"context"
	"fmt"

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
		ExpiredTime:       settings.ExpiredTimeMillis,
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

func (c *Channel) GetPoll(ctx context.Context, pollID int64) (tools.ZaloPollState, error) {
	sess, err := c.guardPoll()
	if err != nil {
		return tools.ZaloPollState{}, err
	}
	d, err := protocol.GetPollDetail(ctx, sess, pollID)
	if err != nil {
		return tools.ZaloPollState{}, err
	}
	return pollDetailToState(d), nil
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
		Options: optionsToState(opts),
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
		Options: optionsToState(opts),
	}, nil
}

// IsGroup mirrors the ZaloPersonalAction surface.
func (c *Channel) IsGroup(chatID string) bool { return c.IsGroupApproved(chatID) }

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

func pollDetailToState(d *protocol.PollDetail) tools.ZaloPollState {
	return tools.ZaloPollState{
		PollID:   d.PollID.String(),
		Question: d.Question,
		Locked:   d.Locked,
		Options:  optionsToState(d.Options),
	}
}

func optionsToState(opts []protocol.PollOption) []tools.ZaloPollStateOption {
	out := make([]tools.ZaloPollStateOption, 0, len(opts))
	for _, o := range opts {
		out = append(out, tools.ZaloPollStateOption{
			OptionID:  o.OptionID,
			Content:   o.Content,
			VoteCount: o.VoteCount,
			Voted:     o.Voted,
		})
	}
	return out
}
