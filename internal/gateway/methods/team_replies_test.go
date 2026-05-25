package methods

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/eventbus"
	"github.com/nextlevelbuilder/goclaw/internal/gateway"
	"github.com/nextlevelbuilder/goclaw/internal/permissions"
	"github.com/nextlevelbuilder/goclaw/internal/store"
	"github.com/nextlevelbuilder/goclaw/pkg/protocol"
)

type stubTeamEvalStore struct {
	rows        []store.TeamReplyEvaluation
	listCalls   int
	mergeCalls  int
	insertCalls int
}

func (s *stubTeamEvalStore) Insert(context.Context, store.TeamReplyEvaluation) (string, error) {
	s.insertCalls++
	return uuid.NewString(), nil
}
func (s *stubTeamEvalStore) UpdateJudgeVerdict(context.Context, string, string, float64, string, string, string, string, int) error {
	return nil
}
func (s *stubTeamEvalStore) MarkJudgeError(context.Context, string, string) error { return nil }
func (s *stubTeamEvalStore) List(_ context.Context, _ string, _ store.TeamReplyEvalFilter) ([]store.TeamReplyEvaluation, error) {
	s.listCalls++
	return s.rows, nil
}
func (s *stubTeamEvalStore) Count(_ context.Context, _ string, _ store.TeamReplyEvalFilter) (int64, error) {
	return int64(len(s.rows)), nil
}
func (s *stubTeamEvalStore) GetByMessageID(_ context.Context, _ string, msgID string) (*store.TeamReplyEvaluation, error) {
	for _, r := range s.rows {
		if r.TeamMsgID == msgID {
			return &r, nil
		}
	}
	return nil, nil
}
func (s *stubTeamEvalStore) ListFailedJudge(context.Context, string, int) ([]store.TeamReplyEvaluation, error) {
	return nil, nil
}
func (s *stubTeamEvalStore) ClearJudgeError(context.Context, []string) (int64, error) {
	return 0, nil
}
func (s *stubTeamEvalStore) ListPendingJudge(context.Context, int) ([]store.TeamReplyEvaluation, error) {
	return nil, nil
}
func (s *stubTeamEvalStore) DeleteByChannel(context.Context, string) (int64, error) { return 0, nil }

type stubInstStoreLite struct {
	insts map[uuid.UUID]*store.ChannelInstanceData
}

func (s *stubInstStoreLite) Create(context.Context, *store.ChannelInstanceData) error { return nil }
func (s *stubInstStoreLite) Get(_ context.Context, id uuid.UUID) (*store.ChannelInstanceData, error) {
	return s.insts[id], nil
}
func (s *stubInstStoreLite) GetByName(context.Context, string) (*store.ChannelInstanceData, error) {
	return nil, nil
}
func (s *stubInstStoreLite) Update(context.Context, uuid.UUID, map[string]any) error { return nil }
func (s *stubInstStoreLite) MergeConfig(_ context.Context, id uuid.UUID, partial map[string]any) error {
	if c, ok := s.insts[id]; ok && c != nil {
		// Persist the merged partial back into Config for visibility in tests.
		b, _ := json.Marshal(partial)
		c.Config = b
	}
	return nil
}
func (s *stubInstStoreLite) Delete(context.Context, uuid.UUID) error { return nil }
func (s *stubInstStoreLite) ListEnabled(context.Context) ([]store.ChannelInstanceData, error) {
	return nil, nil
}
func (s *stubInstStoreLite) ListAll(context.Context) ([]store.ChannelInstanceData, error) {
	return nil, nil
}
func (s *stubInstStoreLite) ListAllInstances(context.Context) ([]store.ChannelInstanceData, error) {
	return nil, nil
}
func (s *stubInstStoreLite) ListAllEnabled(context.Context) ([]store.ChannelInstanceData, error) {
	return nil, nil
}
func (s *stubInstStoreLite) ListPaged(context.Context, store.ChannelInstanceListOpts) ([]store.ChannelInstanceData, error) {
	return nil, nil
}
func (s *stubInstStoreLite) CountInstances(context.Context, store.ChannelInstanceListOpts) (int, error) {
	return 0, nil
}

