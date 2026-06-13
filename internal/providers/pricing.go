package providers

import (
	"encoding/json"
	"os"
	"strings"
	"sync"
)

// PricingRate is per-1M-token list pricing in USD.
type PricingRate struct {
	InputPer1M  float64 `json:"input_per_1m"`
	OutputPer1M float64 `json:"output_per_1m"`
}

// defaultPricing maps a normalized lookup key → rate. Keys are lowercased.
// Two key shapes are supported: bare model id (e.g. "claude-sonnet-4-6") and
// the "provider/model" fallback (e.g. "anthropic/*" via providerDefaultPricing).
//
// ADJUST THESE — public list prices as of 2026-06, verify against your billing.
// Subscription / flat-fee providers are deliberately 0 (not metered per token).
var defaultPricing = map[string]PricingRate{
	// --- Anthropic (metered, USD per 1M tokens) ---
	"claude-opus-4-6":           {InputPer1M: 15.00, OutputPer1M: 75.00},
	"claude-opus-4-1":           {InputPer1M: 15.00, OutputPer1M: 75.00},
	"claude-sonnet-4-6":         {InputPer1M: 3.00, OutputPer1M: 15.00},
	"claude-sonnet-4-5":         {InputPer1M: 3.00, OutputPer1M: 15.00},
	"claude-haiku-4-5":          {InputPer1M: 1.00, OutputPer1M: 5.00},
	"claude-haiku-4-5-20251001": {InputPer1M: 1.00, OutputPer1M: 5.00},
	"claude-3-5-haiku":          {InputPer1M: 0.80, OutputPer1M: 4.00},

	// --- DashScope / Qwen (metered) ---
	"qwen3.6-plus": {InputPer1M: 0.40, OutputPer1M: 1.20},
	"qwen-plus":    {InputPer1M: 0.40, OutputPer1M: 1.20},
	"qwen-max":     {InputPer1M: 1.60, OutputPer1M: 6.40},
	"qwen-turbo":   {InputPer1M: 0.05, OutputPer1M: 0.20},

	// --- Gemini (metered) ---
	"gemini-2.5-pro":   {InputPer1M: 1.25, OutputPer1M: 10.00},
	"gemini-2.5-flash": {InputPer1M: 0.30, OutputPer1M: 2.50},

	// --- Subscription / flat-fee (NOT metered per token → 0) ---
	"glm-5.1":     {},
	"glm-5-turbo": {},
	"codex-work":  {},
	"codex-test":  {},
}

// providerDefaultPricing is the fallback rate when an exact model id is not in
// defaultPricing. Keyed by lowercased provider name.
//
// ADJUST THESE — public list prices as of 2026-06, verify against your billing.
var providerDefaultPricing = map[string]PricingRate{
	"anthropic": {InputPer1M: 3.00, OutputPer1M: 15.00}, // sonnet-tier default
	"dashscope": {InputPer1M: 0.40, OutputPer1M: 1.20},  // qwen-plus tier
	"gemini":    {InputPer1M: 0.30, OutputPer1M: 2.50},  // flash tier
	// OpenRouter is per-underlying-model; this is a sane non-zero placeholder.
	// Verify against the specific routed model in your billing.
	"openrouter": {InputPer1M: 1.00, OutputPer1M: 3.00},

	// Subscription / flat-fee providers → 0 (correct, not metered per token):
	"zai":          {},
	"zai-coding":   {},
	"codex":        {},
	"chatgpt":      {},
	"openai-codex": {},
}

var (
	pricingOnce  sync.Once
	pricingTable map[string]PricingRate
	pricingDefs  map[string]PricingRate
)

// GOCLAW_PRICING_OVERRIDE_JSON optionally overrides/extends the in-code table.
// Shape: {"models":{"<model>":{"input_per_1m":N,"output_per_1m":N}},
//
//	"providers":{"<provider>":{...}}}
const pricingOverrideEnv = "GOCLAW_PRICING_OVERRIDE_JSON"

func loadPricing() {
	pricingOnce.Do(func() {
		pricingTable = make(map[string]PricingRate, len(defaultPricing))
		for k, v := range defaultPricing {
			pricingTable[strings.ToLower(k)] = v
		}
		pricingDefs = make(map[string]PricingRate, len(providerDefaultPricing))
		for k, v := range providerDefaultPricing {
			pricingDefs[strings.ToLower(k)] = v
		}
		applyPricingOverride(os.Getenv(pricingOverrideEnv))
	})
}

func applyPricingOverride(raw string) {
	if strings.TrimSpace(raw) == "" {
		return
	}
	var override struct {
		Models    map[string]PricingRate `json:"models"`
		Providers map[string]PricingRate `json:"providers"`
	}
	if err := json.Unmarshal([]byte(raw), &override); err != nil {
		return
	}
	for k, v := range override.Models {
		pricingTable[strings.ToLower(k)] = v
	}
	for k, v := range override.Providers {
		pricingDefs[strings.ToLower(k)] = v
	}
}

func lookupRate(provider, model string) (PricingRate, bool) {
	loadPricing()
	p := strings.ToLower(strings.TrimSpace(provider))
	m := strings.ToLower(strings.TrimSpace(model))

	if m != "" {
		if rate, ok := pricingTable[m]; ok {
			return rate, true
		}
		if p != "" {
			if rate, ok := pricingTable[p+"/"+m]; ok {
				return rate, true
			}
		}
	}
	if p != "" {
		if rate, ok := pricingDefs[p]; ok {
			return rate, true
		}
		// Substring match: any provider whose name contains a flat-fee marker → 0.
		for _, marker := range []string{"zai", "codex", "chatgpt"} {
			if strings.Contains(p, marker) {
				return PricingRate{}, true
			}
		}
	}
	return PricingRate{}, false
}

// CostUSD computes the dollar cost for a usage event. Unknown model/provider → 0
// (never guessed). Subscription/flat-fee providers resolve to a known 0 rate.
func CostUSD(provider, model string, inputTokens, outputTokens int64) float64 {
	rate, ok := lookupRate(provider, model)
	if !ok {
		return 0
	}
	return (float64(inputTokens)*rate.InputPer1M + float64(outputTokens)*rate.OutputPer1M) / 1e6
}
