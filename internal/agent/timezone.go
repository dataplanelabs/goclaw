package agent

import "time"

// ResolveUserTimezone picks the most specific valid IANA timezone available.
// Order: channel-instance config → workspace default → "" (caller renders as UTC).
func ResolveUserTimezone(channelTZ, workspaceDefault string) string {
	for _, tz := range []string{channelTZ, workspaceDefault} {
		if tz == "" {
			continue
		}
		if _, err := time.LoadLocation(tz); err == nil {
			return tz
		}
	}
	return ""
}
