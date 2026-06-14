package cmd

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/nextlevelbuilder/goclaw/internal/edition"
	"github.com/nextlevelbuilder/goclaw/internal/store"
	"github.com/nextlevelbuilder/goclaw/internal/tools"
)

// builtinToolSeedData returns the canonical list of built-in tools to seed into the database.
// Seed preserves user-customized enabled/settings values across upgrades.
func builtinToolSeedData() []store.BuiltinToolDef {
	defs := []store.BuiltinToolDef{
		// filesystem
		{Name: "read_file", DisplayName: "Read File", Description: "Read the contents of a file from the agent's workspace by path", Category: "filesystem", Enabled: true},
		{Name: "write_file", DisplayName: "Write File", Description: "Write content to a file in the workspace, creating directories as needed", Category: "filesystem", Enabled: true},
		{Name: "list_files", DisplayName: "List Files", Description: "List files and directories in a given path within the workspace", Category: "filesystem", Enabled: true},
		{Name: "edit", DisplayName: "Edit File", Description: "Apply targeted search-and-replace edits to existing files without rewriting the entire file", Category: "filesystem", Enabled: true},

		// runtime
		{Name: "exec", DisplayName: "Execute Command", Description: "Execute a shell command in the workspace and return stdout/stderr", Category: "runtime", Enabled: true,
			Metadata: json.RawMessage(`{"config_hint":"Config → Tools → Exec Approval"}`),
		},
		{Name: "wait", DisplayName: "Wait", Description: "Pause the current agent tool sequence for a bounded number of milliseconds", Category: "runtime", Enabled: true},
		{Name: "datetime", DisplayName: "Date & Time", Description: "Get the current date and time in UTC and a requested timezone", Category: "runtime", Enabled: true},
		{Name: "secure_cli_run", DisplayName: "Secure CLI Run", Description: "Invoke a registered CLI binary (gh, kubectl, etc.) under the secure_cli credentialed exec gate. Credentials are injected per-grant; shell operators and deny_args are blocked", Category: "runtime", Enabled: true},
		{Name: "workstation_exec", DisplayName: "Workstation Exec", Description: "Execute a command on a remote user-owned workstation (SSH or Docker backend). Streams stdout/stderr as events. Returns exit code and output tail", Category: "runtime", Enabled: true,
			Requires: []string{"standard_edition", "workstations"},
		},
		{Name: "claude_remote", DisplayName: "Claude Remote", Description: "Run Claude Code CLI on a remote workstation. Requires Claude CLI installed and authenticated on the workstation. Streams output as workstation.exec.chunk events", Category: "runtime", Enabled: true,
			Requires: []string{"standard_edition", "workstations"},
		},
		{Name: "codex_remote", DisplayName: "Codex Remote", Description: "Run codex exec on a remote sandbox pod over SSH. Requires codex authenticated on the pod. Resumes the last session by default; set fresh:true to start a new session. Streams output as workstation.exec.chunk events", Category: "runtime", Enabled: true,
			Requires: []string{"standard_edition", "workstations"},
		},

		// web
		{Name: "web_search", DisplayName: "Web Search", Description: "Search the web for information using a search engine (Brave or DuckDuckGo)", Category: "web", Enabled: true,
			Metadata: json.RawMessage(`{"config_hint":"Config → Tools → Web Search"}`),
		},
		{Name: "web_fetch", DisplayName: "Web Fetch", Description: "Fetch a web page or API endpoint and extract its text content", Category: "web", Enabled: true,
			Settings: json.RawMessage(`{"extractors":[{"name":"defuddle","enabled":true,"base_url":"https://fetch.goclaw.sh/","max_retries":2},{"name":"html-to-markdown","enabled":true}]}`),
		},

		// memory
		{Name: "memory_search", DisplayName: "Memory Search", Description: "Search through the agent's long-term memory using semantic similarity", Category: "memory", Enabled: true,
			Requires: []string{"memory"},
		},
		{Name: "memory_get", DisplayName: "Memory Get", Description: "Retrieve a specific memory document by its file path", Category: "memory", Enabled: true,
			Requires: []string{"memory"},
		},
		{Name: "memory_expand", DisplayName: "Memory Expand", Description: "Load full content for a memory entry by ID. Returns the complete episodic summary for deep context", Category: "memory", Enabled: true,
			Requires: []string{"memory"},
		},
		{Name: "knowledge_graph_search", DisplayName: "Knowledge Graph Search", Description: "Search entities, relationships, and observations in the agent's knowledge graph", Category: "memory", Enabled: true,
			Settings: json.RawMessage(`{"extract_on_memory_write":false,"extraction_provider":"","extraction_model":"","min_confidence":0.75}`),
			Requires: []string{"knowledge_graph"},
		},
		{Name: "vault_search", DisplayName: "Vault Search", Description: "Search across all knowledge sources (vault docs, memory, knowledge graph). Each result carries a source-specific id field for follow-up tools", Category: "memory", Enabled: true,
			Requires: []string{"vault"},
		},
		{Name: "vault_read", DisplayName: "Vault Read", Description: "Read full content of a vault document by doc_id (obtained from vault_search). Text-only — for media use read_image/read_audio/read_video/read_document", Category: "memory", Enabled: true,
			Requires: []string{"vault"},
		},

		// media — user must configure provider chain via UI before use
		{Name: "read_image", DisplayName: "Read Image", Description: "Analyze images using a vision-capable LLM provider", Category: "media", Enabled: false,
			Requires: []string{"vision_provider"},
		},
		{Name: "read_document", DisplayName: "Read Document", Description: "Analyze documents (PDF, Word, Excel, PowerPoint, CSV, etc.) using a document-capable LLM provider", Category: "media", Enabled: false,
			Requires: []string{"document_provider"},
		},
		{Name: "create_image", DisplayName: "Create Image", Description: "Generate images from text prompts using an image generation provider", Category: "media", Enabled: false,
			Requires: []string{"image_gen_provider"},
		},
		{Name: "read_audio", DisplayName: "Read Audio", Description: "Analyze audio files (speech, music, sounds) using an audio-capable LLM provider", Category: "media", Enabled: false,
			Requires: []string{"audio_provider"},
		},
		{Name: "read_video", DisplayName: "Read Video", Description: "Analyze video files using a video-capable LLM provider", Category: "media", Enabled: false,
			Requires: []string{"video_provider"},
		},
		{Name: "create_video", DisplayName: "Create Video", Description: "Generate videos from text descriptions using AI", Category: "media", Enabled: false,
			Requires: []string{"video_gen_provider"},
		},
		{Name: "create_audio", DisplayName: "Create Audio", Description: "Generate music or sound effects from text descriptions using AI", Category: "media", Enabled: false,
			Requires: []string{"audio_gen_provider"},
		},
		{Name: "tts", DisplayName: "Text to Speech", Description: "Convert text to natural-sounding speech audio", Category: "media", Enabled: true,
			Requires: []string{"tts_provider"},
			Metadata: json.RawMessage(`{"config_hint":"Config → TTS"}`),
		},
		{Name: "stt", DisplayName: "Speech-to-Text", Description: "Transcribe voice/audio messages to text using ElevenLabs Scribe or a proxy service", Category: "media", Enabled: true,
			Requires: []string{"stt_provider"},
			Metadata: json.RawMessage(`{"config_hint":"Config → Audio → STT"}`),
		},

		// browser
		{Name: "browser", DisplayName: "Browser", Description: "Automate browser interactions: navigate pages, click elements, fill forms, take screenshots", Category: "browser", Enabled: true,
			Requires: []string{"browser"},
			Metadata: json.RawMessage(`{"config_hint":"Config → Tools → Browser"}`),
		},

		// sessions
		{Name: "sessions_list", DisplayName: "List Sessions", Description: "List active chat sessions across all channels", Category: "sessions", Enabled: true},
		{Name: "session_status", DisplayName: "Session Status", Description: "Get the current status and metadata of a specific chat session", Category: "sessions", Enabled: true},
		{Name: "sessions_history", DisplayName: "Session History", Description: "Retrieve the message history of a specific chat session", Category: "sessions", Enabled: true},
		{Name: "sessions_send", DisplayName: "Send to Session", Description: "Send a message to an active chat session on behalf of the agent", Category: "sessions", Enabled: true},
		{Name: "list_group_members", DisplayName: "List Group Members", Description: "List all members of the current group chat. Returns member IDs and display names. Only works in group conversations on supported channels (Lark/Feishu)", Category: "sessions", Enabled: true},

		// messaging
		{Name: "message", DisplayName: "Message", Description: "Send a proactive message to a user on a connected channel (Telegram, Discord, etc.)", Category: "messaging", Enabled: true},
		{Name: "send_file", DisplayName: "Send File", Description: "Send an existing workspace file as an attachment in the current chat (does not create or modify the file)", Category: "messaging", Enabled: true},
		{Name: "create_forum_topic", DisplayName: "Create Forum Topic", Description: "Create a new forum topic in a Telegram supergroup. Returns the topic's message_thread_id for routing messages to the topic", Category: "messaging", Enabled: true},
		{Name: "zalo_personal_create_poll", DisplayName: "Zalo: Create Poll", Description: "Create a poll in the current Zalo Personal group chat. Only works in groups, not DMs", Category: "messaging", Enabled: true},
		{Name: "zalo_personal_list_polls", DisplayName: "Zalo: List Polls", Description: "List recent polls from the current Zalo Personal group board with vote counts and voter IDs/names", Category: "messaging", Enabled: true},
		{Name: "zalo_personal_get_poll", DisplayName: "Zalo: Get Poll", Description: "Read the current state of a Zalo Personal poll: options, vote counts, locked flag", Category: "messaging", Enabled: true},
		{Name: "zalo_personal_vote_poll", DisplayName: "Zalo: Vote Poll", Description: "Vote on a Zalo Personal poll. Pass empty option_ids to unvote", Category: "messaging", Enabled: true},
		{Name: "zalo_personal_lock_poll", DisplayName: "Zalo: Lock Poll", Description: "Close a Zalo Personal poll so no more votes are accepted", Category: "messaging", Enabled: true},
		{Name: "zalo_personal_add_poll_options", DisplayName: "Zalo: Add Poll Options", Description: "Append new options to an existing Zalo Personal poll", Category: "messaging", Enabled: true},
		{Name: "zalo_personal_create_reminder", DisplayName: "Zalo: Create Reminder", Description: "Schedule a Zalo Personal reminder in a group or DM. Returns the reminder ID", Category: "messaging", Enabled: true},
		{Name: "zalo_personal_remove_reminder", DisplayName: "Zalo: Remove Reminder", Description: "Remove a previously-created Zalo Personal reminder by ID", Category: "messaging", Enabled: true},

		// scheduling
		{Name: "cron", DisplayName: "Cron Scheduler", Description: "Schedule or manage recurring tasks using cron expressions, at-times, or intervals", Category: "scheduling", Enabled: true,
			Metadata: json.RawMessage(`{"config_hint":"Config → Cron"}`),
		},
		{Name: "heartbeat", DisplayName: "Heartbeat", Description: "Manage agent heartbeat — periodic proactive check-in that wakes the agent on a schedule", Category: "scheduling", Enabled: true},
		{Name: "enter_standby", DisplayName: "Enter Standby", Description: "Pause replies in the current thread for a duration. The agent will still observe and remember messages but will not reply until the pause expires", Category: "scheduling", Enabled: true},

		// subagents
		{Name: "spawn", DisplayName: "Spawn", Description: "Spawn a subagent to handle a task in the background", Category: "subagents", Enabled: true,
			Metadata: json.RawMessage(`{"config_hint":"Config → Agents Defaults"}`),
		},
		{Name: "delegate", DisplayName: "Delegate", Description: "Delegate a task to a linked agent. The target agent must be connected via an agent link", Category: "subagents", Enabled: true},

		// skills
		{Name: "skill_search", DisplayName: "Skill Search", Description: "Search for available skills by keyword or description to find relevant capabilities", Category: "skills", Enabled: true},
		{Name: "use_skill", DisplayName: "Use Skill", Description: "Activate a skill to use its specialized capabilities (tracing marker)", Category: "skills", Enabled: true},
		{Name: "publish_skill", DisplayName: "Publish Skill", Description: "Register a skill directory (created via skill-creator) in the system database, making it discoverable and grantable to agents", Category: "skills", Enabled: true},
		{Name: "skill_manage", DisplayName: "Skill Manager", Description: "Create, patch, or delete skills from conversation experience", Category: "skills", Enabled: true},

		// teams
		{Name: "team_tasks", DisplayName: "Team Tasks", Description: "View, create, update, and complete tasks on the team task board", Category: "teams", Enabled: true,
			Requires: []string{"managed_mode", "teams"},
		},
	}

	// Lite edition: remove skill management tools — not available on desktop.
	if !edition.Current().TeamFullMode {
		liteHidden := map[string]bool{"skill_manage": true, "publish_skill": true}
		filtered := defs[:0]
		for _, d := range defs {
			if !liteHidden[d.Name] {
				filtered = append(filtered, d)
			}
		}
		return filtered
	}
	return defs
}

