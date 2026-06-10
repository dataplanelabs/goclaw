package memory

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/nextlevelbuilder/goclaw/internal/store"
)

func TestBuildReactionFeedbackSection_Empty(t *testing.T) {
	t.Parallel()
	got := buildReactionFeedbackSection(nil, time.Now(), InjectParams{})
	if got != "" {
		t.Errorf("empty input must return empty section, got %q", got)
	}
}

func TestBuildReactionFeedbackSection_GroupsByMessage(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 5, 24, 10, 0, 0, 0, time.UTC)
	rows := []store.EpisodicSummary{
		row("alice", "❤", "positive", "msg-1", "great work!", now.Add(-5*time.Minute), "/-heart"),
		row("bob", "❤", "positive", "msg-1", "great work!", now.Add(-4*time.Minute), "/-heart"),
		row("alice", "😂", "positive", "msg-1", "great work!", now.Add(-3*time.Minute), ":>"),
		row("alice", "😢", "negative", "msg-2", "sorry about that", now.Add(-1*time.Hour), ":-(("),
	}
	got := buildReactionFeedbackSection(rows, now, InjectParams{AgentID: "a", UserID: "u"})

	mustContain(t, got, "## Recent User Reactions (last 24h)")
	mustContain(t, got, "3 positive · 1 negative · 0 surprise")
	mustContain(t, got, "across 2 replies (2 reactors)")
	mustContain(t, got, `"great work!"`)
	mustContain(t, got, `"sorry about that"`)
	mustContain(t, got, "❤×2")
	mustContain(t, got, "😂")
	mustContain(t, got, "total 3")
	mustContain(t, got, "reactors: alice, bob")
	mustContain(t, got, "at 2026-05-24 09:57 UTC")
	mustContain(t, got, "1h ago")
	mustContain(t, got, "3m ago")
}

func TestBuildReactionFeedbackSection_CapsToTopReplies(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 5, 24, 10, 0, 0, 0, time.UTC)
	var rows []store.EpisodicSummary
	for i := range 10 {
		rows = append(rows, row("user", "❤", "positive",
			"msg-"+itoa(i), "reply "+itoa(i), now.Add(-time.Duration(i)*time.Minute), "/-heart"))
	}
	got := buildReactionFeedbackSection(rows, now, InjectParams{})
	mustContain(t, got, "across 10 replies")
	bulletCount := strings.Count(got, "\n- ")
	if bulletCount != reactionFeedbackTopReplies {
		t.Errorf("expected %d bullets (top replies), got %d in:\n%s", reactionFeedbackTopReplies, bulletCount, got)
	}
}

func TestBuildReactionFeedbackSection_FallbackToMsgIDWhenPreviewMissing(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 5, 24, 10, 0, 0, 0, time.UTC)
	// Summary without "on your reply:" — old-style summary using "on message"
	r := store.EpisodicSummary{
		ID:        uuid.Must(uuid.NewV7()),
		Summary:   `Alice reacted ❤ (positive) on message 12345`,
		SourceID:  "react:12345:alice:/-heart",
		CreatedAt: now.Add(-2 * time.Minute),
	}
	got := buildReactionFeedbackSection([]store.EpisodicSummary{r}, now, InjectParams{})
	mustContain(t, got, "on message 12345")
	if strings.Contains(got, `"`) {
		t.Errorf("preview-less row must not produce quotes: %s", got)
	}
}

func TestParseReactionSourceID(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"react:12345:alice:/-heart": "12345",
		"react:abc":                 "abc",
		"react:xyz:user":            "xyz",
		"not-a-reaction-source":     "",
		"":                          "",
	}
	for in, want := range cases {
		if got := parseReactionSourceID(in); got != want {
			t.Errorf("parseReactionSourceID(%q)=%q, want %q", in, got, want)
		}
	}
}

func TestExtractReactionFields(t *testing.T) {
	t.Parallel()
	s := `Van Duc reacted ❤ (positive) on your reply: "Hi anh"`
	if got := extractReactionReactor(s); got != "Van Duc" {
		t.Errorf("reactor=%q, want Van Duc", got)
	}
	if got := extractReactionIcon(s); got != "❤" {
		t.Errorf("icon=%q, want ❤", got)
	}
	if got := extractSentiment(s); got != "positive" {
		t.Errorf("sentiment=%q, want positive", got)
	}
	if got := extractReactionPreview(s); got != "Hi anh" {
		t.Errorf("preview=%q, want Hi anh", got)
	}

	messageSummary := `Nguyen Van A reacted 👍 (positive) on message: "File uploaded" at 2026-06-03T09:53:00+07:00`
	if got := extractReactionPreview(messageSummary); got != "File uploaded" {
		t.Errorf("message preview=%q, want File uploaded", got)
	}

	removedSummary := `Nguyen Van A removed their reaction on your reply: "Hi anh"`
	if got := extractReactionReactor(removedSummary); got != "Nguyen Van A" {
		t.Errorf("removed reactor=%q, want Nguyen Van A", got)
	}
}

func TestFormatEmojiCluster(t *testing.T) {
	t.Parallel()
	got := formatEmojiCluster(map[string]int{"❤": 3, "😂": 1, "👍": 2})
	// Sorted by count desc, then alpha. ❤×3 first, 👍×2 next, 😂 (count=1) last.
	mustContain(t, got, "❤×3")
	mustContain(t, got, "👍×2")
	mustContain(t, got, "😂")
	if strings.Contains(got, "😂×1") {
		t.Errorf("count=1 should not show ×1, got: %s", got)
	}
}

func TestRelTimeAgo(t *testing.T) {
	t.Parallel()
	cases := []struct {
		d    time.Duration
		want string
	}{
		{30 * time.Second, "just now"},
		{2 * time.Minute, "2m ago"},
		{3 * time.Hour, "3h ago"},
		{50 * time.Hour, "2d ago"},
	}
	for _, tc := range cases {
		if got := relTimeAgo(tc.d); got != tc.want {
			t.Errorf("relTimeAgo(%v)=%q, want %q", tc.d, got, tc.want)
		}
	}
}

func TestTruncateRunes(t *testing.T) {
	t.Parallel()
	if got := truncateRunes("hi", 5); got != "hi" {
		t.Errorf("short string passes through, got %q", got)
	}
	if got := truncateRunes("0123456789", 4); got != "0123…" {
		t.Errorf("truncate to 4 chars, got %q", got)
	}
}

func row(reactor, icon, sentiment, msgID, preview string, at time.Time, code string) store.EpisodicSummary {
	summary := reactor + " reacted " + icon + " (" + sentiment + ") on your reply: \"" + preview + "\""
	return store.EpisodicSummary{
		ID:         uuid.Must(uuid.NewV7()),
		Summary:    summary,
		L0Abstract: summary,
		SourceID:   "react:" + msgID + ":" + reactor + ":" + code,
		SourceType: "reaction_feedback",
		CreatedAt:  at,
	}
}

func mustContain(t *testing.T, got, want string) {
	t.Helper()
	if !strings.Contains(got, want) {
		t.Errorf("missing %q in:\n%s", want, got)
	}
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	digits := ""
	neg := i < 0
	if neg {
		i = -i
	}
	for i > 0 {
		digits = string(rune('0'+(i%10))) + digits
		i /= 10
	}
	if neg {
		digits = "-" + digits
	}
	return digits
}
