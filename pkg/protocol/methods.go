package protocol

// RPC method name constants.
// Organized by priority: CRITICAL (Phase 1) → NEEDED (Phase 2) → NICE TO HAVE (Phase 3+).

// Phase 1 - CRITICAL methods
const (
	// Agent
	MethodAgent            = "agent"
	MethodAgentWait        = "agent.wait"
	MethodAgentIdentityGet = "agent.identity.get"

	// Chat
	MethodChatSend          = "chat.send"
	MethodChatHistory       = "chat.history"
	MethodChatAbort         = "chat.abort"
	MethodChatInject        = "chat.inject"
	MethodChatSessionStatus = "chat.session.status"

	// Agents management
	MethodAgentsList     = "agents.list"
	MethodAgentsCreate   = "agents.create"
	MethodAgentsUpdate   = "agents.update"
	MethodAgentsDelete   = "agents.delete"
	MethodAgentsFileList = "agents.files.list"
	MethodAgentsFileGet  = "agents.files.get"
	MethodAgentsFileSet  = "agents.files.set"

	// Config
	MethodConfigGet      = "config.get"
	MethodConfigApply    = "config.apply"
	MethodConfigPatch    = "config.patch"
	MethodConfigSchema   = "config.schema"
	MethodConfigDefaults = "config.defaults"

	// Sessions
	MethodSessionsList    = "sessions.list"
	MethodSessionsPreview = "sessions.preview"
	MethodSessionsPatch   = "sessions.patch"
	MethodSessionsDelete  = "sessions.delete"
	MethodSessionsReset   = "sessions.reset"
	MethodSessionsCompact = "sessions.compact"

	// System
	MethodConnect = "connect"
	MethodHealth  = "health"
	MethodStatus  = "status"
)

// Phase 2 - NEEDED methods
const (
	MethodSkillsList   = "skills.list"
	MethodSkillsGet    = "skills.get"
	MethodSkillsUpdate = "skills.update"

	MethodCronList   = "cron.list"
	MethodCronCreate = "cron.create"
	MethodCronUpdate = "cron.update"
	MethodCronDelete = "cron.delete"
	MethodCronToggle = "cron.toggle"
	MethodCronStatus = "cron.status"
	MethodCronRun    = "cron.run"
	MethodCronRuns   = "cron.runs"

	MethodChannelsList   = "channels.list"
	MethodChannelsStatus = "channels.status"
	MethodChannelsToggle = "channels.toggle"

	MethodPairingRequest = "device.pair.request"
	MethodPairingApprove = "device.pair.approve"
	MethodPairingDeny    = "device.pair.deny"
	MethodPairingList    = "device.pair.list"
	MethodPairingRevoke  = "device.pair.revoke"

	MethodBrowserPairingStatus = "browser.pairing.status"

	MethodApprovalsList    = "exec.approval.list"
	MethodApprovalsApprove = "exec.approval.approve"
	MethodApprovalsDeny    = "exec.approval.deny"

	MethodUsageGet     = "usage.get"
	MethodUsageSummary = "usage.summary"

	MethodQuotaUsage = "quota.usage"

	MethodSend = "send"
)

// Agent heartbeat
const (
	MethodHeartbeatGet          = "heartbeat.get"
	MethodHeartbeatSet          = "heartbeat.set"
	MethodHeartbeatToggle       = "heartbeat.toggle"
	MethodHeartbeatTest         = "heartbeat.test"
	MethodHeartbeatLogs         = "heartbeat.logs"
	MethodHeartbeatChecklistGet = "heartbeat.checklist.get"
	MethodHeartbeatChecklistSet = "heartbeat.checklist.set"
	MethodHeartbeatTargets      = "heartbeat.targets"
)

// Config permissions
const (
	MethodConfigPermissionsList   = "config.permissions.list"
	MethodConfigPermissionsCheck  = "config.permissions.check"
	MethodConfigPermissionsGrant  = "config.permissions.grant"
	MethodConfigPermissionsRevoke = "config.permissions.revoke"
)

// Channel instances management
const (
	MethodChannelInstancesList   = "channels.instances.list"
	MethodChannelInstancesGet    = "channels.instances.get"
	MethodChannelInstancesCreate = "channels.instances.create"
	MethodChannelInstancesUpdate = "channels.instances.update"
	MethodChannelInstancesDelete = "channels.instances.delete"

	// Zalo OA OAuth (paste-code consent flow). zalo_oa-only.
	MethodChannelInstancesZaloOAConsentURL   = "channels.instances.zalo_oa.consent_url"
	MethodChannelInstancesZaloOAExchangeCode = "channels.instances.zalo_oa.exchange_code"

	// Zalo webhook URL discovery — path-only; operator prepends host.
	// Channel-family endpoint (no bot/oa suffix): handler dispatches on
	// the resolved channel_type and serves both zalo_bot and zalo_oa.
	MethodChannelInstancesZaloWebhookURL = "channels.instances.zalo.webhook_url"

	// Standby schedules (Phase 4)
	MethodChannelsScheduleGet          = "channels.schedule_get"
	MethodChannelsScheduleSet          = "channels.schedule_set"
	MethodChannelsScheduleDelete       = "channels.schedule_delete"
	MethodChannelsThreadScheduleList   = "channels.thread_schedule_list"
	MethodChannelsThreadScheduleGet    = "channels.thread_schedule_get"
	MethodChannelsThreadScheduleSet    = "channels.thread_schedule_set"
	MethodChannelsThreadScheduleDelete = "channels.thread_schedule_delete"

	// Team-reply capture + evaluations (Phase 6)
	MethodChannelsTeamRepliesList        = "channels.team_replies_list"
	MethodChannelsTeamRepliesGet         = "channels.team_replies_get"
	MethodChannelsTeamRepliesExportJSONL = "channels.team_replies_export_jsonl"
	MethodChannelsTeamCaptureToggle      = "channels.team_capture_toggle"
)

