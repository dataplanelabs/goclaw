package personal

import (
	"strings"
	"testing"
)

func TestAskerPrepend_AddsMarker(t *testing.T) {
	got := applyAskerPrepend("thanks!", "u_van_duc")
	want := "@[u_van_duc] thanks!"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestAskerPrepend_DedupeIfAlreadyMentioned(t *testing.T) {
	got := applyAskerPrepend("@[u_van_duc] yes", "u_van_duc")
	if got != "@[u_van_duc] yes" {
		t.Fatalf("got %q, expected no double-prepend", got)
	}
}

func TestAskerPrepend_SkipsIfAtAllPresent(t *testing.T) {
	got := applyAskerPrepend("@[all] meeting now", "u_van_duc")
	if strings.HasPrefix(got, "@[u_van_duc]") {
		t.Fatalf("got %q, expected to skip prepend when @[all] present", got)
	}
}

func TestAskerPrepend_EmptyAsker_NoChange(t *testing.T) {
	got := applyAskerPrepend("hello", "")
	if got != "hello" {
		t.Fatalf("got %q, expected unchanged", got)
	}
}

func TestAskerPrepend_EmptyContent_NoChange(t *testing.T) {
	got := applyAskerPrepend("", "u_x")
	if got != "" {
		t.Fatalf("got %q, expected empty", got)
	}
}
