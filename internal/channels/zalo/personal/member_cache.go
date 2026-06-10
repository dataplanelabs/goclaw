package personal

import (
	"sync"
	"time"
)

type MemberCache struct {
	mu      sync.RWMutex
	byGroup map[string]map[string]string
}

func NewMemberCache() *MemberCache {
	return &MemberCache{byGroup: make(map[string]map[string]string)}
}

func (mc *MemberCache) Lookup(threadID, uid string) (string, bool) {
	if mc == nil {
		return "", false
	}
	mc.mu.RLock()
	defer mc.mu.RUnlock()
	if g, ok := mc.byGroup[threadID]; ok {
		name, hit := g[uid]
		return name, hit
	}
	return "", false
}

func (mc *MemberCache) Set(threadID, uid, displayName string) {
	if mc == nil || uid == "" || displayName == "" {
		return
	}
	mc.mu.Lock()
	defer mc.mu.Unlock()
	g, ok := mc.byGroup[threadID]
	if !ok {
		g = make(map[string]string)
		mc.byGroup[threadID] = g
	}
	g[uid] = displayName
}

// FindByName scans the per-group cache for entries whose normalized display
// name matches needle (already normalized by caller). Returns uid+name on
// unique match. Returns ok=false on zero or ≥2 matches (refuse on ambiguity).
func (mc *MemberCache) FindByName(threadID, needle string) (string, string, bool) {
	if mc == nil || needle == "" {
		return "", "", false
	}
	mc.mu.RLock()
	defer mc.mu.RUnlock()
	g, ok := mc.byGroup[threadID]
	if !ok {
		return "", "", false
	}
	var foundUID, foundName string
	hits := 0
	for uid, dn := range g {
		if normalizeName(dn) == needle {
			foundUID = uid
			foundName = dn
			hits++
			if hits > 1 {
				return "", "", false
			}
		}
	}
	if hits == 1 {
		return foundUID, foundName, true
	}
	return "", "", false
}

type MemberFetchLimiter struct {
	window time.Duration
	mu     sync.Mutex
	last   map[string]time.Time
}

func NewMemberFetchLimiter(window time.Duration) *MemberFetchLimiter {
	return &MemberFetchLimiter{window: window, last: make(map[string]time.Time)}
}

func (l *MemberFetchLimiter) Allow(key string) bool {
	if l == nil {
		return true
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	if prev, ok := l.last[key]; ok && now.Sub(prev) < l.window {
		return false
	}
	l.last[key] = now
	return true
}
