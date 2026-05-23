package protocol

// Inbound reaction decoders. Wire frames arrive on cmd=612 (real-time),
// cmd=610 (historical user-thread replay), cmd=611 (historical group-thread
// replay). The TReaction shape mirrors zca-js src/models/Reaction.ts; the
// inner `content` field is a JSON STRING that decodes into tReactionInner.

import (
	"context"
	"encoding/json"
	"fmt"
)

// TReaction is the wire shape for a single reaction frame. There is no
// `groupId` field — zca-js's Reaction class resolves threadID from idTo /
// uidFrom (see decodeReaction below). msgType is a STRING here even though
// the inner rMsg.msgType is numeric.
type TReaction struct {
	ActionID string      `json:"actionId,omitempty"`
	MsgID    json.Number `json:"msgId"`
	CliMsgID json.Number `json:"cliMsgId"`
	MsgType  string      `json:"msgType"`
	UIDFrom  string      `json:"uidFrom"`
	IDTo     string      `json:"idTo"`
	DName    string      `json:"dName,omitempty"`
	Content  string      `json:"content"`
	TS       json.Number `json:"ts"`
	TTL      int         `json:"ttl,omitempty"`
}

type tReactionInner struct {
	RMsg []struct {
		GMsgID  json.Number `json:"gMsgID"`
		CMsgID  json.Number `json:"cMsgID"`
		MsgType int         `json:"msgType"`
	} `json:"rMsg"`
	RIcon  string `json:"rIcon"`
	RType  int    `json:"rType"`
	Source int    `json:"source"`
}

// Reactions returns the inbound reaction event stream. Buffered at
// msgBufferSize with drop-oldest backpressure (same pattern as Messages()).
func (ln *Listener) Reactions() <-chan ReactionEvent { return ln.reactionCh }

// handleReactionEvents handles cmd=612 — real-time reactions. Payload carries
// both DM (`reacts`) and group (`reactGroups`) lists.
func (ln *Listener) handleReactionEvents(ctx context.Context, data string, encType uint) {
	ln.mu.RLock()
	ck := ln.cipherKey
	ln.mu.RUnlock()

	payload, err := ln.decryptEventData(data, encType, ck)
	if err != nil {
		emit(ctx, ln.errorCh, fmt.Errorf("zalo_personal: decrypt reaction event: %w", err))
		return
	}

	var envelope struct {
		Data struct {
			Reacts      []TReaction `json:"reacts"`
			ReactGroups []TReaction `json:"reactGroups"`
		} `json:"data"`
	}
	if err := json.Unmarshal(payload, &envelope); err != nil {
		emit(ctx, ln.errorCh, fmt.Errorf("zalo_personal: parse reaction event: %w", err))
		return
	}

	for _, r := range envelope.Data.Reacts {
		if ev, ok := decodeReaction(ln.sess.UID, r, ThreadTypeUser, false); ok {
			emit(ctx, ln.reactionCh, ev)
		}
	}
	for _, r := range envelope.Data.ReactGroups {
		if ev, ok := decodeReaction(ln.sess.UID, r, ThreadTypeGroup, false); ok {
			emit(ctx, ln.reactionCh, ev)
		}
	}
}

// handleOldReactions handles cmd=610 (user) and cmd=611 (group) — historical
// reaction replays that fire after a reconnect. Flagged IsHistoric=true so
// the channel layer can drop them to avoid double-emit.
func (ln *Listener) handleOldReactions(ctx context.Context, data string, encType uint, isGroup bool) {
	ln.mu.RLock()
	ck := ln.cipherKey
	ln.mu.RUnlock()

	payload, err := ln.decryptEventData(data, encType, ck)
	if err != nil {
		emit(ctx, ln.errorCh, fmt.Errorf("zalo_personal: decrypt old reactions: %w", err))
		return
	}

	listKey := "reacts"
	threadType := ThreadTypeUser
	if isGroup {
		listKey = "reactGroups"
		threadType = ThreadTypeGroup
	}

	var raw struct {
		Data map[string]json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(payload, &raw); err != nil {
		emit(ctx, ln.errorCh, fmt.Errorf("zalo_personal: parse old reactions envelope: %w", err))
		return
	}
	listRaw := raw.Data[listKey]
	if len(listRaw) == 0 {
		return
	}
	var reacts []TReaction
	if err := json.Unmarshal(listRaw, &reacts); err != nil {
		return // empty or malformed list — drop silently
	}
	for _, r := range reacts {
		if ev, ok := decodeReaction(ln.sess.UID, r, threadType, true); ok {
			emit(ctx, ln.reactionCh, ev)
		}
	}
}

// decodeReaction parses a TReaction (with stringified inner content) into a
// ReactionEvent. Returns false when the inner JSON is malformed so the
// listener silently skips it rather than spamming the error channel.
//
// Thread-ID resolution mirrors zca-js's Reaction constructor:
//   threadId = isGroup || uidFrom == "0" ? idTo : uidFrom
// For DMs: chat partner is uidFrom unless uidFrom=="0" (self-sent), then idTo.
// For groups: always idTo.
func decodeReaction(selfUID string, r TReaction, threadType ThreadType, historic bool) (ReactionEvent, bool) {
	if r.Content == "" {
		return ReactionEvent{}, false
	}
	var inner tReactionInner
	if err := json.Unmarshal([]byte(r.Content), &inner); err != nil {
		return ReactionEvent{}, false
	}

	isGroup := threadType == ThreadTypeGroup
	var threadID string
	if isGroup || r.UIDFrom == DefaultUIDSelf {
		threadID = r.IDTo
	} else {
		threadID = r.UIDFrom
	}

	msgID := r.MsgID.String()
	cliMsgID := r.CliMsgID.String()
	if len(inner.RMsg) > 0 {
		// Prefer inner rMsg IDs only when outer is missing/zero — defensive,
		// since real captures sometimes leave the outer fields blank on
		// historical replay frames.
		if msgID == "" || msgID == "0" {
			msgID = inner.RMsg[0].GMsgID.String()
		}
		if cliMsgID == "" || cliMsgID == "0" {
			cliMsgID = inner.RMsg[0].CMsgID.String()
		}
	}

	ts, _ := r.TS.Int64()
	return ReactionEvent{
		MsgID:      msgID,
		CliMsgID:   cliMsgID,
		ThreadID:   threadID,
		ThreadType: threadType,
		UIDFrom:    r.UIDFrom,
		DName:      r.DName,
		Code:       inner.RIcon,
		RType:      inner.RType,
		Source:     inner.Source,
		Timestamp:  ts,
		IsSelf:     r.UIDFrom == selfUID || r.UIDFrom == DefaultUIDSelf,
		IsHistoric: historic,
	}, true
}
