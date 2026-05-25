package methods

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/eventbus"
	"github.com/nextlevelbuilder/goclaw/internal/gateway"
	"github.com/nextlevelbuilder/goclaw/internal/i18n"
	"github.com/nextlevelbuilder/goclaw/internal/permissions"
	"github.com/nextlevelbuilder/goclaw/internal/store"
	"github.com/nextlevelbuilder/goclaw/pkg/protocol"
)

// TeamRepliesMethods exposes captured team-reply evaluations + JSONL export
// + the per-channel capture toggle via WS RPC. Reads require tenant
// membership; toggle requires admin.
type TeamRepliesMethods struct {
	evals     store.TeamReplyEvalStore
	instances store.ChannelInstanceStore
	agents    store.AgentCRUDStore
	bus       eventbus.DomainEventBus
}

func NewTeamRepliesMethods(evals store.TeamReplyEvalStore, instances store.ChannelInstanceStore, agents store.AgentCRUDStore, bus eventbus.DomainEventBus) *TeamRepliesMethods {
	return &TeamRepliesMethods{evals: evals, instances: instances, agents: agents, bus: bus}
}

func (m *TeamRepliesMethods) Register(router *gateway.MethodRouter) {
	router.Register(protocol.MethodChannelsTeamRepliesList, m.handleList)
	router.Register(protocol.MethodChannelsTeamRepliesGet, m.handleGet)
	router.Register(protocol.MethodChannelsTeamRepliesExportJSONL, m.handleExportJSONL)
	router.Register(protocol.MethodChannelsTeamRepliesRejudge, m.handleRejudge)
	router.Register(protocol.MethodChannelsTeamCaptureToggle, m.handleToggle)
}

// resolveInstance enforces tenant scope. Same shape as ChannelSchedulesMethods.
func (m *TeamRepliesMethods) resolveInstance(ctx context.Context, client *gateway.Client, req *protocol.RequestFrame, raw string) *store.ChannelInstanceData {
	locale := store.LocaleFromContext(ctx)
	id, err := uuid.Parse(raw)
	if err != nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgInvalidID, "instance")))
		return nil
	}
	inst, err := m.instances.Get(ctx, id)
	if err != nil || inst == nil || inst.TenantID != client.TenantID() {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrNotFound, i18n.T(locale, i18n.MsgInstanceNotFound)))
		return nil
	}
	return inst
}

func (m *TeamRepliesMethods) requireAdmin(ctx context.Context, client *gateway.Client, req *protocol.RequestFrame) bool {
	if permissions.HasMinRole(client.Role(), permissions.RoleAdmin) {
		return true
	}
	client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrUnauthorized,
		i18n.T(store.LocaleFromContext(ctx), i18n.TeamCaptureRPCNoPermission)))
	return false
}

func (m *TeamRepliesMethods) handleList(ctx context.Context, client *gateway.Client, req *protocol.RequestFrame) {
	var p struct {
		ChannelInstanceID string `json:"channel_instance_id"`
		ThreadKey         string `json:"thread_key,omitempty"`
		Limit             int    `json:"limit,omitempty"`
		Offset            int    `json:"offset,omitempty"`
	}
	decode(req, &p)
	inst := m.resolveInstance(ctx, client, req, p.ChannelInstanceID)
	if inst == nil {
		return
	}
	limit := p.Limit
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	filter := store.TeamReplyEvalFilter{
		ChannelInstanceID: inst.ID.String(),
		ThreadKey:         p.ThreadKey,
		Limit:             limit,
		Offset:            p.Offset,
	}
	rows, err := m.evals.List(ctx, client.TenantID().String(), filter)
	if err != nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInternal, err.Error()))
		return
	}
	total, err := m.evals.Count(ctx, client.TenantID().String(), filter)
	if err != nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInternal, err.Error()))
		return
	}
	client.SendResponse(protocol.NewOKResponse(req.ID, map[string]any{
		"evaluations": serializeEvals(rows),
		"total":       total,
	}))
}

