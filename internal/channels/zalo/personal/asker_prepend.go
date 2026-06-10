package personal

import "strings"

// applyAskerPrepend leads an outbound group reply with @[<askerUID>] so the
// bot's response addresses the message author. Skips when content already
// mentions the asker, when @[all]/@[All] is present (broadcast intent), or
// when askerUID/content is empty.
func applyAskerPrepend(content, askerUID string) string {
	if askerUID == "" || content == "" {
		return content
	}
	askerUID = strings.TrimSpace(askerUID)
	marker := "@[" + askerUID + "]"
	if strings.Contains(content, marker) {
		return content
	}
	if strings.Contains(content, "@[all]") || strings.Contains(content, "@[All]") {
		return content
	}
	return marker + " " + content
}
