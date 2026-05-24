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

		if marker == "all" {
			replacement := "@all"
			out.WriteString(replacement)
			mentions = append(mentions, protocol.Mention{
				UserID:      protocol.MentionAllUID,
				DisplayName: "all",
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