func (m *TeamRepliesMethods) handleGet(ctx context.Context, client *gateway.Client, req *protocol.RequestFrame) {
	var p struct {
		ChannelInstanceID string `json:"channel_instance_id"`
		TeamMsgID         string `json:"team_msg_id"`
	}
	decode(req, &p)
	inst := m.resolveInstance(ctx, client, req, p.ChannelInstanceID)
	if inst == nil {
		return
	}
	row, err := m.evals.GetByMessageID(ctx, inst.ID.String(), p.TeamMsgID)
	if err != nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInternal, err.Error()))
		return
	}
	if row == nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrNotFound,
			i18n.T(store.LocaleFromContext(ctx), i18n.TeamEvalNotFound)))
		return
	}
	client.SendResponse(protocol.NewOKResponse(req.ID, map[string]any{"evaluation": serializeEval(*row)}))
}

func (m *TeamRepliesMethods) handleToggle(ctx context.Context, client *gateway.Client, req *protocol.RequestFrame) {
	var p struct {
		ChannelInstanceID  string `json:"channel_instance_id"`
		CaptureTeamReplies *bool  `json:"capture_team_replies,omitempty"`
		JudgeEvaluation    *bool  `json:"judge_evaluation,omitempty"`
		JudgeAgentKey      string `json:"judge_agent_key,omitempty"`
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
	partial := map[string]any{}
	if p.CaptureTeamReplies != nil {
		partial["capture_team_replies"] = *p.CaptureTeamReplies
	}
	if p.JudgeEvaluation != nil {
		partial["judge_evaluation"] = *p.JudgeEvaluation
	}
	judgeKey := strings.TrimSpace(p.JudgeAgentKey)
	if judgeKey != "" {
		partial["judge_agent_key"] = judgeKey
	}
	if len(partial) == 0 {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest,
			i18n.T(locale, i18n.TeamCaptureRPCInvalidConfig, "empty payload")))
		return
	}
	// Without this gate, the "all Failed" UX from v3.22.0 prod recurs.
	// When judge_key not in payload, fall back to existing channel config so
	// partial-update callers (toggle just judge_evaluation) still work.
	if p.JudgeEvaluation != nil && *p.JudgeEvaluation {
		effectiveKey := judgeKey
		if effectiveKey == "" && len(inst.Config) > 0 {
			var ccfg struct {
				JudgeAgentKey string `json:"judge_agent_key,omitempty"`
			}
			_ = json.Unmarshal(inst.Config, &ccfg)
			effectiveKey = ccfg.JudgeAgentKey
		}
		if effectiveKey == "" {
			client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest,
				i18n.T(locale, i18n.TeamCaptureJudgeKeyRequired)))
			return
		}
		if m.agents != nil {
			ad, err := m.agents.GetByKey(ctx, effectiveKey)
			if err != nil || ad == nil {
				client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest,
					i18n.T(locale, i18n.TeamCaptureJudgeAgentNotFound, effectiveKey)))
				return
			}
		}
	}
	if err := m.instances.MergeConfig(ctx, inst.ID, partial); err != nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInternal, err.Error()))
		return
	}
	client.SendResponse(protocol.NewOKResponse(req.ID, map[string]any{
		"ok":             true,
		"config_updated": partial,
		"hint":           "channel restart required for the toggle to take effect",
	}))
}

