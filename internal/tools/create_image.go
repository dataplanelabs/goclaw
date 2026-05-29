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
	registry  *providers.Registry
	vaultIntc *VaultInterceptor
}

func (t *CreateImageTool) SetVaultInterceptor(v *VaultInterceptor) { t.vaultIntc = v }

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
				"description": "Optional. Reference images for face/composition/style preservation and exact logos. Accepts media IDs from <media:image id='...'> tags, file paths from path='...' attrs, skill asset paths returned by use_skill, or the bare filename (basename of the path). Looks across the current turn, user-uploaded history, and activated skill assets. First entry is the primary reference. Max 4 (provider-aware: Gemini 4, OpenRouter 4, OpenAI gpt-image-* 4, MiniMax 1 — character only). DashScope and BytePlus drop refs silently. Animated GIFs and SVG not supported.",
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
	var unresolvedRefIDs []string
	var requestedRefIDs []string
	availableRefs := availableImageRefs(ctx)
	_, explicitRefIDs := args["reference_image_ids"]
	if idsAny, ok := args["reference_image_ids"]; ok {
		ids := toStringSlice(idsAny)
		if len(ids) > 0 {
			refImages, unresolvedRefIDs = resolveRefImageIDsDetailed(ctx, ids, availableRefs, maxResolvedRefImages)
			requestedRefIDs = ids
			slog.Info("create_image: reference images resolved",
				"requested", len(ids), "loaded", len(refImages), "unresolved", len(unresolvedRefIDs))

			if len(refImages) == 0 {
				return ErrorResult(formatRefResolveError(ids, availableRefs))
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
				requestedRefIDs = ids
				slog.Info("create_image: auto-injected user current-turn images as references",
					"ref_count", len(refImages), "ref_ids", ids)
			}
		}
	}

	originalChain := ResolveMediaProviderChain(ctx, "create_image", "", "",
		imageGenProviderPriority, imageGenModelDefaults, t.registry)

	// Refs-aware chain selection: when refs are present, prefer providers that
	// genuinely support image-edit. If the filtered chain is empty (e.g. tenant
	// configured only dashscope+byteplus which silently drop refs), fall through
	// to text-only generation with an explicit notice — better UX than a 53s
	// chain-exhausted error followed by an LLM retry that lands at the same
	// dead-end via auto-inject.
	chainResult, degradedReason, err := runImageGenChain(ctx, t, originalChain, prompt, aspectRatio, refImages)
	if err != nil {
		if len(refImages) > 0 {
			return ErrorResult(fmt.Sprintf(
				"image generation with reference images failed (chain exhausted with refs; text-only fallback also failed). "+
					"Configured providers may not support image-to-image edits. Underlying error: %v", err))
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
	if len(unresolvedRefIDs) > 0 {
		forLLM += formatRefPartialResolveNote(unresolvedRefIDs, MediaImageRefsFromCtx(ctx))
	}
	if degradedReason != "" {
		forLLM += formatRefsDroppedNote(degradedReason, requestedRefIDs,
			refsCapableProviderNamesInRegistry(ctx, t.registry))
	} else if refCap := imageRefCapForProvider(chainResult.Provider); len(refImages) > refCap {
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

// runImageGenChain tries refs-mode first when refs present + refs-capable
// providers exist, then falls back to text-only. Returns chainResult, a
// degradedReason ("" / "no_refs_capable_provider" / "refs_failed"), and err.
func runImageGenChain(
	ctx context.Context,
	t *CreateImageTool,
	chain []MediaProviderEntry,
	prompt, aspectRatio string,
	refImages []providers.ImageContent,
) (*ChainResult, string, error) {
	if len(refImages) > 0 {
		refsChain := filterChainForRefs(ctx, chain, t.registry)
		if len(refsChain) > 0 {
			injectImageGenParams(refsChain, prompt, aspectRatio, refImages)
			if res, err := ExecuteWithChain(ctx, refsChain, t.registry, t.callProvider); err == nil {
				return res, "", nil
			} else {
				slog.Warn("create_image: refs-mode chain failed, falling back to text-only",
					"err", truncateError(err), "refs_chain_len", len(refsChain))
			}
			injectImageGenParams(chain, prompt, aspectRatio, nil)
			res, err := ExecuteWithChain(ctx, chain, t.registry, t.callProvider)
			return res, "refs_failed", err
		}
		slog.Warn("create_image: no refs-capable provider in chain, falling back to text-only",
			"chain_len", len(chain))
		injectImageGenParams(chain, prompt, aspectRatio, nil)
		res, err := ExecuteWithChain(ctx, chain, t.registry, t.callProvider)
		return res, "no_refs_capable_provider", err
	}
	injectImageGenParams(chain, prompt, aspectRatio, nil)
	res, err := ExecuteWithChain(ctx, chain, t.registry, t.callProvider)
	return res, "", err
}

// formatRefsDroppedNote returns an explicit LLM-facing note explaining why
// the user's reference photo wasn't applied. When availableRefsCapable is
// non-empty, the note suggests reordering by listing actual tenant providers
// that DO support refs — concrete operator-actionable hint vs generic advice.
func formatRefsDroppedNote(reason string, refIDs, availableRefsCapable []string) string {
	switch reason {
	case "refs_failed":
		return fmt.Sprintf(
			"\n\n⚠️ NOTE TO ASSISTANT: Reference image(s) %v could not be applied — refs-capable providers in the chain all failed. Generated from prompt only. **Tell the user explicitly** that their attached photo could not be used and the result is based on the description alone.",
			refIDs)
	case "no_refs_capable_provider":
		suggestion := "add Gemini, OpenAI (gpt-image-*), OpenRouter, or MiniMax to the Create Image provider chain"
		if len(availableRefsCapable) > 0 {
			suggestion = fmt.Sprintf("reorder or add one of these refs-capable providers already enabled in your tenant: %v", availableRefsCapable)
		}
		return fmt.Sprintf(
			"\n\n⚠️ NOTE TO ASSISTANT: Reference image(s) %v dropped — no configured provider in the Create Image chain supports image-to-image edits. Generated from prompt only. **Tell the user explicitly** that the system can't apply their photo as a reference with the current chain order, and suggest the operator %s.",
			refIDs, suggestion)
	default:
		return ""
	}
}

// imageRefCapForProvider returns the max reference images the given provider's
// image path accepts. Mirrors the per-provider caps applied at call time so the
// tool can distinguish a valid-but-truncated ref from a genuinely missing one (#219).
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
			refs, _ := params["reference_images"].([]providers.ImageContent)
			if len(refs) > codexImageRefCap {
				slog.Warn("create_image: native image refs truncated at cap",
					"provider", providerName, "provided", len(refs), "cap", codexImageRefCap)
				refs = refs[:codexImageRefCap]
			}
			result, err := np.GenerateImage(ctx, providers.NativeImageRequest{
				Model:           model,
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
		// route through NativeImageProvider.GenerateImage — currently edit-
		// unsupported (see Phase 06 investigation).
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
	// 5MB matches read_image's image-load expectations and provider edit endpoints.
	maxRefImageBytes = 5 * 1024 * 1024
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

func availableImageRefs(ctx context.Context) []providers.MediaRef {
	skillRefs := skillAssetImageRefsFromCtx(ctx)
	mediaRefs := MediaImageRefsFromCtx(ctx)
	if len(skillRefs) == 0 {
		return mediaRefs
	}
	if len(mediaRefs) == 0 {
		return skillRefs
	}
	out := make([]providers.MediaRef, 0, len(skillRefs)+len(mediaRefs))
	// Put skill refs first so user-uploaded refs win basename collisions in
	// resolveRefImageIDsDetailed's lookup maps.
	out = append(out, skillRefs...)
	out = append(out, mediaRefs...)
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

func resolveRefImageIDs(ctx context.Context, ids []string, refs []providers.MediaRef, maxRefs int) []providers.ImageContent {
	out, _ := resolveRefImageIDsDetailed(ctx, ids, refs, maxRefs)
	return out
}

func resolveRefImageIDsDetailed(_ context.Context, ids []string, refs []providers.MediaRef, maxRefs int) ([]providers.ImageContent, []string) {
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
	out := make([]providers.ImageContent, 0, len(ids))
	var unresolved []string
	var aggregateBytes int64
	for _, id := range ids {
		if seen[id] {
			slog.Warn("create_image: duplicate reference image ID skipped", "id", id)
			continue
		}
		seen[id] = true
		if len(out) >= maxRefs {
			slog.Warn("create_image: reference images truncated at cap", "cap", maxRefs, "requested", len(ids))
			unresolved = append(unresolved, id)
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
			unresolved = append(unresolved, id)
			continue
		}
		if !allowedRefMIMEs[ref.MimeType] {
			slog.Warn("create_image: skipping reference image with unsupported MIME", "id", id, "mime", ref.MimeType)
			unresolved = append(unresolved, id)
			continue
		}
		if ref.Path == "" {
			slog.Warn("create_image: reference image has no path", "id", id)
			unresolved = append(unresolved, id)
			continue
		}
		fi, err := os.Stat(ref.Path)
		if err != nil {
			slog.Warn("create_image: failed to stat reference image", "id", id, "error", err)
			unresolved = append(unresolved, id)
			continue
		}
		if fi.Size() > maxRefImageBytes {
			slog.Warn("create_image: reference image exceeds per-image byte cap",
				"id", id, "size", fi.Size(), "cap", maxRefImageBytes)
			unresolved = append(unresolved, id)
			continue
		}
		if aggregateBytes+fi.Size() > maxRefImagesAggregateBytes {
			slog.Warn("create_image: aggregate reference image bytes exceeded; stopping",
				"accumulated", aggregateBytes, "cap", maxRefImagesAggregateBytes)
			unresolved = append(unresolved, id)
			break
		}
		data, err := os.ReadFile(ref.Path)
		if err != nil {
			slog.Warn("create_image: failed to read reference image", "id", id, "error", err)
			unresolved = append(unresolved, id)
			continue
		}
		aggregateBytes += int64(len(data))
		out = append(out, providers.ImageContent{
			MimeType: ref.MimeType,
			Data:     base64.StdEncoding.EncodeToString(data),
		})
	}
	return out, unresolved
}

func formatRefResolveError(requested []string, available []providers.MediaRef) string {
	if len(available) == 0 {
		return fmt.Sprintf(
			"reference_image_ids %v could not be resolved — no user-uploaded images or activated skill image assets are visible in this conversation. Ask the user to attach the image, or call use_skill if the reference should come from a skill asset, then retry. Do NOT pass sandbox paths (e.g. /tmp/foo.png) or paths from your code-exec tools — only IDs/paths from <media:image id='...' path='...'> tags or use_skill asset_paths will resolve.",
			requested)
	}
	lines := make([]string, 0, len(available))
	for _, r := range available {
		lines = append(lines, fmt.Sprintf("  - id=%q path=%q basename=%q mime=%s",
			r.ID, r.Path, filepath.Base(r.Path), r.MimeType))
	}
	return fmt.Sprintf(
		"reference_image_ids %v could not be resolved (looked up by id, path, basename). Available user-uploaded refs and activated skill image assets in this conversation:\n%s\nRetry with one of the id/path/basename values above — do NOT pass sandbox paths (e.g. /tmp/...) or paths from your code-exec tools.",
		requested, strings.Join(lines, "\n"))
}

func formatRefPartialResolveNote(unresolved []string, available []providers.MediaRef) string {
	if len(unresolved) == 0 {
		return ""
	}
	lines := make([]string, 0, len(available))
	for _, r := range available {
		lines = append(lines, fmt.Sprintf("  - id=%q basename=%q", r.ID, filepath.Base(r.Path)))
	}
	return fmt.Sprintf(
		"\n\nNote: %d reference_image_ids did not resolve: %v. Available refs you could have used:\n%s",
		len(unresolved), unresolved, strings.Join(lines, "\n"))
}
