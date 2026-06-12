package cmd

import (
	"strings"

	"github.com/nextlevelbuilder/goclaw/internal/textguard"
)

// guardCronDelivery gates a cron final response before forwarding to a chat:
// leading English CoT paragraphs are stripped, and pure English meta/failure
// output is suppressed entirely (the cron run log keeps the full text).
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
