package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/audio"
	"github.com/nextlevelbuilder/goclaw/internal/audio/gemini"
	"github.com/nextlevelbuilder/goclaw/internal/i18n"
	"github.com/nextlevelbuilder/goclaw/internal/store"
	"github.com/nextlevelbuilder/goclaw/internal/tts"
)

// TtsTool is an agent tool that converts text to speech audio.
// Matching TS src/agents/tools/tts-tool.ts.
// Implements Tool + ContextualTool interfaces.
// Per-call channel is read from ctx for thread-safety.
type TtsTool struct {
	mu            sync.RWMutex
	manager       *tts.Manager
	vaultIntc     *VaultInterceptor
	systemConfigs store.SystemConfigStore
}

func (t *TtsTool) SetVaultInterceptor(v *VaultInterceptor) { t.vaultIntc = v }

// SetSystemConfigStore wires system_configs as the final voice/model fallback
// so the dashboard /tts page also affects LLM-invoked tts tool calls.
func (t *TtsTool) SetSystemConfigStore(s store.SystemConfigStore) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.systemConfigs = s
}

// NewTtsTool creates a TTS tool backed by the given manager.
func NewTtsTool(mgr *tts.Manager) *TtsTool {
	return &TtsTool{manager: mgr}
}

// UpdateManager swaps the underlying TTS manager (used on config reload).
func (t *TtsTool) UpdateManager(mgr *tts.Manager) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.manager = mgr
}

func (t *TtsTool) Name() string { return "tts" }

func (t *TtsTool) Description() string {
	return "Convert text to speech audio. Returns a MEDIA: path to the generated audio file. " +
		"Long text may time out — see 'text' description for length guidance."
}

func (t *TtsTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"text": map[string]any{
				"type": "string",
				"description": "Text to synthesize. Keep under ~1500 chars per call to stay below the typical 120s timeout. " +
					"For longer content, split into multiple sequential tts calls and stitch the audio, or summarize first.",
			},
			"voice": map[string]any{
				"type":        "string",
				"description": "Voice ID (provider-specific). Optional — uses tenant default if omitted.",
			},
			"model": map[string]any{
				"type":        "string",
				"description": "Model ID (provider-specific, e.g. eleven_v3). Optional — uses tenant default if omitted.",
			},
			"provider": map[string]any{
				"type":        "string",
				"description": "DO NOT PASS unless the user explicitly named a different provider. The tenant-configured primary provider is used automatically and will override any value you pass here.",
			},
		},
		"required": []string{"text"},
	}
}

// ttsOverride is the tenant settings shape for tts
// (stored in builtin_tool_tenant_configs.settings).
type ttsOverride struct {
	Primary        string `json:"primary,omitempty"`          // override primary provider name
	DefaultVoiceID string `json:"default_voice_id,omitempty"` // tenant-default voice id
	DefaultModel   string `json:"default_model,omitempty"`    // tenant-default model id
}

// agentAudioConfig is the JSON shape read from AgentAudioSnapshot.OtherConfig
// for per-agent TTS tuning. Keys match the agents.other_config column.
type agentAudioConfig struct {
	TTSVoiceID string `json:"tts_voice_id,omitempty"`
	TTSModelID string `json:"tts_model_id,omitempty"`
	// TTSParams carries the per-agent generic TTS override keys (speed, emotion, style).
	// Stored as generic keys; AdaptAgentParams converts to provider-specific keys per attempt.
	TTSParams map[string]any `json:"tts_params,omitempty"`
}

// resolveVoiceAndModel precedence: args > agent OtherConfig > tenant builtin
// settings > system_configs[tts.<provider>.voice/model] (dashboard /tts page).
// Empty result = use provider default.
func (t *TtsTool) resolveVoiceAndModel(ctx context.Context, providerName, argVoice, argModel string) (voice, model string) {
	voice, model = argVoice, argModel

	// Pull agent-level config from the dispatcher-injected snapshot.
	var agentCfg agentAudioConfig
	if snap, ok := store.AgentAudioFromCtx(ctx); ok {
		if len(snap.OtherConfig) > 0 {
			if err := json.Unmarshal(snap.OtherConfig, &agentCfg); err != nil {
				slog.Warn("tts: failed to parse agent OtherConfig", "error", err, "agent_id", snap.AgentID)
			}
		}
	} else if agentID := store.AgentIDFromContext(ctx); agentID != uuid.Nil {
		// Producer-consumer contract violation: when an agent ctx is in play
		// (AgentIDFromContext returns non-nil), AgentAudioSnapshot should have
		// been injected by the dispatcher. Log as Warn so ops can spot a
		// dispatch-layer regression. Silent when no agent is scoped (unit
		// tests and callers outside the agent loop).
		slog.Warn("tts: agent audio snapshot missing — dispatcher producer may be offline", "agent_id", agentID)
	}

	// Pull tenant defaults from builtin tool settings.
	var tenantCfg ttsOverride
	if settings := BuiltinToolSettingsFromCtx(ctx); settings != nil {
		if raw, ok := settings["tts"]; ok && len(raw) > 0 {
			if err := json.Unmarshal(raw, &tenantCfg); err != nil {
				slog.Warn("tts: failed to parse tenant settings for voice/model", "error", err)
			}
		}
	}

	if voice == "" {
		if agentCfg.TTSVoiceID != "" {
			voice = agentCfg.TTSVoiceID
		} else if tenantCfg.DefaultVoiceID != "" {
			voice = tenantCfg.DefaultVoiceID
		}
	}
	if model == "" {
		if agentCfg.TTSModelID != "" {
			model = agentCfg.TTSModelID
		} else if tenantCfg.DefaultModel != "" {
			model = tenantCfg.DefaultModel
		}
	}

	// Final fallback: dashboard /tts settings.
	if (voice == "" || model == "") && t.systemConfigs != nil && providerName != "" {
		if voice == "" {
			if v, err := t.systemConfigs.Get(ctx, "tts."+providerName+".voice"); err == nil && v != "" {
				voice = v
			}
		}
		if model == "" {
			if m, err := t.systemConfigs.Get(ctx, "tts."+providerName+".model"); err == nil && m != "" {
				model = m
			}
		}
	}
	return voice, model
}

