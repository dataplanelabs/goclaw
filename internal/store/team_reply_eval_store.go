package store

import (
	"context"
	"time"
)

// TeamReplyEvaluation is a captured human-team reply on a channel + the judge
// verdict (hypothesized bot reply, diff score, reasoning). Captured-reply
// content also lives in sessions.messages JSONB for memory continuity; this
// table is the indexed analytics + JSONL-export source of truth.
type TeamReplyEvaluation struct {
	ID                   string
	ChannelInstanceID    string
	TenantID             string
	ThreadKey            string
	SessionKey           string
	TeamMsgID            string
	CapturedAt           time.Time
	CustomerMessage      string
	TeamReply            string
	HypothesizedBotReply *string
	DiffScore            *float64
	DiffReasoning        *string
	JudgeAgentKey        *string
	JudgeModel           *string
	JudgeProvider        *string
	JudgeLatencyMs       *int
	JudgeError           *string
	JudgeCompletedAt     *time.Time
	CreatedAt            time.Time
	UpdatedAt            time.Time
}

// TeamReplyEvalFilter is the filter shape for List + export queries.
type TeamReplyEvalFilter struct {
	ChannelInstanceID string
	ThreadKey         string
	Since             *time.Time
	Until             *time.Time
	MaxDiffScore      *float64
	JudgeOnlyComplete bool
	// ExcludeFailed drops rows with judge_error set. Default false (include
	// failed evals) so the analytics table shows them — operators want to
	// know what didn't grade. Export defaults true (filter out failed).
	ExcludeFailed bool
	Limit         int
	Offset        int
}

// TeamReplyEvalStore persists judge verdicts on captured team replies.
type TeamReplyEvalStore interface {
	// Insert is idempotent via UNIQUE (channel_instance_id, team_msg_id);
	// ON CONFLICT DO NOTHING. Returns the existing row's ID if conflict.
	Insert(ctx context.Context, e TeamReplyEvaluation) (string, error)

	UpdateJudgeVerdict(ctx context.Context, id string, hypo string, score float64, reasoning, model, provider, agentKey string, latencyMs int) error
	MarkJudgeError(ctx context.Context, id string, errMsg string) error

	List(ctx context.Context, tenantID string, f TeamReplyEvalFilter) ([]TeamReplyEvaluation, error)
	Count(ctx context.Context, tenantID string, f TeamReplyEvalFilter) (int64, error)
	GetByMessageID(ctx context.Context, channelInstanceID, teamMsgID string) (*TeamReplyEvaluation, error)
	ListPendingJudge(ctx context.Context, limit int) ([]TeamReplyEvaluation, error)

	DeleteByChannel(ctx context.Context, channelInstanceID string) (int64, error)
}
