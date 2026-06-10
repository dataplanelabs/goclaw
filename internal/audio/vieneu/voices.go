package vieneu

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/nextlevelbuilder/goclaw/internal/audio"
	"github.com/nextlevelbuilder/goclaw/internal/store"
)

type voiceDTO struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Language string `json:"language"`
	Gender   string `json:"gender,omitempty"`
	Accent   string `json:"accent,omitempty"`
}

type voicesResponse struct {
	Voices []voiceDTO `json:"voices"`
}

func (p *Provider) ListVoices(ctx context.Context) ([]audio.Voice, error) {
	p.voicesMu.RLock()
	cached := p.voices
	p.voicesMu.RUnlock()
	if cached != nil {
		return cached, nil
	}

	body, err := p.client.getJSON(ctx, "/voices")
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrVoicesFetchFailed, err)
	}
	var dto voicesResponse
	if err := json.Unmarshal(body, &dto); err != nil {
		return nil, fmt.Errorf("%w: parse: %v", ErrVoicesFetchFailed, err)
	}
	out := make([]audio.Voice, 0, len(dto.Voices))
	for _, v := range dto.Voices {
		labels := map[string]string{"language": v.Language}
		if v.Gender != "" {
			labels["gender"] = v.Gender
		}
		if v.Accent != "" {
			labels["accent"] = v.Accent
		}
		out = append(out, audio.Voice{
			ID:       v.ID,
			Name:     v.Name,
			Labels:   labels,
			Category: "premade",
		})
	}
	p.voicesMu.Lock()
	p.voices = out
	p.voicesMu.Unlock()

	// Cloned voices are tenant-scoped — merge on each call, do NOT cache.
	if p.cfg.ClonedVoices != nil {
		tenantID := store.TenantIDFromContext(ctx)
		cloned, err := p.cfg.ClonedVoices.List(ctx, tenantID)
		if err != nil {
			slog.Warn("vieneu: cloned voices list failed", "err", err)
		} else {
			merged := make([]audio.Voice, 0, len(out)+len(cloned))
			merged = append(merged, out...)
			for _, c := range cloned {
				merged = append(merged, audio.Voice{
					ID:       c.VoiceID,
					Name:     c.Name,
					Labels:   map[string]string{"language": "vi"},
					Category: "cloned",
				})
			}
			return merged, nil
		}
	}
	return out, nil
}
