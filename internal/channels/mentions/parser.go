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

// ParseMarkersWithStyles is ParseMarkers + UTF-16 style-position adjustment.
// Input styles are positions over the input text (pre-marker-replacement);
// output styles are positions over the returned text. Replaces @[uid] with
// @DisplayName whose length usually differs from the marker; styles to the
// right shift, styles overlapping a marker may grow/shrink or drop.
func ParseMarkersWithStyles(text string, resolve Resolve, styles []Style) (string, []protocol.Mention, []Style) {
	rendered, mentions := ParseMarkers(text, resolve)
	if len(styles) == 0 {
		return rendered, mentions, nil
	}

	matches := markerRE.FindAllStringSubmatchIndex(text, -1)
	if len(matches) == 0 {
		return rendered, mentions, styles
	}

	// Walk markers in order; for each, compute input-UTF16 span and output
	// replacement UTF-16 length, then adjust each style according to 5 cases.
	// Convert byte offsets → UTF-16 offsets via UTF16Len over text prefix.
	cursor := 0
	for _, m := range matches {
		start, end := m[0], m[1]
		capStart, capEnd := m[2], m[3]
		marker := text[capStart:capEnd]

		mStartUTF16 := protocol.UTF16Len(text[:start])
		mEndUTF16 := protocol.UTF16Len(text[:end])
		markerLenUTF16 := mEndUTF16 - mStartUTF16
		_ = cursor // reserved for future incremental walking

		var replacementLenUTF16 int
		if marker == "all" || marker == "All" || marker == "everyone" {
			replacementLenUTF16 = protocol.UTF16Len("@All")
		} else if uid, displayName, ok := resolve(marker); ok && uid != "" {
			replacementLenUTF16 = protocol.UTF16Len("@" + displayName)
		} else {
			replacementLenUTF16 = markerLenUTF16
		}
		delta := replacementLenUTF16 - markerLenUTF16

		var next []Style
		for _, s := range styles {
			styleEnd := s.Start + s.Len
			switch {
			case s.Start >= mEndUTF16:
				next = append(next, Style{Start: s.Start + delta, Len: s.Len, St: s.St})
			case styleEnd <= mStartUTF16:
				next = append(next, s)
			case s.Start >= mStartUTF16 && styleEnd <= mEndUTF16:
				// style entirely inside marker — drop (now meaningless).
			case s.Start <= mStartUTF16 && styleEnd >= mEndUTF16:
				newLen := s.Len + delta
				if newLen > 0 {
					next = append(next, Style{Start: s.Start, Len: newLen, St: s.St})
				}
			default:
				// Partial overlap — conservative drop.
			}
		}
		styles = next
	}

	return rendered, mentions, styles
}
