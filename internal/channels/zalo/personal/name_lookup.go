package personal

import (
	"context"
	"log/slog"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// normalizeName: NFC + casefold + trim. NEVER diacritic-strip.
func normalizeName(s string) string {
	s = norm.NFC.String(s)
	s = strings.ToLower(s)
	return strings.TrimFunc(s, unicode.IsSpace)
}

// LookupGroupMemberByName resolves a display-name marker to a UID. Cache scan
// first; if zero matches, forces one rate-limited slow fetch and re-scans.
// Refuses on ambiguity (≥2 matches).
func (c *Channel) LookupGroupMemberByName(ctx context.Context, threadID, name string) (string, string, bool) {
	needle := normalizeName(name)
	if needle == "" {
		return "", "", false
	}
	if uid, dn, ok := c.memberCache.FindByName(threadID, needle); ok {
		return uid, dn, true
	}
	if !c.memberFetchLimiter.Allow(threadID) {
		return "", "", false
	}
	sess := c.session()
	if sess == nil || c.memberFetcher == nil {
		return "", "", false
	}
	fetchCtx, cancel := context.WithTimeout(ctx, memberFetchTimeout)
	defer cancel()
	members, err := c.memberFetcher(fetchCtx, sess, threadID)
	if err != nil {
		return "", "", false
	}
	for _, m := range members {
		c.memberCache.Set(threadID, m.UID, m.DisplayName)
	}
	uid, dn, ok := c.memberCache.FindByName(threadID, needle)
	if !ok {
		slog.Info("zalo_personal.mention.name_unresolved",
			"thread_id", threadID,
			"name", name,
			"hint", "cache miss after fetch; name not unique or not in group")
	}
	return uid, dn, ok
}