func serializeEval(e store.TeamReplyEvaluation) map[string]any {
	out := map[string]any{
		"id":                  e.ID,
		"channel_instance_id": e.ChannelInstanceID,
		"thread_key":          e.ThreadKey,
		"session_key":         e.SessionKey,
		"team_msg_id":         e.TeamMsgID,
		"captured_at":         e.CapturedAt.UTC().Format(time.RFC3339),
		"updated_at":          e.UpdatedAt.UTC().Format(time.RFC3339Nano),
		"customer_message":    e.CustomerMessage,
		"team_reply":          e.TeamReply,
	}
	if e.HypothesizedBotReply != nil {
		out["hypothesized_bot_reply"] = *e.HypothesizedBotReply
	}
	if e.DiffScore != nil {
		out["diff_score"] = *e.DiffScore
	}
	if e.DiffReasoning != nil {
		out["diff_reasoning"] = *e.DiffReasoning
	}
	if e.JudgeAgentKey != nil {
		out["judge_agent_key"] = *e.JudgeAgentKey
	}
	if e.JudgeModel != nil {
		out["judge_model"] = *e.JudgeModel
	}
	if e.JudgeProvider != nil {
		out["judge_provider"] = *e.JudgeProvider
	}
	if e.JudgeLatencyMs != nil {
		out["judge_latency_ms"] = *e.JudgeLatencyMs
	}
	if e.JudgeError != nil {
		out["judge_error"] = *e.JudgeError
	}
	if e.JudgeCompletedAt != nil {
		out["judge_completed_at"] = e.JudgeCompletedAt.UTC().Format(time.RFC3339)
	}
	return out
}

func serializeEvals(rows []store.TeamReplyEvaluation) []map[string]any {
	out := make([]map[string]any, len(rows))
	for i, r := range rows {
		out[i] = serializeEval(r)
	}
	return out
}

const rejudgeBatchCap = 100

func (m *TeamRepliesMethods) handleRejudge(ctx context.Context, client *gateway.Client, req *protocol.RequestFrame) {
	var p struct {
		ChannelInstanceID string `json:"channel_instance_id"`
		Limit             int    `json:"limit,omitempty"`
	}
	decode(req, &p)
	if !m.requireAdmin(ctx, client, req) {
		return
	}
	inst := m.resolveInstance(ctx, client, req, p.ChannelInstanceID)
	if inst == nil {
		return
	}
	limit := p.Limit
	if limit <= 0 || limit > rejudgeBatchCap {
		limit = rejudgeBatchCap
	}
	failed, err := m.evals.ListFailedJudge(ctx, inst.ID.String(), limit)
	if err != nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInternal, err.Error()))
		return
	}
	sinceTs := time.Now().UTC().Format(time.RFC3339Nano)
	if len(failed) == 0 {
		client.SendResponse(protocol.NewOKResponse(req.ID, map[string]any{
			"rejudged":      0,
			"rejudged_ids":  []string{},
			"since_ts":      sinceTs,
			"batch_capped":  false,
		}))
		return
	}
	ids := make([]string, len(failed))
	for i, e := range failed {
		ids[i] = e.ID
	}
	if _, err := m.evals.ClearJudgeError(ctx, ids); err != nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInternal, err.Error()))
		return
	}
	if m.bus != nil {
		for _, e := range failed {
			ev := eventbus.DomainEvent{
				ID:        uuid.NewString(),
				Type:      eventbus.EventTeamReplyObserved,
				SourceID:  eventbus.TeamReplyObservedSourceID(e.ChannelInstanceID, e.TeamMsgID) + "?rejudge=" + uuid.NewString()[:8],
				TenantID:  e.TenantID,
				Timestamp: time.Now().UTC(),
				Payload: eventbus.TeamReplyObservedPayload{
					EvaluationID:      e.ID,
					TenantID:          e.TenantID,
					ChannelInstanceID: e.ChannelInstanceID,
					ChannelName:       inst.Name,
					ThreadKey:         e.ThreadKey,
					SessionKey:        e.SessionKey,
					TeamMsgID:         e.TeamMsgID,
					TeamReply:         e.TeamReply,
					CustomerMessage:   e.CustomerMessage,
					CapturedAt:        e.CapturedAt,
				},
			}
			m.bus.Publish(ev)
		}
	}
	client.SendResponse(protocol.NewOKResponse(req.ID, map[string]any{
		"rejudged":     len(failed),
		"rejudged_ids": ids,
		"since_ts":     sinceTs,
		"batch_capped": len(failed) == rejudgeBatchCap,
	}))
}
