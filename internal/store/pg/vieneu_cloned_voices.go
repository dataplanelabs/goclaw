package pg

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/store"
)

type PGVieneuClonedVoicesStore struct {
	db *sql.DB
}

func NewPGVieneuClonedVoicesStore(db *sql.DB) *PGVieneuClonedVoicesStore {
	return &PGVieneuClonedVoicesStore{db: db}
}

func (s *PGVieneuClonedVoicesStore) List(ctx context.Context, tenantID uuid.UUID) ([]store.VieneuClonedVoice, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, tenant_id, voice_id, ref_text, name, created_at
		 FROM vieneu_cloned_voices
		 WHERE tenant_id = $1 AND deleted_at IS NULL
		 ORDER BY created_at DESC`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("vieneu cloned voices list: %w", err)
	}
	defer rows.Close()

	var out []store.VieneuClonedVoice
	for rows.Next() {
		var v store.VieneuClonedVoice
		if err := rows.Scan(&v.ID, &v.TenantID, &v.VoiceID, &v.RefText, &v.Name, &v.CreatedAt); err != nil {
			return nil, fmt.Errorf("vieneu cloned voices scan: %w", err)
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (s *PGVieneuClonedVoicesStore) Get(ctx context.Context, tenantID uuid.UUID, voiceID string) (*store.VieneuClonedVoice, error) {
	var v store.VieneuClonedVoice
	err := s.db.QueryRowContext(ctx,
		`SELECT id, tenant_id, voice_id, ref_text, name, created_at
		 FROM vieneu_cloned_voices
		 WHERE tenant_id = $1 AND voice_id = $2 AND deleted_at IS NULL`,
		tenantID, voiceID).
		Scan(&v.ID, &v.TenantID, &v.VoiceID, &v.RefText, &v.Name, &v.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("vieneu cloned voice get: %w", err)
	}
	return &v, nil
}

func (s *PGVieneuClonedVoicesStore) Insert(ctx context.Context, v store.VieneuClonedVoice) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO vieneu_cloned_voices (id, tenant_id, voice_id, ref_text, name, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		v.ID, v.TenantID, v.VoiceID, v.RefText, v.Name, v.CreatedAt)
	if err != nil {
		return fmt.Errorf("vieneu cloned voice insert: %w", err)
	}
	return nil
}

func (s *PGVieneuClonedVoicesStore) Delete(ctx context.Context, tenantID uuid.UUID, voiceID string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE vieneu_cloned_voices
		 SET deleted_at = NOW()
		 WHERE tenant_id = $1 AND voice_id = $2 AND deleted_at IS NULL`,
		tenantID, voiceID)
	if err != nil {
		return fmt.Errorf("vieneu cloned voice delete: %w", err)
	}
	return nil
}

func (s *PGVieneuClonedVoicesStore) Count(ctx context.Context, tenantID uuid.UUID) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM vieneu_cloned_voices
		 WHERE tenant_id = $1 AND deleted_at IS NULL`, tenantID).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("vieneu cloned voice count: %w", err)
	}
	return n, nil
}

// Compile-time interface check.
var _ store.VieneuClonedVoicesStore = (*PGVieneuClonedVoicesStore)(nil)
