package personal

import (
	"reflect"
	"testing"

	"github.com/nextlevelbuilder/goclaw/internal/channels"
	zcommon "github.com/nextlevelbuilder/goclaw/internal/channels/zalo/common"
	pkgproto "github.com/nextlevelbuilder/goclaw/pkg/protocol"
)

func TestMentionsByChunk_AskerAtHeadAlwaysKept(t *testing.T) {
	asker := pkgproto.Mention{Position: 0, Length: 7, UserID: "u1"} // "@Van Duc"
	got, dropped := mentionsByChunk([]pkgproto.Mention{asker}, []chunkRange{{startUTF16: 0, lenUTF16: 2000}})
	if len(got[0]) != 1 || dropped != 0 {
		t.Fatalf("asker mention at head should be kept; got chunks=%v dropped=%d", got, dropped)
	}
}

func TestMentionsByChunk_LocalizesSecondChunk(t *testing.T) {
	mentions := []pkgproto.Mention{
		{Position: 0, Length: 7, UserID: "u_asker"},
		{Position: 1500, Length: 4, UserID: "u_mid"},  // fits in 2000
		{Position: 1998, Length: 5, UserID: "u_edge"}, // straddles boundary → drop
		{Position: 2500, Length: 3, UserID: "u_far"},  // second chunk → localizes to 500
	}
	got, dropped := mentionsByChunk(mentions, []chunkRange{
		{startUTF16: 0, lenUTF16: 2000},
		{startUTF16: 2000, lenUTF16: 1000},
	})
	if len(got[0]) != 2 {
		t.Errorf("chunk 0 kept count: got %d, want 2", len(got[0]))
	}
	if len(got[1]) != 1 || got[1][0].Position != 500 {
		t.Errorf("chunk 1 localized mention = %+v, want position 500", got[1])
	}
	if dropped != 1 {
		t.Errorf("dropped count: got %d, want 1", dropped)
	}
	want := []pkgproto.Mention{
		{Position: 0, Length: 7, UserID: "u_asker"},
		{Position: 1500, Length: 4, UserID: "u_mid"},
	}
	if !reflect.DeepEqual(got[0], want) {
		t.Errorf("chunk 0 = %+v\nwant %+v", got[0], want)
	}
}

func TestMentionsByChunk_ZeroBoundary(t *testing.T) {
	_, dropped := mentionsByChunk([]pkgproto.Mention{{Position: 0, Length: 5}}, []chunkRange{{startUTF16: 0, lenUTF16: 0}})
	if dropped != 1 {
		t.Errorf("zero-len chunk drops all; got %d", dropped)
	}
}

func TestStylesByChunk_LocalizesAcrossChunks(t *testing.T) {
	styles := []zcommon.Style{
		{Start: 0, Len: 4, St: "b"},     // bold over first word — fits
		{Start: 100, Len: 10, St: "i"},  // italic in middle — fits
		{Start: 1990, Len: 20, St: "s"}, // strike spans boundary — drop
		{Start: 2300, Len: 8, St: "b"},  // second chunk → localizes to 300
	}
	got, dropped := stylesByChunk(styles, []chunkRange{
		{startUTF16: 0, lenUTF16: 2000},
		{startUTF16: 2000, lenUTF16: 1000},
	})
	if len(got[0]) != 2 || len(got[1]) != 1 || dropped != 1 {
		t.Fatalf("got chunks=%v dropped=%d", got, dropped)
	}
	if got[1][0] != (zcommon.Style{Start: 300, Len: 8, St: "b"}) {
		t.Fatalf("second chunk style = %+v, want local start 300", got[1][0])
	}
}

func TestStylesByChunk_NegativePositionsDropped(t *testing.T) {
	styles := []zcommon.Style{
		{Start: -1, Len: 5, St: "b"}, // invalid start
		{Start: 0, Len: 5, St: "b"},
	}
	got, dropped := stylesByChunk(styles, []chunkRange{{startUTF16: 0, lenUTF16: 2000}})
	if len(got[0]) != 1 || dropped != 1 {
		t.Errorf("expected to drop neg-start; got chunks=%v dropped=%d", got, dropped)
	}
}

func TestStylesByChunk_RealMarkdownRegression(t *testing.T) {
	input := "**1. Ăn đủ**\n- trước tập\n\n**2. Nạp carbs**\n- trong tập\n\n**3. Phục hồi**\n- sau tập"
	text, styles := zcommon.RenderStyles(input)
	chunks := channels.ChunkMarkdown(text, 34)
	if len(chunks) < 3 {
		t.Fatalf("test fixture must split into at least 3 chunks, got %d: %#v", len(chunks), chunks)
	}

	got, dropped := stylesByChunk(styles, chunkRangesUTF16(text, chunks))
	if dropped != 0 {
		t.Fatalf("expected no dropped styles, got %d", dropped)
	}
	for i := range chunks {
		if len(got[i]) == 0 {
			t.Fatalf("chunk %d lost native style; chunks=%#v styles=%#v", i, chunks, got)
		}
		if got[i][0].Start != 0 {
			t.Fatalf("chunk %d style start = %d, want local offset 0", i, got[i][0].Start)
		}
	}
}
