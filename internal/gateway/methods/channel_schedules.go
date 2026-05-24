package methods

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/channels/schedule"
	"github.com/nextlevelbuilder/goclaw/internal/gateway"
	"github.com/nextlevelbuilder/goclaw/internal/i18n"
	"github.com/nextlevelbuilder/goclaw/internal/permissions"
	"github.com/nextlevelbuilder/goclaw/internal/store"
	"github.com/nextlevelbuilder/goclaw/pkg/protocol"
)

// ChannelSchedulesMethods exposes standby schedule CRUD via WS RPC.
// Writes require RoleAdmin within the caller's tenant; reads require tenant
// membership. Cross-tenant access returns "not found" (no info leak).
type ChannelSchedulesMethods struct {
	store          store.ChannelScheduleStore
	instanceStore  store.ChannelInstanceStore
	registryReload func(channelInstanceID string)
}

func NewChannelSchedulesMethods(s store.ChannelScheduleStore, instances store.ChannelInstanceStore, reload func(string)) *ChannelSchedulesMethods {
	return &ChannelSchedulesMethods{store: s, instanceStore: instances, registryReload: reload}
}

func (m *ChannelSchedulesMethods) Register(router *gateway.MethodRouter) {
	router.Register(protocol.MethodChannelsScheduleGet, m.handleGet)
	router.Register(protocol.MethodChannelsScheduleSet, m.handleSet)
	router.Register(protocol.MethodChannelsScheduleDelete, m.handleDelete)
	router.Register(protocol.MethodChannelsThreadScheduleList, m.handleThreadList)
	router.Register(protocol.MethodChannelsThreadScheduleGet, m.handleThreadGet)
	router.Register(protocol.MethodChannelsThreadScheduleSet, m.handleThreadSet)
	router.Register(protocol.MethodChannelsThreadScheduleDelete, m.handleThreadDelete)
}

func (m *ChannelSchedulesMethods) reload(id string) {
	if m.registryReload != nil {
		m.registryReload(id)
	}
}

// resolveInstance parses + tenant-checks. Sends an error response and returns
// nil on any failure (cross-tenant treated as not-found).
func (m *ChannelSchedulesMethods) resolveInstance(ctx context.Context, client *gateway.Client, req *protocol.RequestFrame, raw string) *store.ChannelInstanceData {
	locale := store.LocaleFromContext(ctx)
	id, err := uuid.Parse(raw)
	if err != nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgInvalidID, "instance")))
		return nil
	}
	inst, err := m.instanceStore.Get(ctx, id)
	if err != nil || inst == nil || inst.TenantID != client.TenantID() {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrNotFound, i18n.T(locale, i18n.MsgInstanceNotFound)))
		return nil
	}
	return inst
}

func (m *ChannelSchedulesMethods) requireAdmin(ctx context.Context, client *gateway.Client, req *protocol.RequestFrame) bool {
	if permissions.HasMinRole(client.Role(), permissions.RoleAdmin) {
		return true
	}
	client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrUnauthorized, i18n.T(store.LocaleFromContext(ctx), i18n.StandbyRPCNoPermission)))
	return false
}

func decode(req *protocol.RequestFrame, dst any) {
	if req.Params != nil {
		_ = json.Unmarshal(req.Params, dst)
	}
}

func (m *ChannelSchedulesMethods) handleGet(ctx context.Context, client *gateway.Client, req *protocol.RequestFrame) {
	var p struct {
		ChannelInstanceID string `json:"channel_instance_id"`
	}
	decode(req, &p)
	inst := m.resolveInstance(ctx, client, req, p.ChannelInstanceID)
	if inst == nil {
		return
	}
	sc, err := m.store.GetInstanceSchedule(ctx, inst.ID.String())
	if err != nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInternal, err.Error()))
		return
	}
	client.SendResponse(protocol.NewOKResponse(req.ID, map[string]any{"schedule": sc}))
}

func (m *ChannelSchedulesMethods) handleSet(ctx context.Context, client *gateway.Client, req *protocol.RequestFrame) {
	var p struct {
		ChannelInstanceID string             `json:"channel_instance_id"`
		Schedule          *schedule.Schedule `json:"schedule"`
	}
	decode(req, &p)
	if !m.requireAdmin(ctx, client, req) {
		return
	}
	inst := m.resolveInstance(ctx, client, req, p.ChannelInstanceID)
	if inst == nil {
		return
	}
	if err := schedule.Validate(p.Schedule); err != nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, i18n.T(store.LocaleFromContext(ctx), i18n.StandbyRPCInvalidSchedule, err.Error())))
		return
	}
	if err := m.store.SetInstanceSchedule(ctx, inst.ID.String(), p.Schedule); err != nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInternal, err.Error()))
		return
	}
	m.reload(inst.ID.String())
	client.SendResponse(protocol.NewOKResponse(req.ID, map[string]any{"ok": true, "schedule": p.Schedule}))
}

func (m *ChannelSchedulesMethods) handleDelete(ctx context.Context, client *gateway.Client, req *protocol.RequestFrame) {
	var p struct {
		ChannelInstanceID string `json:"channel_instance_id"`
	}
	decode(req, &p)
	if !m.requireAdmin(ctx, client, req) {
		return
	}
	inst := m.resolveInstance(ctx, client, req, p.ChannelInstanceID)
	if inst == nil {
		return
	}
	if err := m.store.DeleteInstanceSchedule(ctx, inst.ID.String()); err != nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInternal, err.Error()))
		return
	}
	m.reload(inst.ID.String())
	client.SendResponse(protocol.NewOKResponse(req.ID, map[string]any{"ok": true}))
}

// Thread-level handlers + serializer live in channel_schedules_thread.go.
