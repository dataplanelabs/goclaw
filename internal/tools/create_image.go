package tools

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/nextlevelbuilder/goclaw/internal/providers"
	"github.com/nextlevelbuilder/goclaw/internal/skills"
)

// credentialProvider is a narrow interface for providers that expose API credentials.
type credentialProvider interface {
	APIKey() string
	APIBase() string
}

// imageGenProviderPriority is the default order for image generation providers.
var imageGenProviderPriority = []string{"openrouter", "gemini", "openai", "minimax", "dashscope", "byteplus"}

// imageGenModelDefaults maps provider names to default image generation models.
var imageGenModelDefaults = map[string]string{
	"openrouter": "google/gemini-2.5-flash-image",
	"openai":     "gpt-image-1.5",
	"gemini":     "gemini-2.5-flash-image",
	"minimax":    "image-01",
	"dashscope":  "wan2.6-image",
	"byteplus":   "seedream-5-0-260128",
}

// CreateImageTool generates images using an image generation API.
type CreateImageTool struct {
	registry        *providers.Registry
	vaultIntc       *VaultInterceptor
	allowedPrefixes []string // extra read-allowed prefixes (skills dirs, user paths)
	deniedPrefixes  []string // workspace-relative prefixes to reject (memory.db, config.json)
}

func (t *CreateImageTool) SetVaultInterceptor(v *VaultInterceptor) { t.vaultIntc = v }

// AllowPaths / DenyPaths give create_image the same workspace path-loading reach
// as read_image, so reference_image_ids can name any in-workspace image the LLM
// organized (.uploads/, portraits/, …) — wired identically at startup.
func (t *CreateImageTool) AllowPaths(prefixes ...string) {
	t.allowedPrefixes = append(t.allowedPrefixes, prefixes...)
}

func (t *CreateImageTool) DenyPaths(prefixes ...string) {
	t.deniedPrefixes = append(t.deniedPrefixes, prefixes...)
}

func NewCreateImageTool(registry *providers.Registry) *CreateImageTool {
	return &CreateImageTool{registry: registry}
}

func (t *CreateImageTool) Name() string { return "create_image" }

func (t *CreateImageTool) Description() string {
	return "Generate an image from a text description using an image generation model. " +
		"Optionally pass reference_image_ids (IDs from <media:image id='...'> tags or skill asset paths returned by use_skill) " +
		"to use attached photos or exact logos as reference for face / composition / style preservation. Returns a MEDIA: path to the generated image file."
}

func (t *CreateImageTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"prompt": map[string]any{
				"type":        "string",
				"description": "Text description of the image to generate.",
			},
			"aspect_ratio": map[string]any{
				"type":        "string",
				"description": "Aspect ratio: '1:1' (default), '3:4', '4:3', '9:16', '16:9'.",
			},
			"filename_hint": map[string]any{
				"type":        "string",
				"description": "Short descriptive filename (no extension). Example: 'sunset-beach', 'company-logo'.",
			},
			"reference_image_ids": map[string]any{
				"type":        "array",
				"items":       map[string]any{"type": "string"},
				"description": "Optional. Reference images for face/composition/style preservation and exact logos. Accepts media IDs from <media:image id='...'> tags, file paths from path='...' attrs, skill asset paths returned by use_skill, or the bare filename (basename of the path). Looks across the current turn, user-uploaded history, and activated skill assets. First entry is the primary reference. Max 4 (provider-aware: Gemini 4, OpenRouter 4, OpenAI gpt-image-* 4, MiniMax 1 — character only). Animated GIFs and SVG not supported. If any id cannot be resolved, or if the configured Create Image chain cannot generate with references, the tool returns an error and no prompt-only image is generated.",
			},
		},
		"required": []string{"prompt"},
	}
}

