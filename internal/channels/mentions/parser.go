// Package mentions parses @[uid] / @[all] markers into Zalo mention spans.
package mentions

import (
	"regexp"
	"strings"

	"github.com/nextlevelbuilder/goclaw/pkg/protocol"
)

var markerRE = regexp.MustCompile(`@\[([^\[\]]+)\]`)

// Resolve maps a marker token to a UID + display name. ok=false leaves the
// marker as literal text. "all" is reserved by the parser.
type Resolve func(marker string) (uid, displayName string, ok bool)

// ParseMarkers rewrites @[uid] / @[all] markers to @<DisplayName> and returns
// the rewritten text + mention spans with UTF-16 offsets.
func ParseMarkers(text string, resolve Resolve) (string, []protocol.Mention) {
	if !strings.Contains(text, "@[") {
		return text, nil
	}
	matches := markerRE.FindAllStringSubmatchIndex(text, -1)
	if len(matches) == 0 {
		return text, nil
	}

	var (
		out      strings.Builder
		mentions []protocol.Mention
		cursor   int
		posUTF16 int
	)
	out.Grow(len(text))

	for _, m := range matches {
		start, end := m[0], m[1]
		capStart, capEnd := m[2], m[3]

		if start > cursor {
			segment := text[cursor:start]
			out.WriteString(segment)
			posUTF16 += protocol.UTF16Len(segment)
		}

		marker := text[capStart:capEnd]

		if marker == "all" || marker == "All" || marker == "everyone" {
			replacement := "@All"
			out.WriteString(replacement)
			mentions = append(mentions, protocol.Mention{
				UserID:      protocol.MentionAllUID,
				DisplayName: "All",
				Position:    posUTF16,
				Length:      protocol.UTF16Len(replacement),
				Type:        1,
			})
			posUTF16 += protocol.UTF16Len(replacement)
			cursor = end
			continue
		}

		uid, displayName, ok := resolve(marker)
		if !ok {
			literal := text[start:end]
			out.WriteString(literal)
			posUTF16 += protocol.UTF16Len(literal)
			cursor = end
			continue
		}

		replacement := "@" + displayName
		out.WriteString(replacement)
		mentions = append(mentions, protocol.Mention{
			UserID:      uid,
			DisplayName: displayName,
			Position:    posUTF16,
			Length:      protocol.UTF16Len(replacement),
			Type:        0,
		})
		posUTF16 += protocol.UTF16Len(replacement)
		cursor = end
	}

	if cursor < len(text) {
		out.WriteString(text[cursor:])
	}

	return out.String(), mentions
}

// Style mirrors the zalo/common.Style shape WITHOUT importing the channel
// package — keeps the cross-channel `mentions` package dependency-free.
// Callers cast their own []common.Style ⇄ []Style when invoking.
type Style struct {
	Start int
	Len   int
	St    string
}

// markerSpan captures one @[uid] marker's input-space UTF-16 bounds and the
// output-space length of its replacement.
type markerSpan struct {
	startUTF16 int
	endUTF16   int
	delta      int
}

// ParseMarkersWithStyles is ParseMarkers + UTF-16 style-position adjustment.
// Input styles are positions over the input text (pre-marker-replacement);
// output styles are positions over the returned text. Replaces @[uid] with
// @DisplayName whose length usually differs from the marker; styles to the
// right shift, styles overlapping a marker may grow/shrink or drop.
//
// The marker-loop must operate on each style with ALL marker spans in input
// coordinates — walking marker-by-mutate-styles risks mixing coordinate
// spaces. Approach: collect every marker's input-space span + delta first,
// then for each input-space style aggregate the cumulative shift / length
// adjustment from every marker.
func ParseMarkersWithStyles(text string, resolve Resolve, styles []Style) (string, []protocol.Mention, []Style) {
	rendered, mentions := ParseMarkers(text, resolve)
	if len(styles) == 0 {
		return rendered, mentions, nil
	}

	matches := markerRE.FindAllStringSubmatchIndex(text, -1)
	if len(matches) == 0 {
		return rendered, mentions, styles
	}

	spans := make([]markerSpan, 0, len(matches))
	for _, m := range matches {
		start, end := m[0], m[1]
		capStart, capEnd := m[2], m[3]
		marker := text[capStart:capEnd]

		mStartUTF16 := protocol.UTF16Len(text[:start])
		mEndUTF16 := protocol.UTF16Len(text[:end])
		markerLenUTF16 := mEndUTF16 - mStartUTF16

		var replacementLenUTF16 int
		if marker == "all" || marker == "All" || marker == "everyone" {
			replacementLenUTF16 = protocol.UTF16Len("@All")
		} else if uid, displayName, ok := resolve(marker); ok && uid != "" {
			replacementLenUTF16 = protocol.UTF16Len("@" + displayName)
		} else {
			replacementLenUTF16 = markerLenUTF16
		}
		spans = append(spans, markerSpan{
			startUTF16: mStartUTF16,
			endUTF16:   mEndUTF16,
			delta:      replacementLenUTF16 - markerLenUTF16,
		})
	}

	out := make([]Style, 0, len(styles))
	for _, s := range styles {
		styleStart := s.Start
		styleEnd := s.Start + s.Len
		drop := false
		shift := 0     // cumulative shift to apply to Start
		lenDelta := 0  // cumulative len adjustment for "marker inside style" case
		for _, sp := range spans {
			switch {
			case styleStart >= sp.endUTF16:
				// style entirely right of marker → shift by full delta
				shift += sp.delta
			case styleEnd <= sp.startUTF16:
				// style entirely left of marker → no change
			case styleStart >= sp.startUTF16 && styleEnd <= sp.endUTF16:
				// style entirely inside marker → drop
				drop = true
			case styleStart <= sp.startUTF16 && styleEnd >= sp.endUTF16:
				// marker entirely inside style → len grows/shrinks by delta
				lenDelta += sp.delta
			default:
				// Partial overlap — conservative drop.
				drop = true
			}
			if drop {
				break
			}
		}
		if drop {
			continue
		}
		newLen := s.Len + lenDelta
		if newLen <= 0 {
			continue
		}
		out = append(out, Style{Start: s.Start + shift, Len: newLen, St: s.St})
	}

	return rendered, mentions, out
}
