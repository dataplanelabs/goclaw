package protocol

import "unicode/utf16"

const MentionAllUID = "-1"

// Mention carries display_name for goclaw-side use; direct marshal is NOT
// Zalo wire shape — channel layer strips display_name before sending.
type Mention struct {
	UserID      string `json:"uid"`
	DisplayName string `json:"display_name,omitempty"`
	Position    int    `json:"pos"`
	Length      int    `json:"len"`
	Type        int    `json:"type"`
}

func UTF16Len(s string) int {
	return len(utf16.Encode([]rune(s)))
}