// Agent links (inter-agent delegation)
const (
	MethodAgentsLinksList   = "agents.links.list"
	MethodAgentsLinksCreate = "agents.links.create"
	MethodAgentsLinksUpdate = "agents.links.update"
	MethodAgentsLinksDelete = "agents.links.delete"
)

// Agent teams
const (
	MethodTeamsList                = "teams.list"
	MethodTeamsCreate              = "teams.create"
	MethodTeamsGet                 = "teams.get"
	MethodTeamsDelete              = "teams.delete"
	MethodTeamsTaskList            = "teams.tasks.list"
	MethodTeamsTaskGet             = "teams.tasks.get"
	MethodTeamsTaskGetLight        = "teams.tasks.get-light"
	MethodTeamsTaskApprove         = "teams.tasks.approve"
	MethodTeamsTaskReject          = "teams.tasks.reject"
	MethodTeamsTaskComment         = "teams.tasks.comment"
	MethodTeamsTaskComments        = "teams.tasks.comments"
	MethodTeamsTaskEvents          = "teams.tasks.events"
	MethodTeamsTaskCreate          = "teams.tasks.create"
	MethodTeamsTaskDelete          = "teams.tasks.delete"
	MethodTeamsTaskDeleteBulk      = "teams.tasks.delete-bulk"
	MethodTeamsTaskAssign          = "teams.tasks.assign"
	MethodTeamsTaskActiveBySession = "teams.tasks.active-by-session"
	MethodTeamsMembersAdd          = "teams.members.add"
	MethodTeamsMembersRemove       = "teams.members.remove"
	MethodTeamsUpdate              = "teams.update"
	MethodTeamsKnownUsers          = "teams.known_users"
	MethodTeamsScopes              = "teams.scopes"
)

// Team workspace
const (
	MethodTeamsWorkspaceList   = "teams.workspace.list"
	MethodTeamsWorkspaceRead   = "teams.workspace.read"
	MethodTeamsWorkspaceDelete = "teams.workspace.delete"
)

// Team events
const (
	MethodTeamsEventsList = "teams.events.list"
)

// API key management
const (
	MethodAPIKeysList   = "api_keys.list"
	MethodAPIKeysCreate = "api_keys.create"
	MethodAPIKeysRevoke = "api_keys.revoke"
)

// Voices (ElevenLabs voice picker)
const (
	MethodVoicesList    = "voices.list"
	MethodVoicesRefresh = "voices.refresh"
)

// Phase 3+ - NICE TO HAVE methods
const (
	MethodLogsTail = "logs.tail"

	MethodTTSStatus      = "tts.status"
	MethodTTSEnable      = "tts.enable"
	MethodTTSDisable     = "tts.disable"
	MethodTTSConvert     = "tts.convert"
	MethodTTSSetProvider = "tts.setProvider"
	MethodTTSProviders   = "tts.providers"

	MethodBrowserAct        = "browser.act"
	MethodBrowserSnapshot   = "browser.snapshot"
	MethodBrowserScreenshot = "browser.screenshot"

	// Zalo Personal
	MethodZaloPersonalQRStart  = "zalo.personal.qr.start"
	MethodZaloPersonalContacts = "zalo.personal.contacts"

	// WhatsApp
	MethodWhatsAppQRStart = "whatsapp.qr.start"
)

// Workstations (Standard edition only — gated at router)
const (
	MethodWorkstationsList        = "workstations.list"
	MethodWorkstationsGet         = "workstations.get"
	MethodWorkstationsCreate      = "workstations.create"
	MethodWorkstationsUpdate      = "workstations.update"
	MethodWorkstationsDelete      = "workstations.delete"
	MethodWorkstationsTest        = "workstations.testConnection"
	MethodWorkstationsLinkAgent   = "workstations.linkAgent"
	MethodWorkstationsUnlinkAgent = "workstations.unlinkAgent"

	// Workstation permission allowlist CRUD (Phase 6)
	MethodWorkstationsPermList   = "workstations.permissions.list"
	MethodWorkstationsPermAdd    = "workstations.permissions.add"
	MethodWorkstationsPermRemove = "workstations.permissions.remove"
	MethodWorkstationsPermToggle = "workstations.permissions.toggle"

	// Workstation activity audit log (Phase 7)
	MethodWorkstationsListActivity = "workstations.activity.list"
)

// Agent hooks (Phase 3)
const (
	MethodHooksList    = "hooks.list"
	MethodHooksCreate  = "hooks.create"
	MethodHooksUpdate  = "hooks.update"
	MethodHooksDelete  = "hooks.delete"
	MethodHooksToggle  = "hooks.toggle"
	MethodHooksTest    = "hooks.test"
	MethodHooksHistory = "hooks.history"
)
