package oa

import (
	"context"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/eventbus"
)

func (c *Channel) handleUserFollow(e *oaInboundEvent) {
	slog.Info("zalo_oa.webhook.follow_event",
		"event", "user_follow", "user_id", e.Sender.ID, "display_name", e.Sender.DisplayName)
	if e.Sender.ID == "" {
		return
	}
	if cc := c.ContactCollector(); cc != nil {
		cc.EnsureContact(context.Background(), c.Type(), c.Name(), e.Sender.ID, e.Sender.ID,
			e.Sender.DisplayName, "", "direct", "follower", "", "")
	}
	c.publishContactLifecycle(eventbus.EventContactFollowed, e.Sender.ID, e.Sender.DisplayName)
}

func (c *Channel) handleUserUnfollow(e *oaInboundEvent) {
	slog.Info("zalo_oa.webhook.follow_event",
		"event", "user_unfollow", "user_id", e.Sender.ID)
	if e.Sender.ID == "" {
		return
	}
	c.publishContactLifecycle(eventbus.EventContactUnfollowed, e.Sender.ID, e.Sender.DisplayName)
}

func (c *Channel) publishContactLifecycle(evType eventbus.EventType, senderID, displayName string) {
	if c.teamReplyBus == nil {
		return
	}
	now := time.Now().UTC()
	c.teamReplyBus.Publish(eventbus.DomainEvent{
		ID:        uuid.NewString(),
		Type:      evType,
		SourceID:  eventbus.ContactLifecycleSourceID(evType, c.instanceID.String(), senderID),
		TenantID:  c.TenantID().String(),
		Timestamp: now,
		Payload: eventbus.ContactLifecyclePayload{
			TenantID:          c.TenantID().String(),
			ChannelType:       c.Type(),
			ChannelInstanceID: c.instanceID.String(),
			ChannelName:       c.Name(),
			SenderID:          senderID,
			DisplayName:       displayName,
			Timestamp:         now,
		},
	})
}
