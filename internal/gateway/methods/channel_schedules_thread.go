package methods

import (
	"context"
	"log/slog"
	"time"

	"github.com/nextlevelbuilder/goclaw/internal/channels/schedule"
	"github.com/nextlevelbuilder/goclaw/internal/gateway"
	"github.com/nextlevelbuilder/goclaw/internal/i18n"
	"github.com/nextlevelbuilder/goclaw/internal/store"
	"github.com/nextlevelbuilder/goclaw/pkg/protocol"
)

func (m *ChannelSchedulesMethods) handleThreadList(ctx context.Context, client *gateway.Client, req *protocol.RequestFrame) {
	var p struct {
		ChannelInstanceID string `json:"channel_instance_id"`
	}
	decode(req, &p)
	inst := m.resolveInstance(ctx, client, req, p.ChannelInstanceID)
	if inst == nil {
		return
	}
	list, err := m.store.ListThreadSchedules(ctx, inst.ID.String())
	if err != nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInternal, err.Error()))
		return
	}
	out := make([]map[string]any, 0, len(list))
	for _, t := range list {
		out = append(out, serializeThreadSchedule(t))
	}
	client.SendResponse(protocol.NewOKResponse(req.ID, map[string]any{"threads": out}))
}

func (m *ChannelSchedulesMethods) handleThreadGet(ctx context.Context, client *gateway.Client, req *protocol.RequestFrame) {
	var p struct {
		ChannelInstanceID string `json:"channel_instance_id"`
		ThreadKey         string `json:"thread_key"`
	}
	decode(req, &p)
	inst := m.resolveInstance(ctx, client, req, p.ChannelInstanceID)
	if inst == nil {
		return
	}
	t, err := m.store.GetThreadSchedule(ctx, inst.ID.String(), p.ThreadKey)
	if err != nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInternal, err.Error()))
		return
	}
	if t == nil {
		client.SendResponse(protocol.NewOKResponse(req.ID, map[string]any{"thread": nil}))
		return
	}
	client.SendResponse(protocol.NewOKResponse(req.ID, map[string]any{"thread": serializeThreadSchedule(*t)}))
}

func (m *ChannelSchedulesMethods) handleThreadSet(ctx context.Context, client *gateway.Client, req *protocol.RequestFrame) {
	var p struct {
		ChannelInstanceID string             `json:"channel_instance_id"`
		ThreadKey         string             `json:"thread_key"`
		Schedule          *schedule.Schedule `json:"schedule"`
		ExpiresAt         *time.Time         `json:"expires_at"`
		Reason            string             `json:"reason"`
	}
	decode(req, &p)
	if !m.requireAdmin(ctx, client, req) {
		return
	}
	inst := m.resolveInstance(ctx, client, req, p.ChannelInstanceID)
	if inst == nil {
		return
	}
	locale := store.LocaleFromContext(ctx)
	if p.ThreadKey == "" || p.Schedule == nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgRequired, "thread_key and schedule")))
		return
	}
	if err := schedule.Validate(p.Schedule); err != nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, i18n.T(locale, i18n.StandbyRPCInvalidSchedule, err.Error())))
		return
	}
	if err := m.store.SetThreadSchedule(ctx, store.ThreadSchedule{
		ChannelInstanceID: inst.ID.String(),
		ThreadKey:         p.ThreadKey,
		Schedule:          p.Schedule,
		ExpiresAt:         p.ExpiresAt,
		Reason:            p.Reason,
		CreatedBy:         client.UserID(),
	}); err != nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInternal, err.Error()))
		return
	}
	m.reload(inst.ID.String())
	slog.Info("standby.thread_schedule_set", "instance_id", inst.ID, "thread_key", p.ThreadKey, "reason", p.Reason)
	client.SendResponse(protocol.NewOKResponse(req.ID, map[string]any{"ok": true}))
}

func (m *ChannelSchedulesMethods) handleThreadDelete(ctx context.Context, client *gateway.Client, req *protocol.RequestFrame) {
	var p struct {
		ChannelInstanceID string `json:"channel_instance_id"`
		ThreadKey         string `json:"thread_key"`
	}
	decode(req, &p)
	if !m.requireAdmin(ctx, client, req) {
		return
	}
	inst := m.resolveInstance(ctx, client, req, p.ChannelInstanceID)
	if inst == nil {
		return
	}
	if err := m.store.DeleteThreadSchedule(ctx, inst.ID.String(), p.ThreadKey); err != nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInternal, err.Error()))
		return
	}
	m.reload(inst.ID.String())
	client.SendResponse(protocol.NewOKResponse(req.ID, map[string]any{"ok": true}))
}

func serializeThreadSchedule(t store.ThreadSchedule) map[string]any {
	m := map[string]any{
		"channel_instance_id": t.ChannelInstanceID,
		"thread_key":          t.ThreadKey,
		"schedule":            t.Schedule,
		"reason":              t.Reason,
		"created_by":          t.CreatedBy,
		"created_at":          t.CreatedAt,
		"updated_at":          t.UpdatedAt,
	}
	if t.ExpiresAt != nil {
		m["expires_at"] = *t.ExpiresAt
	}
	return m
}
