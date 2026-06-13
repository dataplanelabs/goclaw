package pg

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/store"
)

// PGPendingMessageStore implements store.PendingMessageStore backed by Postgres.
type PGPendingMessageStore struct {
	db *sql.DB
}

// NewPGPendingMessageStore creates a new PGPendingMessageStore.
func NewPGPendingMessageStore(db *sql.DB) *PGPendingMessageStore {
	return &PGPendingMessageStore{db: db}
}

func (s *PGPendingMessageStore) AppendBatch(ctx context.Context, msgs []store.PendingMessage) error {
	if len(msgs) == 0 {
		return nil
	}

	// Build multi-row INSERT: VALUES ($1,$2,...,$12), ($13,...,$24), ...
	const cols = 12
	placeholders := make([]string, len(msgs))
	args := make([]any, 0, len(msgs)*cols)
	now := time.Now()
	tid := tenantIDForInsert(ctx)

	for i := range msgs {
		if msgs[i].ID == uuid.Nil {
			msgs[i].ID = uuid.Must(uuid.NewV7())
		}
		base := i * cols
		placeholders[i] = fmt.Sprintf("($%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d)",
			base+1, base+2, base+3, base+4, base+5, base+6, base+7, base+8, base+9, base+10, base+11, base+12)
		var mediaPaths any
		if len(msgs[i].MediaPaths) > 0 {
			if b, err := json.Marshal(msgs[i].MediaPaths); err == nil {
				mediaPaths = string(b)
			}
		}
		args = append(args, msgs[i].ID, msgs[i].ChannelName, msgs[i].HistoryKey,
			msgs[i].Sender, msgs[i].SenderID, msgs[i].Body, msgs[i].PlatformMsgID, msgs[i].IsSummary, now, now, tid, mediaPaths)
	}

	_, err := s.db.ExecContext(ctx,
		`INSERT INTO channel_pending_messages (id, channel_name, history_key, sender, sender_id, body, platform_msg_id, is_summary, created_at, updated_at, tenant_id, media_paths)
		 VALUES `+strings.Join(placeholders, ","),
		args...,
	)
	return err
}