// seedBuiltinTools seeds built-in tool definitions into the database.
// Idempotent: preserves user-customized enabled/settings on conflict.
func seedBuiltinTools(ctx context.Context, bts store.BuiltinToolStore) {
	seeds := builtinToolSeedData()
	if err := bts.Seed(ctx, seeds); err != nil {
		slog.Error("failed to seed builtin tools", "error", err)
		return
	}
	slog.Info("builtin tools seeded", "count", len(seeds))
}

// mediaToolNames lists media tools whose settings should use chain format.
var mediaToolNames = map[string]bool{
	"read_image": true, "read_document": true, "create_image": true,
	"read_audio": true, "read_video": true, "create_video": true, "create_audio": true,
}

// migrateBuiltinToolSettings converts legacy flat settings {"provider":"X","model":"Y"}
// to chain format {"providers":[...]} in the database. Runs once at startup.
func migrateBuiltinToolSettings(ctx context.Context, bts store.BuiltinToolStore) {
	all, err := bts.List(ctx)
	if err != nil {
		slog.Warn("builtin_tools: failed to list for migration", "error", err)
		return
	}

	var migrated int
	for _, t := range all {
		if !mediaToolNames[t.Name] {
			continue
		}
		if len(t.Settings) == 0 || string(t.Settings) == "{}" {
			continue
		}

		// Detect legacy flat format: has "provider" key but no "providers" key
		var raw map[string]json.RawMessage
		if err := json.Unmarshal(t.Settings, &raw); err != nil {
			continue
		}
		if _, hasProviders := raw["providers"]; hasProviders {
			continue // already chain format
		}
		if _, hasProvider := raw["provider"]; !hasProvider {
			continue // neither format, skip
		}

		// Parse legacy flat fields
		var flat struct {
			Provider string `json:"provider"`
			Model    string `json:"model"`
		}
		if err := json.Unmarshal(t.Settings, &flat); err != nil || flat.Provider == "" {
			continue
		}

		// Convert to chain format
		chain := map[string]any{
			"providers": []map[string]any{{
				"provider":    flat.Provider,
				"model":       flat.Model,
				"enabled":     true,
				"timeout":     120,
				"max_retries": 2,
			}},
		}
		newSettings, err := json.Marshal(chain)
		if err != nil {
			continue
		}

		if err := bts.Update(ctx, t.Name, map[string]any{"settings": json.RawMessage(newSettings)}); err != nil {
			slog.Warn("builtin_tools: failed to migrate settings", "tool", t.Name, "error", err)
			continue
		}
		migrated++
	}

	if migrated > 0 {
		slog.Info("builtin_tools: migrated legacy settings to chain format", "count", migrated)
	}
}

