package methods

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/channels/schedule"
	"github.com/nextlevelbuilder/goclaw/internal/gateway"
	"github.com/nextlevelbuilder/goclaw/internal/permissions"
	"github.com/nextlevelbuilder/goclaw/internal/store"
	"github.com/nextlevelbuilder/goclaw/pkg/protocol"
)

// --- stubs ---

type stubScheduleStore struct {
	resolveResp  string
	getInstResp  *schedule.Schedule
	setInstCalls int
	delInstCalls int
	setThrCalls  int
	delThrCalls  int
}

func (s *stubScheduleStore) ResolveInstanceIDByName(_ context.Context, _, _ string) (string, error) {
	return s.resolveResp, nil
}
func (s *stubScheduleStore) GetInstanceSchedule(_ context.Context, _ string) (*schedule.Schedule, error) {
	return s.getInstResp, nil
}
func (s *stubScheduleStore) SetInstanceSchedule(_ context.Context, _ string, sc *schedule.Schedule) error {
	s.setInstCalls++
	return nil
}
func (s *stubScheduleStore) DeleteInstanceSchedule(_ context.Context, _ string) error {
	s.delInstCalls++
	return nil
}
func (s *stubScheduleStore) ListThreadSchedules(_ context.Context, _ string) ([]store.ThreadSchedule, error) {
	return nil, nil
}
func (s *stubScheduleStore) GetThreadSchedule(_ context.Context, _, _ string) (*store.ThreadSchedule, error) {
	return nil, nil
}
func (s *stubScheduleStore) SetThreadSchedule(_ context.Context, _ store.ThreadSchedule) error {
	s.setThrCalls++
	return nil
}
func (s *stubScheduleStore) DeleteThreadSchedule(_ context.Context, _, _ string) error {
	s.delThrCalls++
	return nil
}
func (s *stubScheduleStore) PurgeExpiredThreadSchedules(_ context.Context, _ time.Time) (int64, error) {
	return 0, nil
}

type stubInstanceStore struct {
	byID map[uuid.UUID]*store.ChannelInstanceData
}

func (s *stubInstanceStore) Create(_ context.Context, _ *store.ChannelInstanceData) error { return nil }
func (s *stubInstanceStore) Get(_ context.Context, id uuid.UUID) (*store.ChannelInstanceData, error) {
	return s.byID[id], nil
}
func (s *stubInstanceStore) GetByName(_ context.Context, _ string) (*store.ChannelInstanceData, error) {
	return nil, nil
}
func (s *stubInstanceStore) Update(_ context.Context, _ uuid.UUID, _ map[string]any) error {
	return nil
}
func (s *stubInstanceStore) MergeConfig(_ context.Context, _ uuid.UUID, _ map[string]any) error {
	return nil
}
func (s *stubInstanceStore) Delete(_ context.Context, _ uuid.UUID) error { return nil }
func (s *stubInstanceStore) ListEnabled(_ context.Context) ([]store.ChannelInstanceData, error) {
	return nil, nil
}
func (s *stubInstanceStore) ListAll(_ context.Context) ([]store.ChannelInstanceData, error) {
	return nil, nil
}
func (s *stubInstanceStore) ListAllInstances(_ context.Context) ([]store.ChannelInstanceData, error) {
	return nil, nil
}
func (s *stubInstanceStore) ListAllEnabled(_ context.Context) ([]store.ChannelInstanceData, error) {
	return nil, nil
}
func (s *stubInstanceStore) ListPaged(_ context.Context, _ store.ChannelInstanceListOpts) ([]store.ChannelInstanceData, error) {
	return nil, nil
}
func (s *stubInstanceStore) CountInstances(_ context.Context, _ store.ChannelInstanceListOpts) (int, error) {
	return 0, nil
}

// --- helpers ---

func newSchedReq(t *testing.T, method, body string) *protocol.RequestFrame {
	t.Helper()
	return &protocol.RequestFrame{
		Type: protocol.FrameTypeRequest, ID: "r1", Method: method, Params: json.RawMessage(body),
	}
}

