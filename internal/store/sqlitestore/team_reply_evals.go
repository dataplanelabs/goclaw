//go:build sqlite || sqliteonly

package sqlitestore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/store"
)

type SQLiteTeamReplyEvalStore struct {
	db *sql.DB
}

func NewSQLiteTeamReplyEvalStore(db *sql.DB) *SQLiteTeamReplyEvalStore {
	return &SQLiteTeamReplyEvalStore{db: db}
}

func (s *SQLiteTeamReplyEvalStore) Insert(ctx context.Context, e store.TeamReplyEvaluation) (string, error) {
	if e.ChannelInstanceID == "" || e.TenantID == "" || e.TeamMsgID == "" {
		return "", fmt.Errorf("channel_instance_id, tenant_id, team_msg_id required")
	}
	if e.CapturedAt.IsZero() {
		e.CapturedAt = time.Now().UTC()
	}
	id := e.ID
	if id == "" {
		id = uuid.NewString()
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO team_reply_evaluations
		   (id, channel_instance_id, tenant_id, thread_key, session_key, team_msg_id,
		    captured_at, customer_message, team_reply)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(channel_instance_id, team_msg_id) DO NOTHING`,
		id, e.ChannelInstanceID, e.TenantID, e.ThreadKey, e.SessionKey, e.TeamMsgID,
		e.CapturedAt, e.CustomerMessage, e.TeamReply)
	if err != nil {
		return "", fmt.Errorf("insert team_reply_evaluation: %w", err)
	}
	// Fetch winning ID (might be the existing one on conflict).
	row := s.db.QueryRowContext(ctx,
		`SELECT id FROM team_reply_evaluations
		   WHERE channel_instance_id = ? AND team_msg_id = ?`,
		e.ChannelInstanceID, e.TeamMsgID)
	if err := row.Scan(&id); err != nil {
		return "", fmt.Errorf("lookup inserted id: %w", err)
	}
	return id, nil
}

func (s *SQLiteTeamReplyEvalStore) UpdateJudgeVerdict(ctx context.Context, id string, hypo string, score float64, reasoning, model, provider, agentKey string, latencyMs int) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE team_reply_evaluations
		    SET hypothesized_bot_reply = ?,
		        diff_score             = ?,
		        diff_reasoning         = ?,
		        judge_model            = ?,
		        judge_provider         = ?,
		        judge_agent_key        = ?,
		        judge_latency_ms       = ?,
		        judge_completed_at     = CURRENT_TIMESTAMP,
		        judge_error            = NULL,
		        updated_at             = CURRENT_TIMESTAMP
		  WHERE id = ? AND judge_completed_at IS NULL`,
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

func (s *SQLiteTeamReplyEvalStore) MarkJudgeError(ctx context.Context, id string, errMsg string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE team_reply_evaluations
		    SET judge_error = ?,
		        updated_at  = CURRENT_TIMESTAMP
		  WHERE id = ?`, errMsg, id)
	if err != nil {
		return fmt.Errorf("mark judge error: %w", err)
	}
	return nil
}

func buildFilterClauseSQLite(tenantID string, f store.TeamReplyEvalFilter) ([]string, []any) {
	conds := []string{"tenant_id = ?"}
	args := []any{tenantID}
	if f.ChannelInstanceID != "" {
		conds = append(conds, "channel_instance_id = ?")
		args = append(args, f.ChannelInstanceID)
	}
	if f.ThreadKey != "" {
		conds = append(conds, "thread_key = ?")
		args = append(args, f.ThreadKey)
	}
	if f.Since != nil {
		conds = append(conds, "captured_at >= ?")
		args = append(args, *f.Since)
	}
	if f.Until != nil {
		conds = append(conds, "captured_at <= ?")
		args = append(args, *f.Until)
	}
	if f.MaxDiffScore != nil {
		conds = append(conds, "diff_score IS NOT NULL AND diff_score <= ?")
		args = append(args, *f.MaxDiffScore)
	}
	if f.JudgeOnlyComplete {
		conds = append(conds, "judge_completed_at IS NOT NULL")
	}
	if f.ExcludeFailed {
		conds = append(conds, "judge_error IS NULL")
	}
	return conds, args
}

func (s *SQLiteTeamReplyEvalStore) List(ctx context.Context, tenantID string, f store.TeamReplyEvalFilter) ([]store.TeamReplyEvaluation, error) {
	if tenantID == "" {
		return nil, nil
	}
	conds, args := buildFilterClauseSQLite(tenantID, f)
	limit := f.Limit
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	args = append(args, limit)
	q := `SELECT id, channel_instance_id, tenant_id, thread_key, session_key, team_msg_id,
	             captured_at, customer_message, team_reply, hypothesized_bot_reply,
	             diff_score, diff_reasoning, judge_agent_key, judge_model, judge_provider,
	             judge_latency_ms, judge_error, judge_completed_at, created_at, updated_at
	        FROM team_reply_evaluations
	       WHERE ` + strings.Join(conds, " AND ") + `
	    ORDER BY captured_at DESC
	       LIMIT ?`
	if f.Offset > 0 {
		q += " OFFSET ?"
		args = append(args, f.Offset)
	}
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("list team_reply_evals: %w", err)
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

func (s *SQLiteTeamReplyEvalStore) Count(ctx context.Context, tenantID string, f store.TeamReplyEvalFilter) (int64, error) {
	if tenantID == "" {
		return 0, nil
	}
	conds, args := buildFilterClauseSQLite(tenantID, f)
	q := "SELECT COUNT(*) FROM team_reply_evaluations WHERE " + strings.Join(conds, " AND ")
	var n int64
	if err := s.db.QueryRowContext(ctx, q, args...).Scan(&n); err != nil {
		return 0, fmt.Errorf("count team_reply_evals: %w", err)
	}
	return n, nil
}

func (s *SQLiteTeamReplyEvalStore) GetByMessageID(ctx context.Context, channelInstanceID, teamMsgID string) (*store.TeamReplyEvaluation, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, channel_instance_id, tenant_id, thread_key, session_key, team_msg_id,
		        captured_at, customer_message, team_reply, hypothesized_bot_reply,
		        diff_score, diff_reasoning, judge_agent_key, judge_model, judge_provider,
		        judge_latency_ms, judge_error, judge_completed_at, created_at, updated_at
		   FROM team_reply_evaluations
		  WHERE channel_instance_id = ? AND team_msg_id = ?`,
		channelInstanceID, teamMsgID)
	e, err := scanTeamReplyEval(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return e, err
}

func (s *SQLiteTeamReplyEvalStore) ListPendingJudge(ctx context.Context, limit int) ([]store.TeamReplyEvaluation, error) {
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
		  LIMIT ?`, limit)
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

func (s *SQLiteTeamReplyEvalStore) DeleteByChannel(ctx context.Context, channelInstanceID string) (int64, error) {
	res, err := s.db.ExecContext(ctx,
		`DELETE FROM team_reply_evaluations WHERE channel_instance_id = ?`, channelInstanceID)
	if err != nil {
		return 0, fmt.Errorf("delete by channel: %w", err)
	}
	return res.RowsAffected()
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
