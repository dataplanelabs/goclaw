package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/agent"
	"github.com/nextlevelbuilder/goclaw/internal/bus"
	httpapi "github.com/nextlevelbuilder/goclaw/internal/http"
	"github.com/nextlevelbuilder/goclaw/internal/store"
)

// retryAgentRouter adapts *agent.Router to the httpapi.RetryAgentRunner
// interface; keeps the http package import-cycle clean.
type retryAgentRouter struct {
	r *agent.Router
}

func newRetryAgentRouter(r *agent.Router) httpapi.RetryAgentRunner {
	return &retryAgentRouter{r: r}
}

func (r *retryAgentRouter) GetAgent(ctx context.Context, agentID string) (httpapi.RetryAgent, error) {
	a, err := r.r.Get(ctx, agentID)
	if err != nil {
		return nil, err
	}
	return a, nil
}

// wireTraceRetry registers the retry deps + replay-payload runner if all stores
// + the agent router are available. No-op otherwise — endpoint stays unregistered.
func wireTraceRetry(tracesH *httpapi.TracesHandler, stores *store.Stores, router *agent.Router, msgBus *bus.MessageBus) {
	if tracesH == nil || stores == nil || router == nil {
		return
	}
	if stores.Agents == nil || stores.ReplayPayloads == nil || stores.RetryLocks == nil || stores.Tenants == nil {
		return
	}
	tracesH.SetRetryDeps(stores.Agents, stores.ReplayPayloads, stores.RetryLocks, stores.Tenants, newRetryAgentRouter(router))

	httpapi.SetRetryRunner(func(ctx context.Context, agentKey string, envelope []byte, originalTraceID uuid.UUID) error {
		env, err := httpapi.DecodeReplayEnvelope(envelope)
		if err != nil {
			return fmt.Errorf("decode envelope: %w", err)
		}
		req, err := deserializeReplayRunRequest(env.Payload, originalTraceID)
		if err != nil {
			return fmt.Errorf("deserialize replay: %w", err)
		}
		ag, err := router.Get(ctx, agentKey)
		if err != nil {
			return fmt.Errorf("router.Get: %w", err)
		}
		result, runErr := ag.Run(ctx, *req)
		if runErr != nil {
			slog.Warn("trace.retry.run_failed", "original_trace_id", originalTraceID, "err", runErr)
			return runErr
		}
		if result != nil {
			slog.Info("trace.retry.run_completed",
				"original_trace_id", originalTraceID,
				"new_trace_id", result.TraceID,
				"iterations", result.Iterations)
			dispatchRetryOutbound(ctx, msgBus, req, result, ag, originalTraceID)
		}
		return nil
	})
}

// dispatchRetryOutbound publishes the retry's RunResult back to the channel —
// matching the regular consumer's dispatch shape so the Retry button's
// "re-invoke through the regular agent pipeline" contract actually completes
// end-to-end. Skips when:
//   - msgBus is nil (test wiring)
//   - request lacks Channel/ChatID (internal task; no channel target)
//   - content is empty or NO_REPLY-style (no user-visible reply to send)
func dispatchRetryOutbound(ctx context.Context, msgBus *bus.MessageBus, req *agent.RunRequest, result *agent.RunResult, ag agent.Agent, originalTraceID uuid.UUID) {
	if msgBus == nil || req == nil || result == nil {
		return
	}
	if req.Channel == "" || req.ChatID == "" {
		return
	}
	if result.Content == "" || agent.IsSilentReply(result.Content) {
		slog.Info("trace.retry.dispatch_skipped_silent",
			"original_trace_id", originalTraceID,
			"new_trace_id", result.TraceID)
		return
	}
	outMsg := bus.OutboundMessage{
		Channel:          req.Channel,
		ChatID:           req.ChatID,
		Content:          result.Content,
		TenantID:         store.TenantIDFromContext(ctx),
		AgentID:          ag.UUID(),
		AgentOtherConfig: ag.OtherConfig(),
		TraceID:          result.TraceID,
	}
	if req.PeerKind == "group" && req.ChatID != "" {
		outMsg.Metadata = map[string]string{"group_id": req.ChatID}
	}
	appendMediaToOutbound(&outMsg, result.Media)
	msgBus.PublishOutbound(outMsg)
	slog.Info("trace.retry.dispatched",
		"original_trace_id", originalTraceID,
		"new_trace_id", result.TraceID,
		"channel", req.Channel,
		"chat_id", req.ChatID,
		"has_media", len(result.Media) > 0,
	)
}

// deserializeReplayRunRequest unmarshals the captured RunRequest payload and
// links the new run back to the original failed trace via LinkedTraceID so
// the child trace stamps parent_trace_id = original.
func deserializeReplayRunRequest(payload json.RawMessage, originalTraceID uuid.UUID) (*agent.RunRequest, error) {
	if len(payload) == 0 {
		return nil, errors.New("empty payload")
	}
	var s agent.SerializableRunRequest
	if err := json.Unmarshal(payload, &s); err != nil {
		return nil, err
	}
	req := s.ToRunRequest()
	req.LinkedTraceID = originalTraceID
	req.RunID = "retry-" + originalTraceID.String()[:8]
	return req, nil
}
