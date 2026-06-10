package oa

// Zalo endpoint surface. Version prefixes are load-bearing — Zalo mixes
// API versions per family. v2.0: read + upload. v3.0: send. v4: OAuth.
const (
	defaultAPIBase   = "https://openapi.zalo.me"
	defaultOAuthBase = "https://oauth.zaloapp.com/v4"

	pathSendMessage    = "/v3.0/oa/message/cs"
	pathListRecentChat = "/v2.0/oa/listrecentchat"

	// Phase 4 team-reply polling. The plain /listrecentchat + /conversation
	// endpoints return ALL OA↔user messages (customer + bot-API + Manager
	// app). The original `/onbehalf/*` paths from research return Zalo
	// error 404 "empty or invalid API" — that namespace does not exist on
	// the live API as of 2026-05.
	pathOnBehalfListRecentChat = "/v2.0/oa/listrecentchat"
	pathOnBehalfConversation   = "/v2.0/oa/conversation"

	// Reactions ride the v2.0 message endpoint with a sender_action body —
	// distinct from pathSendMessage (v3.0/cs) by both version and shape.
	pathSendReaction = "/v2.0/oa/message"

	// Upload caps enforced by Zalo: image 1MB, file 5MB, gif 5MB.
	pathUploadImage = "/v2.0/oa/upload/image"
	pathUploadFile  = "/v2.0/oa/upload/file"
	pathUploadGIF   = "/v2.0/oa/upload/gif"

	// Joined onto defaultOAuthBase (which already carries /v4).
	pathOAuthAccessToken = "/oa/access_token"
)
