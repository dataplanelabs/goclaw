package personal

import (
	"encoding/json"

	"github.com/nextlevelbuilder/goclaw/internal/channels/zalo/personal/protocol"
	pkgproto "github.com/nextlevelbuilder/goclaw/pkg/protocol"
)

// stampMentionsMetadata writes []pkgproto.Mention as JSON to metadata["mentions"].
// resolveDisplayName may be nil.
func stampMentionsMetadata(metadata map[string]string, raw []*protocol.TMention, resolveDisplayName func(uid string) string) {
	if len(raw) == 0 {
		return
	}
	if resolveDisplayName == nil {
		resolveDisplayName = func(string) string { return "" }
	}
	out := make([]pkgproto.Mention, 0, len(raw))
	for _, m := range raw {
		if m == nil {
			continue
		}
		out = append(out, pkgproto.Mention{
			UserID:      m.UID,
			DisplayName: resolveDisplayName(m.UID),
			Position:    m.Pos,
			Length:      m.Len,
			Type:        int(m.Type),
		})
	}
	if len(out) == 0 {
		return
	}
	b, err := json.Marshal(out)
	if err != nil {
		return
	}
	metadata["mentions"] = string(b)
}