type stubAgentStoreLite struct {
	byKey map[string]*store.AgentData
}

var _ store.AgentCRUDStore = (*stubAgentStoreLite)(nil)

func (s *stubAgentStoreLite) Create(context.Context, *store.AgentData) error { return nil }
func (s *stubAgentStoreLite) GetByKey(_ context.Context, key string) (*store.AgentData, error) {
	return s.byKey[key], nil
}
func (s *stubAgentStoreLite) GetByID(context.Context, uuid.UUID) (*store.AgentData, error) {
	return nil, nil
}
func (s *stubAgentStoreLite) GetByIDUnscoped(context.Context, uuid.UUID) (*store.AgentData, error) {
	return nil, nil
}
func (s *stubAgentStoreLite) GetByKeys(context.Context, []string) ([]store.AgentData, error) {
	return nil, nil
}
func (s *stubAgentStoreLite) GetByIDs(context.Context, []uuid.UUID) ([]store.AgentData, error) {
	return nil, nil
}
func (s *stubAgentStoreLite) Update(context.Context, uuid.UUID, map[string]any) error { return nil }
func (s *stubAgentStoreLite) Delete(context.Context, uuid.UUID) error                 { return nil }
func (s *stubAgentStoreLite) List(context.Context, string) ([]store.AgentData, error) {
	return nil, nil
}
func (s *stubAgentStoreLite) GetDefault(context.Context) (*store.AgentData, error) { return nil, nil }
func (s *stubAgentStoreLite) ResetStuckSummoning(context.Context) (int64, error)   { return 0, nil }

func setupTeamRepliesHandlers(tenantID, instID uuid.UUID) (*TeamRepliesMethods, *stubTeamEvalStore, *stubInstStoreLite) {
	insts := &stubInstStoreLite{insts: map[uuid.UUID]*store.ChannelInstanceData{
		instID: {BaseModel: store.BaseModel{ID: instID}, TenantID: tenantID, Name: "zalo-oa-test"},
	}}
	evals := &stubTeamEvalStore{}
	agents := &stubAgentStoreLite{byKey: map[string]*store.AgentData{}}
	return NewTeamRepliesMethods(evals, insts, agents, nil), evals, insts
}

func newTRReq(method, body string) *protocol.RequestFrame {
	return &protocol.RequestFrame{Type: protocol.FrameTypeRequest, ID: "r1", Method: method, Params: json.RawMessage(body)}
}

func TestTeamReplies_CrossTenantBlocked(t *testing.T) {
	callerTID := uuid.New()
	ownerTID := uuid.New()
	instID := uuid.New()
	m, ev, _ := setupTeamRepliesHandlers(ownerTID, instID)
	client := gateway.NewTestClient(permissions.RoleAdmin, callerTID, "user-1")
	body := `{"channel_instance_id":"` + instID.String() + `"}`
	m.handleList(wsCallCtx(client), client, newTRReq(protocol.MethodChannelsTeamRepliesList, body))
	if ev.listCalls != 0 {
		t.Fatalf("cross-tenant list should not reach store, got %d", ev.listCalls)
	}
}

func TestTeamReplies_ListInTenant(t *testing.T) {
	tenantID := uuid.New()
	instID := uuid.New()
	m, ev, _ := setupTeamRepliesHandlers(tenantID, instID)
	ev.rows = []store.TeamReplyEvaluation{
		{ID: "1", ChannelInstanceID: instID.String(), TenantID: tenantID.String(), TeamMsgID: "m1", TeamReply: "hi"},
	}
	client := gateway.NewTestClient(permissions.RoleViewer, tenantID, "user-1")
	body := `{"channel_instance_id":"` + instID.String() + `"}`
	m.handleList(wsCallCtx(client), client, newTRReq(protocol.MethodChannelsTeamRepliesList, body))
	if ev.listCalls != 1 {
		t.Fatalf("expected list call, got %d", ev.listCalls)
	}
}

