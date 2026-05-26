package vieneu

import "github.com/nextlevelbuilder/goclaw/internal/audio"

var (
	speedMin  = 0.5
	speedMax  = 2.0
	speedStep = 0.1
)

var vieneuParams = []audio.ParamSchema{
	{
		Key:                "speed",
		Type:               audio.ParamTypeRange,
		Label:              "Speed",
		Description:        "Speech tempo multiplier (0.5 = half speed, 2.0 = double).",
		Default:            1.0,
		Min:                &speedMin,
		Max:                &speedMax,
		Step:               &speedStep,
		AgentOverridableAs: "speed",
	},
	{
		Key:         "emotion",
		Type:        audio.ParamTypeEnum,
		Label:       "Emotion",
		Description: "Delivery style.",
		Default:     "natural",
		Enum: []audio.EnumOption{
			{Value: "natural", Label: "Natural"},
			{Value: "storytelling", Label: "Storytelling"},
		},
		AgentOverridableAs: "emotion",
	},
	{
		Key:         "format",
		Type:        audio.ParamTypeEnum,
		Label:       "Output format",
		Description: "Audio container. opus (ogg) is preferred for Telegram voice bubble; mp3 is the safe default.",
		Default:     "mp3",
		Enum: []audio.EnumOption{
			{Value: "mp3", Label: "MP3"},
			{Value: "opus", Label: "Opus (OGG)"},
			{Value: "m4a", Label: "M4A"},
			{Value: "wav", Label: "WAV (no transcode)"},
		},
	},
}

func (p *Provider) Capabilities() audio.ProviderCapabilities {
	return audio.ProviderCapabilities{
		Provider:       "vieneu",
		DisplayName:    "VieNeu Vietnamese TTS",
		RequiresAPIKey: false,
		Models:         []string{"standard", "turbo"},
		Voices:         nil, // dynamic via ListVoices
		Params:         vieneuParams,
		CustomFeatures: map[string]any{
			"voices_dynamic": true,
			"voice_cloning":  true,
			"language":       "vi",
		},
	}
}
