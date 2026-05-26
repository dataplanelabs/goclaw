package vieneu

const (
	defaultEndpoint  = "http://127.0.0.1:7333"
	defaultVoiceID   = "truc_ly"
	defaultModel     = "standard"
	defaultEmotion   = "natural"
	defaultTimeoutMs = 30000
)

type Config struct {
	Endpoint  string
	VoiceID   string
	Model     string
	Emotion   string
	TimeoutMs int
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
