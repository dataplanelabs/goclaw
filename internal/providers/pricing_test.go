package providers

import (
	"math"
	"testing"
)

func TestCostUSD(t *testing.T) {
	cases := []struct {
		name     string
		provider string
		model    string
		in       int64
		out      int64
		want     float64
	}{
		{"subscription glm flat-fee", "zai-coding", "glm-5.1", 1_000_000, 1_000_000, 0},
		{"subscription codex provider substring", "openai-codex", "codex-work", 500_000, 500_000, 0},
		{"subscription zai substring match", "zai", "anything-unknown", 1_000_000, 1_000_000, 0},
		{"metered anthropic exact sonnet", "anthropic", "claude-sonnet-4-6", 1_000_000, 1_000_000, 18.00},
		{"metered anthropic opus", "anthropic", "claude-opus-4-6", 2_000_000, 1_000_000, 105.00},
		{"metered dashscope qwen", "dashscope", "qwen3.6-plus", 1_000_000, 1_000_000, 1.60},
		{"case-insensitive model id", "ANTHROPIC", "Claude-Sonnet-4-6", 1_000_000, 0, 3.00},
		{"provider default fallback (unknown model, known provider)", "anthropic", "claude-future-99", 1_000_000, 0, 3.00},
		{"openrouter provider default", "openrouter", "some/routed-model", 1_000_000, 1_000_000, 4.00},
		{"unknown provider and model", "mystery", "totally-unknown", 1_000_000, 1_000_000, 0},
		{"empty provider and model", "", "", 1_000_000, 1_000_000, 0},
		{"partial tokens math", "anthropic", "claude-sonnet-4-6", 1_500, 500, (1500*3.0 + 500*15.0) / 1e6},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := CostUSD(tc.provider, tc.model, tc.in, tc.out)
			if math.Abs(got-tc.want) > 1e-9 {
				t.Fatalf("CostUSD(%q,%q,%d,%d) = %v, want %v",
					tc.provider, tc.model, tc.in, tc.out, got, tc.want)
			}
		})
	}
}

func TestCostUSDProviderModelFallbackKey(t *testing.T) {
	// "provider/model" fallback key resolves when the bare model id is absent.
	loadPricing()
	pricingTable["openrouter/special-model"] = PricingRate{InputPer1M: 2.0, OutputPer1M: 6.0}
	t.Cleanup(func() { delete(pricingTable, "openrouter/special-model") })

	got := CostUSD("openrouter", "special-model", 1_000_000, 1_000_000)
	if math.Abs(got-8.0) > 1e-9 {
		t.Fatalf("provider/model fallback = %v, want 8.0", got)
	}
}

func TestApplyPricingOverride(t *testing.T) {
	loadPricing()
	pricingTable["override-model"] = PricingRate{} // ensure clean slate
	t.Cleanup(func() { delete(pricingTable, "override-model") })

	applyPricingOverride(`{"models":{"override-model":{"input_per_1m":10,"output_per_1m":20}}}`)
	got := CostUSD("", "override-model", 1_000_000, 1_000_000)
	if math.Abs(got-30.0) > 1e-9 {
		t.Fatalf("override CostUSD = %v, want 30.0", got)
	}
}
