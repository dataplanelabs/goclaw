package oa

import "context"

// upsertSenderContact records the inbound sender's display name in the
// contacts store. 30-min in-memory dedup keeps this safe to call on
// every message. No-op when the channel was constructed without a
// ContactCollector.
func (c *Channel) upsertSenderContact(senderID, displayName string) {
	cc := c.ContactCollector()
	if cc == nil || senderID == "" {
		return
	}
	cc.EnsureContact(context.Background(), c.Type(), c.Name(), senderID, senderID, displayName, "", "direct", "user", "", "")
}