func (t *CreateImageTool) Execute(ctx context.Context, args map[string]any) (result *Result) {
	timeout := toolTimeoutFromEnv("CREATE_IMAGE")
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	defer func() {
		if result != nil && result.IsError && errors.Is(ctx.Err(), context.DeadlineExceeded) {
			result = ErrorResult(fmt.Sprintf("create_image timed out after %s (set CREATE_IMAGE_TIMEOUT_SEC to adjust). Provider may be slow or unreachable; retry or switch provider.", timeout))
		}
	}()

	prompt, _ := args["prompt"].(string)
	if prompt == "" {
		return ErrorResult("prompt is required")
	}
	aspectRatio, _ := args["aspect_ratio"].(string)
	if aspectRatio == "" {
		aspectRatio = "1:1"
	}
	filenameHint, _ := args["filename_hint"].(string)

	var refImages []providers.ImageContent
	var trimmedRefIDs []string
	_, explicitRefIDs := args["reference_image_ids"]
	if idsAny, ok := args["reference_image_ids"]; ok {
		ids := toStringSlice(idsAny)
		if len(ids) > 0 {
			availableRefs := t.appendWorkspaceImageRefs(ctx, availableImageRefs(ctx), ids)
			var missingRefIDs, unusableRefIDs []string
			refImages, missingRefIDs, unusableRefIDs, trimmedRefIDs = resolveRefImageIDsDetailed(ids, availableRefs, maxResolvedRefImages)
			slog.Info("create_image: reference images resolved",
				"requested", len(ids), "loaded", len(refImages), "missing", len(missingRefIDs),
				"unusable", len(unusableRefIDs), "trimmed", len(trimmedRefIDs))

			// Fail fast on unresolved refs: generating from the resolved subset
			// yields the wrong face/logo (trace 019e7256). Trimmed ≠ error.
			if len(refImages) == 0 && len(unusableRefIDs) == 0 {
				return ErrorResult(formatRefResolveError(ids, availableRefs))
			}
			if len(missingRefIDs) > 0 {
				return ErrorResult(formatRefPartialResolveError(missingRefIDs, availableRefs))
			}
			if len(unusableRefIDs) > 0 {
				return ErrorResult(formatRefUnusableError(unusableRefIDs))
			}
		}
	}

	if len(refImages) == 0 && !explicitRefIDs {
		if skillRefs := skillAssetImageRefsFromCtx(ctx); len(skillRefs) > 0 && promptAppearsToRequireImageRef(prompt) {
			return ErrorResult(formatMissingSkillAssetRefsError(skillRefs))
		}
	}

	// Defensive auto-inject: when the LLM omits reference_image_ids despite
	// the user having uploaded an image in the current turn, inject those
	// refs anyway. Triggered by weaker tool-using models (e.g. glm-5-turbo)
	// that compose detailed prompts from read_image output but forget to
	// carry the photo's id forward to create_image — producing generated
	// images with random faces instead of the user's actual photo subject.
	if len(refImages) == 0 {
		if currentRefs := CurrentTurnUserImageRefsFromCtx(ctx); len(currentRefs) > 0 {
			ids := make([]string, 0, len(currentRefs))
			for _, r := range currentRefs {
				ids = append(ids, r.ID)
			}
			refImages = resolveRefImageIDs(ctx, ids, currentRefs, maxRefImages)
			if len(refImages) > 0 {
				slog.Info("create_image: auto-injected user current-turn images as references",
					"ref_count", len(refImages), "ref_ids", ids)
			}
		}
	}

	originalChain := ResolveMediaProviderChain(ctx, "create_image", "", "",
		imageGenProviderPriority, imageGenModelDefaults, t.registry)

	// Refs-aware chain selection: when refs are present, use only providers that
	// genuinely support image-edit. Do not degrade to text-only generation for
	// explicit refs: producing an image without the requested face/logo is worse
	// than surfacing a tool error.
	chainResult, err := runImageGenChain(ctx, t, originalChain, prompt, aspectRatio, refImages)
	if err != nil {
		if len(refImages) > 0 {
			return ErrorResult(fmt.Sprintf(
				"image generation with reference images failed; no prompt-only image was generated because the reference image(s) are required. "+
					"Fix the Create Image provider chain or retry with a refs-capable provider. Underlying error: %v", err))
		}
		return ErrorResult(fmt.Sprintf("image generation failed: %v", err))
	}

	// Embed prompt into PNG tEXt metadata before writing to disk.
	// If embedding fails (malformed bytes, non-PNG) the original data is used unchanged.
	imageData := embedPromptIntoPNG(chainResult.Data, prompt)

	// Save to workspace under date-based folder (e.g. generated/2026-03-02/)
	workspace := ToolWorkspaceFromCtx(ctx)
	if workspace == "" {
		workspace = os.TempDir()
	}
	dateDir := filepath.Join(workspace, "generated", time.Now().Format("2006-01-02"))
	if err := os.MkdirAll(dateDir, 0755); err != nil {
		return ErrorResult(fmt.Sprintf("failed to create output directory: %v", err))
	}
	imagePath := filepath.Join(dateDir, mediaFileName(ctx, "image", filenameHint, "png"))
	if err := os.WriteFile(imagePath, imageData, 0644); err != nil {
		return ErrorResult(fmt.Sprintf("failed to save generated image: %v", err))
	}

	// Verify file was persisted (diagnostic for disappearing files).
	if fi, err := os.Stat(imagePath); err != nil {
		slog.Warn("create_image: file missing immediately after write", "path", imagePath, "error", err)
		return ErrorResult(fmt.Sprintf("generated image file missing after write: %v", err))
	} else {
		slog.Info("create_image: file saved", "path", imagePath, "size", fi.Size(), "data_len", len(imageData))
	}

	forLLM := fmt.Sprintf(
		"Generated image saved to: %s\n"+
			"To share it with the user, call `send_file(path=%q)` in this turn — "+
			"the image is NOT auto-delivered. Pass the path EXACTLY as shown.",
		imagePath, imagePath,
	)
	if len(trimmedRefIDs) > 0 {
		forLLM += formatRefTrimmedNote(trimmedRefIDs)
	}
	if refCap := t.refCapForProvider(ctx, chainResult.Provider); len(refImages) > refCap {
		// Refs were valid but the selected provider accepts fewer than supplied —
		// distinct from "did not resolve" (genuinely missing) above.
		forLLM += formatRefsOverProviderCapNote(chainResult.Provider, refCap, len(refImages))
	}
	out := &Result{ForLLM: forLLM}
	out.MediaPrompts = map[int]string{0: prompt}
	out.Deliverable = fmt.Sprintf("[Generated image: %s]\nPrompt: %s", filepath.Base(imagePath), prompt)
	if t.vaultIntc != nil {
		go t.vaultIntc.AfterWriteMedia(context.WithoutCancel(ctx), imagePath, prompt, "image/png")
	}
	out.Provider = chainResult.Provider
	out.Model = chainResult.Model
	if chainResult.Usage != nil {
		out.Usage = chainResult.Usage
	}
	return out
}

// filterChainForRefs returns chain entries whose underlying provider self-reports
// ImageRefs capability via providers.CapabilitiesAware. Operator-configured order
// is preserved. Caller falls back to text-only when the filtered chain is empty.
//
// Sole source of truth = each provider's Capabilities().ImageRefs. No name-pattern
// inference — operators decide capability by picking the right provider+type.
func filterChainForRefs(ctx context.Context, chain []MediaProviderEntry, registry *providers.Registry) []MediaProviderEntry {
	out := make([]MediaProviderEntry, 0, len(chain))
	for _, entry := range chain {
		p, err := registry.Get(ctx, entry.Provider)
		if err != nil {
			continue
		}
		ca, ok := p.(providers.CapabilitiesAware)
		if !ok {
			continue
		}
		if ca.Capabilities().ImageRefs {
			out = append(out, entry)
		}
	}
	return out
}

// injectImageGenParams sets the prompt + aspect_ratio + refs onto every
// chain entry's Params. Idempotent across re-runs: replaces the prior value
// rather than appending.
func injectImageGenParams(chain []MediaProviderEntry, prompt, aspectRatio string, refs []providers.ImageContent) {
	for i := range chain {
		if chain[i].Params == nil {
			chain[i].Params = make(map[string]any)
		}
		chain[i].Params["prompt"] = prompt
		chain[i].Params["aspect_ratio"] = aspectRatio
		chain[i].Params["reference_images"] = refs
	}
}