func TestTeamReplies_ToggleNonAdminDenied(t *testing.T) {
	tenantID := uuid.New()
	instID := uuid.New()
	m, _, insts := setupTeamRepliesHandlers(tenantID, instID)
	client := gateway.NewTestClient(permissions.RoleOperator, tenantID, "user-1")
	body := `{"channel_instance_id":"` + instID.String() + `","capture_team_replies":true}`
	m.handleToggle(wsCallCtx(client), client, newTRReq(protocol.MethodChannelsTeamCaptureToggle, body))
	if len(insts.insts[instID].Config) != 0 {
		t.Fatalf("non-admin should not modify config, got %s", insts.insts[instID].Config)
	}
}

func TestTeamReplies_ToggleAdminHappyPath(t *testing.T) {
	tenantID := uuid.New()
	instID := uuid.New()
	m, _, insts := setupTeamRepliesHandlers(tenantID, instID)
	m.agents.(*stubAgentStoreLite).byKey["j1"] = &store.AgentData{BaseModel: store.BaseModel{ID: uuid.New()}, AgentKey: "j1"}
	client := gateway.NewTestClient(permissions.RoleAdmin, tenantID, "user-1")
	body := `{"channel_instance_id":"` + instID.String() + `","capture_team_replies":true,"judge_evaluation":true,"judge_agent_key":"j1"}`
	m.handleToggle(wsCallCtx(client), client, newTRReq(protocol.MethodChannelsTeamCaptureToggle, body))
	if len(insts.insts[instID].Config) == 0 {
		t.Fatal("expected config updated")
	}
	if !strings.Contains(string(insts.insts[instID].Config), `"capture_team_replies":true`) {
		t.Fatalf("missing key in merged config: %s", insts.insts[instID].Config)
	}
}

func TestTeamReplies_ToggleRejectsMissingJudgeKey(t *testing.T) {
	tenantID := uuid.New()
	instID := uuid.New()
	m, _, insts := setupTeamRepliesHandlers(tenantID, instID)
	client := gateway.NewTestClient(permissions.RoleAdmin, tenantID, "user-1")
	body := `{"channel_instance_id":"` + instID.String() + `","judge_evaluation":true}`
	m.handleToggle(wsCallCtx(client), client, newTRReq(protocol.MethodChannelsTeamCaptureToggle, body))
	if len(insts.insts[instID].Config) != 0 {
		t.Fatalf("config should not be updated when judge_key missing: %s", insts.insts[instID].Config)
	}
}

type stubFailedEvalStore struct {
	stubTeamEvalStore
	failedRows []store.TeamReplyEvaluation
	cleared    []string
}

func (s *stubFailedEvalStore) ListFailedJudge(_ context.Context, _ string, _ int) ([]store.TeamReplyEvaluation, error) {
	return s.failedRows, nil
}
func (s *stubFailedEvalStore) ClearJudgeError(_ context.Context, ids []string) (int64, error) {
	s.cleared = append(s.cleared, ids...)
	return int64(len(ids)), nil
}

func TestTeamReplies_RejudgeNonAdminDenied(t *testing.T) {
	tenantID := uuid.New()
	instID := uuid.New()
	m, ev, _ := setupTeamRepliesHandlers(tenantID, instID)
	client := gateway.NewTestClient(permissions.RoleOperator, tenantID, "user-1")
	body := `{"channel_instance_id":"` + instID.String() + `"}`
	m.handleRejudge(wsCallCtx(client), client, newTRReq(protocol.MethodChannelsTeamRepliesRejudge, body))
	if ev.listCalls != 0 {
		t.Fatal("non-admin should not reach store")
	}
}

