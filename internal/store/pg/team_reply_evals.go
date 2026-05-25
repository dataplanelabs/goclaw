package pg

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/lib/pq"

	"github.com/nextlevelbuilder/goclaw/internal/store"
)

type PGTeamReplyEvalStore struct {
	db *sql.DB
}

func NewPGTeamReplyEvalStore(db *sql.DB) *PGTeamReplyEvalStore {
	return &PGTeamReplyEvalStore{db: db}
}

func (s *PGTeamReplyEvalStore) Insert(ctx context.Context, e store.TeamReplyEvaluation) (string, error) {
	if e.ChannelInstanceID == "" || e.TenantID == "" || e.TeamMsgID == "" {
		return "", fmt.Errorf("channel_instance_id, tenant_id, team_msg_id required")
	}
	if e.CapturedAt.IsZero() {
		e.CapturedAt = time.Now().UTC()
	}
	var id string
	err := s.db.QueryRowContext(ctx,
		`INSERT INTO team_reply_evaluations
		   (channel_instance_id, tenant_id, thread_key, session_key, team_msg_id,
		    captured_at, customer_message, team_reply)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		 ON CONFLICT (channel_instance_id, team_msg_id) DO NOTHING
		 RETURNING id`,
		e.ChannelInstanceID, e.TenantID, e.ThreadKey, e.SessionKey, e.TeamMsgID,
		e.CapturedAt, e.CustomerMessage, e.TeamReply).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		// Existing row from prior insert — fetch its ID.
		row := s.db.QueryRowContext(ctx,
			`SELECT id FROM team_reply_evaluations
			   WHERE channel_instance_id = $1 AND team_msg_id = $2`,
			e.ChannelInstanceID, e.TeamMsgID)
		if err2 := row.Scan(&id); err2 != nil {
			return "", fmt.Errorf("insert conflict lookup: %w", err2)
		}
		return id, nil
	}
	if err != nil {
		return "", fmt.Errorf("insert team_reply_evaluation: %w", err)
	}
	return id, nil
}

func (s *PGTeamReplyEvalStore) UpdateJudgeVerdict(ctx context.Context, id string, hypo string, score float64, reasoning, model, provider, agentKey string, latencyMs int) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE team_reply_evaluations
		    SET hypothesized_bot_reply = $1,
		        diff_score             = $2,
		        diff_reasoning         = $3,
		        judge_model            = $4,
		        judge_provider         = $5,
		        judge_agent_key        = $6,
		        judge_latency_ms       = $7,
		        judge_completed_at     = NOW(),
		        judge_error            = NULL,
		        updated_at             = NOW()
		  WHERE id = $8 AND judge_completed_at IS NULL`,
		hypo, score, reasoning, model, provider, agentKey, latencyMs, id)
	if err != nil {
		return fmt.Errorf("update judge verdict: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("no pending eval row for id %s", id)
	}
	return nil
}

func (s *PGTeamReplyEvalStore) MarkJudgeError(ctx context.Context, id string, errMsg string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE team_reply_evaluations
		    SET judge_error = $1,
		        updated_at  = NOW()
		  WHERE id = $2`, errMsg, id)
	if err != nil {
		return fmt.Errorf("mark judge error: %w", err)
	}
	return nil
}

// buildFilterClause builds WHERE conditions. `alias` is the table alias to
// prefix column names with (e.g. "e." for joined queries). Pass "" for
// unjoined queries.
func buildFilterClause(tenantID string, f store.TeamReplyEvalFilter) ([]string, []any) {
	return buildFilterClauseAlias(tenantID, f, "")
}

func buildFilterClauseAlias(tenantID string, f store.TeamReplyEvalFilter, alias string) ([]string, []any) {
	conds := []string{alias + "tenant_id = $1"}
	args := []any{tenantID}
	if f.ChannelInstanceID != "" {
		args = append(args, f.ChannelInstanceID)
		conds = append(conds, fmt.Sprintf("%schannel_instance_id = $%d", alias, len(args)))
	}
	if f.ThreadKey != "" {
		args = append(args, f.ThreadKey)
		conds = append(conds, fmt.Sprintf("%sthread_key = $%d", alias, len(args)))
	}
	if f.Since != nil {
		args = append(args, *f.Since)
		conds = append(conds, fmt.Sprintf("%scaptured_at >= $%d", alias, len(args)))
	}
	if f.Until != nil {
		args = append(args, *f.Until)
		conds = append(conds, fmt.Sprintf("%scaptured_at <= $%d", alias, len(args)))
	}
	if f.MaxDiffScore != nil {
		args = append(args, *f.MaxDiffScore)
		conds = append(conds, fmt.Sprintf("%sdiff_score IS NOT NULL AND %sdiff_score <= $%d", alias, alias, len(args)))
	}
	if f.JudgeOnlyComplete {
		conds = append(conds, alias+"judge_completed_at IS NOT NULL")
	}
	if f.ExcludeFailed {
		conds = append(conds, alias+"judge_error IS NULL")
	}
	return conds, args
}

