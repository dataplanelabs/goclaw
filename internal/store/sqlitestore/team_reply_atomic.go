//go:build sqlite || sqliteonly

package sqlitestore

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/providers"
	"github.com/nextlevelbuilder/goclaw/internal/store"
)

type SQLiteTeamReplyAtomicWriter struct {
	db       *sql.DB
	sessions *SQLiteSessionStore
}

func NewSQLiteTeamReplyAtomicWriter(db *sql.DB, sessions *SQLiteSessionStore) *SQLiteTeamReplyAtomicWriter {
	return &SQLiteTeamReplyAtomicWriter{db: db, sessions: sessions}
}

var _ store.AtomicTeamReplyWriter = (*SQLiteTeamReplyAtomicWriter)(nil)

func (w *SQLiteTeamReplyAtomicWriter) WriteTeamReplyAtomic(ctx context.Context, e store.TeamReplyEvaluation, sessionKey string, msg providers.Message) (string, bool, error) {
	tx, err := w.db.BeginTx(ctx, nil)
	if err != nil {
		return "", false, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	id := e.ID
	if id == "" {
		id = uuid.NewString()
	}
	_, err = tx.ExecContext(ctx,
		`INSERT INTO team_reply_evaluations
		   (id, channel_instance_id, tenant_id, thread_key, session_key, team_msg_id,
		    captured_at, customer_message, team_reply)
		 VALUES (?,?,?,?,?,?,?,?,?)
		 ON CONFLICT(channel_instance_id, team_msg_id) DO NOTHING`,
		id, e.ChannelInstanceID, e.TenantID, e.ThreadKey, e.SessionKey, e.TeamMsgID,
		e.CapturedAt, e.CustomerMessage, e.TeamReply)
	if err != nil {
		return "", false, fmt.Errorf("insert eval: %w", err)
	}
	var winnerID string
	if err := tx.QueryRowContext(ctx,
		`SELECT id FROM team_reply_evaluations WHERE channel_instance_id = ? AND team_msg_id = ?`,
		e.ChannelInstanceID, e.TeamMsgID).Scan(&winnerID); err != nil {
		return "", false, fmt.Errorf("lookup inserted id: %w", err)
	}
	wasNew := winnerID == id
	id = winnerID

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
	var raw sql.NullString
	err = tx.QueryRowContext(ctx,
		`SELECT messages FROM sessions WHERE session_key = ? AND tenant_id = ?`,
		sessionKey, e.TenantID).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		if err := tx.Commit(); err != nil {
			return "", false, fmt.Errorf("commit: %w", err)
		}
		return id, true, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("load session messages: %w", err)
	}

	var arr []json.RawMessage
	if raw.Valid && raw.String != "" {
		_ = json.Unmarshal([]byte(raw.String), &arr)
	}
	arr = append(arr, msgJSON)
	merged, _ := json.Marshal(arr)
	if _, err := tx.ExecContext(ctx,
		`UPDATE sessions SET messages = ?, updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
		  WHERE session_key = ? AND tenant_id = ?`,
		string(merged), sessionKey, e.TenantID); err != nil {
		return "", false, fmt.Errorf("update session messages: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return "", false, fmt.Errorf("commit: %w", err)
	}
	if w.sessions != nil {
		w.sessions.invalidateCache(ctx, sessionKey)
	}
	return id, true, nil
}
