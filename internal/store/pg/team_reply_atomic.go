package pg

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/nextlevelbuilder/goclaw/internal/providers"
	"github.com/nextlevelbuilder/goclaw/internal/store"
)

type PGTeamReplyAtomicWriter struct {
	db       *sql.DB
	sessions *PGSessionStore
}

func NewPGTeamReplyAtomicWriter(db *sql.DB, sessions *PGSessionStore) *PGTeamReplyAtomicWriter {
	return &PGTeamReplyAtomicWriter{db: db, sessions: sessions}
}

var _ store.AtomicTeamReplyWriter = (*PGTeamReplyAtomicWriter)(nil)

func (w *PGTeamReplyAtomicWriter) WriteTeamReplyAtomic(ctx context.Context, e store.TeamReplyEvaluation, sessionKey string, msg providers.Message) (string, bool, error) {
	tx, err := w.db.BeginTx(ctx, nil)
	if err != nil {
		return "", false, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var id string
	wasNew := true
	err = tx.QueryRowContext(ctx,
		`INSERT INTO team_reply_evaluations
		   (channel_instance_id, tenant_id, thread_key, session_key, team_msg_id,
		    captured_at, customer_message, team_reply)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
		 ON CONFLICT (channel_instance_id, team_msg_id) DO NOTHING
		 RETURNING id`,
		e.ChannelInstanceID, e.TenantID, e.ThreadKey, e.SessionKey, e.TeamMsgID,
		e.CapturedAt, e.CustomerMessage, e.TeamReply).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		wasNew = false
		if err = tx.QueryRowContext(ctx,
			`SELECT id FROM team_reply_evaluations
			  WHERE channel_instance_id = $1 AND team_msg_id = $2`,
			e.ChannelInstanceID, e.TeamMsgID).Scan(&id); err != nil {
			return "", false, fmt.Errorf("conflict lookup: %w", err)
		}
	} else if err != nil {
		return "", false, fmt.Errorf("insert eval: %w", err)
	}

	if !wasNew {
		if err := tx.Commit(); err != nil {
			return "", false, fmt.Errorf("commit: %w", err)
		}
		return id, false, nil
	}

	msgJSON, jerr := json.Marshal(msg)
	if jerr != nil {
		return "", false, fmt.Errorf("marshal message: %w", jerr)
	}
	res, err := tx.ExecContext(ctx,
		`UPDATE sessions
		    SET messages   = COALESCE(messages, '[]'::jsonb) || $1::jsonb,
		        updated_at = NOW()
		  WHERE session_key = $2 AND tenant_id = $3`,
		string(msgJSON), sessionKey, e.TenantID)
	if err != nil {
		return "", false, fmt.Errorf("append session message: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return id, true, nil
	}

	if err := tx.Commit(); err != nil {
		return "", false, fmt.Errorf("commit: %w", err)
	}
	if w.sessions != nil {
		w.sessions.invalidateCache(ctx, sessionKey)
	}
	return id, true, nil
}