func (s *PGTeamReplyEvalStore) List(ctx context.Context, tenantID string, f store.TeamReplyEvalFilter) ([]store.TeamReplyEvaluation, error) {
	if tenantID == "" {
		return nil, nil
	}
	conds, args := buildFilterClauseAlias(tenantID, f, "e.")

	limit := f.Limit
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	args = append(args, limit)
	limitClause := fmt.Sprintf("LIMIT $%d", len(args))
	offsetClause := ""
	if f.Offset > 0 {
		args = append(args, f.Offset)
		offsetClause = fmt.Sprintf("OFFSET $%d", len(args))
	}

	// LEFT JOIN channel_contacts so the analytics UI can label rows with the
	// customer's display name. Falls back to '' for group threads or unknown
	// contacts; UI then shows the truncated thread_key.
	q := `SELECT e.id, e.channel_instance_id, e.tenant_id, e.thread_key, e.session_key, e.team_msg_id,
	             e.captured_at, e.customer_message, e.team_reply, e.hypothesized_bot_reply,
	             e.diff_score, e.diff_reasoning, e.judge_agent_key, e.judge_model, e.judge_provider,
	             e.judge_latency_ms, e.judge_error, e.judge_completed_at, e.created_at, e.updated_at,
	             COALESCE(cc.display_name, '') AS customer_name
	        FROM team_reply_evaluations e
	        JOIN channel_instances ci ON ci.id = e.channel_instance_id
	   LEFT JOIN channel_contacts cc ON cc.tenant_id = e.tenant_id
	                                AND cc.channel_type = ci.channel_type
	                                AND cc.sender_id = SUBSTRING(e.thread_key FROM 'direct:(.+)')
	       WHERE ` + strings.Join(conds, " AND ") + `
	    ORDER BY e.captured_at DESC ` + limitClause + " " + offsetClause
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("list team_reply_evals: %w", err)
	}
	defer rows.Close()
	var out []store.TeamReplyEvaluation
	for rows.Next() {
		e, err := scanTeamReplyEvalWithName(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *e)
	}
	return out, rows.Err()
}

func (s *PGTeamReplyEvalStore) Count(ctx context.Context, tenantID string, f store.TeamReplyEvalFilter) (int64, error) {
	if tenantID == "" {
		return 0, nil
	}
	conds, args := buildFilterClause(tenantID, f)
	q := "SELECT COUNT(*) FROM team_reply_evaluations WHERE " + strings.Join(conds, " AND ")
	var n int64
	if err := s.db.QueryRowContext(ctx, q, args...).Scan(&n); err != nil {
		return 0, fmt.Errorf("count team_reply_evals: %w", err)
	}
	return n, nil
}

func (s *PGTeamReplyEvalStore) GetByMessageID(ctx context.Context, channelInstanceID, teamMsgID string) (*store.TeamReplyEvaluation, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, channel_instance_id, tenant_id, thread_key, session_key, team_msg_id,
		        captured_at, customer_message, team_reply, hypothesized_bot_reply,
		        diff_score, diff_reasoning, judge_agent_key, judge_model, judge_provider,
		        judge_latency_ms, judge_error, judge_completed_at, created_at, updated_at
		   FROM team_reply_evaluations
		  WHERE channel_instance_id = $1 AND team_msg_id = $2`,
		channelInstanceID, teamMsgID)
	e, err := scanTeamReplyEval(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return e, err
}

func (s *PGTeamReplyEvalStore) ListPendingJudge(ctx context.Context, limit int) ([]store.TeamReplyEvaluation, error) {
	if limit <= 0 || limit > 500 {
		limit = 50
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, channel_instance_id, tenant_id, thread_key, session_key, team_msg_id,
		        captured_at, customer_message, team_reply, hypothesized_bot_reply,
		        diff_score, diff_reasoning, judge_agent_key, judge_model, judge_provider,
		        judge_latency_ms, judge_error, judge_completed_at, created_at, updated_at
		   FROM team_reply_evaluations
		  WHERE judge_completed_at IS NULL AND judge_error IS NULL
		  ORDER BY captured_at ASC
		  LIMIT $1`, limit)
	if err != nil {
		return nil, fmt.Errorf("list pending judge: %w", err)
	}
	defer rows.Close()
	var out []store.TeamReplyEvaluation
	for rows.Next() {
		e, err := scanTeamReplyEval(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *e)
	}
	return out, rows.Err()
}

