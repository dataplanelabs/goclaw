package protocol

import (
	"testing"
)

func TestResolveReactionCode_UnicodeRoundtrip(t *testing.T) {
	for unicode, want := range unicodeToZalo {
		got, ok := ResolveReactionCode(unicode)
		if !ok {
			t.Errorf("ResolveReactionCode(%q) returned ok=false", unicode)
			continue
		}
		if got != want {
			t.Errorf("ResolveReactionCode(%q) = %q, want %q", unicode, got, want)
		}
		if _, exists := reactionMetaTable[got]; !exists {
			t.Errorf("ResolveReactionCode(%q) → %q which has no reactionMeta entry", unicode, got)
		}
	}
}

func TestResolveReactionCode_EnglishName(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"heart", ReactionHeart},
		{"HEART", ReactionHeart},
		{" heart ", ReactionHeart},
		{"like", ReactionLike},
		{"thumbs_up", ReactionLike},
		{"thumbsup", ReactionLike},
		{"thumbs_down", ReactionDislike},
		{"laugh", ReactionTearsOfJoy},
		{"angry", ReactionAngry},
		{"remove", ReactionNone},
		{"none", ReactionNone},
		{"unreact", ReactionNone},
	}
	for _, tc := range cases {
		got, ok := ResolveReactionCode(tc.in)
		if !ok {
			t.Errorf("ResolveReactionCode(%q) ok=false", tc.in)
			continue
		}
		if got != tc.want {
			t.Errorf("ResolveReactionCode(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestResolveReactionCode_RawCode(t *testing.T) {
	got, ok := ResolveReactionCode(ReactionHeart)
	if !ok || got != ReactionHeart {
		t.Errorf("ResolveReactionCode(%q) = (%q, %v), want (%q, true)", ReactionHeart, got, ok, ReactionHeart)
	}
}

func TestResolveReactionCode_Empty(t *testing.T) {
	got, ok := ResolveReactionCode("")
	if !ok || got != ReactionNone {
		t.Errorf("ResolveReactionCode(\"\") = (%q, %v), want (%q, true)", got, ok, ReactionNone)
	}
}

func TestResolveReactionCode_Unknown(t *testing.T) {
	got, ok := ResolveReactionCode("🚀")
	if ok || got != "" {
		t.Errorf("ResolveReactionCode(rocket) = (%q, %v), want (\"\", false)", got, ok)
	}
}

func TestLookupReactionMeta_AllWiredCodes(t *testing.T) {
	for code, meta := range reactionMetaTable {
		if meta.Source != 6 {
			t.Errorf("code %q: source=%d, want 6", code, meta.Source)
		}
		if meta.RType < 0 {
			t.Errorf("code %q: rType=%d, want >= 0", code, meta.RType)
		}
		got := LookupReactionMeta(code)
		if got != meta {
			t.Errorf("LookupReactionMeta(%q) = %+v, want %+v", code, got, meta)
		}
	}
}

func TestLookupReactionMeta_UnknownCode(t *testing.T) {
	got := LookupReactionMeta("not-a-real-code")
	want := reactionMeta{RType: -1, Source: 6}
	if got != want {
		t.Errorf("LookupReactionMeta(unknown) = %+v, want %+v", got, want)
	}
}

func TestCatalogCount(t *testing.T) {
	// 54 reactions as of 2026-05-23. Bump when adding new wired codes.
	const want = 54
	if got := len(reactionMetaTable); got != want {
		t.Errorf("reactionMetaTable has %d entries, want %d", got, want)
	}
}

func TestReactionCodeToUnicode_Heart(t *testing.T) {
	got := ReactionCodeToUnicode(ReactionHeart)
	if got != "❤️" && got != "❤" {
		t.Errorf("ReactionCodeToUnicode(heart) = %q, want a heart emoji", got)
	}
}

func TestReactionCodeToUnicode_AllMappedCodesRoundtrip(t *testing.T) {
	// Every Zalo code that appears as a value in unicodeToZalo must have a
	// non-empty reverse lookup. First-write-wins decides which unicode wins
	// when multiple map to the same code; only the existence matters here.
	wanted := make(map[string]bool)
	for _, code := range unicodeToZalo {
		wanted[code] = true
	}
	for code := range wanted {
		if got := ReactionCodeToUnicode(code); got == "" {
			t.Errorf("ReactionCodeToUnicode(%q) returned empty; expected a unicode pair", code)
		}
	}
}

func TestReactionCodeToUnicode_UnmappedCode(t *testing.T) {
	// /-li (sun) is in reactionMetaTable but the unicodeToZalo map points
	// ☀️ at ReactionSun, so it DOES roundtrip. Pick a code that is in the
	// catalog but NOT a value in unicodeToZalo, like /-shit or /-fade.
	if got := ReactionCodeToUnicode(ReactionShit); got != "" {
		t.Errorf("ReactionCodeToUnicode(%q) = %q, want empty (no unicode mapping)", ReactionShit, got)
	}
}
