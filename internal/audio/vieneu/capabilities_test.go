package vieneu

import (
	"testing"

	"github.com/nextlevelbuilder/goclaw/internal/audio"
)

func TestCapabilities_ProviderShape(t *testing.T) {
	caps := NewProvider(Config{}).Capabilities()
	if caps.Provider != "vieneu" {
		t.Errorf("Provider = %q", caps.Provider)
	}
	if caps.RequiresAPIKey {
		t.Error("VieNeu should not require an API key")
	}
	wantModels := map[string]bool{"standard": true, "turbo": true}
	if len(caps.Models) != len(wantModels) {
		t.Errorf("Models = %v", caps.Models)
	}
	for _, m := range caps.Models {
		if !wantModels[m] {
			t.Errorf("unexpected model %q", m)
		}
	}
	if v, ok := caps.CustomFeatures["voices_dynamic"].(bool); !ok || !v {
		t.Error("CustomFeatures.voices_dynamic should be true")
	}
	if v, ok := caps.CustomFeatures["voice_cloning"].(bool); !ok || !v {
		t.Error("CustomFeatures.voice_cloning should be true")
	}
}

func TestCapabilities_ParamsInvariant(t *testing.T) {
	caps := NewProvider(Config{}).Capabilities()
	for _, p := range caps.Params {
		switch p.Type {
		case audio.ParamTypeRange, audio.ParamTypeNumber, audio.ParamTypeInteger:
			if p.Default == nil {
				t.Errorf("param %q numeric type missing Default", p.Key)
				continue
			}
			d := paramDefaultFloat(p.Default)
			if p.Min != nil && d < *p.Min {
				t.Errorf("param %q default %v < Min %v", p.Key, d, *p.Min)
			}
			if p.Max != nil && d > *p.Max {
				t.Errorf("param %q default %v > Max %v", p.Key, d, *p.Max)
			}
		case audio.ParamTypeEnum:
			if len(p.Enum) == 0 {
				t.Errorf("param %q enum has no options", p.Key)
			}
			if p.Default == nil {
				continue
			}
			defStr, ok := p.Default.(string)
			if !ok {
				t.Errorf("param %q enum default not string: %T", p.Key, p.Default)
				continue
			}
			found := false
			for _, opt := range p.Enum {
				if opt.Value == defStr {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("param %q default %q not in enum", p.Key, defStr)
			}
		}
	}
}

func paramDefaultFloat(v any) float64 {
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
	return 0
}

// Compile-time interface check.
var (
	_ audio.TTSProvider         = (*Provider)(nil)
	_ audio.VoiceListProvider   = (*Provider)(nil)
	_ audio.DescribableProvider = (*Provider)(nil)
)
