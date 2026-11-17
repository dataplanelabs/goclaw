package providers

import "strings"

// OpenAIForwardCompat resolves unknown OpenAI models by cloning from known templates.
type OpenAIForwardCompat struct{}

// openAIForwardCompatMap maps unknown model prefixes to template + optional spec patch.
var openAIForwardCompatMap = map[string]struct {
	Template string
	Patch    *ModelSpec
}{
	"gpt-5.5": {
		Template: "gpt-5.5",
	},
	"gpt-5.6": {
		Template: "gpt-5.6-sol",
	},
	"gpt-6": {
		Template: "gpt-6-sol",
	},
	"gpt-6.1": {
		Template: "gpt-6.1-sol",
	},
	"o5-mini": {
		Template: "o4-mini",
		Patch:    &ModelSpec{ContextWindow: 300_000},
	},
}

// ResolveForwardCompat handles future model aliases by cloning from the latest known template.
func (r *OpenAIForwardCompat) ResolveForwardCompat(modelID string, registry ModelRegistry) *ModelSpec {
	// Direct map lookup (exact match)
	if entry, ok := openAIForwardCompatMap[modelID]; ok {
		return CloneFromTemplate(registry, "openai", modelID, []string{entry.Template}, entry.Patch)
	}

	// Longest prefix wins, so "gpt-6.1-x" follows gpt-6.1 rather than gpt-6.
	best := ""
	for prefix := range openAIForwardCompatMap {
		if strings.HasPrefix(modelID, prefix) && len(prefix) > len(best) {
			best = prefix
		}
	}
	if best != "" {
		entry := openAIForwardCompatMap[best]
		return CloneFromTemplate(registry, "openai", modelID, []string{entry.Template}, entry.Patch)
	}

	return nil
}