// runImageGenChain tries refs-mode when refs are present. It intentionally
// refuses prompt-only fallback for ref requests: a wrong face/logo/subject is a
// bad artifact, not a degraded success.
func runImageGenChain(
	ctx context.Context,
	t *CreateImageTool,
	chain []MediaProviderEntry,
	prompt, aspectRatio string,
	refImages []providers.ImageContent,
) (*ChainResult, error) {
	if len(refImages) > 0 {
		refsChain := filterChainForRefs(ctx, chain, t.registry)
		if len(refsChain) > 0 {
			injectImageGenParams(refsChain, prompt, aspectRatio, refImages)
			if res, err := ExecuteWithChain(ctx, refsChain, t.registry, t.callProvider); err == nil {
				return res, nil
			} else {
				slog.Warn("create_image: refs-mode chain failed; refusing text-only fallback",
					"err", truncateError(err), "refs_chain_len", len(refsChain))
				return nil, fmt.Errorf("refs-capable provider chain failed: %w", err)
			}
		}
		slog.Warn("create_image: no refs-capable provider in chain; refusing text-only fallback",
			"chain_len", len(chain))
		suggestion := "add Gemini, OpenAI (gpt-image-*), OpenRouter, or MiniMax to the Create Image provider chain"
		if available := refsCapableProviderNamesInRegistry(ctx, t.registry); len(available) > 0 {
			suggestion = fmt.Sprintf("reorder or add one of these refs-capable providers already enabled in your tenant: %v", available)
		}
		return nil, fmt.Errorf("reference images were requested but no configured provider in the Create Image chain supports image-to-image edits; %s", suggestion)
	}
	injectImageGenParams(chain, prompt, aspectRatio, nil)
	res, err := ExecuteWithChain(ctx, chain, t.registry, t.callProvider)
	return res, err
}

// refCapForProvider mirrors callProvider's branching: native providers
// (Codex/ChatGPT-OAuth) cap at codexImageRefCap regardless of instance name;
// others map by media type via ResolveProviderType (avoids the name-switch miss
// that made "codex-cnb" report the default cap).
func (t *CreateImageTool) refCapForProvider(ctx context.Context, providerName string) int {
	p, err := t.registry.Get(ctx, providerName)
	if err != nil {
		return imageRefCapForProvider(providerName)
	}
	if _, ok := p.(providers.NativeImageProvider); ok {
		return codexImageRefCap
	}
	return imageRefCapForProvider(ResolveProviderType(p))
}

// imageRefCapForProvider maps a media TYPE (not a raw instance name — pass via
// refCapForProvider) to the per-provider call-time ref cap (#219).
func imageRefCapForProvider(provider string) int {
	switch strings.ToLower(provider) {
	case "openai":
		return openAIEditRefCap // 16 — /v1/images/edits
	case "codex", "chatgpt", "chatgpt-oauth":
		return codexImageRefCap // 16 — native image tool
	case "gemini":
		return geminiRefCap // 4
	case "openrouter":
		return openRouterRefCap // 4
	case "minimax":
		return minimaxRefCap // 1 — character reference only
	case "dashscope", "byteplus":
		return 0 // refs unsupported; dropped
	default:
		return maxRefImages // conservative 4
	}
}

// formatRefsOverProviderCapNote tells the assistant that valid refs were dropped
// because the resolved provider's cap is lower than the number supplied — kept
// distinct from the "did not resolve" (missing) note.
func formatRefsOverProviderCapNote(provider string, maxRefs, supplied int) string {
	dropped := supplied - maxRefs
	return fmt.Sprintf(
		"\n\n⚠️ NOTE TO ASSISTANT: %d of %d reference images were valid but not sent — provider %q accepts at most %d. The first %d (in the order you listed them) were used. To include the rest, retry with ≤%d refs or switch to an OpenAI/Codex image provider (cap 16).",
		dropped, supplied, provider, maxRefs, maxRefs, maxRefs)
}

// refsCapableProviderNamesInRegistry returns names of providers in the registry
// whose Capabilities().ImageRefs is true. Used to enrich the dropped-refs note
// with operator-actionable provider names from the current tenant.
func refsCapableProviderNamesInRegistry(ctx context.Context, registry *providers.Registry) []string {
	if registry == nil {
		return nil
	}
	names := registry.List(ctx)
	out := make([]string, 0, len(names))
	for _, name := range names {
		p, err := registry.Get(ctx, name)
		if err != nil {
			continue
		}
		if ca, ok := p.(providers.CapabilitiesAware); ok && ca.Capabilities().ImageRefs {
			out = append(out, name)
		}
	}
	return out
}

// embedPromptIntoPNG wraps agent.EmbedPNGPrompt for the tools package.
// Logs a warning on error but always returns usable bytes.
func embedPromptIntoPNG(data []byte, prompt string) []byte {
	if prompt == "" {
		return data
	}
	// Import cycle guard: tools → agent is not allowed. Use the local pngEmbed function.
	out, err := pngEmbedPrompt(data, prompt)
	if err != nil {
		slog.Warn("create_image: failed to embed prompt into PNG metadata", "error", err)
		return data
	}
	return out
}

