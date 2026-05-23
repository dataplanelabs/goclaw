package protocol

// Reaction catalog — Zalo reaction code constants, rType/source lookup table,
// and unicode↔Zalo translator. Faithful to zca-js src/models/Reaction.ts +
// src/apis/addReaction.ts. Changes here MUST be mirrored back to the catalog
// drift test (TestCatalogCount).

import "strings"

// Reaction code constants. The string value is the Zalo wire code as emitted
// by zca-js's Reactions enum. ReactionNone ("") is the removal marker.
const (
	ReactionNone        = ""
	ReactionHeart       = "/-heart"
	ReactionLike        = "/-strong"
	ReactionHaha        = ":>"
	ReactionWow         = ":o"
	ReactionCry         = ":-(("
	ReactionAngry       = ":-h"
	ReactionKiss        = ":-*"
	ReactionTearsOfJoy  = ":')"
	ReactionShit        = "/-shit"
	ReactionRose        = "/-rose"
	ReactionBrokenHeart = "/-break"
	ReactionDislike     = "/-weak"
	ReactionLove        = ";xx"
	ReactionConfused    = ";-/"
	ReactionWink        = ";-)"
	ReactionFade        = "/-fade"
	ReactionSun         = "/-li"
	ReactionBirthday    = "/-bd"
	ReactionBomb        = "/-bome"
	ReactionOK          = "/-ok"
	ReactionPeace       = "/-v"
	ReactionThanks      = "/-thanks"
	ReactionPunch       = "/-punch"
	ReactionShare       = "/-share"
	ReactionPray        = "_()_"
	ReactionNo          = "/-no"
	ReactionBad         = "/-bad"
	ReactionLoveYou     = "/-loveu"
	ReactionSad         = "--b"
	ReactionVerySad     = ":(("
	ReactionCool        = "x-)"
	ReactionNerd        = "8-)"
	ReactionBigSmile    = ";-d"
	ReactionSunglasses  = "b-)"
	ReactionNeutral     = ":--|"
	ReactionSadFace     = "p-("
	ReactionBye         = ":-bye"
	ReactionSleepy      = "|-)"
	ReactionWipe        = ":wipe"
	ReactionDig         = ":-dig"
	ReactionAnguish     = "&-("
	ReactionHandclap    = ":handclap"
	ReactionAngryFace   = ">-|"
	ReactionFChair      = ":-f"
	ReactionLChair      = ":-l"
	ReactionRChair      = ":-r"
	ReactionSilent      = ";-x"
	ReactionSurprise    = ":-o"
	ReactionEmbarrassed = ";-s"
	ReactionAfraid      = ";-a"
	ReactionSad2        = ":-<"
	ReactionBigLaugh    = ":))"
	ReactionRich        = "$-)"
	ReactionBeer        = "/-beer"
)

// reactionMeta carries the rType + source pair Zalo expects in the inner
// reaction payload. zca-js's addReaction.ts switch covers 53 wired reactions;
// unknown codes (including ReactionNone for removal) fall back to {-1, 6}.
type reactionMeta struct {
	RType  int
	Source int
}

// reactionMetaTable mirrors zca-js src/apis/addReaction.ts case-by-case.
// Length is asserted by TestCatalogCount as a drift detector against upstream.
var reactionMetaTable = map[string]reactionMeta{
	ReactionHaha:        {0, 6},
	ReactionSad:         {1, 6},
	ReactionCry:         {2, 6},
	ReactionLike:        {3, 6},
	ReactionDislike:     {4, 6},
	ReactionHeart:       {5, 6},
	ReactionTearsOfJoy:  {7, 6},
	ReactionKiss:        {8, 6},
	ReactionVerySad:     {16, 6},
	ReactionAngry:       {20, 6},
	ReactionCool:        {21, 6},
	ReactionNerd:        {22, 6},
	ReactionBigSmile:    {23, 6},
	ReactionSunglasses:  {26, 6},
	ReactionLove:        {29, 6},
	ReactionNeutral:     {30, 6},
	ReactionWow:         {32, 6},
	ReactionSadFace:     {35, 6},
	ReactionBye:         {36, 6},
	ReactionSleepy:      {38, 6},
	ReactionWipe:        {39, 6},
	ReactionDig:         {42, 6},
	ReactionAnguish:     {44, 6},
	ReactionWink:        {45, 6},
	ReactionHandclap:    {46, 6},
	ReactionAngryFace:   {47, 6},
	ReactionFChair:      {48, 6},
	ReactionLChair:      {49, 6},
	ReactionRChair:      {50, 6},
	ReactionConfused:    {51, 6},
	ReactionSilent:      {52, 6},
	ReactionSurprise:    {53, 6},
	ReactionEmbarrassed: {54, 6},
	ReactionAfraid:      {60, 6},
	ReactionSad2:        {61, 6},
	ReactionBigLaugh:    {62, 6},
	ReactionRich:        {63, 6},
	ReactionBrokenHeart: {65, 6},
	ReactionShit:        {66, 6},
	ReactionSun:         {67, 6},
	ReactionOK:          {68, 6},
	ReactionPeace:       {69, 6},
	ReactionThanks:      {70, 6},
	ReactionPunch:       {71, 6},
	ReactionShare:       {72, 6},
	ReactionPray:        {73, 6},
	ReactionBeer:        {99, 6},
	ReactionRose:        {120, 6},
	ReactionFade:        {121, 6},
	ReactionBirthday:    {126, 6},
	ReactionBomb:        {127, 6},
	ReactionNo:          {131, 6},
	ReactionBad:         {132, 6},
	ReactionLoveYou:     {133, 6},
}

