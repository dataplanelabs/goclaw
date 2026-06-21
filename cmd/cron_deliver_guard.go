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
	if isCronNoReply(cleaned) {
		return "", false
	}
	if textguard.IsMetaFailure(cleaned) {
		return "", false
	}
	return cleaned, true
}

func isCronNoReply(content string) bool {
	text := strings.ToLower(strings.TrimSpace(content))
	for _, phrase := range []string{
		"send nothing",
		"don't send",
		"do not send",
		"no reply needed",
		"skip reply",
		"no action needed",
		"nothing to do",
		"không gửi gì",
		"không cần gửi",
		"không cần nhắc",
		"không cần làm gì",
		"không còn gì cần làm",
		"khỏi gửi",
		"đừng gửi",
	} {
		if strings.Contains(text, phrase) {
			return true
		}
	}
	return false
}
