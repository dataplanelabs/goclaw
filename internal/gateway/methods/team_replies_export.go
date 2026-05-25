package methods

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/nextlevelbuilder/goclaw/internal/gateway"
	"github.com/nextlevelbuilder/goclaw/internal/store"
	"github.com/nextlevelbuilder/goclaw/pkg/protocol"
)

// jsonlExportMaxRows + jsonlExportMaxBytes cap a single export call so a
// WS frame stays well under broker limits. Larger windows = paginate via
// since/until filters.
const (
	jsonlExportMaxRows  = 5000
	jsonlExportMaxBytes = 5 * 1024 * 1024
)

func (m *TeamRepliesMethods) handleExportJSONL(ctx context.Context, client *gateway.Client, req *protocol.RequestFrame) {
	var p struct {
		ChannelInstanceID string   `json:"channel_instance_id"`
		Since             string   `json:"since,omitempty"` // RFC3339
		Until             string   `json:"until,omitempty"`
		MaxDiffScore      *float64 `json:"max_diff_score,omitempty"`
		IncludePending    bool     `json:"include_pending,omitempty"`
		IncludeFailed     bool     `json:"include_failed,omitempty"`
	}
	decode(req, &p)
	inst := m.resolveInstance(ctx, client, req, p.ChannelInstanceID)
	if inst == nil {
		return
	}
	filter := store.TeamReplyEvalFilter{
		ChannelInstanceID: inst.ID.String(),
		MaxDiffScore:      p.MaxDiffScore,
		JudgeOnlyComplete: !p.IncludePending,
		ExcludeFailed:     !p.IncludeFailed,
		Limit:             jsonlExportMaxRows,
	}
	if p.Since != "" {
		if ts, err := time.Parse(time.RFC3339, p.Since); err == nil {
			filter.Since = &ts
		}
	}
	if p.Until != "" {
		if ts, err := time.Parse(time.RFC3339, p.Until); err == nil {
			filter.Until = &ts
		}
	}
	rows, err := m.evals.List(ctx, client.TenantID().String(), filter)
	if err != nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInternal, err.Error()))
		return
	}
	body, count, truncated, err := buildOpenAITrainingJSONL(rows, jsonlExportMaxBytes)
	if err != nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInternal, err.Error()))
		return
	}
	client.SendResponse(protocol.NewOKResponse(req.ID, map[string]any{
		"jsonl":     body,
		"count":     count,
		"bytes":     len(body),
		"truncated": truncated,
	}))
}

// buildOpenAITrainingJSONL emits one line per evaluation in OpenAI
// chat-completions training format: {"messages":[{role,content}*]}.
// System prompt is intentionally omitted (operator-side concern — they can
// prepend a uniform system line at fine-tuning time via `jq`).
// Returns (body, count, truncated, err). truncated=true when maxBytes was
// hit before all rows were rendered.
func buildOpenAITrainingJSONL(rows []store.TeamReplyEvaluation, maxBytes int) (string, int, bool, error) {
	var sb strings.Builder
	count := 0
	for _, r := range rows {
		msgs := []map[string]string{}
		if strings.TrimSpace(r.CustomerMessage) != "" {
			msgs = append(msgs, map[string]string{
				"role":    "user",
				"content": r.CustomerMessage,
			})
		}
		msgs = append(msgs, map[string]string{
			"role":    "assistant",
			"content": r.TeamReply,
		})
		line, err := json.Marshal(map[string]any{"messages": msgs})
		if err != nil {
			return "", 0, false, err
		}
		if maxBytes > 0 && sb.Len()+len(line)+1 > maxBytes {
			return sb.String(), count, true, nil
		}
		sb.Write(line)
		sb.WriteByte('\n')
		count++
	}
	return sb.String(), count, false, nil
}

// keep import referenced after refactor.
var _ = context.Background
