package mentions

import (
	"reflect"
	"testing"

	"github.com/nextlevelbuilder/goclaw/pkg/protocol"
)

func TestSelectMentionsInRange_AllInside(t *testing.T) {
	all := []protocol.Mention{
		{UserID: "a", Position: 5, Length: 6},
		{UserID: "b", Position: 20, Length: 4},
	}
	kept, dropped := SelectMentionsInRange(all, 0, 100)
	if !reflect.DeepEqual(kept, all) {
		t.Fatalf("kept=%+v", kept)
	}
	if dropped != nil {
		t.Fatalf("dropped=%+v", dropped)
	}
}

func TestSelectMentionsInRange_OutsideBefore(t *testing.T) {
	all := []protocol.Mention{{UserID: "a", Position: 5, Length: 4}}
	kept, dropped := SelectMentionsInRange(all, 100, 50)
	if kept != nil || dropped != nil {
		t.Fatalf("kept=%+v dropped=%+v", kept, dropped)
	}
}

func TestSelectMentionsInRange_StraddlesBoundary(t *testing.T) {
	// mention spans [95, 105); chunk [0, 100) → straddles upper boundary.
	all := []protocol.Mention{{UserID: "a", Position: 95, Length: 10}}
	kept, dropped := SelectMentionsInRange(all, 0, 100)
	if kept != nil {
		t.Fatalf("kept=%+v", kept)
	}
	if len(dropped) != 1 {
		t.Fatalf("dropped=%+v", dropped)
	}
}

func TestSelectMentionsInRange_ExactlyAtEnd(t *testing.T) {
	// mention spans [96, 100); chunk [0, 100) → fits exactly (end exclusive).
	all := []protocol.Mention{{UserID: "a", Position: 96, Length: 4}}
	kept, _ := SelectMentionsInRange(all, 0, 100)
	if len(kept) != 1 {
		t.Fatalf("kept=%+v", kept)
	}
}

func TestSelectMentionsInRange_ExactlyAtStart(t *testing.T) {
	all := []protocol.Mention{{UserID: "a", Position: 100, Length: 4}}
	kept, _ := SelectMentionsInRange(all, 100, 50)
	if len(kept) != 1 || kept[0].Position != 100 {
		t.Fatalf("kept=%+v", kept)
	}
}

func TestSelectMentionsInRange_SecondChunk(t *testing.T) {
	all := []protocol.Mention{
		{UserID: "a", Position: 10, Length: 4},
		{UserID: "b", Position: 110, Length: 6},
	}
	kept, dropped := SelectMentionsInRange(all, 100, 50)
	if dropped != nil {
		t.Fatalf("dropped=%+v", dropped)
	}
	if len(kept) != 1 || kept[0].UserID != "b" {
		t.Fatalf("kept=%+v", kept)
	}
}
