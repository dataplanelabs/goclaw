//go:build sqlite || sqliteonly

package sqlitestore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/store"
)

const sqliteTimeFormat = "2006-01-02T15:04:05.000Z"

type SQLiteVieneuClonedVoicesStore struct {
	db *sql.DB
}

func NewSQLiteVieneuClonedVoicesStore(db *sql.DB) *SQLiteVieneuClonedVoicesStore {
	return &SQLiteVieneuClonedVoicesStore{db: db}
}

func (s *SQLiteVieneuClonedVoicesStore) List(ctx context.Context, tenantID uuid.UUID) ([]store.VieneuClonedVoice, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, tenant_id, voice_id, ref_text, name, created_at
		 FROM vieneu_cloned_voices
		 WHERE tenant_id = ? AND deleted_at IS NULL
		 ORDER BY created_at DESC`, tenantID.String())
	if err != nil {
		return nil, fmt.Errorf("vieneu cloned voices list: %w", err)
	}
	defer rows.Close()

	var out []store.VieneuClonedVoice
	for rows.Next() {
		v, err := scanClonedVoice(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (s *SQLiteVieneuClonedVoicesStore) Get(ctx context.Context, tenantID uuid.UUID, voiceID string) (*store.VieneuClonedVoice, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, tenant_id, voice_id, ref_text, name, created_at
		 FROM vieneu_cloned_voices
		 WHERE tenant_id = ? AND voice_id = ? AND deleted_at IS NULL`,
		tenantID.String(), voiceID)
	v, err := scanClonedVoice(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &v, nil
}

func (s *SQLiteVieneuClonedVoicesStore) Insert(ctx context.Context, v store.VieneuClonedVoice) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO vieneu_cloned_voices (id, tenant_id, voice_id, ref_text, name, created_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		v.ID.String(), v.TenantID.String(), v.VoiceID, v.RefText, v.Name,
		v.CreatedAt.UTC().Format(sqliteTimeFormat))
	if err != nil {
		return fmt.Errorf("vieneu cloned voice insert: %w", err)
	}
	return nil
}

func (s *SQLiteVieneuClonedVoicesStore) Delete(ctx context.Context, tenantID uuid.UUID, voiceID string) error {
	now := time.Now().UTC().Format(sqliteTimeFormat)
	_, err := s.db.ExecContext(ctx,
		`UPDATE vieneu_cloned_voices SET deleted_at = ?
		 WHERE tenant_id = ? AND voice_id = ? AND deleted_at IS NULL`,
		now, tenantID.String(), voiceID)
	if err != nil {
		return fmt.Errorf("vieneu cloned voice delete: %w", err)
	}
	return nil
}

func (s *SQLiteVieneuClonedVoicesStore) Count(ctx context.Context, tenantID uuid.UUID) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM vieneu_cloned_voices
		 WHERE tenant_id = ? AND deleted_at IS NULL`, tenantID.String()).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("vieneu cloned voice count: %w", err)
	}
	return n, nil
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanClonedVoice(r rowScanner) (store.VieneuClonedVoice, error) {
	var v store.VieneuClonedVoice
	var idStr, tenantStr, createdStr string
	if err := r.Scan(&idStr, &tenantStr, &v.VoiceID, &v.RefText, &v.Name, &createdStr); err != nil {
		return v, err
	}
	id, err := uuid.Parse(idStr)
	if err != nil {
		return v, fmt.Errorf("parse id: %w", err)
	}
	tid, err := uuid.Parse(tenantStr)
	if err != nil {
		return v, fmt.Errorf("parse tenant_id: %w", err)
	}
	t, err := time.Parse(sqliteTimeFormat, createdStr)
	if err != nil {
		// Try RFC3339Nano fallback for older rows.
		if t2, err2 := time.Parse(time.RFC3339Nano, createdStr); err2 == nil {
			t = t2
		} else {
			return v, fmt.Errorf("parse created_at: %w", err)
		}
	}
	v.ID = id
	v.TenantID = tid
	v.CreatedAt = t
	return v, nil
}

var _ store.VieneuClonedVoicesStore = (*SQLiteVieneuClonedVoicesStore)(nil)
