package mentions

import "github.com/nextlevelbuilder/goclaw/pkg/protocol"

// SelectMentionsInRange splits mentions into those fully within
// [offsetUTF16, offsetUTF16+chunkLenUTF16) and those straddling the boundary.
// Positions are NOT translated to chunk-local offsets.
func SelectMentionsInRange(all []protocol.Mention, offsetUTF16, chunkLenUTF16 int) (kept, dropped []protocol.Mention) {
	end := offsetUTF16 + chunkLenUTF16
	for _, m := range all {
		mStart := m.Position
		mEnd := m.Position + m.Length
		if mStart >= offsetUTF16 && mEnd <= end {
			kept = append(kept, m)
		} else if mEnd > offsetUTF16 && mStart < end {
			dropped = append(dropped, m)
		}
	}
	return kept, dropped
}