// backfillWebFetchSettings ensures the web_fetch tool has extractor chain settings.
// Existing deployments may have a web_fetch row with null/empty settings from a prior seed.
// This backfills the default chain so Defuddle is available out of the box.
func backfillWebFetchSettings(ctx context.Context, bts store.BuiltinToolStore) {
	t, err := bts.Get(ctx, "web_fetch")
	if err != nil || t == nil {
		return // not seeded yet, will be populated by next seed
	}
	if len(t.Settings) > 0 && string(t.Settings) != "{}" && string(t.Settings) != "null" {
		return // already has settings, don't overwrite
	}
	defaultSettings := json.RawMessage(`{"extractors":[{"name":"defuddle","enabled":true,"base_url":"https://fetch.goclaw.sh/","max_retries":2},{"name":"html-to-markdown","enabled":true}]}`)
	if err := bts.Update(ctx, "web_fetch", map[string]any{"settings": defaultSettings}); err != nil {
		slog.Warn("builtin_tools: failed to backfill web_fetch settings", "error", err)
		return
	}
	slog.Info("builtin_tools: backfilled web_fetch extractor chain settings")
}

// applyBuiltinToolDisables unregisters disabled builtin tools from the registry.
// Called at startup and on cache invalidation.
func applyBuiltinToolDisables(ctx context.Context, bts store.BuiltinToolStore, toolsReg *tools.Registry) {
	all, err := bts.List(ctx)
	if err != nil {
		slog.Warn("failed to list builtin tools for disable check", "error", err)
		return
	}

	var disabledCount, enabledCount int
	for _, t := range all {
		if !t.Enabled {
			toolsReg.Disable(t.Name)
			disabledCount++
		} else {
			toolsReg.Enable(t.Name)
			enabledCount++
		}
	}
	if disabledCount > 0 {
		slog.Info("builtin tools updated", "disabled", disabledCount, "enabled", enabledCount)
	}
}