// callProvider dispatches to the correct image generation implementation based on provider type.
// If the resolved provider implements NativeImageProvider (e.g. CodexProvider via OAuth),
// the native path is used and cp may be nil. The credentialProvider path is only reached
// for API-key-backed providers.
func (t *CreateImageTool) callProvider(ctx context.Context, cp credentialProvider, providerName, model string, params map[string]any) ([]byte, *providers.Usage, error) {
	// Native path: provider implements the image_generation tool natively (e.g. Codex/OAuth).
	// The raw provider object is injected into params["_native_provider"] by ExecuteWithChain.
	// Must check before the cp==nil guard — these providers intentionally have no APIKey/APIBase.
	if rawProvider, ok := params["_native_provider"]; ok {
		if np, ok := rawProvider.(providers.NativeImageProvider); ok {
			prompt := GetParamString(params, "prompt", "")
			aspectRatio := GetParamString(params, "aspect_ratio", "1:1")
			imageModel := GetParamString(params, "image_model", "")
			parentModel, imageModel := normalizeNativeImageModels(rawProvider, model, imageModel)
			refs, _ := params["reference_images"].([]providers.ImageContent)
			if len(refs) > codexImageRefCap {
				slog.Warn("create_image: native image refs truncated at cap",
					"provider", providerName, "provided", len(refs), "cap", codexImageRefCap)
				refs = refs[:codexImageRefCap]
			}
			result, err := np.GenerateImage(ctx, providers.NativeImageRequest{
				Model:           parentModel,
				ImageModel:      imageModel,
				Prompt:          prompt,
				AspectRatio:     aspectRatio,
				OutputFormat:    "png",
				ReferenceImages: refs,
			})
			if err != nil {
				return nil, nil, fmt.Errorf("native image generation: %w", err)
			}
			return result.Data, result.Usage, nil
		}
	}

	if cp == nil {
		return nil, nil, fmt.Errorf("provider %q does not expose API credentials required for image generation", providerName)
	}
	prompt := GetParamString(params, "prompt", "")
	aspectRatio := GetParamString(params, "aspect_ratio", "1:1")

	slog.Info("create_image: calling image generation API",
		"provider", providerName, "model", model, "aspect_ratio", aspectRatio)

	switch GetParamString(params, "_provider_type", providerTypeFromName(providerName)) {
	case "gemini":
		return t.callGeminiNativeImageGen(ctx, cp.APIKey(), cp.APIBase(), model, prompt, params)
	case "openrouter":
		return t.callImageGenAPI(ctx, cp.APIKey(), cp.APIBase(), model, prompt, aspectRatio, params)
	case "openai":
		// Route to /v1/images/edits (multipart) when refs present; otherwise
		// /v1/images/generations (JSON). NOTE: this branch only fires for
		// API-key-backed OpenAI providers. OAuth-backed providers (Codex,
		// ChatGPTOAuthRouter) short-circuit above on `_native_provider` and
		// route through NativeImageProvider.GenerateImage.
		if refImages, _ := params["reference_images"].([]providers.ImageContent); len(refImages) > 0 {
			return callOpenAIImageEdit(ctx, cp.APIKey(), cp.APIBase(), model, prompt, params)
		}
		return t.callStandardImageGenAPI(ctx, cp.APIKey(), cp.APIBase(), model, prompt, params)
	case "minimax":
		return callMinimaxImageGen(ctx, cp.APIKey(), cp.APIBase(), model, prompt, params)
	case "dashscope":
		warnIfRefsDropped(params, providerName, "dashscope refs path pending Phase 04")
		return callDashScopeImageGen(ctx, cp.APIKey(), cp.APIBase(), model, prompt, params)
	case "byteplus":
		warnIfRefsDropped(params, providerName, "byteplus refs path pending Phase 04")
		return callBytePlusImageGen(ctx, cp.APIKey(), cp.APIBase(), model, prompt, params)
	default:
		warnIfRefsDropped(params, providerName, "openai-compat fallthrough does not forward refs")
		return t.callStandardImageGenAPI(ctx, cp.APIKey(), cp.APIBase(), model, prompt, params)
	}
}

func normalizeNativeImageModels(rawProvider any, parentModel, imageModel string) (string, string) {
	parentModel = strings.TrimSpace(parentModel)
	imageModel = strings.TrimSpace(imageModel)

	if imageModel == "" && isNativeImageModel(parentModel) {
		imageModel = parentModel
		parentModel = ""
	}

	if isCodexOnlyChatModel(parentModel) {
		if p, ok := rawProvider.(interface{ DefaultModel() string }); ok {
			if fallback := strings.TrimSpace(p.DefaultModel()); fallback != "" {
				slog.Warn("create_image: native image parent model normalized to provider default",
					"configured_model", parentModel, "fallback_model", fallback)
				parentModel = fallback
			}
		}
	}

	return parentModel, imageModel
}

func isNativeImageModel(model string) bool {
	return strings.HasPrefix(model, "gpt-image-")
}

func isCodexOnlyChatModel(model string) bool {
	return strings.Contains(model, "-codex")
}

// warnIfRefsDropped logs (best-effort) when a provider call function will
// silently ignore reference_images, so operators can see the gap in logs.
// Used by call paths that have no refs implementation yet.
func warnIfRefsDropped(params map[string]any, providerName, reason string) {
	if refs, _ := params["reference_images"].([]providers.ImageContent); len(refs) > 0 {
		slog.Warn("create_image: reference images dropped (provider has no refs support)",
			"provider", providerName, "count", len(refs), "reason", reason)
	}
}

// callImageGenAPI calls the OpenAI-compatible chat completions endpoint with image modalities.
// Works with OpenRouter (modalities: ["image","text"]).
//
// When params["reference_images"] holds ≥1 ImageContent, content becomes an
// array of {type:"text"} + {type:"image_url"} parts (data URL); refs are capped
// at openRouterRefCap. NO automatic model upgrade — caller's model is preserved
// (see plan codex review #5).
func (t *CreateImageTool) callImageGenAPI(ctx context.Context, apiKey, apiBase, model, prompt, aspectRatio string, params map[string]any) ([]byte, *providers.Usage, error) {
	refImages, _ := params["reference_images"].([]providers.ImageContent)
	if len(refImages) > openRouterRefCap {
		slog.Warn("create_image: truncating reference images for OpenRouter",
			"provided", len(refImages), "cap", openRouterRefCap)
		refImages = refImages[:openRouterRefCap]
	}

	var content any = prompt
	if len(refImages) > 0 {
		parts := make([]map[string]any, 0, len(refImages)+1)
		parts = append(parts, map[string]any{
			"type": "text",
			"text": prompt,
		})
		for _, img := range refImages {
			parts = append(parts, map[string]any{
				"type": "image_url",
				"image_url": map[string]any{
					"url": fmt.Sprintf("data:%s;base64,%s", img.MimeType, img.Data),
				},
			})
		}
		content = parts
	}

	body := map[string]any{
		"model": model,
		"messages": []map[string]any{
			{"role": "user", "content": content},
		},
		"modalities": []string{"image", "text"},
	}
	if aspectRatio != "" && aspectRatio != "1:1" {
		body["image_config"] = map[string]any{
			"aspect_ratio": aspectRatio,
		}
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal request: %w", err)
	}

	url := strings.TrimRight(apiBase, "/") + "/chat/completions"
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	client := &http.Client{} // timeout governed by chain context
	resp, err := client.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, nil, fmt.Errorf("API error %d: %s", resp.StatusCode, truncateBytes(respBody, 500))
	}

	return t.parseImageResponse(respBody)
}

