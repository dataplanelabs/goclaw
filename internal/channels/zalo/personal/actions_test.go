package personal

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/nextlevelbuilder/goclaw/internal/bus"
	"github.com/nextlevelbuilder/goclaw/internal/channels/zalo/personal/protocol"
	"github.com/nextlevelbuilder/goclaw/internal/config"
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
