package bootstrap

import (
	_ "embed"
)

//go:embed templates/ZALO_PERSONAL_ADDENDUM.md
var zaloPersonalAddendum string

var channelAddenda = map[string]string{
	"zalo_personal": zaloPersonalAddendum,
}

// ChannelAddendum returns the per-channel system-prompt addendum, or
// ok=false when none is registered (zero token cost).
func ChannelAddendum(channelType string) (string, bool) {
	if channelType == "" {
		return "", false
	}
	addendum, ok := channelAddenda[channelType]
	return addendum, ok
}