// callStandardImageGenAPI uses the /images/generations endpoint (OpenAI and compatible providers).
func (t *CreateImageTool) callStandardImageGenAPI(ctx context.Context, apiKey, apiBase, model, prompt string, params map[string]any) ([]byte, *providers.Usage, error) {
	body := map[string]any{
		"model":           model,
		"prompt":          prompt,
		"n":               1,
		"response_format": "b64_json",
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal request: %w", err)
	}

	url := strings.TrimRight(apiBase, "/") + "/images/generations"
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	client := &http.Client{} // timeout governed by chain context
	resp, err := client.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, nil, fmt.Errorf("API error %d: %s", resp.StatusCode, truncateBytes(respBody, 500))
	}

	var imgResp struct {
		Data []struct {
			B64JSON string `json:"b64_json"`
		} `json:"data"`
	}
	if err := json.Unmarshal(respBody, &imgResp); err != nil {
		return nil, nil, fmt.Errorf("parse response: %w", err)
	}
	if len(imgResp.Data) == 0 || imgResp.Data[0].B64JSON == "" {
		return nil, nil, fmt.Errorf("no image data in response")
	}

	imageBytes, err := base64.StdEncoding.DecodeString(imgResp.Data[0].B64JSON)
	if err != nil {
		return nil, nil, fmt.Errorf("decode base64: %w", err)
	}

	return imageBytes, nil, nil
}

// callGeminiNativeImageGen uses the native Gemini generateContent API with responseModalities.
// Gemini image models require this endpoint — they don't support OpenAI-compat endpoints.
//
// When params["reference_images"] holds ≥1 ImageContent, inline_data parts are
// appended after the text part (text-first per Gemini docs). Refs are capped
// at geminiRefCap (4 — Gemini face-preservation limit). NO automatic model
// upgrade — caller's model is preserved (see plan codex review #5).
func (t *CreateImageTool) callGeminiNativeImageGen(ctx context.Context, apiKey, apiBase, model, prompt string, params map[string]any) ([]byte, *providers.Usage, error) {
	// Derive native Gemini base from OpenAI-compat base (strip /openai suffix)
	nativeBase := strings.TrimRight(apiBase, "/")
	nativeBase = strings.TrimSuffix(nativeBase, "/openai")

	url := fmt.Sprintf("%s/models/%s:generateContent?key=%s", nativeBase, model, apiKey)

	refImages, _ := params["reference_images"].([]providers.ImageContent)
	if len(refImages) > geminiRefCap {
		slog.Warn("create_image: truncating reference images for Gemini",
			"provided", len(refImages), "cap", geminiRefCap)
		refImages = refImages[:geminiRefCap]
	}

	parts := make([]map[string]any, 0, 1+len(refImages))
	parts = append(parts, map[string]any{"text": prompt})
	for _, img := range refImages {
		parts = append(parts, map[string]any{
			"inline_data": map[string]any{
				"mime_type": img.MimeType,
				"data":      img.Data,
			},
		})
	}

	body := map[string]any{
		"contents": []map[string]any{
			{"parts": parts},
		},
		"generationConfig": map[string]any{
			"responseModalities": []string{"TEXT", "IMAGE"},
		},
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{} // timeout governed by chain context
	resp, err := client.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, nil, fmt.Errorf("API error %d: %s", resp.StatusCode, truncateBytes(respBody, 500))
	}

	// Parse native Gemini response: {candidates: [{content: {parts: [{inlineData: {mimeType, data}}]}}]}
	var gemResp struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					InlineData *struct {
						MimeType string `json:"mimeType"`
						Data     string `json:"data"`
					} `json:"inlineData"`
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
		UsageMetadata *struct {
			PromptTokenCount     int `json:"promptTokenCount"`
			CandidatesTokenCount int `json:"candidatesTokenCount"`
			TotalTokenCount      int `json:"totalTokenCount"`
		} `json:"usageMetadata"`
	}
	if err := json.Unmarshal(respBody, &gemResp); err != nil {
		return nil, nil, fmt.Errorf("parse response: %w", err)
	}

	// Extract first image from parts
	for _, cand := range gemResp.Candidates {
		for _, part := range cand.Content.Parts {
			if part.InlineData != nil && strings.HasPrefix(part.InlineData.MimeType, "image/") {
				imageBytes, err := base64.StdEncoding.DecodeString(part.InlineData.Data)
				if err != nil {
					return nil, nil, fmt.Errorf("decode base64: %w", err)
				}
				var usage *providers.Usage
				if gemResp.UsageMetadata != nil {
					usage = &providers.Usage{
						PromptTokens:     gemResp.UsageMetadata.PromptTokenCount,
						CompletionTokens: gemResp.UsageMetadata.CandidatesTokenCount,
						TotalTokens:      gemResp.UsageMetadata.TotalTokenCount,
					}
				}
				return imageBytes, usage, nil
			}
		}
	}

	return nil, nil, fmt.Errorf("no image data in Gemini response")
}

