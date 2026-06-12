package cmd

import (
	"strings"

	"github.com/nextlevelbuilder/goclaw/internal/textguard"
)

// guardCronDelivery gates a cron final response before forwarding to a chat:
// leading English first-person CoT paragraphs are stripped (never inside code
// fences, top-only, capped at half the message), then the forward is suppressed
// ONLY when the remainder is empty or is itself a pure English CoT leak. Legit
// content (product lists, headlines, status alerts) is delivered as-is.
func guardCronDelivery(content string) (string, bool) {
	cleaned := textguard.StripLeadingInternal(content)
	if strings.TrimSpace(cleaned) == "" {
		return "", false
	}
	if textguard.IsMetaFailure(cleaned) {
		return "", false
	}
	return cleaned, true
}
