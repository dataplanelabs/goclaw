package http

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/i18n"
	"github.com/nextlevelbuilder/goclaw/internal/store"
)

const retryLockTTL = 60 * time.Second

// RetryRunner submits a deserialized RunRequest envelope through the regular
// agent loop, decoupled from agent package so http stays import-cycle clean.
type RetryRunner func(ctx context.Context, agentKey string, envelope []byte, originalTraceID uuid.UUID) error

var retryRunner RetryRunner

// SetRetryRunner registers the agent-runner callback. Wired from cmd/.
func SetRetryRunner(fn RetryRunner) {
	retryRunner = fn
}

func (h *TracesHandler) handleRetry(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	locale := store.LocaleFromContext(ctx)

	if !requireTenantAdmin(w, r, h.tenants) {
		return
	}

	traceID, err := uuid.Parse(r.PathValue("traceID"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": i18n.T(locale, i18n.MsgInvalidID, "trace")})
		return
	}

	confirmDoubleSend := r.URL.Query().Get("confirm_double_send") == "true"

	trace, err := h.tracing.GetTrace(ctx, traceID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": i18n.T(locale, i18n.MsgNotFound, "trace", traceID.String())})
			return
		}
		slog.Warn("trace.retry.get_failed", "trace_id", traceID, "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	if trace.Status != store.TraceStatusError && trace.Status != store.TraceStatusCancelled {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": i18n.T(locale, i18n.TraceRetryNotFailed)})
		return
	}
	if trace.OutboundEmitted && !confirmDoubleSend {
		writeJSON(w, http.StatusConflict, map[string]string{
			"error": i18n.T(locale, i18n.TraceRetryConfirmRequired),
			"code":  "confirm_required",
		})
		return
	}

	row, err := h.replay.Get(ctx, traceID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeJSON(w, http.StatusGone, map[string]string{
				"error": i18n.T(locale, i18n.TraceRetryPayloadMissing),
				"code":  "payload_missing",
			})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if row.Oversize || len(row.Payload) == 0 {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{
			"error": i18n.T(locale, i18n.TraceRetryPayloadOversize),
			"code":  "payload_oversize",
		})
		return
	}
	if row.Version != store.CurrentReplayPayloadVersion {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{
			"error": "payload version mismatch (binary upgraded)",
			"code":  "payload_version_mismatch",
		})
		return
	}

	if trace.AgentID == nil {
		writeJSON(w, http.StatusGone, map[string]string{
			"error": i18n.T(locale, i18n.TraceRetryAgentGone),
			"code":  "agent_gone",
		})
		return
	}
	agent, err := h.agents.GetByID(ctx, *trace.AgentID)
	if err != nil || agent == nil {
		writeJSON(w, http.StatusGone, map[string]string{
			"error": i18n.T(locale, i18n.TraceRetryAgentGone),
			"code":  "agent_gone",
		})
		return
	}

	runner, err := h.router.GetAgent(ctx, agent.AgentKey)
	if err != nil || runner == nil {
		writeJSON(w, http.StatusGone, map[string]string{
			"error": i18n.T(locale, i18n.TraceRetryProviderGone),
			"code":  "provider_gone",
		})
		return
	}

	userID := store.UserIDFromContext(ctx)
	lockedBy, _ := uuid.Parse(userID)
	if lockedBy == uuid.Nil {
		lockedBy = uuid.New()
	}
	acquired, err := h.retryLocks.Acquire(ctx, traceID, lockedBy, retryLockTTL)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if !acquired {
		writeJSON(w, http.StatusLocked, map[string]string{
			"error": i18n.T(locale, i18n.TraceRetryLocked),
			"code":  "locked",
		})
		return
	}

	if retryRunner == nil {
		_ = h.retryLocks.Release(ctx, traceID)
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "retry runner not configured"})
		return
	}

	detached := context.WithoutCancel(ctx)
	detached = store.WithTenantID(detached, store.TenantIDFromContext(ctx))
	detached = store.WithUserID(detached, userID)
	detached = store.WithLocale(detached, locale)

	go func() {
		defer func() {
			if rec := recover(); rec != nil {
				slog.Error("trace.retry.panic", "trace_id", traceID, "panic", rec)
			}
			if err := h.retryLocks.Release(context.WithoutCancel(detached), traceID); err != nil {
				slog.Warn("trace.retry.release_lock_failed", "trace_id", traceID, "err", err)
			}
		}()
		if err := retryRunner(detached, agent.AgentKey, row.Payload, traceID); err != nil {
			slog.Warn("trace.retry.runner_failed", "trace_id", traceID, "err", err)
		}
	}()

	resp := map[string]any{
		"message":           i18n.T(locale, i18n.TraceRetryStarted),
		"original_trace_id": traceID.String(),
		"provider":          runner.ProviderName(),
	}
	writeJSON(w, http.StatusAccepted, resp)
}

// DecodeReplayEnvelope unmarshals a stored envelope payload and returns the
// decoded RunRequest payload bytes. Called from cmd/-side wiring (kept here
// so http and store share the format definition).
func DecodeReplayEnvelope(envelope []byte) (*store.RunRequestEnvelope, error) {
	var env store.RunRequestEnvelope
	if err := json.Unmarshal(envelope, &env); err != nil {
		return nil, err
	}
	return &env, nil
}
