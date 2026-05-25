package oa

import (
	"strings"
	"testing"

	"github.com/nextlevelbuilder/goclaw/internal/providers"
)

func TestRecentCustomerContext_RapidFireUserBurst(t *testing.T) {
	msgs := []providers.Message{
		{Role: "user", Content: "Xin chào"},
		{Role: "assistant", Content: ""},
		{Role: "assistant", Content: "Dạ chào anh"},
		{Role: "user", Content: "0329575792"},
		{Role: "assistant", Content: ""},
		{Role: "user", Content: "đây ạ"},
		{Role: "assistant", Content: ""},
	}
	got := recentCustomerContext(msgs)
	want := "0329575792\nđây ạ"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestRecentCustomerContext_StopsAtTeamReply(t *testing.T) {
	msgs := []providers.Message{
		{Role: "user", Content: "first question"},
		{Role: "assistant", Content: "team answer"},
		{Role: "user", Content: "follow up"},
	}
	got := recentCustomerContext(msgs)
	if got != "follow up" {
		t.Fatalf("got %q, want \"follow up\" (must not cross team boundary)", got)
	}
}

func TestRecentCustomerContext_EmptyHistory(t *testing.T) {
	if got := recentCustomerContext(nil); got != "" {
		t.Fatalf("got %q want empty", got)
	}
}

func TestRecentCustomerContext_NoUserMessages(t *testing.T) {
	msgs := []providers.Message{
		{Role: "assistant", Content: "hi there"},
	}
	if got := recentCustomerContext(msgs); got != "" {
		t.Fatalf("got %q want empty", got)
	}
}

func TestRecentCustomerContext_SkipsEmptyUserMessages(t *testing.T) {
	msgs := []providers.Message{
		{Role: "user", Content: "real text"},
		{Role: "user", Content: "   "},
		{Role: "user", Content: ""},
	}
	if got := recentCustomerContext(msgs); got != "real text" {
		t.Fatalf("got %q want \"real text\"", got)
	}
}

func TestRecentCustomerContext_RespectsCharCap(t *testing.T) {
	big := strings.Repeat("x", teamReplyCustomerContextChars+100)
	msgs := []providers.Message{
		{Role: "user", Content: big},
	}
	got := recentCustomerContext(msgs)
	if len(got) != teamReplyCustomerContextChars {
		t.Fatalf("len=%d want %d", len(got), teamReplyCustomerContextChars)
	}
}

func TestRecentCustomerContext_ChronologicalOrder(t *testing.T) {
	msgs := []providers.Message{
		{Role: "user", Content: "A"},
		{Role: "user", Content: "B"},
		{Role: "user", Content: "C"},
	}
	got := recentCustomerContext(msgs)
	if got != "A\nB\nC" {
		t.Fatalf("got %q want \"A\\nB\\nC\" (chronological)", got)
	}
}
