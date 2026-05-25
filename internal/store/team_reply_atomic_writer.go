package store

import (
	"context"

	"github.com/nextlevelbuilder/goclaw/internal/providers"
)

// AtomicTeamReplyWriter is a focused capability for the team-reply poll
// worker: insert an eval row AND append the message to sessions.messages
// in a single DB transaction so a partial write never leaves a split-brain
// state (eval row without session msg, or vice versa).
//
// Returns the eval row id and a wasNew flag so callers can skip downstream
// fan-out (event publish) on retries that hit the ON CONFLICT path.
type AtomicTeamReplyWriter interface {
	WriteTeamReplyAtomic(ctx context.Context, e TeamReplyEvaluation, sessionKey string, msg providers.Message) (id string, wasNew bool, err error)
}