// tenantPrimaryProvider returns the tenant-configured primary provider name
// when set, else "". Checks builtin_tool_tenant_configs first (tools UI), then
// falls back to system_configs.tts.provider (dashboard /tts page) since users
// configure the primary on the dashboard, not the tools UI.
func (t *TtsTool) tenantPrimaryProvider(ctx context.Context, mgr *tts.Manager) string {
	if settings := BuiltinToolSettingsFromCtx(ctx); settings != nil {
		if raw, ok := settings["tts"]; ok && len(raw) > 0 {
			var override ttsOverride
			if err := json.Unmarshal(raw, &override); err != nil {
				slog.Warn("tts: failed to parse tenant override", "error", err)
			} else if override.Primary != "" {
				if _, exists := mgr.GetProvider(override.Primary); exists {
					return override.Primary
				}
				slog.Warn("tts: tenant override references unknown provider", "primary", override.Primary)
			}
		}
	}
	if t.systemConfigs != nil {
		if v, err := t.systemConfigs.Get(ctx, "tts.provider"); err == nil && v != "" {
			if _, exists := mgr.GetProvider(v); exists {
				return v
			}
			slog.Warn("tts: dashboard primary references unknown provider", "primary", v)
		}
	}
	return ""
}

// resolvePrimary returns the effective primary provider when the LLM did not
// supply one. Tenant override wins over manager default.
func (t *TtsTool) resolvePrimary(ctx context.Context, mgr *tts.Manager) string {
	if p := t.tenantPrimaryProvider(ctx, mgr); p != "" {
		return p
	}
	return mgr.PrimaryProvider()
}

// resolveAgentGenericTTSParams reads the per-agent TTSParams generic map from
// the dispatcher-injected AgentAudioSnapshot. Returns nil when no snapshot
// is present or no tts_params are configured. The caller is responsible for
// calling audio.AdaptAgentParams(generic, providerName) PER-ATTEMPT to convert
// generic keys to provider-specific keys (Finding #1 CRITICAL).
func (t *TtsTool) resolveAgentGenericTTSParams(ctx context.Context) map[string]any {
	snap, ok := store.AgentAudioFromCtx(ctx)
	if !ok || len(snap.OtherConfig) == 0 {
		return nil
	}
	var agentCfg agentAudioConfig
	if err := json.Unmarshal(snap.OtherConfig, &agentCfg); err != nil {
		slog.Warn("tts: failed to parse agent OtherConfig for tts_params", "error", err)
		return nil
	}
	return agentCfg.TTSParams
}

// SetContext is a no-op; channel is now read from ctx (thread-safe).
func (t *TtsTool) SetContext(channel, _ string) {}

// mergeParams returns a new map with base keys overwritten by overrides.
// Returns overrides directly when base is nil (avoids allocation).
func mergeParams(base, overrides map[string]any) map[string]any {
	if len(base) == 0 {
		return overrides
	}
	out := make(map[string]any, len(base)+len(overrides))
	maps.Copy(out, base)
	maps.Copy(out, overrides)
	return out
}