func (s *PGTeamReplyEvalStore) ListFailedJudge(ctx context.Context, channelInstanceID string, limit int) ([]store.TeamReplyEvaluation, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, channel_instance_id, tenant_id, thread_key, session_key, team_msg_id,
		        captured_at, customer_message, team_reply, hypothesized_bot_reply,
		        diff_score, diff_reasoning, judge_agent_key, judge_model, judge_provider,
		        judge_latency_ms, judge_error, judge_completed_at, created_at, updated_at
		   FROM team_reply_evaluations
		  WHERE channel_instance_id = $1 AND judge_error IS NOT NULL
		  ORDER BY captured_at DESC
		  LIMIT $2`, channelInstanceID, limit)
	if err != nil {
		return nil, fmt.Errorf("list failed judge: %w", err)
	}
	defer rows.Close()
	var out []store.TeamReplyEvaluation
	for rows.Next() {
		e, err := scanTeamReplyEval(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *e)
	}
	return out, rows.Err()
}

func (s *PGTeamReplyEvalStore) ClearJudgeError(ctx context.Context, ids []string) (int64, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	res, err := s.db.ExecContext(ctx,
		`UPDATE team_reply_evaluations
		    SET judge_error = NULL, updated_at = NOW()
		  WHERE id = ANY($1::uuid[])`, pq.Array(ids))
	if err != nil {
		return 0, fmt.Errorf("clear judge error: %w", err)
	}
	return res.RowsAffected()
}

func (s *PGTeamReplyEvalStore) DeleteByChannel(ctx context.Context, channelInstanceID string) (int64, error) {
	res, err := s.db.ExecContext(ctx,
		`DELETE FROM team_reply_evaluations WHERE channel_instance_id = $1`, channelInstanceID)
	if err != nil {
		return 0, fmt.Errorf("delete by channel: %w", err)
	}
	return res.RowsAffected()
}

// scanTeamReplyEvalWithName scans the List() column shape, which includes
// COALESCE(cc.display_name, '') AS customer_name as the trailing column.
func scanTeamReplyEvalWithName(s scanner) (*store.TeamReplyEvaluation, error) {
	var (
		e                store.TeamReplyEvaluation
		hypo             sql.NullString
		score            sql.NullFloat64
		reasoning        sql.NullString
		agentKey         sql.NullString
		model            sql.NullString
		provider         sql.NullString
		latencyMs        sql.NullInt64
		judgeErr         sql.NullString
		judgeCompletedAt sql.NullTime
	)
	if err := s.Scan(&e.ID, &e.ChannelInstanceID, &e.TenantID, &e.ThreadKey, &e.SessionKey, &e.TeamMsgID,
		&e.CapturedAt, &e.CustomerMessage, &e.TeamReply, &hypo,
		&score, &reasoning, &agentKey, &model, &provider,
		&latencyMs, &judgeErr, &judgeCompletedAt, &e.CreatedAt, &e.UpdatedAt,
		&e.CustomerName); err != nil {
		return nil, err
	}
	if hypo.Valid {
		e.HypothesizedBotReply = &hypo.String
	}
	if score.Valid {
		e.DiffScore = &score.Float64
	}
	if reasoning.Valid {
		e.DiffReasoning = &reasoning.String
	}
	if agentKey.Valid {
		e.JudgeAgentKey = &agentKey.String
	}
	if model.Valid {
		e.JudgeModel = &model.String
	}
	if provider.Valid {
		e.JudgeProvider = &provider.String
	}
	if latencyMs.Valid {
		v := int(latencyMs.Int64)
		e.JudgeLatencyMs = &v
	}
	if judgeErr.Valid {
		e.JudgeError = &judgeErr.String
	}
	if judgeCompletedAt.Valid {
		e.JudgeCompletedAt = &judgeCompletedAt.Time
	}
	return &e, nil
}

func scanTeamReplyEval(s scanner) (*store.TeamReplyEvaluation, error) {
	var (
		e                store.TeamReplyEvaluation
		hypo             sql.NullString
		score            sql.NullFloat64
		reasoning        sql.NullString
		agentKey         sql.NullString
		model            sql.NullString
		provider         sql.NullString
		latencyMs        sql.NullInt64
		judgeErr         sql.NullString
		judgeCompletedAt sql.NullTime
	)
	if err := s.Scan(&e.ID, &e.ChannelInstanceID, &e.TenantID, &e.ThreadKey, &e.SessionKey, &e.TeamMsgID,
		&e.CapturedAt, &e.CustomerMessage, &e.TeamReply, &hypo,
		&score, &reasoning, &agentKey, &model, &provider,
		&latencyMs, &judgeErr, &judgeCompletedAt, &e.CreatedAt, &e.UpdatedAt); err != nil {
		return nil, err
	}
	if hypo.Valid {
		e.HypothesizedBotReply = &hypo.String
	}
	if score.Valid {
		e.DiffScore = &score.Float64
	}
	if reasoning.Valid {
		e.DiffReasoning = &reasoning.String
	}
	if agentKey.Valid {
		e.JudgeAgentKey = &agentKey.String
	}
	if model.Valid {
		e.JudgeModel = &model.String
	}
	if provider.Valid {
		e.JudgeProvider = &provider.String
	}
	if latencyMs.Valid {
		v := int(latencyMs.Int64)
		e.JudgeLatencyMs = &v
	}
	if judgeErr.Valid {
		e.JudgeError = &judgeErr.String
	}
	if judgeCompletedAt.Valid {
		e.JudgeCompletedAt = &judgeCompletedAt.Time
	}
	return &e, nil
}
