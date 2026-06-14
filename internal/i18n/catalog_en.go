package i18n

func init() {
	register(LocaleEN, map[string]string{
		// Common validation
		MsgRequired:         "%s is required",
		MsgInvalidID:        "invalid %s ID",
		MsgNotFound:         "%s not found: %s",
		MsgAlreadyExists:    "%s already exists: %s",
		MsgInvalidRequest:   "invalid request: %s",
		MsgInvalidJSON:      "invalid JSON",
		MsgUnauthorized:     "unauthorized",
		MsgPermissionDenied: "permission denied: %s",
		MsgInternalError:    "internal error: %s",
		MsgInvalidSlug:      "%s must be a valid slug (lowercase letters, numbers, hyphens only)",
		MsgFailedToList:     "failed to list %s",
		MsgFailedToCreate:   "failed to create %s: %s",
		MsgFailedToUpdate:   "failed to update %s: %s",
		MsgFailedToDelete:   "failed to delete %s: %s",
		MsgFailedToSave:     "failed to save %s: %s",
		MsgInvalidUpdates:   "invalid updates",

		// Agent
		MsgAgentNotFound:       "agent not found: %s",
		MsgCannotDeleteDefault: "cannot delete the default agent",
		MsgUserCtxRequired:     "user context required",

		// Chat
		MsgRateLimitExceeded: "rate limit exceeded — please wait",
		MsgNoUserMessage:     "no user message found",
		MsgUserIDRequired:    "user_id is required",
		MsgMsgRequired:       "message is required",

		// Abort
		MsgAbortStopped:         "run stopped",
		MsgAbortForced:          "run force-aborted (3s grace exceeded)",
		MsgAbortAlreadyAborting: "abort already in progress",
		MsgAbortNotFound:        "run not found or already finished",
		MsgAbortUnauthorized:    "not authorized to abort this run",
		MsgAbortFailed:          "failed to abort run: %s",

		// Channel instances
		MsgInvalidChannelType: "invalid channel_type",
		MsgInstanceNotFound:   "instance not found",

		// Cron
		MsgJobNotFound:                "job not found",
		MsgInvalidCronExpr:            "invalid cron expression: %s",
		MsgCronDeliverChannelRequired: "cron job with deliver=true requires deliverChannel (channel-instance name like 'zalo-annhien')",
		MsgCronDeliverToRequired:      "cron job with deliver=true requires deliverTo (chat ID)",

		// Config
		MsgConfigHashMismatch: "config has changed (hash mismatch)",

		// Exec approval
		MsgExecApprovalDisabled: "exec approval is not enabled",

		// Pairing
		MsgSenderChannelRequired: "senderId and channel are required",
		MsgCodeRequired:          "code is required",
		MsgSenderIDRequired:      "sender_id is required",

		// HTTP API
		MsgInvalidAuth:           "invalid authentication",
		MsgMsgsRequired:          "messages is required",
		MsgUserIDHeader:          "X-GoClaw-User-Id header is required",
		MsgFileTooLarge:          "file too large or invalid multipart form",
		MsgMissingFileField:      "missing 'file' field",
		MsgInvalidFilename:       "invalid filename",
		MsgChannelKeyReq:         "channel and key are required",
		MsgMethodNotAllowed:      "method not allowed",
		MsgStreamingNotSupported: "streaming not supported",
		MsgOwnerOnly:             "only owner can %s",
		MsgNoAccess:              "no access to this %s",
		MsgAlreadySummoning:      "agent is already being summoned",
		MsgSummoningUnavailable:  "summoning not available",
		MsgNoDescription:         "agent has no description to resummon from",
		MsgSummonCancelled:       "summon cancelled by user",
		MsgCannotCancel:          "agent is not being summoned",
		MsgInvalidPath:           "invalid path",

		// Tenant backup / restore
		MsgRestoreNewModeRejectsTenantID: "mode=new creates a fresh tenant; pass tenant_slug (not tenant_id) as the new tenant's target slug",

		// Scheduler
		MsgQueueFull:    "session queue is full",
		MsgShuttingDown: "gateway is shutting down, please retry shortly",

		// Provider
		MsgProviderReqFailed: "%s: request failed: %s",

		// Unknown method
		MsgUnknownMethod: "unknown method: %s",

		// Not implemented
		MsgNotImplemented: "%s not yet implemented",

		// Agent links
		MsgLinksNotConfigured: "agent links not configured",
		MsgInvalidDirection:   "direction must be outbound, inbound, or bidirectional",
		MsgSourceTargetSame:   "source and target must be different agents",
		MsgCannotDelegateOpen: "cannot delegate to open agents — only predefined agents can be delegation targets",
		MsgNoUpdatesProvided:  "no updates provided",
		MsgInvalidLinkStatus:  "status must be active or disabled",

		// Teams
		MsgTeamsNotConfigured:   "teams not configured",
		MsgAgentIsTeamLead:      "agent is already the team lead",
		MsgCannotRemoveTeamLead: "cannot remove the team lead",

		// Channels
		MsgCannotDeleteDefaultInst: "cannot delete default channel instance",
		MsgCannotRemoveLastWriter:  "cannot remove the last file writer",

		// Skills
		MsgSkillsUpdateNotSupported: "skills.update not supported for file-based skills",
		MsgCannotResolveSkillID:     "cannot resolve skill ID for file-based skill",
		MsgSkillManagedOverwrite:    "This skill is gcplane-managed. Update via gcplane apply, or re-upload with force_imperative=true (audit-logged).",
		MsgSkillInvalidSource:       "invalid source value %q; allowed: unknown, cli, gcplane",
		MsgInvalidVisibility:        "invalid visibility %q: must be one of private, public",

		// Logs
		MsgInvalidLogAction: "action must be 'start' or 'stop'",

		// Config
		MsgRawConfigRequired:     "raw config is required",
		MsgRawPatchRequired:      "raw patch is required",
		MsgConfigMasterScopeOnly: "config.* methods are master-scope only; use tenant tool config endpoints for per-tenant overrides",
		MsgMasterScopeRequired:   "this action requires master tenant scope",

		// Storage / File
		MsgCannotDeleteSkillsDir: "cannot delete skills directories",
		MsgFailedToReadFile:      "failed to read file",
		MsgFileNotFound:          "file not found",
		MsgInvalidVersion:        "invalid version",
		MsgVersionNotFound:       "version not found",
		MsgFailedToDeleteFile:    "failed to delete",

		// OAuth
		MsgNoPendingOAuth:    "no pending OAuth flow",
		MsgFailedToSaveToken: "failed to save token",

		// Intent Classify
		MsgStatusWorking:       "🔄 I'm working on your request... Please wait.",
		MsgStatusDetailed:      "🔄 I'm currently working on your request...\n%s (iteration %d)\nRunning for: %s\n\nPlease wait — I'll respond when done.",
		MsgStatusPhaseThinking: "Phase: Thinking...",
		MsgStatusPhaseToolExec: "Phase: Running %s",
		MsgStatusPhaseTools:    "Phase: Executing tools...",
		MsgStatusPhaseCompact:  "Phase: Compacting context...",
		MsgStatusPhaseDefault:  "Phase: Processing...",
		MsgCancelledReply:      "✋ Cancelled. What would you like to do next?",
		MsgInjectedAck:         "Got it, I'll incorporate that into what I'm working on.",

		// Knowledge Graph
		MsgEntityIDRequired:       "entity_id is required",
		MsgEntityFieldsRequired:   "external_id, name, and entity_type are required",
		MsgTextRequired:           "text is required",
		MsgProviderModelRequired:  "provider and model are required",
		MsgInvalidProviderOrModel: "invalid provider or model",

		// Builtin tool descriptions
		MsgToolReadFile:        "Read the contents of a file from the agent's workspace by path",
		MsgToolWriteFile:       "Write content to a file in the workspace, creating directories as needed",
		MsgToolListFiles:       "List files and directories in a given path within the workspace",
		MsgToolEdit:            "Apply targeted search-and-replace edits to existing files without rewriting the entire file",
		MsgToolExec:            "Execute a shell command in the workspace and return stdout/stderr",
		MsgToolWebSearch:       "Search the web for information using a search engine (Brave or DuckDuckGo)",
		MsgToolWebFetch:        "Fetch a web page or API endpoint and extract its text content",
		MsgToolMemorySearch:    "Search through the agent's long-term memory using semantic similarity",
		MsgToolMemoryGet:       "Retrieve a specific memory document by its file path",
		MsgToolKGSearch:        "Search entities, relationships, and observations in the agent's knowledge graph",
		MsgToolReadImage:       "Analyze images using a vision-capable LLM provider",
		MsgToolReadDocument:    "Analyze documents (PDF, Word, Excel, PowerPoint, CSV, etc.) using a document-capable LLM provider",
		MsgToolCreateImage:     "Generate images from text prompts using an image generation provider",
		MsgToolReadAudio:       "Analyze audio files (speech, music, sounds) using an audio-capable LLM provider",
		MsgToolReadVideo:       "Analyze video files using a video-capable LLM provider",
		MsgToolCreateVideo:     "Generate videos from text descriptions using AI",
		MsgToolCreateAudio:     "Generate music or sound effects from text descriptions using AI",
		MsgToolTTS:             "Convert text to natural-sounding speech audio",
		MsgToolBrowser:         "Automate browser interactions: navigate pages, click elements, fill forms, take screenshots",
		MsgToolSessionsList:    "List active chat sessions across all channels",
		MsgToolSessionStatus:   "Get the current status and metadata of a specific chat session",
		MsgToolSessionsHistory: "Retrieve the message history of a specific chat session",
		MsgToolSessionsSend:    "Send a message to an active chat session on behalf of the agent",
		MsgToolMessage:         "Send a proactive message to a user on a connected channel (Telegram, Discord, etc.)",
		MsgToolCron:            "Schedule or manage recurring tasks using cron expressions, at-times, or intervals",
		MsgToolSpawn:           "Spawn a subagent for background work or delegate a task to a linked agent",
		MsgToolSkillSearch:     "Search for available skills by keyword or description to find relevant capabilities",
		MsgToolUseSkill:        "Activate a skill to use its specialized capabilities (tracing marker)",
		MsgToolSkillManage:     "Create, patch, or delete skills from conversation experience",
		MsgToolPublishSkill:    "Register a skill directory in the system database, making it discoverable",
		MsgToolTeamTasks:       "View, create, update, and complete tasks on the team task board",

		MsgSkillNudgePostscript: "This task involved several steps. Want me to save the process as a reusable skill? Reply **\"save as skill\"** or **\"skip\"**.",
		MsgSkillNudge70Pct:      "[System] You are at 70% of your iteration budget. Consider whether any patterns from this session would make a good skill.",
		MsgSkillNudge90Pct:      "[System] You are at 90% of your iteration budget. If this session involved reusable patterns, consider saving them as a skill before completing.",

		MsgInvalidRole: "invalid role: allowed values are owner, admin, operator, member, viewer",

		MsgContactIDsRequired:  "contact_ids is required",
		MsgMergeTargetRequired: "exactly one of tenant_user_id or create_user is required",
		MsgTenantUserNotFound:  "tenant user not found",
		MsgTenantMismatch:      "tenant user does not belong to this tenant",
		MsgTenantScopeRequired: "tenant scope is required for this operation",

		// TTS / Voices
		MsgTtsUnknownModel:       "unknown tts model: %s",
		MsgVoicesListFailed:      "failed to list voices: %s",
		MsgTtsGeminiInvalidVoice: "invalid Gemini voice: %s",
		MsgTtsGeminiSpeakerLimit: "Gemini TTS supports at most 2 speakers",
		MsgTtsGeminiInvalidModel:  "invalid Gemini TTS model: %s",
		MsgTtsGeminiTextOnly:      "Gemini refused to generate audio. Try simpler text without translation or commentary.",
		MsgTtsParamOutOfRange:     "TTS param %q value %v is out of range [%v, %v]",
		MsgTtsParamUnknownKey:     "TTS param %q is not supported by this provider",
		MsgTtsMiniMaxVoicesFailed: "failed to fetch MiniMax voices: %s",

		// VieNeu
		MsgTtsVieneuSynthesisFailed:   "VieNeu synthesis failed: %s",
		MsgTtsVieneuVoicesFailed:      "failed to fetch VieNeu voices: %s",
		MsgTtsVieneuRefAudioInvalid:   "reference audio invalid: %s",
		MsgTtsVieneuDaemonUnreachable: "VieNeu daemon unreachable; ensure goclaw is built with ENABLE_FULL_SKILLS",
		MsgVieneuRefAudioTooShort:     "reference audio too short: %s",
		MsgVieneuRefAudioTooLong:      "reference audio too long: %s",
		MsgVieneuRefTextRequired:      "ref_text required for voice cloning",
		MsgVieneuMaxClonedVoices:      "max cloned voices per tenant reached (%d)",
		MsgVieneuClonedVoiceNotFound:  "cloned voice not found: %s",

		// STT
		MsgSTTAllProvidersFailed:     "All STT providers failed",
		MsgSTTLegacyConfigDeprecated: "Legacy STT config deprecated; migrate to builtin_tools[stt]",
		MsgSTTWhatsappPrivacyWarning: "Enabling STT for WhatsApp breaks end-to-end encryption for voice messages sent to this agent.",
		MsgVoiceMessageFallback:      "[Voice message]",

		// Workstation
		MsgWorkstationNotFound:     "workstation not found: %s",
		MsgWorkstationKeyExists:    "workstation key already in use: %s",
		MsgInvalidBackend:          "invalid backend type: %s (must be ssh|docker)",
		MsgWorkstationInactive:     "workstation is inactive: %s",
		MsgInvalidMetadataShape:    "invalid metadata for %s backend: %s",
		MsgWorkstationRequired:     "no workstation bound to agent; pass workstation_id",
		MsgWorkstationAccessDenied: "agent %s not authorized for workstation %s",
		MsgBackendNotReady:         "workstation backend not ready: %s",

		// Webhooks
		MsgWebhookAuthFailed:              "webhook authentication failed",
		MsgWebhookHMACInvalid:             "HMAC signature is invalid",
		MsgWebhookHMACTimestampSkew:       "request timestamp outside acceptable window",
		MsgWebhookBearerRequiredHMAC:      "this webhook requires HMAC authentication",
		MsgWebhookRevoked:                 "webhook has been revoked",
		MsgWebhookKindMismatch:            "request kind does not match webhook configuration",
		MsgWebhookRateLimited:             "webhook rate limit exceeded",
		MsgWebhookBodyTooLarge:            "request body exceeds size limit",
		MsgWebhookIdempotencyConflict:     "idempotency key conflict: request body mismatch",
		MsgWebhookTenantMismatch:          "webhook tenant mismatch",
		MsgWebhookAgentNotFound:           "webhook agent not found",
		MsgWebhookChannelNotFound:         "webhook channel not found",
		MsgWebhookMediaSSRFBlocked:        "media URL blocked by SSRF policy",
		MsgWebhookMediaTooLarge:           "media file exceeds size limit",
		MsgWebhookMediaMIMEDenied:         "media MIME type is not allowed",
		MsgWebhookCallbackURLInvalid:      "callback URL is invalid or blocked",
		MsgWebhookLLMTimeout:              "LLM processing timed out",
		MsgWebhookLaneSaturated:           "webhook processing lane is at capacity",
		MsgWebhookLocalhostOnlyViolation:  "this webhook is restricted to localhost callers",
		MsgWebhookMediaChannelUnsupported: "channel does not support media attachments",
		MsgWebhookIPDenied:                "request origin is not in the IP allowlist",
		MsgWebhookEncryptionUnavailable:   "webhook encryption key not configured; set GOCLAW_ENCRYPTION_KEY to enable webhooks",

		// Hooks
		MsgHookInvalidMatcher:          "invalid matcher regex: %s",
		MsgHookCommandDisabledStandard: "command-type hooks are only available on Lite edition",
		MsgHookPromptRequiresMatcher:   "prompt hooks require a matcher or if_expr (runaway-cost guard)",
		MsgHookCircuitBreakerTripped:   "hook auto-disabled after repeated failures",
		MsgHookBudgetExceeded:          "tenant hook token budget exceeded",
		MsgHookPerTurnCapReached:       "hook invocation per-turn cap reached",
		MsgHookBuiltinReadOnly:         "builtin hooks are read-only except for the enabled toggle",

		// Zalo OA OAuth channel
		MsgZaloOACodeExchangeFailed: "zalo oauth code exchange failed: %s",
		MsgZaloOAInvalidChannelType: "instance is not a zalo_oa channel",
		MsgZaloOAConnected:           "zalo official account connected: %s",
		MsgZaloOAInvalidState:        "oauth state token is invalid or expired",
		MsgZaloOARedirectURIRequired: "credentials.redirect_uri is required and must exactly match the callback registered in your Zalo developer console",
		MsgZaloOAMissingAppID:        "credentials.app_id is required — set it on the channel before requesting the consent URL",
		MsgZaloOAStateGenFailed:      "failed to generate consent state token; please retry",
		MsgZaloOAOAIDMismatch:        "callback URL belongs to a different OA — paste the URL from THIS instance's consent page",

		// Zalo webhook URL RPC
		MsgZaloWebhookWrongChannelType: "channels.instances.zalo.webhook_url only applies to zalo_bot or zalo_oa instances",
		MsgZaloWebhookPathHint:         "Prepend your gateway's externally-reachable URL (e.g. https://gw.example.com) to the path, then register the full URL in the Zalo developer console.",

		// Zalo OA runtime error catalog. Args: (code int, raw_message string)
		MsgZaloOAErrAuth:              "Zalo rejected the access token after a refresh retry (code %d: %s); re-authorize the OA",
		MsgZaloOAErrRefreshExpired:    "Zalo refresh token has expired (code %d: %s); operator must re-consent in the OA console",
		MsgZaloOAErrPayload:           "Zalo rejected the request payload (code %d: %s); verify message shape and required fields",
		MsgZaloOAErrSize:              "Zalo upload exceeds the size cap (code %d: %s); image 1MB / file 5MB / gif 5MB",
		MsgZaloOAErrPermission:        "Zalo requires additional permission for this call (code %d: %s); grant the missing scope to the OA app",
		MsgZaloOAErrInteractionWindow: "Recipient is outside Zalo's messaging window (code %d: %s); wait for the user to message first or use a paid template",
		MsgZaloOAErrUserNotVisible:    "Target user is not visible to this OA (code %d: %s)",
		MsgZaloOAErrAppDisabled:       "Zalo app is disabled or banned (code %d: %s); contact Zalo support",
		MsgZaloOAErrRate:              "Zalo quota exhausted (code %d: %s); wait for the quota window to reset",
		MsgZaloOAErrServer:            "Zalo returned a temporary server error (code %d: %s); retry later",
		MsgZaloOAErrRedirectURI:       "Zalo rejected the OAuth redirect_uri (code %d: %s); update the redirect URI in the Zalo console to match the channel config",
		MsgZaloOAReauthDueSoon:        "Refresh token expires in %d day(s); re-authorize the OA to avoid downtime",
		MsgZaloOAUnsupportedAttachment: "(File %q (%s) cannot be delivered via Zalo OA — only PDF/DOC/DOCX are accepted. Content described above.)",

		// Workstation permissions (Phase 6)
		MsgWorkstationCmdDenied:    "command denied by workstation policy: %s",
		MsgWorkstationEnvDenied:    "env var denied by policy: %s",
		MsgWorkstationInputInvalid: "command contains invalid characters: %s",
		MsgWorkstationRateLimit:    "workstation rate limit exceeded",
		MsgWorkstationPermNotFound: "permission entry not found: %s",
		// Workstation activity (Phase 7)
		MsgWorkstationActivityTitle: "Recent Activity",
		MsgWorkstationActionExec:    "Exec",
		MsgWorkstationActionDeny:    "Denied",

		// Package updates (Phase 4+5)
		MsgPackageNotInstalled:  "Package %s is not installed",
		MsgPackageUpdateLocked:  "Package %s is being updated by another request",
		MsgReleaseNotFound:      "Release %s not found for %s",
		MsgAssetNotFound:        "No compatible asset for %s/%s",
		MsgChecksumMismatch:     "Checksum mismatch for %s",
		MsgUpdateSwapFailed:     "Failed to install %s; previous version restored",
		MsgUpdateManifestDesync: "Binary updated but manifest save failed — manual recovery required for %s",
		MsgUpdateCacheStale:     "Updates cache stale; run refresh before applying an update",

		// Grant env validation
		MsgGrantEnvDeniedKeys:   "env keys not allowed: %s",
		MsgGrantEnvValueInvalid: "invalid env value: %s",
		MsgGrantEnvTooManyKeys:  "too many env keys: max 50",
		MsgGrantEnvRevealLimit:  "rate limit exceeded for env reveal — try again later",

		// Secure CLI execution
		MsgSecureCliBinaryNotFound: "binary %q is not registered for secure exec",
		MsgSecureCliNoGrant:        "agent has no grant for binary %q",
		MsgSecureCliDeniedByPolicy: "call denied by deny_args policy: %s",

		// OAuth integrations
		MsgOAuthStateMismatch:       "OAuth state token mismatch or expired — please try again",
		MsgOAuthExchangeFailed:      "OAuth code exchange failed: %s",
		MsgOAuthBinaryNotFound:      "secure CLI binary %q is not registered for this tenant",
		MsgOAuthIntegrationNotFound: "no integration found for %q",
		MsgOAuthRevoked:             "Google credentials revoked — please reconnect via Settings → Integrations",
		MsgOAuthNotConfigured:       "Google OAuth is not configured on this server",

		// Standby mode
		StandbyToolDescription:      "Pause replies in the current thread for a duration. The agent will still observe and remember messages but will not reply until the pause expires.",
		StandbyToolParamDuration:    "Pause duration in seconds (60-86400).",
		StandbyToolParamReason:      "Optional reason recorded with the pause.",
		StandbyErrorInvalidDuration: "duration_seconds must be between 60 and 86400",
		StandbyErrorNoChannelCtx:    "enter_standby requires channel context and cannot be called from this caller type",
		StandbyEntered:              "Entered standby mode for %s (reason: %s)",
		StandbyRPCInvalidSchedule:   "invalid schedule: %s",
		StandbyRPCNoPermission:      "tenant admin required to edit channel schedules",

		TeamCaptureRPCNoPermission:    "tenant admin required to toggle team-reply capture",
		TeamCaptureRPCInvalidConfig:   "invalid capture config: %s",
		TeamCaptureJudgeAgentNotFound: "judge agent %q not found in this tenant — create it first or pick an existing agent_key",
		TeamCaptureJudgeKeyRequired:   "judge_agent_key is required when judge_evaluation is enabled",
		TeamCaptureScheduleInvalid:    "invalid judge schedule %q — use a 5-field cron expression",
		TeamEvalNotFound:              "team reply evaluation not found",
		TeamEvalJudgeError:            "judge evaluation failed: %s",

		TraceRetryPayloadOversize: "Payload too large to replay (>2 MB).",
		TraceRetryLocked:          "Retry already in progress.",
		TraceRetryAgentGone:       "The agent for this trace was deleted.",
		TraceRetryProviderGone:    "The provider for this trace was removed.",
		TraceRetryPayloadMissing:  "Replay data no longer available.",
		TraceRetryConfirmRequired: "This run already sent a message — confirm to retry.",
		TraceRetryStarted:         "Retry started.",
		TraceRetryNotFailed:       "Only finished traces can be retried (run is still in progress).",

		// Message tool cross-target forward notice
		MessageCrossTargetForwarded: "📤 Forwarded to %s as requested: %q",

		// Package update source labels
		MsgPackagesUpdatesSourceGithub: "GitHub",
		MsgPackagesUpdatesSourcePip:    "pip",
		MsgPackagesUpdatesSourceNpm:    "npm",
		MsgPackagesUpdatesSourceApk:    "apk",

		// Package update availability messages
		MsgPackagesUpdatesUnavailablePip: "pip not installed on this system",
		MsgPackagesUpdatesUnavailableNpm: "npm not installed on this system",
		MsgPackagesUpdatesUnavailableApk: "apk not available on this system",

		// Package update failure reasons
		MsgPackagesUpdatesReasonDependencyConflict: "Dependency conflict",
		MsgPackagesUpdatesReasonPermission:         "Permission denied",
		MsgPackagesUpdatesReasonNetwork:            "Network error",
		MsgPackagesUpdatesReasonNotFound:           "Package not found",
		MsgPackagesUpdatesReasonTargetMissing:      "Version not available",
		MsgPackagesUpdatesReasonExternallyManaged:  "Environment externally managed",
		MsgPackagesUpdatesReasonLocked:             "Package database is locked",
		MsgPackagesUpdatesReasonDiskFull:           "Disk full",
		MsgPackagesUpdatesReasonHelperUnavailable:  "Privileged helper unavailable",

		MsgSkillEvolutionNotConfigured: "skill evolution store is not configured",
		MsgActivityStoreNotConfigured:  "activity store is not configured",
		MsgInvalidEvolutionMode:        "invalid evolution mode",
		MsgSystemSkillMutationBlocked:  "system skill mutation is blocked",
		MsgSuggestionMustBeApproved:    "suggestion must be approved before apply",
		MsgInvalidDraftPatch:           "invalid draft_patch: %s",
		MsgDraftPatchRequired:          "draft_patch requires content or find/replace",
		MsgFindTextNotFound:            "find text not found in target file",

		MsgLoginCodexStarted:       "Starting codex device auth on coding-agent pod…",
		MsgLoginCodexSuccess:       "Open this URL on your phone and approve:\n%s\nCode: <code>%s</code>\n\nThe pod will be re-authed once you approve.",
		MsgLoginCodexFailed:        "codex login failed: %s",
		MsgLoginCodexUnknownSvc:    "Unknown service %q — only 'codex' is supported.",
		MsgLoginCodexNoWorkstation: "/login codex requires the workstation store — not configured on this channel.",
	})
}
