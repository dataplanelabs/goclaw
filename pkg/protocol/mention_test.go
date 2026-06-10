package protocol

import (
	"encoding/json"
	"testing"
)

func TestUTF16Len(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want int
	}{
		{"ASCII", "hello", 5},
		{"Empty", "", 0},
		{"VietnameseCamOn", "Cảm ơn", 6},
		{"VietnameseDuc", "Đức", 3},
		{"VietnameseViet", "Việt", 4},
		{"EmojiSimple", "🎉", 2},
		{"EmojiWithSkinTone", "👋🏽", 4},
		{"CJK", "中文", 2},
		{"MixedHiEmojiDuc", "Hi 🎉 đức", 9},
		{"AtDuc", "@Đức", 4},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := UTF16Len(tc.in)
			if got != tc.want {
				t.Fatalf("UTF16Len(%q) = %d, want %d", tc.in, got, tc.want)
			}
		})
	}
}

func TestMention_JSONRoundtrip(t *testing.T) {
	original := Mention{
		UserID:      "123",
		DisplayName: "Đức",
		Position:    5,
		Length:      4,
		Type:        1,
	}
	encoded, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded Mention
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded != original {
		t.Fatalf("roundtrip mismatch: got %+v, want %+v", decoded, original)
	}
}

func TestMention_JSONOmitsEmptyDisplayName(t *testing.T) {
	m := Mention{UserID: "-1", Position: 0, Length: 4, Type: 1}
	b, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	got := string(b)
	want := `{"uid":"-1","pos":0,"len":4,"type":1}`
	if got != want {
		t.Fatalf("JSON shape: got %s, want %s", got, want)
	}
}

func TestMention_JSONShapeMatchesZcaJs(t *testing.T) {
	mentions := []Mention{
		{UserID: "u_a", DisplayName: "Alice", Position: 7, Length: 6, Type: 0},
		{UserID: MentionAllUID, DisplayName: "all", Position: 18, Length: 4, Type: 1},
	}
	b, err := json.Marshal(mentions)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	got := string(b)
	want := `[{"uid":"u_a","display_name":"Alice","pos":7,"len":6,"type":0},{"uid":"-1","display_name":"all","pos":18,"len":4,"type":1}]`
	if got != want {
		t.Fatalf("wire shape drift:\n got  %s\n want %s", got, want)
	}
}
