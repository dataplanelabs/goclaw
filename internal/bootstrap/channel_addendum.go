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
	// EnableNativeStyles selects between two formatting guidance blocks:
	//   true  → keep BEGIN_NATIVE_STYLES block (markdown→native styles),
	//           strip BEGIN_PLAIN_TEXT block
	//   false → strip BEGIN_NATIVE_STYLES block, keep BEGIN_PLAIN_TEXT block
	//           (LLM learns NOT to emit markdown that the strip path removes)
	EnableNativeStyles bool
}

// blockRE matches a sentinel-bracketed conditional block (greedy across
// lines), including any leading/trailing blank lines for a clean strip.
var (
	nativeStylesBlockRE = regexp.MustCompile(`(?s)\n*<!--\s*BEGIN_NATIVE_STYLES\s*-->.*?<!--\s*END_NATIVE_STYLES\s*-->\n*`)
	plainTextBlockRE    = regexp.MustCompile(`(?s)\n*<!--\s*BEGIN_PLAIN_TEXT\s*-->.*?<!--\s*END_PLAIN_TEXT\s*-->\n*`)
)

// stripSentinelMarkers removes the BEGIN/END comment lines but keeps the
// body. Used on the block we want to retain.
func stripSentinelMarkers(s, name string) string {
	for _, marker := range []string{
		"<!-- BEGIN_" + name + " -->\n",
		"<!-- END_" + name + " -->\n",
		"<!-- BEGIN_" + name + " -->",
		"<!-- END_" + name + " -->",
	} {
		s = strings.ReplaceAll(s, marker, "")
	}
	return s
}

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
	if opts.EnableNativeStyles {
		addendum = plainTextBlockRE.ReplaceAllString(addendum, "\n")
		addendum = stripSentinelMarkers(addendum, "NATIVE_STYLES")
	} else {
		addendum = nativeStylesBlockRE.ReplaceAllString(addendum, "\n")
		addendum = stripSentinelMarkers(addendum, "PLAIN_TEXT")
	}
	return addendum, true
}
