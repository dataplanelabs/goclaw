package vieneu

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/nextlevelbuilder/goclaw/internal/audio"
)

// Provider implements audio.TTSProvider + audio.VoiceListProvider + audio.DescribableProvider
// against the in-pod Python daemon at cfg.Endpoint (default http://127.0.0.1:7333).
type Provider struct {
	cfg    Config
	client *client

	voicesMu sync.RWMutex
	voices   []audio.Voice
}

func NewProvider(cfg Config) *Provider {
	cfg = cfg.withDefaults()
	return &Provider{
		cfg:    cfg,
		client: newClient(cfg.Endpoint, cfg.TimeoutMs),
	}
}

func (p *Provider) Name() string { return "vieneu" }

type synthRequest struct {
	Text         string  `json:"text"`
	VoiceID      string  `json:"voice_id,omitempty"`
	RefAudioPath string  `json:"ref_audio_path,omitempty"`
	RefText      string  `json:"ref_text,omitempty"`
	Format       string  `json:"format"`
	Speed        float64 `json:"speed"`
	Emotion      string  `json:"emotion"`
	Mode         string  `json:"mode"`
}

func (p *Provider) Synthesize(ctx context.Context, text string, opts audio.TTSOptions) (*audio.SynthResult, error) {
	if strings.TrimSpace(text) == "" {
		return nil, fmt.Errorf("%w: empty text", ErrSynthFailed)
	}

	voice := opts.Voice
	if voice == "" {
		voice = p.cfg.VoiceID
	}
	model := opts.Model
	if model == "" {
		model = p.cfg.Model
	}
	format := opts.Format
	if format == "" {
		format = "mp3"
	}

	speed := 1.0
	emotion := p.cfg.Emotion
	var refAudioPath, refText string
	if opts.Params != nil {
		if v, ok := audio.GetNested(opts.Params, "speed"); ok {
			speed = toFloat(v, 1.0)
		}
		if v, ok := audio.GetNested(opts.Params, "emotion"); ok {
			if s, ok := v.(string); ok && s != "" {
				emotion = s
			}
		}
		if v, ok := audio.GetNested(opts.Params, "ref_audio_path"); ok {
			if s, ok := v.(string); ok {
				refAudioPath = s
			}
		}
		if v, ok := audio.GetNested(opts.Params, "ref_text"); ok {
			if s, ok := v.(string); ok {
				refText = s
			}
		}
	}
	if refAudioPath != "" && refText == "" {
		return nil, fmt.Errorf("%w: ref_text required when ref_audio_path set", ErrRefAudioInvalid)
	}

	req := synthRequest{
		Text:         text,
		VoiceID:      voice,
		RefAudioPath: refAudioPath,
		RefText:      refText,
		Format:       format,
		Speed:        speed,
		Emotion:      emotion,
		Mode:         model,
	}
	body, hdr, err := p.client.postJSON(ctx, "/synthesize", req)
	if err != nil {
		if errors.Is(err, ErrDaemonUnreachable) {
			return nil, err
		}
		return nil, fmt.Errorf("%w: %v", ErrSynthFailed, err)
	}

	ext, mime := extensionForFormat(format)
	if ct := hdr.Get("Content-Type"); ct != "" {
		mime = ct
	}
	return &audio.SynthResult{Audio: body, Extension: ext, MimeType: mime}, nil
}

func extensionForFormat(fmt string) (ext, mime string) {
	switch fmt {
	case "wav":
		return "wav", "audio/wav"
	case "m4a":
		return "m4a", "audio/mp4"
	case "opus":
		return "ogg", "audio/ogg"
	default:
		return "mp3", "audio/mpeg"
	}
}

func toFloat(v any, fallback float64) float64 {
	switch x := v.(type) {
	case float64:
		return x
	case float32:
		return float64(x)
	case int:
		return float64(x)
	case int64:
		return float64(x)
	}
	return fallback
}
