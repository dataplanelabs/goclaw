package agent

import (
	"context"
	"time"

	"github.com/nextlevelbuilder/goclaw/internal/channels"
)

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

// resolveChannelTimezone looks up the channel-instance row by name and
// extracts its config.timezone. Returns "" on any miss (no store, no name,
// row not found, key absent) so callers can fall through to the workspace
// default via ResolveUserTimezone.
func (l *Loop) resolveChannelTimezone(ctx context.Context, channelName string) string {
	if channelName == "" || l.channelInstances == nil {
		return ""
	}
	inst, err := l.channelInstances.GetByName(ctx, channelName)
	if err != nil || inst == nil {
		return ""
	}
	return channels.TimezoneFromConfig(inst.Config)
}
