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
func guardCronDelivery(content string, noReplyKeywords []string) (string, bool) {
	cleaned := textguard.StripLeadingInternal(content)
	if strings.TrimSpace(cleaned) == "" {
		return "", false
	}
	if isCronNoReply(cleaned, noReplyKeywords) {
		return "", false
	}
	if textguard.IsMetaFailure(cleaned) {
		return "", false
	}
	return cleaned, true
}

func isCronNoReply(content string, keywords []string) bool {
	text := strings.ToLower(strings.TrimSpace(content))
	for _, keyword := range keywords {
		keyword = strings.ToLower(strings.TrimSpace(keyword))
		if keyword != "" && strings.Contains(text, keyword) {
			return true
		}
	}
	return false
}