// parseImageResponse extracts base64 image data from the OpenAI-compat chat response.
// Looks for images in choices[0].message.content (multipart) or choices[0].message.images.
func (t *CreateImageTool) parseImageResponse(respBody []byte) ([]byte, *providers.Usage, error) {
	var resp struct {
		Choices []struct {
			Message struct {
				Content any `json:"content"`
				Images  []struct {
					ImageURL struct {
						URL string `json:"url"`
					} `json:"image_url"`
				} `json:"images"`
			} `json:"message"`
		} `json:"choices"`
		Usage *struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
			TotalTokens      int `json:"total_tokens"`
		} `json:"usage"`
	}

	if err := json.Unmarshal(respBody, &resp); err != nil {
		return nil, nil, fmt.Errorf("parse response: %w", err)
	}
	if len(resp.Choices) == 0 {
		return nil, nil, fmt.Errorf("no choices in response")
	}

	msg := resp.Choices[0].Message

	// Try images array first (OpenRouter format)
	for _, img := range msg.Images {
		if imageBytes, err := decodeDataURL(img.ImageURL.URL); err == nil {
			return imageBytes, convertUsage(resp.Usage), nil
		}
	}

	// Try multipart content array (some providers return content as array of parts)
	if parts, ok := msg.Content.([]any); ok {
		for _, part := range parts {
			if m, ok := part.(map[string]any); ok {
				if m["type"] == "image_url" {
					if imgURL, ok := m["image_url"].(map[string]any); ok {
						if url, ok := imgURL["url"].(string); ok {
							if imageBytes, err := decodeDataURL(url); err == nil {
								return imageBytes, convertUsage(resp.Usage), nil
							}
						}
					}
				}
			}
		}
	}

	return nil, nil, fmt.Errorf("no image data found in response")
}

// decodeDataURL decodes a data:image/...;base64,... URL into raw bytes.
func decodeDataURL(dataURL string) ([]byte, error) {
	_, after, ok := strings.Cut(dataURL, ";base64,")
	if !ok {
		return nil, fmt.Errorf("not a base64 data URL")
	}
	b64 := after
	return base64.StdEncoding.DecodeString(b64)
}

func convertUsage(u *struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}) *providers.Usage {
	if u == nil {
		return nil
	}
	return &providers.Usage{
		PromptTokens:     u.PromptTokens,
		CompletionTokens: u.CompletionTokens,
		TotalTokens:      u.TotalTokens,
	}
}

func truncateBytes(b []byte, max int) string {
	if len(b) <= max {
		return string(b)
	}
	return string(b[:max]) + "..."
}

// --- Reference image helpers (Phase 01 of image-gen reference-images plan) ---

const (
	// maxRefImages caps total references at the Gemini face limit. Per-provider
	// call functions cap further (MiniMax 1, DashScope 3, OpenAI 16).
	maxRefImages = 4
	// maxRefImageBytes is the per-image byte cap before base64 expansion.
	// 10MB matches read_image's maxImageFileBytes so an image read_image accepts
	// can also be used as a reference.
	maxRefImageBytes = 10 * 1024 * 1024
	// maxRefImagesAggregateBytes bounds the total reference payload to keep
	// multipart/JSON bodies sane when many refs supplied.
	maxRefImagesAggregateBytes = 20 * 1024 * 1024

	// Per-provider caps applied inside each call function.
	geminiRefCap     = 4 // Gemini face-preservation limit (verified 2026-05).
	openRouterRefCap = 4 // Matches Gemini face limit for OR-routed Gemini models.
	// codexImageRefCap bounds refs sent to the Codex/ChatGPT-OAuth native image
	// tool. Matches OpenAI's image-edit cap pending live validation (#219).
	codexImageRefCap = 16

	// maxResolvedRefImages bounds how many references the tool resolves before
	// provider routing. Set to the largest per-provider cap (OpenAI/Codex 16) so
	// capable providers receive every valid ref; lower-cap providers truncate at
	// call time. Resolving here is subject only to safety caps (MIME, per-image
	// and aggregate bytes) — NOT a global 4-ref cut that hid valid refs (#219).
	maxResolvedRefImages = openAIEditRefCap // 16
)

// allowedRefMIMEs whitelists reference-image MIME types. GIF is omitted because
// most edit endpoints reject animated GIFs (and frame extraction is out of scope).
var allowedRefMIMEs = map[string]bool{
	"image/jpeg": true,
	"image/jpg":  true,
	"image/png":  true,
	"image/webp": true,
}

// toStringSlice coerces []any or []string into []string. Empty strings dropped.
func toStringSlice(v any) []string {
	switch s := v.(type) {
	case []string:
		out := make([]string, 0, len(s))
		for _, item := range s {
			if item != "" {
				out = append(out, item)
			}
		}
		return out
	case []any:
		out := make([]string, 0, len(s))
		for _, item := range s {
			if str, ok := item.(string); ok && str != "" {
				out = append(out, str)
			}
		}
		return out
	}
	return nil
}

// availableImageRefs is the single set of references create_image can resolve:
// activated skill assets, user uploads still in the conversation window, and
// every image on disk in the session .uploads/ folder (so uploads that aged out
// of the window still resolve — trace 019e7256). Same-path aliases are
// intentionally preserved: the LLM may have seen a current-turn <media:image
// id="..."> while an older history ref points at the same persisted upload.
// resolveRefImageIDsDetailed dedupes provider payloads by path after lookup.
func availableImageRefs(ctx context.Context) []providers.MediaRef {
	out := append([]providers.MediaRef(nil), skillAssetImageRefsFromCtx(ctx)...)
	out = append(out, MediaImageRefsFromCtx(ctx)...)
	out = append(out, uploadsImageRefs(ToolWorkspaceFromCtx(ctx))...)
	return out
}

// appendWorkspaceImageRefs resolves path-type ref ids (absolute or containing a
// separator) against the workspace boundary the same way read_image does, so the
// LLM can reference any in-workspace image it organized — e.g. a portrait it
// copied from .uploads/ into portraits/ (trace 019e728d). Bare ids/basenames are
// left to the in-context lookup + .uploads/ enumeration.
func (t *CreateImageTool) appendWorkspaceImageRefs(ctx context.Context, refs []providers.MediaRef, ids []string) []providers.MediaRef {
	workspace := ToolWorkspaceFromCtx(ctx)
	if workspace == "" {
		return refs
	}
	covered := make(map[string]bool, len(refs))
	for _, r := range refs {
		covered[r.ID] = true
		if r.Path != "" {
			covered[r.Path] = true
		}
	}
	allowed := allowedWithTeamWorkspace(ctx, t.allowedPrefixes)
	for _, id := range ids {
		if id == "" || covered[id] {
			continue
		}
		if !filepath.IsAbs(id) && !strings.ContainsRune(id, '/') {
			continue
		}
		if !allowedRefMIMEs[imageMIMEFromPath(id)] {
			continue
		}
		resolved, err := resolvePathWithAllowed(id, workspace, effectiveRestrict(ctx, true), allowed)
		if err != nil {
			continue
		}
		if err := checkDeniedPath(resolved, workspace, t.deniedPrefixes); err != nil {
			continue
		}
		if covered[resolved] {
			continue // same file already in the ref set (e.g. via .uploads/ enumeration)
		}
		if fi, err := os.Stat(resolved); err != nil || fi.IsDir() {
			continue
		}
		refs = append(refs, providers.MediaRef{ID: id, Path: resolved, MimeType: imageMIMEFromPath(resolved), Kind: "image"})
		covered[id] = true
		covered[resolved] = true
	}
	return refs
}