func setupScheduleHandlers(tenantID uuid.UUID, instID uuid.UUID) (*ChannelSchedulesMethods, *stubScheduleStore, *stubInstanceStore) {
	insts := &stubInstanceStore{byID: map[uuid.UUID]*store.ChannelInstanceData{
		instID: {BaseModel: store.BaseModel{ID: instID}, TenantID: tenantID, Name: "tg"},
	}}
	sch := &stubScheduleStore{}
	m := NewChannelSchedulesMethods(sch, insts, nil)
	return m, sch, insts
}

// --- tests ---

func TestSchedule_CrossTenantBlocked(t *testing.T) {
	callerTID := uuid.New()
	ownerTID := uuid.New()
	instID := uuid.New()
	m, sched, _ := setupScheduleHandlers(ownerTID, instID)

	client := gateway.NewTestClient(permissions.RoleAdmin, callerTID, "user-1")
	body := `{"channel_instance_id":"` + instID.String() + `","schedule":{"default_mode":"standby"}}`
	m.handleSet(wsCallCtx(client), client, newSchedReq(t, protocol.MethodChannelsScheduleSet, body))

	if sched.setInstCalls != 0 {
		t.Fatalf("cross-tenant set should not call store, got %d", sched.setInstCalls)
	}
}

func TestSchedule_NonAdminDenied(t *testing.T) {
	tenantID := uuid.New()
	instID := uuid.New()
	m, sched, _ := setupScheduleHandlers(tenantID, instID)

	client := gateway.NewTestClient(permissions.RoleOperator, tenantID, "user-1")
	body := `{"channel_instance_id":"` + instID.String() + `","schedule":{"default_mode":"standby"}}`
	m.handleSet(wsCallCtx(client), client, newSchedReq(t, protocol.MethodChannelsScheduleSet, body))

	if sched.setInstCalls != 0 {
		t.Fatalf("non-admin should not be able to set: got %d", sched.setInstCalls)
	}
}

func TestSchedule_InvalidScheduleRejected(t *testing.T) {
	tenantID := uuid.New()
	instID := uuid.New()
	m, sched, _ := setupScheduleHandlers(tenantID, instID)

	client := gateway.NewTestClient(permissions.RoleAdmin, tenantID, "user-1")
	// missing fields → empty window
	body := `{"channel_instance_id":"` + instID.String() + `","schedule":{"windows":[{"mode":"standby"}]}}`
	m.handleSet(wsCallCtx(client), client, newSchedReq(t, protocol.MethodChannelsScheduleSet, body))

	if sched.setInstCalls != 0 {
		t.Fatalf("invalid schedule should be rejected before store, got %d", sched.setInstCalls)
	}
}

func TestSchedule_AdminHappyPath(t *testing.T) {
	tenantID := uuid.New()
	instID := uuid.New()
	var reloaded string
	m, sched, _ := setupScheduleHandlers(tenantID, instID)
	m.registryReload = func(id string) { reloaded = id }

	client := gateway.NewTestClient(permissions.RoleAdmin, tenantID, "user-1")
	body := `{"channel_instance_id":"` + instID.String() + `","schedule":{"default_mode":"standby"}}`
	m.handleSet(wsCallCtx(client), client, newSchedReq(t, protocol.MethodChannelsScheduleSet, body))

	if sched.setInstCalls != 1 {
		t.Fatalf("expected 1 set call, got %d", sched.setInstCalls)
	}
	if reloaded != instID.String() {
		t.Fatalf("expected reload(%s), got %q", instID, reloaded)
	}
}

func TestSchedule_ThreadSetMissingFields(t *testing.T) {
	tenantID := uuid.New()
	instID := uuid.New()
	m, sched, _ := setupScheduleHandlers(tenantID, instID)

	client := gateway.NewTestClient(permissions.RoleAdmin, tenantID, "user-1")
	body := `{"channel_instance_id":"` + instID.String() + `"}`
	m.handleThreadSet(wsCallCtx(client), client, newSchedReq(t, protocol.MethodChannelsThreadScheduleSet, body))

	if sched.setThrCalls != 0 {
		t.Fatalf("missing fields should not invoke store, got %d", sched.setThrCalls)
	}
}
