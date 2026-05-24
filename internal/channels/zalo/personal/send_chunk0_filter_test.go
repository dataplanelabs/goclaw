package personal

import (
	"reflect"
	"testing"

	zcommon "github.com/nextlevelbuilder/goclaw/internal/channels/zalo/common"
	pkgproto "github.com/nextlevelbuilder/goclaw/pkg/protocol"
)

func TestMentionsFittingChunk0_AskerAtHeadAlwaysKept(t *testing.T) {
	asker := pkgproto.Mention{Position: 0, Length: 7, UserID: "u1"} // "@Van Duc"
	kept, dropped := mentionsFittingChunk0([]pkgproto.Mention{asker}, 2000)
	if len(kept) != 1 || dropped != 0 {
		t.Fatalf("asker mention at head should be kept; got kept=%v dropped=%d", kept, dropped)
	}
}

func TestMentionsFittingChunk0_DropsBeyondBoundary(t *testing.T) {
	mentions := []pkgproto.Mention{
		{Position: 0, Length: 7, UserID: "u_asker"},
		{Position: 1500, Length: 4, UserID: "u_mid"},  // fits in 2000
		{Position: 1998, Length: 5, UserID: "u_edge"}, // straddles boundary → drop
		{Position: 2500, Length: 3, UserID: "u_far"},  // past chunk 0 → drop
	}
	kept, dropped := mentionsFittingChunk0(mentions, 2000)
	if len(kept) != 2 {
		t.Errorf("kept count: got %d, want 2", len(kept))
	}
	if dropped != 2 {
		t.Errorf("dropped count: got %d, want 2", dropped)
	}
	want := []pkgproto.Mention{
		{Position: 0, Length: 7, UserID: "u_asker"},
		{Position: 1500, Length: 4, UserID: "u_mid"},
	}
	if !reflect.DeepEqual(kept, want) {
		t.Errorf("kept = %+v\nwant %+v", kept, want)
	}
}

func TestMentionsFittingChunk0_ZeroBoundary(t *testing.T) {
	_, dropped := mentionsFittingChunk0([]pkgproto.Mention{{Position: 0, Length: 5}}, 0)
	if dropped != 1 {
		t.Errorf("zero-len chunk drops all; got %d", dropped)
	}
}

func TestStylesFittingChunk0_AskerBoldAtHead(t *testing.T) {
	styles := []zcommon.Style{
		{Start: 0, Len: 4, St: "b"},     // bold over first word — fits
		{Start: 100, Len: 10, St: "i"},  // italic in middle — fits
		{Start: 1990, Len: 20, St: "s"}, // strike spans boundary — drop
	}
	kept, dropped := stylesFittingChunk0(styles, 2000)
	if len(kept) != 2 || dropped != 1 {
		t.Fatalf("got kept=%v dropped=%d", kept, dropped)
	}
}

func TestStylesFittingChunk0_NegativePositionsDropped(t *testing.T) {
	styles := []zcommon.Style{
		{Start: -1, Len: 5, St: "b"}, // invalid start
		{Start: 0, Len: 5, St: "b"},
	}
	kept, dropped := stylesFittingChunk0(styles, 2000)
	if len(kept) != 1 || dropped != 1 {
		t.Errorf("expected to drop neg-start; got kept=%v dropped=%d", kept, dropped)
	}
}
