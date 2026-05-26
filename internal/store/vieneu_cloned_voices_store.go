package store

import (
	"context"
	"time"

	"github.com/google/uuid"
)

type VieneuClonedVoice struct {
	ID        uuid.UUID `json:"id" db:"id"`
	TenantID  uuid.UUID `json:"tenant_id" db:"tenant_id"`
	VoiceID   string    `json:"voice_id" db:"voice_id"`
	RefText   string    `json:"ref_text" db:"ref_text"`
	Name      string    `json:"name" db:"name"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
}

type VieneuClonedVoicesStore interface {
	List(ctx context.Context, tenantID uuid.UUID) ([]VieneuClonedVoice, error)
	Get(ctx context.Context, tenantID uuid.UUID, voiceID string) (*VieneuClonedVoice, error)
	Insert(ctx context.Context, v VieneuClonedVoice) error
	Delete(ctx context.Context, tenantID uuid.UUID, voiceID string) error
	Count(ctx context.Context, tenantID uuid.UUID) (int, error)
}