// uploadsImageRefs lists image files in <workspace>/.uploads/ as MediaRefs.
// Symlink-resolved, scoped to .uploads/, hardlinks rejected — same guards as
// resolvePath, since .uploads/ is agent-writable.
func uploadsImageRefs(workspace string) []providers.MediaRef {
	if workspace == "" {
		return nil
	}
	uploadsReal, err := filepath.EvalSymlinks(filepath.Join(workspace, ".uploads"))
	if err != nil {
		return nil
	}
	entries, err := os.ReadDir(uploadsReal)
	if err != nil {
		return nil
	}
	var out []providers.MediaRef
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		mime := imageMIMEFromPath(e.Name())
		if !allowedRefMIMEs[mime] {
			continue
		}
		real, err := filepath.EvalSymlinks(filepath.Join(uploadsReal, e.Name()))
		if err != nil || !isPathInside(real, uploadsReal) || checkHardlink(real) != nil {
			continue
		}
		out = append(out, providers.MediaRef{ID: e.Name(), Path: real, MimeType: mime, Kind: "image"})
	}
	return out
}

func skillAssetImageRefsFromCtx(ctx context.Context) []providers.MediaRef {
	sc := skills.SkillContextFromContext(ctx)
	if sc == nil {
		return nil
	}
	snapshot := sc.Snapshot()
	if len(snapshot) == 0 {
		return nil
	}
	var refs []providers.MediaRef
	for slug, activated := range snapshot {
		for _, path := range activated.AssetPaths {
			mimeType := imageMIMEFromPath(path)
			if !allowedRefMIMEs[mimeType] {
				continue
			}
			refs = append(refs, providers.MediaRef{
				ID:       "skill:" + slug + ":" + filepath.Base(path),
				MimeType: mimeType,
				Kind:     "image",
				Path:     path,
			})
		}
	}
	return refs
}

func imageMIMEFromPath(path string) string {
	mimeType := mime.TypeByExtension(strings.ToLower(filepath.Ext(path)))
	switch mimeType {
	case "image/jpeg", "image/jpg", "image/png", "image/webp":
		return mimeType
	default:
		return ""
	}
}

func promptAppearsToRequireImageRef(prompt string) bool {
	p := strings.ToLower(prompt)
	if strings.Contains(p, "without logo") ||
		strings.Contains(p, "no logo") ||
		strings.Contains(p, "không logo") ||
		strings.Contains(p, "khong logo") ||
		strings.Contains(p, "bỏ logo") ||
		strings.Contains(p, "bo logo") {
		return false
	}
	return strings.Contains(p, "logo") || strings.Contains(p, "reference image") || strings.Contains(p, "ảnh reference")
}

func formatMissingSkillAssetRefsError(skillRefs []providers.MediaRef) string {
	lines := make([]string, 0, len(skillRefs))
	for _, r := range skillRefs {
		lines = append(lines, fmt.Sprintf("  - id=%q path=%q basename=%q mime=%s",
			r.ID, r.Path, filepath.Base(r.Path), r.MimeType))
	}
	return fmt.Sprintf(
		"reference_image_ids is required because the prompt asks for a logo/reference image and activated skill assets are available:\n%s\nRetry create_image with reference_image_ids containing the asset path or id above. If the user explicitly asked for no logo/reference, retry with reference_image_ids: [] and remove logo/reference wording from the prompt.",
		strings.Join(lines, "\n"))
}

func resolveRefImageIDs(_ context.Context, ids []string, refs []providers.MediaRef, maxRefs int) []providers.ImageContent {
	out, _, _, _ := resolveRefImageIDsDetailed(ids, refs, maxRefs)
	return out
}

