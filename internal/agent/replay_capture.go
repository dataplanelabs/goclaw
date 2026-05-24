package agent

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/store"
	"github.com/nextlevelbuilder/goclaw/internal/tracing"
)

// SerializableRunRequest is the JSON-safe projection of RunRequest; drops
// callbacks, channels, and live provider — retry rebuilds those from config.
// Exported because the retry handler in cmd/ deserializes captured payloads
// back into RunRequest via ToRunRequest.
type SerializableRunRequest struct {
	SessionKey         string         `json:"session_key"`
	Message            string         `json:"message"`
	Media              json.RawMessage `json:"media,omitempty"`
	ForwardMedia       json.RawMessage `json:"forward_media,omitempty"`
	Channel            string         `json:"channel,omitempty"`
	ChannelType        string         `json:"channel_type,omitempty"`
	ChatTitle          string         `json:"chat_title,omitempty"`
	ChatID             string         `json:"chat_id,omitempty"`
	PeerKind           string         `json:"peer_kind,omitempty"`
	RunID              string         `json:"run_id,omitempty"`
	UserID             string         `json:"user_id,omitempty"`
	SenderID           string         `json:"sender_id,omitempty"`
	SenderName         string         `json:"sender_name,omitempty"`
	Role               string         `json:"role,omitempty"`
	Stream             bool           `json:"stream,omitempty"`
	ExtraSystemPrompt  string         `json:"extra_system_prompt,omitempty"`
	SkillFilter        []string       `json:"skill_filter,omitempty"`
	HistoryLimit       int            `json:"history_limit,omitempty"`
	ToolAllow          []string       `json:"tool_allow,omitempty"`
	LocalKey           string         `json:"local_key,omitempty"`
	TraceName          string         `json:"trace_name,omitempty"`
	TraceTags          []string       `json:"trace_tags,omitempty"`
	MaxIterations      int            `json:"max_iterations,omitempty"`
	ModelOverride      string         `json:"model_override,omitempty"`
	LightContext       bool           `json:"light_context,omitempty"`
	EnableNativeStyles bool           `json:"enable_native_styles,omitempty"`
	RunKind            string         `json:"run_kind,omitempty"`
	HideInput          bool           `json:"hide_input,omitempty"`
	ContentSuffix      string         `json:"content_suffix,omitempty"`
	DelegationID       string         `json:"delegation_id,omitempty"`
	TeamID             string         `json:"team_id,omitempty"`
	TeamTaskID         string         `json:"team_task_id,omitempty"`
	ParentAgentID      string         `json:"parent_agent_id,omitempty"`
	LeaderAgentID      string         `json:"leader_agent_id,omitempty"`
	WorkspaceChannel   string         `json:"workspace_channel,omitempty"`
	WorkspaceChatID    string         `json:"workspace_chat_id,omitempty"`
	TeamWorkspace      string         `json:"team_workspace,omitempty"`
}

// ToRunRequest reverses toSerializableRunRequest. JSON-encoded Media is
// re-decoded; non-serializable fields (callbacks, InjectCh, ProviderOverride)
// stay nil — the retry handler rebuilds them.
func (s SerializableRunRequest) ToRunRequest() *RunRequest {
	req := &RunRequest{
		SessionKey:         s.SessionKey,
		Message:            s.Message,
		Channel:            s.Channel,
		ChannelType:        s.ChannelType,
		ChatTitle:          s.ChatTitle,
		ChatID:             s.ChatID,
		PeerKind:           s.PeerKind,
		RunID:              s.RunID,
		UserID:             s.UserID,
		SenderID:           s.SenderID,
		SenderName:         s.SenderName,
		Role:               s.Role,
		Stream:             s.Stream,
		ExtraSystemPrompt:  s.ExtraSystemPrompt,
		SkillFilter:        s.SkillFilter,
		HistoryLimit:       s.HistoryLimit,
		ToolAllow:          s.ToolAllow,
		LocalKey:           s.LocalKey,
		TraceName:          s.TraceName,
		TraceTags:          s.TraceTags,
		MaxIterations:      s.MaxIterations,
		ModelOverride:      s.ModelOverride,
		LightContext:       s.LightContext,
		EnableNativeStyles: s.EnableNativeStyles,
		RunKind:            s.RunKind,
		HideInput:          s.HideInput,
		ContentSuffix:      s.ContentSuffix,
		DelegationID:       s.DelegationID,
		TeamID:             s.TeamID,
		TeamTaskID:         s.TeamTaskID,
		ParentAgentID:      s.ParentAgentID,
		LeaderAgentID:      s.LeaderAgentID,
		WorkspaceChannel:   s.WorkspaceChannel,
		WorkspaceChatID:    s.WorkspaceChatID,
		TeamWorkspace:      s.TeamWorkspace,
	}
	if len(s.Media) > 0 {
		_ = json.Unmarshal(s.Media, &req.Media)
	}
	if len(s.ForwardMedia) > 0 {
		_ = json.Unmarshal(s.ForwardMedia, &req.ForwardMedia)
	}
	return req
}

