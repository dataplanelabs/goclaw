package personal

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/nextlevelbuilder/goclaw/internal/bus"
	"github.com/nextlevelbuilder/goclaw/internal/channels"
	"github.com/nextlevelbuilder/goclaw/internal/channels/zalo/personal/protocol"
	"github.com/nextlevelbuilder/goclaw/internal/config"
	"github.com/nextlevelbuilder/goclaw/internal/store"
	"github.com/nextlevelbuilder/goclaw/internal/tools"
)

func TestParseRepeatMode(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in   string
		want protocol.RepeatMode
		err  bool
	}{
		{"", protocol.RepeatNone, false},
		{"none", protocol.RepeatNone, false},
		{"  None ", protocol.RepeatNone, false},
		{"daily", protocol.RepeatDaily, false},
		{"WEEKLY", protocol.RepeatWeekly, false},
		{"Monthly", protocol.RepeatMonthly, false},
		{"yearly", 0, true},
		{"bogus", 0, true},
	}
	for _, c := range cases {
		got, err := parseRepeatMode(c.in)
		if (err != nil) != c.err {
			t.Errorf("parseRepeatMode(%q) err=%v want_err=%v", c.in, err, c.err)
		}
		if err == nil && got != c.want {
			t.Errorf("parseRepeatMode(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestOptionsToStateIncludesVoters(t *testing.T) {
	t.Parallel()
	got := optionsToState([]protocol.PollOption{{
		OptionID:   10,
		Content:    "Brazil",
		Votes:      3,
		Voted:      true,
		Voters:     []string{"u1", "u2"},
		VotedUsers: []string{"u2", "u3"},
	}}, func(uid string) (string, bool) {
		names := map[string]string{
			"u1": "Alice",
			"u3": "Charlie",
		}
		name, ok := names[uid]
		return name, ok
	})

	if len(got) != 1 {
		t.Fatalf("options=%d, want 1", len(got))
	}
	opt := got[0]
	if opt.VoteCount != 3 || !opt.Voted {
		t.Fatalf("vote fields not preserved: %+v", opt)
	}
	if want := []string{"u1", "u2", "u3"}; !reflect.DeepEqual(opt.VoterIDs, want) {
		t.Fatalf("voter_ids=%v, want %v", opt.VoterIDs, want)
	}
	wantVoters := []tools.ZaloPollVoter{
		{UserID: "u1", DisplayName: "Alice"},
		{UserID: "u2"},
		{UserID: "u3", DisplayName: "Charlie"},
	}
	if !reflect.DeepEqual(opt.Voters, wantVoters) {
		t.Fatalf("voters=%+v, want %+v", opt.Voters, wantVoters)
	}
}

func TestPollDetailToStateUsesFallbackGroupAndContactsForVoterNames(t *testing.T) {
	t.Parallel()
	const uid = "u_contact"
	ch, cs := newChannelWithContacts(t, map[string]store.ChannelContact{
		uid: contactRow(channels.TypeZaloPersonal, uid, "DB Name"),
	})

	got := ch.pollDetailToState(context.Background(), &protocol.PollDetail{
		PollID:   json.Number("42"),
		Question: "Q",
		Options: []protocol.PollOption{{
			OptionID:  10,
			Content:   "A",
			VoteCount: 1,
			Voters:    []string{uid},
		}},
	}, "group-1")

	if got.GroupID != "group-1" {
		t.Fatalf("group_id=%q, want fallback group-1", got.GroupID)
	}
	if len(got.Options) != 1 || len(got.Options[0].Voters) != 1 {
		t.Fatalf("voters missing: %+v", got.Options)
	}
	voter := got.Options[0].Voters[0]
	if voter.UserID != uid || voter.DisplayName != "DB Name" {
		t.Fatalf("voter=%+v, want id + DB display name", voter)
	}
	if got := cs.lookups.Load(); got != 1 {
		t.Fatalf("contact lookups=%d, want 1", got)
	}
}

func newReminderTestChannel(t *testing.T) *Channel {
	t.Helper()
	mb := bus.New()
	ch, err := New(config.ZaloPersonalConfig{
		Enabled:     true,
		DMPolicy:    "open",
		GroupPolicy: "open",
	}, mb, nil, nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return ch
}

func TestCreateReminder_ChannelNotRunning(t *testing.T) {
	t.Parallel()
	c := newReminderTestChannel(t)
	_, err := c.CreateReminder(context.Background(), "g1", true, tools.ZaloReminderSettings{Title: "x"})
	if err == nil || !strings.Contains(err.Error(), "not running") {
		t.Fatalf("expected not-running error, got %v", err)
	}
}

func TestRemoveReminder_ChannelNotRunning(t *testing.T) {
	t.Parallel()
	c := newReminderTestChannel(t)
	err := c.RemoveReminder(context.Background(), "r1", "g1")
	if err == nil || !strings.Contains(err.Error(), "not running") {
		t.Fatalf("expected not-running error, got %v", err)
	}
}