// LookupReactionMeta returns the rType+source pair for a Zalo reaction code.
// Unknown codes (including the empty NONE marker used to remove a reaction)
// return {-1, 6} — matching zca-js's default case in addReaction.ts.
func LookupReactionMeta(code string) reactionMeta {
	if m, ok := reactionMetaTable[code]; ok {
		return m
	}
	return reactionMeta{RType: -1, Source: 6}
}

// unicodeToZalo maps the most common agent-emitted unicode emoji to the
// corresponding Zalo reaction code. Multiple unicode variants may share a
// single Zalo code (e.g. ❤️ + ❤ both map to ReactionHeart). Coverage is the
// ~30 emoji decisions.md flagged as the high-frequency set.
var unicodeToZalo = map[string]string{
	"❤️":  ReactionHeart,
	"❤":   ReactionHeart,
	"👍":  ReactionLike,
	"👎":  ReactionDislike,
	"😂":  ReactionTearsOfJoy,
	"😅":  ReactionTearsOfJoy,
	"😍":  ReactionLove,
	"😘":  ReactionKiss,
	"😢":  ReactionCry,
	"😭":  ReactionVerySad,
	"😡":  ReactionAngry,
	"😠":  ReactionAngryFace,
	"😮":  ReactionWow,
	"😯":  ReactionSurprise,
	"😱":  ReactionWow,
	"😞":  ReactionSad,
	"😔":  ReactionSadFace,
	"😴":  ReactionSleepy,
	"😎":  ReactionSunglasses,
	"🤓":  ReactionNerd,
	"🤔":  ReactionConfused,
	"😏":  ReactionWink,
	"🙄":  ReactionNeutral,
	"😶":  ReactionSilent,
	"🎉":  ReactionBirthday,
	"🎂":  ReactionBirthday,
	"🙏":  ReactionPray,
	"🙏🏻": ReactionThanks,
	"👏":  ReactionHandclap,
	"🌹":  ReactionRose,
	"🍺":  ReactionBeer,
	"💣":  ReactionBomb,
	"💔":  ReactionBrokenHeart,
	"👋":  ReactionBye,
	"✌️":  ReactionPeace,
	"☀️":  ReactionSun,
}

// englishNameToZalo lets agents specify reactions by friendly name. Casing is
// normalized in ResolveReactionCode before lookup.
var englishNameToZalo = map[string]string{
	"heart":       ReactionHeart,
	"love":        ReactionLove,
	"like":        ReactionLike,
	"thumbs_up":   ReactionLike,
	"thumbsup":    ReactionLike,
	"dislike":     ReactionDislike,
	"thumbs_down": ReactionDislike,
	"haha":        ReactionHaha,
	"laugh":       ReactionTearsOfJoy,
	"wow":         ReactionWow,
	"surprise":    ReactionSurprise,
	"cry":         ReactionCry,
	"sad":         ReactionSad,
	"very_sad":    ReactionVerySad,
	"angry":       ReactionAngry,
	"kiss":        ReactionKiss,
	"rose":        ReactionRose,
	"thanks":      ReactionThanks,
	"pray":        ReactionPray,
	"ok":          ReactionOK,
	"peace":       ReactionPeace,
	"share":       ReactionShare,
	"birthday":    ReactionBirthday,
	"bomb":        ReactionBomb,
	"remove":      ReactionNone,
	"none":        ReactionNone,
	"unreact":     ReactionNone,
}

// ResolveReactionCode accepts unicode emoji, English friendly name, or a raw
// Zalo code and returns the canonical Zalo reaction string. The boolean is
// false when the input is unrecognized (caller should refuse the request and
// surface a helpful error rather than send rType=-1).
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

// zaloToUnicode is the reverse of unicodeToZalo. Built once at package init so
// runtime lookups are O(1); first-write-wins means each Zalo code gets a single
// canonical unicode (good enough for human display).
var zaloToUnicode = func() map[string]string {
	out := make(map[string]string, len(unicodeToZalo))
	for u, z := range unicodeToZalo {
		if _, exists := out[z]; !exists {
			out[z] = u
		}
	}
	return out
}()

// ReactionCodeToUnicode returns a canonical unicode emoji for a Zalo reaction
// code, or "" if the code is not in the unicode map. Used by the channel layer
// to format synthetic inbound reaction lines (single source of truth for the
// unicode↔Zalo mapping).
func ReactionCodeToUnicode(code string) string {
	return zaloToUnicode[code]
}
