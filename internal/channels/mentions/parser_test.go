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
	want := "@All meeting at 3pm"
	if got != want {
		t.Fatalf("rendered = %q, want %q", got, want)
	}
	wantMs := []protocol.Mention{
		{UserID: "-1", DisplayName: "All", Position: 0, Length: 4, Type: 1},
	}
	if !reflect.DeepEqual(ms, wantMs) {
		t.Fatalf("mentions:\n got  %+v\n want %+v", ms, wantMs)
	}
}

func TestParseMarkers_AtAllAliases(t *testing.T) {
	for _, marker := range []string{"all", "All", "everyone"} {
		got, ms := ParseMarkers("@["+marker+"] hello", fixtureResolver(t))
		if got != "@All hello" {
			t.Errorf("marker %q rendered %q, want %q", marker, got, "@All hello")
		}
		if len(ms) != 1 || ms[0].UserID != "-1" || ms[0].DisplayName != "All" {
			t.Errorf("marker %q mentions: %+v", marker, ms)
		}
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

func TestParseMarkers_UnresolvedNameMarker_DowngradedToPlainText(t *testing.T) {
	got, ms := ParseMarkers("hi @[unknown]!", fixtureResolver(t))
	want := "hi @unknown!"
	if got != want {
		t.Fatalf("rendered=%q, want %q", got, want)
	}
	if ms != nil {
		t.Fatalf("expected nil mentions, got %+v", ms)
	}
}

func TestParseMarkers_UnresolvedUIDMarker_StrippedWithSpaceCollapse(t *testing.T) {
	got, ms := ParseMarkers("@[583199907997701467] Do Loi", fixtureResolver(t))
	want := "Do Loi"
	if got != want {
		t.Fatalf("rendered=%q, want %q", got, want)
	}
	if ms != nil {
		t.Fatalf("expected nil mentions, got %+v", ms)
	}
}

func TestParseMarkers_UnresolvedUIDMarker_MidTextCollapsesDoubledSpace(t *testing.T) {
	got, _ := ParseMarkers("xin chào @[123] cả nhà", fixtureResolver(t))
	want := "xin chào cả nhà"
	if got != want {
		t.Fatalf("rendered=%q, want %q", got, want)
	}
}

func TestParseMarkers_UnresolvedUIDMarker_AtEndStripped(t *testing.T) {
	// No following space to collapse; trailing space is tolerated.
	got, _ := ParseMarkers("Do Loi @[123]", fixtureResolver(t))
	want := "Do Loi "
	if got != want {
		t.Fatalf("rendered=%q, want %q", got, want)
	}
}

func TestParseMarkers_StrippedUID_FollowingResolvedMentionOffsetCorrect(t *testing.T) {
	got, ms := ParseMarkers("@[999] xin chào @[u_vn]!", fixtureResolver(t))
	want := "xin chào @Đức!"
	if got != want {
		t.Fatalf("rendered=%q, want %q", got, want)
	}
	if len(ms) != 1 {
		t.Fatalf("mentions=%+v, want exactly 1", ms)
	}
	// "xin chào " = 9 UTF-16 code units after the stripped marker+space; @Đức = 4.
	if ms[0].UserID != "u_vn" || ms[0].Position != 9 || ms[0].Length != 4 {
		t.Fatalf("offsets wrong: %+v", ms[0])
	}
}

func TestParseMarkers_UnresolvedUID_DoesNotAffectAtAll(t *testing.T) {
	got, ms := ParseMarkers("@[123] @[all] họp lúc 3h", fixtureResolver(t))
	want := "@All họp lúc 3h"
	if got != want {
		t.Fatalf("rendered=%q, want %q", got, want)
	}
	if len(ms) != 1 || ms[0].UserID != "-1" || ms[0].Position != 0 || ms[0].Length != 4 {
		t.Fatalf("ms=%+v", ms)
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
	if got != "@All" {
		t.Fatalf("rendered=%q, want %q", got, "@All")
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

// ParseMarkersWithStyles: 5-case algorithm coverage.

func TestParseMarkersWithStyles_NilStyles_DelegatesPlain(t *testing.T) {
	got, ms, styles := ParseMarkersWithStyles("@[u_a] hi", fixtureResolver(t), nil)
	if got != "@Alice hi" {
		t.Fatalf("rendered=%q", got)
	}
	if len(ms) != 1 || styles != nil {
		t.Fatalf("ms=%+v styles=%+v", ms, styles)
	}
}

func TestParseMarkersWithStyles_StyleRightOfMarker_Shifts(t *testing.T) {
	// "@[u_a] hi" — input UTF-16: @[u_a]=6, " "=1, hi=2.
	// Marker [0,6) replaced with "@Alice" (6 cu) → delta=0.
	// Style over "hi" at input pos 7 len 2 → stays at 7 len 2 in output.
	in := "@[u_a] hi"
	style := Style{Start: 7, Len: 2, St: "b"}
	_, _, got := ParseMarkersWithStyles(in, fixtureResolver(t), []Style{style})
	if len(got) != 1 || got[0].Start != 7 || got[0].Len != 2 {
		t.Fatalf("got=%+v want=[{7,2,b}]", got)
	}
}

func TestParseMarkersWithStyles_StyleRightOfMarker_WithDelta(t *testing.T) {
	// Marker @[u_vn] → @Đức. @[u_vn]=7 input UTF-16. "@Đức"=4 output UTF-16.
	// delta = 4 - 7 = -3.
	// "@[u_vn] hi" — style over "hi" at input pos 8 len 2 → 8-3=5, len 2.
	in := "@[u_vn] hi"
	style := Style{Start: 8, Len: 2, St: "b"}
	_, _, got := ParseMarkersWithStyles(in, fixtureResolver(t), []Style{style})
	if len(got) != 1 || got[0].Start != 5 || got[0].Len != 2 {
		t.Fatalf("got=%+v want=[{5,2,b}]", got)
	}
}

func TestParseMarkersWithStyles_StyleLeftOfMarker_Unchanged(t *testing.T) {
	in := "hi @[u_a]"
	style := Style{Start: 0, Len: 2, St: "b"}
	_, _, got := ParseMarkersWithStyles(in, fixtureResolver(t), []Style{style})
	if len(got) != 1 || got[0].Start != 0 || got[0].Len != 2 {
		t.Fatalf("got=%+v want=[{0,2,b}]", got)
	}
}

func TestParseMarkersWithStyles_MarkerInsideStyle_LenAdjusts(t *testing.T) {
	// "**@[u_vn] hi**" — bold over the inner content
	// Inner: "@[u_vn] hi" = 10 UTF-16 input.
	// After replacement: "@Đức hi" = 7 UTF-16. delta = -3.
	// Style {Start:0, Len:10, "b"} → Len becomes 10-3=7.
	in := "@[u_vn] hi"
	style := Style{Start: 0, Len: 10, St: "b"}
	rendered, _, got := ParseMarkersWithStyles(in, fixtureResolver(t), []Style{style})
	if rendered != "@Đức hi" {
		t.Fatalf("rendered=%q", rendered)
	}
	if len(got) != 1 || got[0].Start != 0 || got[0].Len != 7 {
		t.Fatalf("got=%+v want=[{0,7,b}]", got)
	}
}

func TestParseMarkersWithStyles_StyleInsideMarker_Dropped(t *testing.T) {
	// Style exactly covering the marker — meaningless after replacement.
	in := "@[u_a]"
	style := Style{Start: 0, Len: 6, St: "b"}
	_, _, got := ParseMarkersWithStyles(in, fixtureResolver(t), []Style{style})
	if len(got) != 0 {
		t.Fatalf("got=%+v want=nil (drop)", got)
	}
}

func TestParseMarkersWithStyles_MultipleMarkers_CumulativeDelta(t *testing.T) {
	// "@[u_vn] hi @[u_vn] bye" — two markers each with delta=-3.
	// Style over "bye" at input pos 18 len 3:
	//   marker1 [0,7)  delta -3, style right of marker1 → shift -3
	//   marker2 [11,18) delta -3, styleStart=18, sp.endUTF16=18 → 18>=18 → shift -3
	// total shift = -6 → output Start = 18-6 = 12. In output "@Đức hi @Đức bye"
	// = 4+1+2+1+4+1=13... let me recompute: "@Đức hi @Đức " = 4+1+2+1+4+1=13.
	// "bye" starts at 12. So Start = 12, Len = 3.
	in := "@[u_vn] hi @[u_vn] bye"
	style := Style{Start: 18, Len: 3, St: "b"}
	rendered, _, got := ParseMarkersWithStyles(in, fixtureResolver(t), []Style{style})
	if rendered != "@Đức hi @Đức bye" {
		t.Fatalf("rendered=%q", rendered)
	}
	if len(got) != 1 || got[0].Start != 12 || got[0].Len != 3 {
		t.Fatalf("got=%+v want=[{12,3,b}]", got)
	}
}

func TestParseMarkersWithStyles_FiveStylesAcrossThreeMarkers(t *testing.T) {
	// Sanity: lots of moving parts.
	// "@[u_a] **bold** @[u_b] **italic**" — bold over "bold" at pos 9 len 4,
	// italic over "italic" at pos 25 len 6.
	in := "@[u_a] bold @[u_b] italic"
	styles := []Style{
		{Start: 7, Len: 4, St: "b"},  // "bold" at input pos 7
		{Start: 19, Len: 6, St: "i"}, // "italic" at input pos 19
	}
	// markers: [0,6) delta 0 (@[u_a]→@Alice), [12,18) delta -2 (@[u_b]→@Bob)
	// Style "bold" at 7: right of m1 (shift 0), left of m2 → final 7,4
	// Style "italic" at 19: right of m1 (shift 0), right of m2 (shift -2) → 17,6
	rendered, _, got := ParseMarkersWithStyles(in, fixtureResolver(t), styles)
	if rendered != "@Alice bold @Bob italic" {
		t.Fatalf("rendered=%q", rendered)
	}
	expected := []Style{
		{Start: 7, Len: 4, St: "b"},
		{Start: 17, Len: 6, St: "i"},
	}
	if !reflect.DeepEqual(got, expected) {
		t.Fatalf("got=%+v want=%+v", got, expected)
	}
}

func TestParseMarkersWithStyles_StrippedUIDMarker_StyleRightShifts(t *testing.T) {
	// "@[12345] hello bold" — marker [0,8) stripped + following space consumed
	// → cumulative shift -9. Style over "bold" at input pos 15 len 4 → 6,4.
	in := "@[12345] hello bold"
	style := Style{Start: 15, Len: 4, St: "b"}
	rendered, ms, got := ParseMarkersWithStyles(in, fixtureResolver(t), []Style{style})
	if rendered != "hello bold" {
		t.Fatalf("rendered=%q", rendered)
	}
	if ms != nil {
		t.Fatalf("ms=%+v, want nil", ms)
	}
	if len(got) != 1 || got[0].Start != 6 || got[0].Len != 4 {
		t.Fatalf("got=%+v want=[{6,4,b}]", got)
	}
}

func TestParseMarkersWithStyles_StrippedUIDMarkerInsideStyle_LenShrinks(t *testing.T) {
	// Bold over the whole "@[123] hi" — marker+space span [0,7) delta -7 →
	// style len 9-7=2 over output "hi".
	in := "@[123] hi"
	style := Style{Start: 0, Len: 9, St: "b"}
	rendered, _, got := ParseMarkersWithStyles(in, fixtureResolver(t), []Style{style})
	if rendered != "hi" {
		t.Fatalf("rendered=%q", rendered)
	}
	if len(got) != 1 || got[0].Start != 0 || got[0].Len != 2 {
		t.Fatalf("got=%+v want=[{0,2,b}]", got)
	}
}

func TestParseMarkersWithStyles_StrippedUIDThenResolvedMention(t *testing.T) {
	// "@[999] hi @[u_b] bold": marker1 [0,6)+space stripped (delta -7),
	// marker2 "@[u_b]" [10,16) → "@Bob" (delta -2).
	// Style over "bold" at input pos 17 len 4 → 17-7-2=8.
	in := "@[999] hi @[u_b] bold"
	style := Style{Start: 17, Len: 4, St: "b"}
	rendered, ms, got := ParseMarkersWithStyles(in, fixtureResolver(t), []Style{style})
	if rendered != "hi @Bob bold" {
		t.Fatalf("rendered=%q", rendered)
	}
	if len(ms) != 1 || ms[0].UserID != "u_b" || ms[0].Position != 3 || ms[0].Length != 4 {
		t.Fatalf("ms=%+v", ms)
	}
	if len(got) != 1 || got[0].Start != 8 || got[0].Len != 4 {
		t.Fatalf("got=%+v want=[{8,4,b}]", got)
	}
}

// Regression pin: ParseMarkers (non-styles) must keep identical behavior to
// before the ParseMarkersWithStyles addition — used by zalo/bot caller.
func TestParseMarkers_BotPathUnchanged(t *testing.T) {
	in := "@[u_a] hello @[u_b]"
	want := "@Alice hello @Bob"
	got, ms := ParseMarkers(in, fixtureResolver(t))
	if got != want {
		t.Fatalf("rendered=%q want=%q", got, want)
	}
	if len(ms) != 2 {
		t.Fatalf("mentions=%+v", ms)
	}
}

// With styles present, each unique marker must resolve exactly once across both
// the render pass and the span pass (memoization halves DB lookups).
func TestParseMarkersWithStyles_ResolvesEachMarkerOnce(t *testing.T) {
	calls := map[string]int{}
	resolve := func(m string) (string, string, bool) {
		calls[m]++
		switch m {
		case "u_a":
			return m, "Alice", true
		case "u_b":
			return m, "Bob", true
		default:
			return "", "", false
		}
	}
	in := "@[u_a] @[u_b] @[u_a] @[u_zz]"
	style := Style{Start: 0, Len: protocol.UTF16Len(in), St: "b"}
	ParseMarkersWithStyles(in, resolve, []Style{style})
	for marker, n := range calls {
		if n != 1 {
			t.Errorf("marker %q resolved %d times, want 1", marker, n)
		}
	}
	if calls["u_a"] != 1 {
		t.Errorf("repeated marker u_a resolved %d times, want 1", calls["u_a"])
	}
}
