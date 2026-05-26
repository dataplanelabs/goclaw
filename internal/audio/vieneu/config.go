package vieneu

import (
	"context"

	"github.com/google/uuid"
)

const (
	defaultEndpoint  = "http://127.0.0.1:7333"
	defaultVoiceID   = "Ly"
	defaultModel     = "standard"
	defaultEmotion   = "natural"
	defaultTimeoutMs = 30000
)

// ClonedVoiceLookup resolves a "cloned:<id>" voice to its on-disk reference
// audio path + transcription, scoped by tenant. The interface lets the
// provider remain free of import cycles to internal/store + refstore.
type ClonedVoiceLookup interface {
	// Get returns (refAudioPath, refText, name, found). found=false → caller
	// surfaces a "not found" error.
	Get(ctx context.Context, tenantID uuid.UUID, voiceID string) (refAudioPath, refText, name string, found bool, err error)
	// List returns the tenant's cloned voices as audio.Voice entries with
	// Category="cloned". May return empty slice + nil error.
	List(ctx context.Context, tenantID uuid.UUID) ([]ClonedVoiceListItem, error)
}

// ClonedVoiceListItem is the minimal shape voices.go uses to merge cloned
// voices into the preset response. Kept local so internal/audio/vieneu does
// not depend on internal/store types.
type ClonedVoiceListItem struct {
	VoiceID string
	Name    string
}

type Config struct {
	Endpoint     string
	VoiceID      string
	Model        string
	Emotion      string
	TimeoutMs    int
	ClonedVoices ClonedVoiceLookup // optional; nil = no cloning support
}

func (c Config) withDefaults() Config {
	if c.Endpoint == "" {
		c.Endpoint = defaultEndpoint
	}
	if c.VoiceID == "" {
		c.VoiceID = defaultVoiceID
	}
	if c.Model == "" {
		c.Model = defaultModel
	}
	if c.Emotion == "" {
		c.Emotion = defaultEmotion
	}
	if c.TimeoutMs <= 0 {
		c.TimeoutMs = defaultTimeoutMs
	}
	return c
}
