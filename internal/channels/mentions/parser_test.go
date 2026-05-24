package mentions

import (
	"reflect"
	"testing"

	"github.com/nextlevelbuilder/goclaw/pkg/protocol"
)

func fixtureResolver(t *testing.T) Resolve {
	t.Helper()
	names := map[string]string{
		"u_a":     "Alice",
		"u_b":     "Bob",
		"u_vn":    "Đức",
		"u_emoji": "🎉Team",
	}
	return func(m string) (string, string, bool) {
		n, ok := names[m]
		if !ok {
			return "", "", false
		}
		return m, n, true
	}
}

func TestParseMarkers_NoMarkers_ReturnsInputUntouched(t *testing.T) {
	in := "Just plain text, no markers."
	got, ms := ParseMarkers(in, fixtureResolver(t))
	if got != in {
		t.Fatalf("text mutated: got %q, want %q", got, in)
	}
	if ms != nil {
		t.Fatalf("mentions = %+v, want nil", ms)
	}
}

func TestParseMarkers_SingleIndividualMention(t *testing.T) {
	got, ms := ParseMarkers("hi @[u_a]!", fixtureResolver(t))
	want := "hi @Alice!"
	if got != want {
		t.Fatalf("rendered = %q, want %q", got, want)
	}
	wantMs := []protocol.Mention{
		{UserID: "u_a", DisplayName: "Alice", Position: 3, Length: 6, Type: 0},
	}
	if !reflect.DeepEqual(ms, wantMs) {
		t.Fatalf("mentions:\n got  %+v\n want %+v", ms, wantMs)
	}
}

func TestParseMarkers_AtAll(t *testing.T) {
	got, ms := ParseMarkers("@[all] meeting at 3pm", fixtureResolver(t))
	want := "@all meeting at 3pm"
	if got != want {
		t.Fatalf("rendered = %q, want %q", got, want)
	}
	wantMs := []protocol.Mention{
		{UserID: "-1", DisplayName: "all", Position: 0, Length: 4, Type: 1},
	}
	if !reflect.DeepEqual(ms, wantMs) {
		t.Fatalf("mentions:\n got  %+v\n want %+v", ms, wantMs)
	}
}

func TestParseMarkers_MultipleMentionsDifferentUIDs(t *testing.T) {
	got, ms := ParseMarkers("hey @[u_a] and @[u_b]!", fixtureResolver(t))
	want := "hey @Alice and @Bob!"
	if got != want {
		t.Fatalf("rendered = %q, want %q", got, want)
	}
	if len(ms) != 2 {
		t.Fatalf("len(mentions) = %d, want 2", len(ms))
	}
	if ms[0].UserID != "u_a" || ms[0].Position != 4 || ms[0].Length != 6 {
		t.Fatalf("ms[0] = %+v", ms[0])
	}
	if ms[1].UserID != "u_b" || ms[1].Position != 15 || ms[1].Length != 4 {
		t.Fatalf("ms[1] = %+v", ms[1])
	}
}

func TestParseMarkers_MultipleMentionsSameUID(t *testing.T) {
	got, ms := ParseMarkers("@[u_a] then @[u_a]", fixtureResolver(t))
	want := "@Alice then @Alice"
	if got != want {
		t.Fatalf("rendered = %q, want %q", got, want)
	}
	if len(ms) != 2 {
		t.Fatalf("expected 2 mentions, got %d", len(ms))
	}
	if ms[0].Position == ms[1].Position {
		t.Fatalf("positions collided: %+v", ms)
	}
}

func TestParseMarkers_MarkerAtStart(t *testing.T) {
	got, ms := ParseMarkers("@[u_a] hello", fixtureResolver(t))
	want := "@Alice hello"
	if got != want || ms[0].Position != 0 {
		t.Fatalf("rendered=%q, pos=%d", got, ms[0].Position)
	}
}

func TestParseMarkers_MarkerAtEnd(t *testing.T) {
	got, ms := ParseMarkers("hello @[u_a]", fixtureResolver(t))
	want := "hello @Alice"
	if got != want || ms[0].Position != 6 {
		t.Fatalf("rendered=%q, pos=%d", got, ms[0].Position)
	}
}

func TestParseMarkers_AdjacentMarkers(t *testing.T) {
	got, ms := ParseMarkers("@[u_a]@[u_b]", fixtureResolver(t))
	want := "@Alice@Bob"
	if got != want {
		t.Fatalf("rendered=%q, want %q", got, want)
	}
	if len(ms) != 2 {
		t.Fatalf("len(mentions)=%d", len(ms))
	}
	if ms[0].Position != 0 || ms[0].Length != 6 {
		t.Fatalf("ms[0]=%+v", ms[0])
	}
	if ms[1].Position != 6 || ms[1].Length != 4 {
		t.Fatalf("ms[1]=%+v", ms[1])
	}
}