func (s *PGPendingMessageStore) ListByKey(ctx context.Context, channelName, historyKey string) ([]store.PendingMessage, error) {
	tClause, tArgs, _, err := scopeClause(ctx, 3)
	if err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, channel_name, history_key, sender, sender_id, body, platform_msg_id, is_summary, created_at, updated_at, media_paths
		 FROM channel_pending_messages
		 WHERE channel_name = $1 AND history_key = $2`+tClause+`
		 ORDER BY created_at ASC`,
		append([]any{channelName, historyKey}, tArgs...)...,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []store.PendingMessage
	for rows.Next() {
		var m store.PendingMessage
		var mediaPaths sql.NullString
		if err := rows.Scan(&m.ID, &m.ChannelName, &m.HistoryKey, &m.Sender, &m.SenderID,
			&m.Body, &m.PlatformMsgID, &m.IsSummary, &m.CreatedAt, &m.UpdatedAt, &mediaPaths); err != nil {
			return nil, err
		}
		if mediaPaths.Valid && mediaPaths.String != "" {
			_ = json.Unmarshal([]byte(mediaPaths.String), &m.MediaPaths)
		}
		result = append(result, m)
	}
	return result, rows.Err()
}

func (s *PGPendingMessageStore) DeleteByKey(ctx context.Context, channelName, historyKey string) error {
	tClause, tArgs, _, err := scopeClause(ctx, 3)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx,
		`DELETE FROM channel_pending_messages WHERE channel_name = $1 AND history_key = $2`+tClause,
		append([]any{channelName, historyKey}, tArgs...)...,
	)
	return err
}

func (s *PGPendingMessageStore) Compact(ctx context.Context, deleteIDs []uuid.UUID, summary *store.PendingMessage) error {
	if len(deleteIDs) == 0 {
		return nil
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin compact tx: %w", err)
	}
	defer tx.Rollback()

	// Build placeholder list for DELETE IN clause
	placeholders := make([]string, len(deleteIDs))
	args := make([]any, len(deleteIDs))
	for i, id := range deleteIDs {
		placeholders[i] = fmt.Sprintf("$%d", i+1)
		args[i] = id
	}

	res, err := tx.ExecContext(ctx,
		fmt.Sprintf("DELETE FROM channel_pending_messages WHERE id IN (%s)", strings.Join(placeholders, ",")),
		args...,
	)
	if err != nil {
		return fmt.Errorf("compact delete: %w", err)
	}

	// Guard: if another compaction already deleted these rows, skip summary insertion
	affected, _ := res.RowsAffected()
	if affected == 0 {
		return nil // already compacted by concurrent caller
	}

	// Insert summary row
	if summary.ID == uuid.Nil {
		summary.ID = uuid.Must(uuid.NewV7())
	}
	now := time.Now()
	_, err = tx.ExecContext(ctx,
		`INSERT INTO channel_pending_messages (id, channel_name, history_key, sender, sender_id, body, platform_msg_id, is_summary, created_at, updated_at, tenant_id)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`,
		summary.ID, summary.ChannelName, summary.HistoryKey, summary.Sender, summary.SenderID, summary.Body, summary.PlatformMsgID, true, now, now, tenantIDForInsert(ctx),
	)
	if err != nil {
		return fmt.Errorf("compact insert summary: %w", err)
	}

	return tx.Commit()
}

func (s *PGPendingMessageStore) DeleteStale(ctx context.Context, olderThan time.Duration) (int64, error) {
	cutoff := time.Now().Add(-olderThan)
	tid := tenantIDForInsert(ctx)
	result, err := s.db.ExecContext(ctx,
		`DELETE FROM channel_pending_messages WHERE updated_at < $1 AND tenant_id = $2`,
		cutoff, tid,
	)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func (s *PGPendingMessageStore) ListGroups(ctx context.Context) ([]store.PendingMessageGroup, error) {
	tClause, tArgs, _, err := scopeClause(ctx, 1)
	if err != nil {
		return nil, err
	}
	var where string
	if tClause != "" {
		where = ` WHERE m.tenant_id = $1`
	}
	var result []store.PendingMessageGroup
	err = pkgSqlxDB.SelectContext(ctx, &result,
		`SELECT channel_name, history_key,
		        COUNT(*) AS message_count,
		        BOOL_OR(is_summary)
		          AND NOT EXISTS (
		            SELECT 1 FROM channel_pending_messages n
		            WHERE n.channel_name = m.channel_name
		              AND n.history_key  = m.history_key
		              AND NOT n.is_summary
		              AND n.created_at > (
		                SELECT MAX(s.created_at)
		                FROM channel_pending_messages s
		                WHERE s.channel_name = m.channel_name
		                  AND s.history_key  = m.history_key
		                  AND s.is_summary
		              )
		          ) AS has_summary,
		        MAX(created_at) AS last_activity
		 FROM channel_pending_messages m`+where+`
		 GROUP BY channel_name, history_key
		 ORDER BY last_activity DESC`,
		tArgs...,
	)
	return result, err
}

func (s *PGPendingMessageStore) CountAll(ctx context.Context) (int64, error) {
	tClause, tArgs, _, err := scopeClause(ctx, 1)
	if err != nil {
		return 0, err
	}
	var count int64
	var query string
	if tClause != "" {
		query = `SELECT COUNT(*) FROM channel_pending_messages WHERE tenant_id = $1`
	} else {
		query = `SELECT COUNT(*) FROM channel_pending_messages`
	}
	err = s.db.QueryRowContext(ctx, query, tArgs...).Scan(&count)
	return count, err
}

func (s *PGPendingMessageStore) CountByKey(ctx context.Context, channelName, historyKey string) (int, error) {
	tClause, tArgs, _, err := scopeClause(ctx, 3)
	if err != nil {
		return 0, err
	}
	var count int
	err = s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM channel_pending_messages WHERE channel_name = $1 AND history_key = $2`+tClause,
		append([]any{channelName, historyKey}, tArgs...)...,
	).Scan(&count)
	return count, err
}