// resolveRefImageIDsDetailed resolves reference IDs (by id, path, or basename)
// against the available ref set. Buckets: missing = not found / file gone (fix
// the id or ask the user to resend); unusable = present but can't be sent as-is
// (too large or unsupported format → recompress/convert); trimmed = dropped by
// the resolution/byte cap (non-blocking note). Every distinct id lands in
// exactly one bucket (or is a dup) so the caller's fail-fast stays sound.
func resolveRefImageIDsDetailed(ids []string, refs []providers.MediaRef, maxRefs int) (out []providers.ImageContent, missing, unusable, trimmed []string) {
	refByID := make(map[string]providers.MediaRef, len(refs))
	refByPath := make(map[string]providers.MediaRef, len(refs))
	refByBase := make(map[string]providers.MediaRef, len(refs))
	for _, r := range refs {
		refByID[r.ID] = r
		if r.Path != "" {
			refByPath[r.Path] = r
			refByBase[filepath.Base(r.Path)] = r
		}
	}
	seen := make(map[string]bool, len(ids))
	seenPath := make(map[string]bool, len(ids))
	out = make([]providers.ImageContent, 0, len(ids))
	var aggregateBytes int64
	for _, id := range ids {
		if seen[id] {
			slog.Warn("create_image: duplicate reference image ID skipped", "id", id)
			continue
		}
		seen[id] = true
		if len(out) >= maxRefs {
			slog.Warn("create_image: reference images truncated at cap", "cap", maxRefs, "requested", len(ids))
			trimmed = append(trimmed, id)
			continue
		}
		ref, ok := refByID[id]
		if !ok {
			if ref, ok = refByPath[id]; ok {
				slog.Debug("create_image: resolved reference image by path", "key", id, "id", ref.ID)
			} else if ref, ok = refByBase[filepath.Base(id)]; ok {
				slog.Debug("create_image: resolved reference image by basename", "key", id, "id", ref.ID)
			}
		}
		if !ok {
			slog.Warn("create_image: reference image not found by id/path/basename", "key", id)
			missing = append(missing, id)
			continue
		}
		if ref.Path != "" && seenPath[ref.Path] {
			continue // same file via another id form — already categorized/loaded
		}
		if !allowedRefMIMEs[ref.MimeType] {
			slog.Warn("create_image: reference image has unsupported MIME", "id", id, "mime", ref.MimeType)
			unusable = append(unusable, id)
			continue
		}
		if ref.Path == "" {
			slog.Warn("create_image: reference image has no path", "id", id)
			missing = append(missing, id)
			continue
		}
		fi, err := os.Stat(ref.Path)
		if err != nil {
			slog.Warn("create_image: failed to stat reference image", "id", id, "error", err)
			missing = append(missing, id)
			continue
		}
		if fi.Size() > maxRefImageBytes {
			slog.Warn("create_image: reference image exceeds per-image byte cap",
				"id", id, "size", fi.Size(), "cap", maxRefImageBytes)
			unusable = append(unusable, id)
			continue
		}
		if aggregateBytes+fi.Size() > maxRefImagesAggregateBytes {
			// continue (not break) so every remaining id is still categorized —
			// a break left trailing ids in no bucket, defeating the fail-fast.
			slog.Warn("create_image: reference image skipped; aggregate byte cap reached",
				"id", id, "accumulated", aggregateBytes, "cap", maxRefImagesAggregateBytes)
			trimmed = append(trimmed, id)
			continue
		}
		data, err := os.ReadFile(ref.Path)
		if err != nil {
			slog.Warn("create_image: failed to read reference image", "id", id, "error", err)
			unusable = append(unusable, id)
			continue
		}
		seenPath[ref.Path] = true
		aggregateBytes += int64(len(data))
		out = append(out, providers.ImageContent{
			MimeType: ref.MimeType,
			Data:     base64.StdEncoding.EncodeToString(data),
		})
	}
	return out, missing, unusable, trimmed
}

// maxListedRefs bounds how many available refs an error lists (token budget);
// .uploads/ can hold many files. Truncation is flagged with a resend hint.
const maxListedRefs = 30

// formatAvailableRefLines renders available refs as id/path/basename/mime lines,
// capped at maxListedRefs.
func formatAvailableRefLines(available []providers.MediaRef) string {
	if len(available) == 0 {
		return "  (none currently available — ask the user to resend the image)"
	}
	n := len(available)
	shown := available
	if n > maxListedRefs {
		shown = available[:maxListedRefs]
	}
	lines := make([]string, 0, len(shown))
	for _, r := range shown {
		lines = append(lines, fmt.Sprintf("  - id=%q path=%q basename=%q mime=%s",
			r.ID, r.Path, filepath.Base(r.Path), r.MimeType))
	}
	if n > maxListedRefs {
		lines = append(lines, fmt.Sprintf("  ... (+%d more; if the one you need isn't shown, ask the user to resend it)", n-maxListedRefs))
	}
	return strings.Join(lines, "\n")
}

func formatRefResolveError(requested []string, available []providers.MediaRef) string {
	if len(available) == 0 {
		return fmt.Sprintf(
			"reference_image_ids %v could not be resolved — no user-uploaded images or activated skill image assets are visible in this conversation. Ask the user to attach the image, or call use_skill if the reference should come from a skill asset, then retry. Do NOT pass sandbox paths (e.g. /tmp/foo.png) or paths from your code-exec tools — only IDs/paths from <media:image id='...' path='...'> tags or use_skill asset_paths will resolve.",
			requested)
	}
	return fmt.Sprintf(
		"reference_image_ids %v could not be resolved (looked up by id, path, basename, and the session .uploads/ folder). Available user-uploaded refs and activated skill image assets:\n%s\nRetry with one of the id/path/basename values above — do NOT pass sandbox paths (e.g. /tmp/...) or paths from your code-exec tools.",
		requested, formatAvailableRefLines(available))
}

// formatRefPartialResolveError lists the available refs (same source the
// resolver uses — skill assets, recent uploads, and the .uploads/ folder) and
// tells the LLM to fix the id or ask the user to resend.
func formatRefPartialResolveError(missing []string, available []providers.MediaRef) string {
	return fmt.Sprintf(
		"reference_image_ids %v could not be resolved (looked up by id, path, basename, and the session .uploads/ folder). The other references resolved, but generating without these would produce the wrong face/logo/subject — so nothing was generated. Available references you CAN use right now:\n%s\nRetry create_image with corrected id/path/basename values from the list above. If a reference you need is NOT listed, its original file is unavailable — ask the user to resend that image, then retry. Do NOT pass sandbox paths (e.g. /tmp/...) or code-exec output paths.",
		missing, formatAvailableRefLines(available))
}

// formatRefUnusableError covers refs present on disk that can't be sent as-is —
// too large or an unsupported/animated format. Tells the LLM to recompress or
// convert (it has exec) and retry, NOT to ask the user to resend.
func formatRefUnusableError(unusable []string) string {
	return fmt.Sprintf(
		"reference_image_ids %v are present but cannot be used as-is — each is over the %dMB per-image limit or not a supported still image (JPEG/PNG/WebP). Recompress or convert them under the limit (e.g. exec: convert IN -resize '2048x2048>' -quality 85 OUT.jpg) and retry create_image with the smaller/converted file. The files ARE present — do NOT ask the user to resend.",
		unusable, maxRefImageBytes/(1024*1024))
}

// formatRefTrimmedNote covers refs dropped by the resolution/byte cap (valid but
// over budget) — distinct from the not-found error.
func formatRefTrimmedNote(trimmed []string) string {
	if len(trimmed) == 0 {
		return ""
	}
	return fmt.Sprintf(
		"\n\nNote: %d reference image(s) were not sent — the request exceeded the %d-reference limit or the total size budget. Trimmed: %v. Retry with fewer references if these are required.",
		len(trimmed), maxResolvedRefImages, trimmed)
}