func TestParseMarkers_FollowedByPunctuation(t *testing.T) {
	got, ms := ParseMarkers("@[u_a], thanks", fixtureResolver(t))
	want := "@Alice, thanks"
	if got != want {
		t.Fatalf("rendered=%q", got)
	}
	if ms[0].Length != 6 {
		t.Fatalf("length includes comma? %+v", ms[0])
	}
}

func TestParseMarkers_VietnameseDisplayName_UTF16OffsetsCorrect(t *testing.T) {
	got, ms := ParseMarkers("Cảm ơn @[u_vn]!", fixtureResolver(t))
	want := "Cảm ơn @Đức!"
	if got != want {
		t.Fatalf("rendered=%q, want %q", got, want)
	}
	// "Cảm ơn " is 7 UTF-16 code units; @Đức is 4.
	if ms[0].Position != 7 || ms[0].Length != 4 {
		t.Fatalf("offsets wrong: %+v", ms[0])
	}
}

func TestParseMarkers_EmojiDisplayName_SurrogatePairLength(t *testing.T) {
	got, ms := ParseMarkers("yo @[u_emoji]", fixtureResolver(t))
	want := "yo @🎉Team"
	if got != want {
		t.Fatalf("rendered=%q, want %q", got, want)
	}
	// "yo " = 3 cu, @🎉Team = 1 + 2 + 4 = 7 cu
	if ms[0].Position != 3 || ms[0].Length != 7 {
		t.Fatalf("offsets wrong: %+v", ms[0])
	}
}

func TestParseMarkers_PreviousVietnameseTextShiftsPosition(t *testing.T) {
	// "Đức said " = 9 UTF-16 cu (3 + 5 + 1)
	got, ms := ParseMarkers("Đức said @[u_a]", fixtureResolver(t))
	want := "Đức said @Alice"
	if got != want {
		t.Fatalf("rendered=%q", got)
	}
	if ms[0].Position != 9 {
		t.Fatalf("position = %d, want 9", ms[0].Position)
	}
}

func TestParseMarkers_UnresolvedMarker_PreservedLiteral(t *testing.T) {
	got, ms := ParseMarkers("hi @[unknown]!", fixtureResolver(t))
	want := "hi @[unknown]!"
	if got != want {
		t.Fatalf("rendered=%q, want %q", got, want)
	}
	if ms != nil {
		t.Fatalf("expected nil mentions, got %+v", ms)
	}
}

func TestParseMarkers_MarkdownLinkNoCollision(t *testing.T) {
	in := "see [link](https://example.com) for info"
	got, ms := ParseMarkers(in, fixtureResolver(t))
	if got != in {
		t.Fatalf("markdown link mutated: %q", got)
	}
	if ms != nil {
		t.Fatalf("mentions=%+v", ms)
	}
}

func TestParseMarkers_AllMarkerIgnoresResolver(t *testing.T) {
	resolverCalled := false
	resolver := func(m string) (string, string, bool) {
		resolverCalled = true
		return "should-not-be-used", "should-not-be-used", true
	}
	got, ms := ParseMarkers("@[all]", resolver)
	if resolverCalled {
		t.Fatal("resolver invoked for @[all]")
	}
	if got != "@all" {
		t.Fatalf("rendered=%q, want %q", got, "@all")
	}
	if ms[0].UserID != "-1" || ms[0].Type != 1 {
		t.Fatalf("ms[0]=%+v", ms[0])
	}
}

func TestParseMarkers_EmptyBracketIsLiteral(t *testing.T) {
	got, ms := ParseMarkers("hello @[] world", fixtureResolver(t))
	if got != "hello @[] world" {
		t.Fatalf("rendered=%q", got)
	}
	if ms != nil {
		t.Fatalf("ms=%+v", ms)
	}
}

func TestParseMarkers_TextBetweenMarkersHandled(t *testing.T) {
	got, ms := ParseMarkers("@[u_a] hi @[u_b]", fixtureResolver(t))
	want := "@Alice hi @Bob"
	if got != want {
		t.Fatalf("rendered=%q", got)
	}
	if ms[0].Position != 0 || ms[0].Length != 6 {
		t.Fatalf("ms[0]=%+v", ms[0])
	}
	// "@Alice hi " = 10 UTF-16 cu
	if ms[1].Position != 10 || ms[1].Length != 4 {
		t.Fatalf("ms[1]=%+v", ms[1])
	}
}
