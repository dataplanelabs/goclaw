package personal

import (
	"context"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/sync/singleflight"

	"github.com/nextlevelbuilder/goclaw/internal/channels/zalo/personal/protocol"
	"github.com/nextlevelbuilder/goclaw/internal/store"
)

const (
	groupBootstrapDebounce = 5 * time.Minute
	groupCacheTTL          = time.Hour
	groupFetchTimeout      = 30 * time.Second
)

// nowFn is the clock used for debounce + cache TTL; tests may override.
var nowFn = time.Now

// groupCache tracks recently-upserted group IDs; bounded by groups the bot
// belongs to (small — no eviction). sf dedups concurrent lazy fetches.
type groupCache struct {
	m  sync.Map // groupID -> time.Time of last upsert
	sf singleflight.Group
}

func newGroupCache() *groupCache { return &groupCache{} }

func (g *groupCache) mark(groupID string) {
	g.m.Store(groupID, nowFn())
}

func (g *groupCache) fresh(groupID string) bool {
	v, ok := g.m.Load(groupID)
	if !ok {
		return false
	}
	ts, ok := v.(time.Time)
	if !ok {
		return false
	}
	return nowFn().Sub(ts) < groupCacheTTL
}

// shouldBootstrap returns true if the last bootstrap was longer than
// groupBootstrapDebounce ago and atomically claims the slot. Reconnect storms
// only fire FetchGroups once per debounce window.
func shouldBootstrap(lastNS *atomic.Int64) bool {
	prev := lastNS.Load()
	now := nowFn().UnixNano()
	if prev != 0 && time.Duration(now-prev) < groupBootstrapDebounce {
		return false
	}
	return lastNS.CompareAndSwap(prev, now)
}

// upsertGroupContact writes a single group's display_name into channel_contacts.
// Avatar + member-count intentionally NOT persisted — EnsureContact surface
// lacks those fields (tracked as follow-up).
func upsertGroupContact(ctx context.Context, cc *store.ContactCollector, cache *groupCache, channelType, channelInstance string, group protocol.GroupListInfo) {
	if cc == nil || group.GroupID == "" {
		return
	}
	cc.EnsureContact(ctx, channelType, channelInstance, group.GroupID, group.GroupID, group.Name, "", "group", "group", "", "")
	cache.mark(group.GroupID)
}

// bootstrapGroups fetches every group the bot belongs to and upserts each as a
// contact_type='group' row. Best-effort: errors logged, never returned.
func bootstrapGroups(ctx context.Context, sess *protocol.Session, cc *store.ContactCollector, cache *groupCache, channelType, channelInstance string) {
	if sess == nil || cc == nil {
		return
	}
	fctx, cancel := context.WithTimeout(ctx, groupFetchTimeout)
	defer cancel()

	groups, err := protocol.FetchGroups(fctx, sess)
	if err != nil {
		slog.Warn("zalo_personal: bootstrap groups failed", "error", err)
		return
	}
	upCtx := context.WithoutCancel(ctx)
	for _, g := range groups {
		upsertGroupContact(upCtx, cc, cache, channelType, channelInstance, g)
	}
	slog.Info("zalo_personal: bootstrapped group contacts", "count", len(groups))
}

// ensureGroupKnown is a non-blocking forward-fix for groups joined mid-session
// that the last bootstrap missed. Cache-hit returns immediately; cache-miss
// fires a goroutine that fetches + upserts via singleflight (one in-flight
// call per groupID even under concurrent message bursts).
func ensureGroupKnown(ctx context.Context, sess *protocol.Session, cc *store.ContactCollector, cache *groupCache, channelType, channelInstance, groupID string) {
	if sess == nil || cc == nil || groupID == "" {
		return
	}
	if cache.fresh(groupID) {
		return
	}
	go func() {
		_, _, _ = cache.sf.Do(groupID, func() (any, error) {
			fctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), groupFetchTimeout)
			defer cancel()
			groups, err := protocol.FetchGroups(fctx, sess)
			if err != nil {
				slog.Warn("zalo_personal: lazy fetch groups failed", "group_id", groupID, "error", err)
				return nil, nil
			}
			for _, g := range groups {
				if g.GroupID == groupID {
					upsertGroupContact(context.WithoutCancel(ctx), cc, cache, channelType, channelInstance, g)
					slog.Info("zalo_personal: lazy-upserted group contact", "group_id", groupID, "name", g.Name)
					return nil, nil
				}
			}
			slog.Warn("zalo_personal: lazy fetch — group not in FetchGroups result", "group_id", groupID)
			return nil, nil
		})
	}()
}
