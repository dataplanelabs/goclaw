package vieneu

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/nextlevelbuilder/goclaw/internal/audio"
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
	return out, nil
}