func TestTeamReplies_RejudgeClearsErrorsAndPublishes(t *testing.T) {
	tenantID := uuid.New()
	instID := uuid.New()
	insts := &stubInstStoreLite{insts: map[uuid.UUID]*store.ChannelInstanceData{
		instID: {BaseModel: store.BaseModel{ID: instID}, TenantID: tenantID, Name: "zalo-oa-test"},
	}}
	failedEv := &stubFailedEvalStore{
		failedRows: []store.TeamReplyEvaluation{
			{ID: uuid.NewString(), ChannelInstanceID: instID.String(), TenantID: tenantID.String(), TeamMsgID: "f1"},
			{ID: uuid.NewString(), ChannelInstanceID: instID.String(), TenantID: tenantID.String(), TeamMsgID: "f2"},
		},
	}
	agents := &stubAgentStoreLite{byKey: map[string]*store.AgentData{}}
	publishedCount := 0
	bus := &fakeBusForRejudge{onPublish: func() { publishedCount++ }}
	m := NewTeamRepliesMethods(failedEv, insts, agents, bus)
	client := gateway.NewTestClient(permissions.RoleAdmin, tenantID, "user-1")
	body := `{"channel_instance_id":"` + instID.String() + `"}`
	m.handleRejudge(wsCallCtx(client), client, newTRReq(protocol.MethodChannelsTeamRepliesRejudge, body))
	if len(failedEv.cleared) != 2 {
		t.Fatalf("cleared = %v, want 2 ids", failedEv.cleared)
	}
	if publishedCount != 2 {
		t.Fatalf("published = %d, want 2", publishedCount)
	}
}

type fakeBusForRejudge struct {
	onPublish func()
}

func (f *fakeBusForRejudge) Publish(_ eventbus.DomainEvent) {
	if f.onPublish != nil {
		f.onPublish()
	}
}
func (f *fakeBusForRejudge) Subscribe(eventbus.EventType, eventbus.DomainEventHandler) func() {
	return func() {}
}
func (f *fakeBusForRejudge) Start(context.Context)         {}
func (f *fakeBusForRejudge) Drain(time.Duration) error     { return nil }

func TestTeamReplies_ToggleRejectsUnknownJudgeAgent(t *testing.T) {
	tenantID := uuid.New()
	instID := uuid.New()
	m, _, insts := setupTeamRepliesHandlers(tenantID, instID)
	client := gateway.NewTestClient(permissions.RoleAdmin, tenantID, "user-1")
	body := `{"channel_instance_id":"` + instID.String() + `","judge_evaluation":true,"judge_agent_key":"ghost"}`
	m.handleToggle(wsCallCtx(client), client, newTRReq(protocol.MethodChannelsTeamCaptureToggle, body))
	if len(insts.insts[instID].Config) != 0 {
		t.Fatalf("config should not be updated when judge agent not found: %s", insts.insts[instID].Config)
	}
}

func TestTeamReplies_ExportJSONLFormat(t *testing.T) {
	rows := []store.TeamReplyEvaluation{
		{TeamMsgID: "m1", CustomerMessage: "where?", TeamReply: "here"},
		{TeamMsgID: "m2", CustomerMessage: "", TeamReply: "ok"},
	}
	body, count, truncated, err := buildOpenAITrainingJSONL(rows, jsonlExportMaxBytes)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if truncated {
		t.Fatal("unexpected truncation on small fixture")
	}
	if count != 2 {
		t.Fatalf("count = %d", count)
	}
	lines := strings.Split(strings.TrimSpace(body), "\n")
	if len(lines) != 2 {
		t.Fatalf("lines = %d", len(lines))
	}
	for i, line := range lines {
		var obj struct {
			Messages []map[string]string `json:"messages"`
		}
		if err := json.Unmarshal([]byte(line), &obj); err != nil {
			t.Fatalf("line %d parse: %v", i, err)
		}
		if obj.Messages == nil || obj.Messages[len(obj.Messages)-1]["role"] != "assistant" {
			t.Fatalf("line %d shape: %+v", i, obj.Messages)
		}
	}
}
