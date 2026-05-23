package personal

import (
	"sync"
	"time"
)

type outboundEntry struct {
	preview   string
	expiresAt time.Time
}

type outboundCache struct {
	mu      sync.RWMutex
	entries map[string]outboundEntry
	ttl     time.Duration
}

func newOutboundCache(ttl time.Duration) *outboundCache {
	return &outboundCache{entries: make(map[string]outboundEntry), ttl: ttl}
}

func (c *outboundCache) set(msgID, preview string) {
	if msgID == "" || preview == "" {
		return
	}
	c.mu.Lock()
	c.entries[msgID] = outboundEntry{preview: preview, expiresAt: time.Now().Add(c.ttl)}
	c.sweepLocked()
	c.mu.Unlock()
}

func (c *outboundCache) get(msgID string) string {
	c.mu.RLock()
	e, ok := c.entries[msgID]
	c.mu.RUnlock()
	if !ok || time.Now().After(e.expiresAt) {
		return ""
	}
	return e.preview
}

func (c *outboundCache) sweepLocked() {
	now := time.Now()
	for k, e := range c.entries {
		if now.After(e.expiresAt) {
			delete(c.entries, k)
		}
	}
}

func previewText(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "…"
}
