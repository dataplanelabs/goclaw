package schedule

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"time"
)

// ScheduleSource is the lookup contract the registry needs. Caller (gateway
// startup) wraps a store.ChannelScheduleStore to satisfy this. Keeps the
// schedule pkg free of an import on store.
type ScheduleSource struct {
	ResolveInstanceID  func(ctx context.Context, tenantID, channelName string) (string, error)
	LoadInstance       func(ctx context.Context, channelInstanceID string) (*Schedule, error)
	LoadThreadOverride func(ctx context.Context, channelInstanceID, threadKey string) (sc *Schedule, expiresAt *time.Time, err error)
}

// ScheduleRegistry resolves the effective Mode for a (tenant, channel, thread)
// triplet with a 2-tier in-memory cache (TTL refresh + push-reload).
type ScheduleRegistry struct {
	src ScheduleSource
	ttl time.Duration

	mu            sync.RWMutex
	instanceIDs   map[string]instanceIDEntry // tenantID|channelName → instanceID
	instanceCache map[string]resolverEntry   // instanceID → instance schedule resolver
	threadCache   map[string]resolverEntry   // instanceID|threadKey → thread override resolver
}

type instanceIDEntry struct {
	id    string
	until time.Time
}

type resolverEntry struct {
	r     *Resolver // nil = "row exists but no usable schedule"
	until time.Time
}

func NewRegistry(src ScheduleSource, ttl time.Duration) *ScheduleRegistry {
	return &ScheduleRegistry{
		src:           src,
		ttl:           ttl,
		instanceIDs:   make(map[string]instanceIDEntry),
		instanceCache: make(map[string]resolverEntry),
		threadCache:   make(map[string]resolverEntry),
	}
}

func (r *ScheduleRegistry) ResolveMode(ctx context.Context, tenantID, channelName, threadKey string, now time.Time) Mode {
	if r == nil || tenantID == "" || channelName == "" {
		return ModeActive
	}
	instID := r.lookupInstanceID(ctx, tenantID, channelName, now)
	if instID == "" {
		return ModeActive
	}
	if threadKey != "" {
		if res := r.lookupThreadResolver(ctx, instID, threadKey, now); res != nil {
			return res.ModeAt(now)
		}
	}
	if res := r.lookupInstanceResolver(ctx, instID, now); res != nil {
		return res.ModeAt(now)
	}
	return ModeActive
}

// Reload clears all cached entries for the given channel_instance_id — call
// after schedule writes so the editing pod sees changes immediately.
func (r *ScheduleRegistry) Reload(channelInstanceID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.instanceCache, channelInstanceID)
	prefix := channelInstanceID + "|"
	for k := range r.threadCache {
		if strings.HasPrefix(k, prefix) {
			delete(r.threadCache, k)
		}
	}
}

// InvalidateInstance clears the name→id cache row for the given
// (tenant, channelName). Call from channel_instances rename/delete handlers
// (rename: invalidate both old and new names).
func (r *ScheduleRegistry) InvalidateInstance(tenantID, channelName string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.instanceIDs, tenantID+"|"+channelName)
}

func (r *ScheduleRegistry) lookupInstanceID(ctx context.Context, tenantID, channelName string, now time.Time) string {
	key := tenantID + "|" + channelName
	r.mu.RLock()
	if e, ok := r.instanceIDs[key]; ok && now.Before(e.until) {
		r.mu.RUnlock()
		return e.id
	}
	r.mu.RUnlock()
	id, err := r.src.ResolveInstanceID(ctx, tenantID, channelName)
	if err != nil {
		slog.Warn("schedule.registry: resolve instance id failed",
			"tenant", tenantID, "channel", channelName, "err", err)
		return ""
	}
	r.mu.Lock()
	r.instanceIDs[key] = instanceIDEntry{id: id, until: now.Add(r.ttl)}
	r.mu.Unlock()
	return id
}

func (r *ScheduleRegistry) lookupInstanceResolver(ctx context.Context, instID string, now time.Time) *Resolver {
	r.mu.RLock()
	if e, ok := r.instanceCache[instID]; ok && now.Before(e.until) {
		r.mu.RUnlock()
		return e.r
	}
	r.mu.RUnlock()
	sc, err := r.src.LoadInstance(ctx, instID)
	if err != nil {
		slog.Warn("schedule.registry: load instance schedule failed", "instance_id", instID, "err", err)
		return nil
	}
	var res *Resolver
	if sc != nil {
		built, berr := NewResolver(sc)
		if berr != nil {
			slog.Warn("schedule.registry: build resolver failed", "instance_id", instID, "err", berr)
		} else {
			res = built
		}
	}
	r.mu.Lock()
	r.instanceCache[instID] = resolverEntry{r: res, until: now.Add(r.ttl)}
	r.mu.Unlock()
	return res
}

func (r *ScheduleRegistry) lookupThreadResolver(ctx context.Context, instID, threadKey string, now time.Time) *Resolver {
	key := instID + "|" + threadKey
	r.mu.RLock()
	if e, ok := r.threadCache[key]; ok && now.Before(e.until) {
		r.mu.RUnlock()
		return e.r
	}
	r.mu.RUnlock()
	sc, expires, err := r.src.LoadThreadOverride(ctx, instID, threadKey)
	if err != nil {
		slog.Warn("schedule.registry: load thread schedule failed",
			"instance_id", instID, "thread_key", threadKey, "err", err)
		return nil
	}
	var res *Resolver
	if sc != nil && (expires == nil || expires.After(now)) {
		built, berr := NewResolver(sc)
		if berr != nil {
			slog.Warn("schedule.registry: build thread resolver failed",
				"instance_id", instID, "thread_key", threadKey, "err", berr)
		} else {
			res = built
		}
	}
	// Cap TTL at expires_at so a one-shot pause doesn't keep replying as standby
	// past expiry (review M3: cached resolver outlived expires_at by up to ttl).
	until := now.Add(r.ttl)
	if expires != nil && expires.Before(until) {
		until = *expires
	}
	r.mu.Lock()
	r.threadCache[key] = resolverEntry{r: res, until: until}
	r.mu.Unlock()
	return res
}
