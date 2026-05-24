package bootstrap

import (
	_ "embed"
	"regexp"
	"strings"
)

//go:embed templates/ZALO_PERSONAL_ADDENDUM.md
var zaloPersonalAddendum string

var channelAddenda = map[string]string{
	"zalo_personal": zaloPersonalAddendum,
}

// AddendumOpts controls conditional sections in per-channel addendums.
type AddendumOpts struct {
	// EnableNativeStyles toggles the inclusion of the markdown→Zalo styling
	// guidance block (bracketed in the source with BEGIN_NATIVE_STYLES /
	// END_NATIVE_STYLES sentinels). When false, the block + its surrounding
	// blank lines are stripped so the LLM doesn't see contradictory advice
	// to emit `**bold**` markdown that the legacy strip path would remove.
	EnableNativeStyles bool
}

// nativeStylesBlockRE matches the BEGIN/END sentinel block (greedy across lines),
// including any leading/trailing blank lines so the strip is clean.
var nativeStylesBlockRE = regexp.MustCompile(`(?s)\n*<!--\s*BEGIN_NATIVE_STYLES\s*-->.*?<!--\s*END_NATIVE_STYLES\s*-->\n*`)

// ChannelAddendum returns the per-channel system-prompt addendum, or
// ok=false when none is registered (zero token cost).
func ChannelAddendum(channelType string, opts AddendumOpts) (string, bool) {
	if channelType == "" {
		return "", false
	}
	addendum, ok := channelAddenda[channelType]
	if !ok {
		return "", false
	}
	if !opts.EnableNativeStyles {
		addendum = nativeStylesBlockRE.ReplaceAllString(addendum, "\n")
	} else {
		// Strip just the marker comment lines, keep the body.
		addendum = strings.ReplaceAll(addendum, "<!-- BEGIN_NATIVE_STYLES -->\n", "")
		addendum = strings.ReplaceAll(addendum, "<!-- END_NATIVE_STYLES -->\n", "")
		addendum = strings.ReplaceAll(addendum, "<!-- BEGIN_NATIVE_STYLES -->", "")
		addendum = strings.ReplaceAll(addendum, "<!-- END_NATIVE_STYLES -->", "")
	}
	return addendum, true
}