func (t *TtsTool) Execute(ctx context.Context, args map[string]any) *Result {
	text, _ := args["text"].(string)
	if text == "" {
		return &Result{ForLLM: "error: text is required", IsError: true}
	}

	// Default 180s — CPU vieneu synthesis can take 50-75s for ~200 chars, plus
	// fallback chain on failure. Tenant tts.timeout_ms overrides via system_configs.
	timeoutMs := 180000
	if t.systemConfigs != nil {
		if v, err := t.systemConfigs.Get(ctx, "tts.timeout_ms"); err == nil && v != "" {
			if n, perr := strconv.Atoi(v); perr == nil && n > 0 {
				timeoutMs = n
			}
		}
	}
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(timeoutMs)*time.Millisecond)
		defer cancel()
	}

	argVoice, _ := args["voice"].(string)
	argModel, _ := args["model"].(string)
	providerName, _ := args["provider"].(string)

	// Read generic agent TTS params once; adapt PER-ATTEMPT below (Finding #1 CRITICAL).
	// Storing generic keys here so each fallback provider gets its own adapted copy.
	genericAgentParams := t.resolveAgentGenericTTSParams(ctx)

	// Snapshot manager pointer under read lock so config reloads don't race.
	t.mu.RLock()
	mgr := t.manager
	t.mu.RUnlock()

	if tenantPrimary := t.tenantPrimaryProvider(ctx, mgr); tenantPrimary != "" && providerName != "" && providerName != tenantPrimary {
		slog.Warn("tts: ignoring llm-supplied provider, tenant primary takes precedence",
			"llm_provider", providerName, "tenant_primary", tenantPrimary)
		providerName = ""
	}

	effectiveProvider := providerName
	if effectiveProvider == "" {
		effectiveProvider = t.resolvePrimary(ctx, mgr)
	}
	voice, model := t.resolveVoiceAndModel(ctx, effectiveProvider, argVoice, argModel)

	// Determine format based on channel (read from ctx — thread-safe)
	channel := ToolChannelFromCtx(ctx)
	opts := tts.Options{Voice: voice, Model: model}
	if channel == "telegram" {
		opts.Format = "opus"
	}

	var result *tts.SynthResult
	var err error

	if providerName != "" {
		// Use specific provider (explicit call param takes precedence).
		// Adapt generic agent params to this specific provider's native keys.
		p, ok := mgr.GetProvider(providerName)
		if !ok {
			return &Result{ForLLM: fmt.Sprintf("error: tts provider not found: %s", providerName), IsError: true}
		}
		if adapted := audio.AdaptAgentParams(genericAgentParams, providerName); len(adapted) > 0 {
			opts.Params = mergeParams(opts.Params, adapted)
		}
		result, err = p.Synthesize(ctx, text, opts)
	} else {
		// Resolve primary from tenant settings or default.
		primary := t.resolvePrimary(ctx, mgr)
		if p, ok := mgr.GetProvider(primary); ok {
			// Adapt for the primary provider attempt specifically.
			primaryOpts := opts
			if adapted := audio.AdaptAgentParams(genericAgentParams, primary); len(adapted) > 0 {
				primaryOpts.Params = mergeParams(opts.Params, adapted)
			}
			result, err = p.Synthesize(ctx, text, primaryOpts)
			if err != nil {
				slog.Warn("tts primary provider failed, trying fallback", "provider", primary, "error", err)
				// SynthesizeWithFallbackAdapted adapts genericAgentParams per-attempt
				// (Finding #1 CRITICAL): each fallback provider receives its own
				// provider-native keys, not the primary's adapted map.
				result, err = mgr.SynthesizeWithFallbackAdapted(ctx, text, opts, genericAgentParams)
			}
		} else {
			result, err = mgr.SynthesizeWithFallbackAdapted(ctx, text, opts, genericAgentParams)
		}
	}

	if err != nil {
		if errors.Is(err, gemini.ErrTextOnlyResponse) {
			locale := store.LocaleFromContext(ctx)
			msg := i18n.T(locale, i18n.MsgTtsGeminiTextOnly)
			return &Result{ForLLM: "error: " + msg, IsError: true}
		}
		return &Result{ForLLM: fmt.Sprintf("error: tts failed: %s", err.Error()), IsError: true}
	}

	// Write audio to workspace/tts/ so the agent can access the file.
	// Falls back to os.TempDir() if workspace is not available.
	ttsDir := os.TempDir()
	if ws := ToolWorkspaceFromCtx(ctx); ws != "" {
		ttsDir = filepath.Join(ws, "tts")
	}
	if err := os.MkdirAll(ttsDir, 0755); err != nil {
		return &Result{ForLLM: fmt.Sprintf("error: create tts directory: %s", err.Error()), IsError: true}
	}
	audioPath := filepath.Join(ttsDir, fmt.Sprintf("tts-%d.%s", time.Now().UnixNano(), result.Extension))
	if err := os.WriteFile(audioPath, result.Audio, 0644); err != nil {
		return &Result{ForLLM: fmt.Sprintf("error: write tts audio: %s", err.Error()), IsError: true}
	}

	// Return MEDIA: path (matching TS pattern)
	voiceTag := ""
	if channel == "telegram" && result.Extension == "ogg" {
		voiceTag = "[[audio_as_voice]]\n"
	}

	forLLM := fmt.Sprintf(
		"%sGenerated audio saved to: %s\n"+
			"To share it with the user, call `send_file(path=%q)` in this turn — "+
			"the audio is NOT auto-delivered. Pass the path EXACTLY as shown.",
		voiceTag, audioPath, audioPath,
	)
	r := &Result{ForLLM: forLLM}
	r.Deliverable = fmt.Sprintf("[Generated audio: %s]\nText: %s", filepath.Base(audioPath), text)
	if t.vaultIntc != nil {
		mimeType := "audio/" + result.Extension
		go t.vaultIntc.AfterWriteMedia(context.WithoutCancel(ctx), audioPath, text, mimeType)
	}
	return r
}
