package agent

import (
	"context"
	"time"

	"github.com/nextlevelbuilder/goclaw/internal/bootstrap"
	"github.com/nextlevelbuilder/goclaw/internal/channels"
	"github.com/nextlevelbuilder/goclaw/internal/store"
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

// userTimezone picks the IANA timezone for the current turn. Called once in
// injectContext and stashed on RunContext so the per-iteration buildMessages
// path stays DB-free.
func userTimezone(meta *bootstrap.ChannelMeta, workspaceDefault string) string {
	if meta == nil {
		return ResolveUserTimezone("", workspaceDefault)
	}
	return ResolveUserTimezone(meta.ChannelTimezone, workspaceDefault)
}

// turnTimezone picks the IANA timezone for this run. A request-level override
// wins over channel/default config so scheduled jobs can render prompt times in
// their cron timezone even when the delivery channel has no timezone metadata.
func turnTimezone(override string, meta *bootstrap.ChannelMeta, workspaceDefault string) string {
	if override != "" {
		if _, err := time.LoadLocation(override); err == nil {
			return override
		}
	}
	return userTimezone(meta, workspaceDefault)
}

// userTimezoneFromCtx reads the per-turn resolved timezone off RunContext.
// Returns "" if RunContext is absent (subagent paths, tests) — buildTimeSection
// then falls back to UTC.
func userTimezoneFromCtx(ctx context.Context) string {
	if rc := store.RunContextFromCtx(ctx); rc != nil {
		return rc.UserTimezone
	}
	return ""
}
