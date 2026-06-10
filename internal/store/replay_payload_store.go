package store

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// MaxReplayPayloadBytes caps capture at 2 MiB; over-cap rows store a sentinel.
const MaxReplayPayloadBytes = 2 << 20

// CurrentReplayPayloadVersion stamps the envelope so future shape changes
// can be skip+logged instead of crashing the decoder.
const CurrentReplayPayloadVersion = 1

type RunRequestEnvelope struct {
	Version  int             `json:"v"`
	Captured time.Time       `json:"captured_at"`
	Payload  json.RawMessage `json:"payload"`
}

type ReplayPayloadRow struct {
	TraceID    uuid.UUID `json:"trace_id" db:"trace_id"`
	TenantID   uuid.UUID `json:"tenant_id" db:"tenant_id"`
	SessionKey string    `json:"session_key" db:"session_key"`
	Payload    []byte    `json:"payload,omitempty" db:"payload"`
	Version    int       `json:"payload_version" db:"payload_version"`
	Oversize   bool      `json:"oversize" db:"oversize"`
	ByteSize   int       `json:"byte_size" db:"byte_size"`
	CreatedAt  time.Time `json:"created_at" db:"created_at"`
}

// ReplayPayloadStore captures inbound RunRequest envelopes; Capture is
// best-effort so a write outage cannot drop a user message.
type ReplayPayloadStore interface {
	Capture(ctx context.Context, traceID uuid.UUID, sessionKey string, envelope []byte) error
	CaptureOversize(ctx context.Context, traceID uuid.UUID, sessionKey string, byteSize int) error
	Get(ctx context.Context, traceID uuid.UUID) (*ReplayPayloadRow, error)
	DropForSession(ctx context.Context, sessionKey string, before time.Time) (int, error)
}
