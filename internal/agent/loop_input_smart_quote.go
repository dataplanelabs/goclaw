package agent

import (
	"fmt"
	"strings"

	"github.com/nextlevelbuilder/goclaw/internal/providers"
)

// selfQuoteMarkers are the prefix substrings the channel layer emits when a
// user quote-replies their own earlier message. Used as a cheap trigger; the
// actual lookup still walks session history to find the related bot reply.
var selfQuoteMarkers = []string{
	"[Replying to their own image",
	"[Replying to their own media",
	"[Replying to their own sticker",
	"[Replying to their own voice message",
}

// appendSelfQuoteGenerationHint mutates the last user message in-place. When
// the message text indicates a self-quote with media AND the previous bot
// reply to the *quoted* user message produced image MediaRefs, the function
// appends a one-line hint listing the file paths so the LLM can call
// read_image(path=...) to inspect them. No-op when nothing matches.
//
// Matching strategy: scan backwards for the most recent prior user message
// that had image MediaRefs (i.e. the candidate quoted message), then look
// forward from there for the first assistant message with image MediaRefs.
// Path-only — no caption fuzzy matching, no cli_msg_id lookup (session
// messages don't preserve cli_msg_id today).
func appendSelfQuoteGenerationHint(messages []providers.Message) {
	if len(messages) == 0 {
		return
	}
	last := &messages[len(messages)-1]
	if last.Role != "user" || !containsSelfQuoteMarker(last.Content) {
		return
	}

	quotedIdx := findPriorUserMessageWithImage(messages, len(messages)-1)
	if quotedIdx < 0 {
		return
	}
	paths := collectReplyImagePaths(messages, quotedIdx)
	if len(paths) == 0 {
		return
	}

	last.Content += fmt.Sprintf(
		"\n[Hint: a prior bot reply to the quoted message produced these images — call read_image(path=...) to inspect: %s]",
		strings.Join(paths, ", "),
	)
}

func containsSelfQuoteMarker(s string) bool {
	for _, m := range selfQuoteMarkers {
		if strings.Contains(s, m) {
			return true
		}
	}
	return false
}

// findPriorUserMessageWithImage walks backwards from `before` (exclusive) and
// returns the index of the most recent user message carrying at least one
// image MediaRef. -1 when none found.
func findPriorUserMessageWithImage(messages []providers.Message, before int) int {
	for i := before - 1; i >= 0; i-- {
		if messages[i].Role != "user" {
			continue
		}
		for _, ref := range messages[i].MediaRefs {
			if ref.Kind == "image" {
				return i
			}
		}
	}
	return -1
}

// collectReplyImagePaths returns image paths from the first assistant message
// after `userIdx`. Stops at the first assistant message regardless of whether
// it has images — multi-turn replies are not aggregated.
func collectReplyImagePaths(messages []providers.Message, userIdx int) []string {
	for j := userIdx + 1; j < len(messages); j++ {
		if messages[j].Role != "assistant" {
			continue
		}
		var paths []string
		for _, ref := range messages[j].MediaRefs {
			if ref.Kind == "image" && ref.Path != "" {
				paths = append(paths, ref.Path)
			}
		}
		return paths
	}
	return nil
}