func (s *PGPendingMessageStore) ResolveGroupTitles(ctx context.Context, groups []store.PendingMessageGroup) (map[string]string, error) {
	if len(groups) == 0 {
		return nil, nil
	}

	// Build OR conditions: session_key LIKE '%:{channel}:group:{key}%'
	conditions := make([]string, 0, len(groups))
	args := make([]any, 0, len(groups)*2)
	for i, g := range groups {
		conditions = append(conditions, fmt.Sprintf(
			"(session_key LIKE '%%:' || $%d || ':group:' || $%d || '%%')",
			i*2+1, i*2+2,
		))
		args = append(args, g.ChannelName, g.HistoryKey)
	}

	tenantFilter := ""
	if !store.IsCrossTenant(ctx) {
		tid := store.TenantIDFromContext(ctx)
		if tid == uuid.Nil {
			tid = store.MasterTenantID
		}
		argIdx := len(args) + 1
		tenantFilter = fmt.Sprintf(" AND tenant_id = $%d", argIdx)
		args = append(args, tid)
	}

	rows, err := s.db.QueryContext(ctx,
		"SELECT session_key, metadata->>'chat_title'"+
			" FROM sessions"+
			" WHERE metadata->>'chat_title' != ''"+
			" AND ("+strings.Join(conditions, " OR ")+")"+tenantFilter,
		args...,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[string]string)
	for rows.Next() {
		var sessionKey, title string
		if err := rows.Scan(&sessionKey, &title); err != nil {
			return nil, err
		}
		// Match session_key back to channel:key pair
		for _, g := range groups {
			pattern := ":" + g.ChannelName + ":group:" + g.HistoryKey
			if strings.Contains(sessionKey, pattern) {
				mapKey := g.ChannelName + ":" + g.HistoryKey
				if _, exists := result[mapKey]; !exists {
					result[mapKey] = title
				}
				break
			}
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Fallback to channel_contacts for groups missing chat_title in session metadata.
	resolveFromContacts(ctx, s.db, groups, result)
	return result, nil
}

func resolveFromContacts(ctx context.Context, db *sql.DB, groups []store.PendingMessageGroup, result map[string]string) {
	if db == nil {
		return
	}
	type pair struct{ channelType, senderID string }
	var missing []pair
	for _, g := range groups {
		mapKey := g.ChannelName + ":" + g.HistoryKey
		if _, ok := result[mapKey]; !ok {
			missing = append(missing, pair{g.ChannelName, g.HistoryKey})
		}
	}
	if len(missing) == 0 {
		return
	}
	conds := make([]string, 0, len(missing))
	args := make([]any, 0, len(missing)*2+1)
	for i, p := range missing {
		conds = append(conds, fmt.Sprintf("(channel_type = $%d AND sender_id = $%d)", i*2+1, i*2+2))
		args = append(args, p.channelType, p.senderID)
	}
	tenantFilter := ""
	if !store.IsCrossTenant(ctx) {
		tid := store.TenantIDFromContext(ctx)
		if tid == uuid.Nil {
			tid = store.MasterTenantID
		}
		tenantFilter = fmt.Sprintf(" AND tenant_id = $%d", len(args)+1)
		args = append(args, tid)
	}
	q := `SELECT channel_type, sender_id, display_name FROM channel_contacts
	       WHERE contact_type = 'group' AND display_name IS NOT NULL AND display_name != ''
	         AND (` + strings.Join(conds, " OR ") + `)` + tenantFilter
	rows, err := db.QueryContext(ctx, q, args...)
	if err != nil {
		return
	}
	defer rows.Close()
	for rows.Next() {
		var ct, sid string
		var name *string
		if err := rows.Scan(&ct, &sid, &name); err != nil {
			return
		}
		if name == nil || *name == "" {
			continue
		}
		mapKey := ct + ":" + sid
		if _, exists := result[mapKey]; !exists {
			result[mapKey] = *name
		}
	}
}

// ListReferencedMediaPaths returns durable media paths still referenced by any
// pending message (all tenants) — a system-level GC query, intentionally unscoped.
func (s *PGPendingMessageStore) ListReferencedMediaPaths(ctx context.Context) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT media_paths FROM channel_pending_messages WHERE media_paths IS NOT NULL`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var mp sql.NullString
		if err := rows.Scan(&mp); err != nil {
			return nil, err
		}
		if !mp.Valid || mp.String == "" {
			continue
		}
		var paths []string
		if json.Unmarshal([]byte(mp.String), &paths) == nil {
			out = append(out, paths...)
		}
	}
	return out, rows.Err()
}
