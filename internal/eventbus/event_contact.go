package eventbus

import "time"

const (
	EventContactFollowed   EventType = "contact.followed"
	EventContactUnfollowed EventType = "contact.unfollowed"
)

// ContactLifecyclePayload carries follow/unfollow context for downstream
// workers (welcome-agent dispatch, analytics, churn tracking).
type ContactLifecyclePayload struct {
	TenantID          string
	ChannelType       string
	ChannelInstanceID string
	ChannelName       string
	SenderID          string
	DisplayName       string
	Timestamp         time.Time
}

// ContactLifecycleSourceID is the canonical dedup key — bus suppresses
// duplicate follow events within the 5-min TTL window.
func ContactLifecycleSourceID(evType EventType, channelInstanceID, senderID string) string {
	return string(evType) + ":" + channelInstanceID + ":" + senderID
}
