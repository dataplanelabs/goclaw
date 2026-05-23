package protocol

import "strings"

const (
	ReactionNone  = ""
	ReactionHaha  = ":>"
	ReactionCry   = ":-(("
	ReactionLike  = "/-strong"
	ReactionHeart = "/-heart"
	ReactionAngry = ":-h"
	ReactionWow   = ":o"
)

type reactionMeta struct {
	RType  int
	Source int
}

var reactionMetaTable = map[string]reactionMeta{
	ReactionHaha:  {0, 6},
	ReactionCry:   {2, 6},
	ReactionLike:  {3, 6},
	ReactionHeart: {5, 6},
	ReactionAngry: {20, 6},
	ReactionWow:   {32, 6},
}

// LookupReactionMeta returns {-1, 6} for unknown codes — Zalo's marker for
// removal / unhandled reactions.
func LookupReactionMeta(code string) reactionMeta {
	if m, ok := reactionMetaTable[code]; ok {
		return m
	}
	return reactionMeta{RType: -1, Source: 6}
}

var unicodeToZalo = map[string]string{
	"❤️": ReactionHeart,
	"❤":  ReactionHeart,
	"👍": ReactionLike,
	"😂": ReactionHaha,
	"😢": ReactionCry,
	"😭": ReactionCry,
	"😡": ReactionAngry,
	"😠": ReactionAngry,
	"😮": ReactionWow,
	"😯": ReactionWow,
	"😱": ReactionWow,
}

var englishNameToZalo = map[string]string{
	"heart":     ReactionHeart,
	"love":      ReactionHeart,
	"like":      ReactionLike,
	"thumbs_up": ReactionLike,
	"thumbsup":  ReactionLike,
	"haha":      ReactionHaha,
	"laugh":     ReactionHaha,
	"wow":       ReactionWow,
	"surprise":  ReactionWow,
	"cry":       ReactionCry,
	"sad":       ReactionCry,
	"angry":     ReactionAngry,
	"remove":    ReactionNone,
	"none":      ReactionNone,
	"unreact":   ReactionNone,
}

// ResolveReactionCode accepts unicode emoji, English friendly name, or raw
// Zalo code. Returns (code, true) on success; ("", false) when unknown.
func ResolveReactionCode(input string) (string, bool) {
	if input == "" {
		return ReactionNone, true
	}
	if _, ok := reactionMetaTable[input]; ok {
		return input, true
	}
	if z, ok := unicodeToZalo[input]; ok {
		return z, true
	}
	if z, ok := englishNameToZalo[strings.ToLower(strings.TrimSpace(input))]; ok {
		return z, true
	}
	return "", false
}

var zaloToUnicode = func() map[string]string {
	out := make(map[string]string, len(unicodeToZalo))
	for u, z := range unicodeToZalo {
		if _, exists := out[z]; !exists {
			out[z] = u
		}
	}
	return out
}()

// ReactionCodeToUnicode returns a canonical unicode emoji for a Zalo code,
// or "" if no mapping exists. Used by the synthetic inbound formatter.
func ReactionCodeToUnicode(code string) string {
	return zaloToUnicode[code]
}