func toSerializableRunRequest(req *RunRequest) *SerializableRunRequest {
	out := &SerializableRunRequest{
		SessionKey:         req.SessionKey,
		Message:            req.Message,
		Channel:            req.Channel,
		ChannelType:        req.ChannelType,
		ChatTitle:          req.ChatTitle,
		ChatID:             req.ChatID,
		PeerKind:           req.PeerKind,
		RunID:              req.RunID,
		UserID:             req.UserID,
		SenderID:           req.SenderID,
		SenderName:         req.SenderName,
		Role:               req.Role,
		Stream:             req.Stream,
		ExtraSystemPrompt:  req.ExtraSystemPrompt,
		SkillFilter:        req.SkillFilter,
		HistoryLimit:       req.HistoryLimit,
		ToolAllow:          req.ToolAllow,
		LocalKey:           req.LocalKey,
		TraceName:          req.TraceName,
		TraceTags:          req.TraceTags,
		MaxIterations:      req.MaxIterations,
		ModelOverride:      req.ModelOverride,
		LightContext:       req.LightContext,
		EnableNativeStyles: req.EnableNativeStyles,
		RunKind:            req.RunKind,
		HideInput:          req.HideInput,
		ContentSuffix:      req.ContentSuffix,
		DelegationID:       req.DelegationID,
		TeamID:             req.TeamID,
		TeamTaskID:         req.TeamTaskID,
		ParentAgentID:      req.ParentAgentID,
		LeaderAgentID:      req.LeaderAgentID,
		WorkspaceChannel:   req.WorkspaceChannel,
		WorkspaceChatID:    req.WorkspaceChatID,
		TeamWorkspace:      req.TeamWorkspace,
	}
	if len(req.Media) > 0 {
		if b, err := json.Marshal(req.Media); err == nil {
			out.Media = b
		}
	}
	if len(req.ForwardMedia) > 0 {
		if b, err := json.Marshal(req.ForwardMedia); err == nil {
			out.ForwardMedia = b
		}
	}
	return out
}

// captureReplayPayload writes the inbound RunRequest envelope; best-effort,
// errors are logged. Skipped for child/announce/team-task runs.
func (l *Loop) captureReplayPayload(ctx context.Context, req *RunRequest) {
	if l.replayStore == nil {
		return
	}
	if req == nil || req.SessionKey == "" {
		return
	}
	if req.ParentTraceID != uuid.Nil || req.LinkedTraceID != uuid.Nil {
		return
	}
	traceID := tracing.TraceIDFromContext(ctx)
	if traceID == uuid.Nil {
		return
	}
	if store.TenantIDFromContext(ctx) == uuid.Nil {
		return
	}

	payload, err := json.Marshal(toSerializableRunRequest(req))
	if err != nil {
		slog.Warn("replay_payload: marshal failed", "err", err, "trace_id", traceID)
		return
	}
	if len(payload) > store.MaxReplayPayloadBytes {
		if err := l.replayStore.CaptureOversize(ctx, traceID, req.SessionKey, len(payload)); err != nil {
			slog.Warn("replay_payload: oversize capture failed", "err", err, "trace_id", traceID)
			return
		}
		slog.Info("replay_payload: oversize sentinel stored", "bytes", len(payload), "trace_id", traceID, "session_key", req.SessionKey)
		return
	}
	envelope, err := json.Marshal(store.RunRequestEnvelope{
		Version:  store.CurrentReplayPayloadVersion,
		Captured: time.Now().UTC(),
		Payload:  payload,
	})
	if err != nil {
		slog.Warn("replay_payload: envelope marshal failed", "err", err, "trace_id", traceID)
		return
	}
	if err := l.replayStore.Capture(ctx, traceID, req.SessionKey, envelope); err != nil {
		slog.Warn("replay_payload: capture failed", "err", err, "trace_id", traceID, "session_key", req.SessionKey)
	}
}
