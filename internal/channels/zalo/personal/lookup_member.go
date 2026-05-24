package personal

import (
	"context"
	"time"
)

const memberFetchTimeout = 3 * time.Second

// LookupGroupMember resolves uid → display name: recent posters → cache →
// rate-limited slow fetch. ok=false leaves the marker literal.
func (c *Channel) LookupGroupMember(ctx context.Context, threadID, uid string) (string, bool) {
	if resolver := c.groupNameResolver(threadID); resolver != nil {
		if name := resolver(uid); name != "" {
			return name, true
		}
	}
	if name, hit := c.memberCache.Lookup(threadID, uid); hit {
		return name, true
	}
	if !c.memberFetchLimiter.Allow(threadID) {
		return "", false
	}
	sess := c.session()
	if sess == nil || c.memberFetcher == nil {
		return "", false
	}
	fetchCtx, cancel := context.WithTimeout(ctx, memberFetchTimeout)
	defer cancel()
	members, err := c.memberFetcher(fetchCtx, sess, threadID)
	if err != nil {
		return "", false
	}
	for _, m := range members {
		c.memberCache.Set(threadID, m.UID, m.DisplayName)
	}
	if name, hit := c.memberCache.Lookup(threadID, uid); hit {
		return name, true
	}
	return "", false
}
