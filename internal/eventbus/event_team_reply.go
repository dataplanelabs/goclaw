package eventbus

import "time"

// EventTeamReplyObserved fires when a human team reply is captured on a
// channel. The eventbus dedups on SourceID — publishers MUST set it to
// TeamReplyObservedSourceID(channelInstanceID, teamMsgID).
const EventTeamReplyObserved EventType = "team.reply.observed"

// TeamReplyObservedPayload carries the captured reply + context needed by
// the JudgeWorker subscriber (Phase 5). CustomerMessage is the last
// user-role message in the same thread, included so the judge can compose
// a hypothesized bot reply without re-fetching session history.
type TeamReplyObservedPayload struct {
	EvaluationID      string
	TenantID          string
	ChannelInstanceID string
	ChannelName       string
	ThreadKey         string
	SessionKey        string
	TeamMsgID         string
	TeamReply         string
	CustomerMessage   string
	CapturedAt        time.Time
}

// TeamReplyObservedSourceID is the canonical dedup key.
func TeamReplyObservedSourceID(channelInstanceID, teamMsgID string) string {
	return "team-reply:" + channelInstanceID + ":" + teamMsgID
}
