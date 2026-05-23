package tools

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/nextlevelbuilder/goclaw/internal/bus"
	"github.com/nextlevelbuilder/goclaw/internal/providers"
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
		"Optionally pass reference_image_ids (IDs from <media:image id='...'> tags) to use attached photos " +
		"as reference for face / composition / style preservation. Returns a MEDIA: path to the generated image file."
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
				"description": "Optional. Reference images for face/composition/style preservation. Accepts media IDs from <media:image id='...'> tags, file paths from path='...' attrs, or the bare filename (basename of the path). Looks across the current turn AND user-uploaded history. First entry is the primary reference. Max 4 (provider-aware: Gemini 4, OpenRouter 4, OpenAI gpt-image-* 4, MiniMax 1 — character only). DashScope and BytePlus drop refs silently. Animated GIFs and SVG not supported.",
			},
		},
		"required": []string{"prompt"},
	}
}

func (t *CreateImageTool) Execute(ctx context.Context, args map[string]any) *Result {
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
	if idsAny, ok := args["reference_image_ids"]; ok {
		ids := toStringSlice(idsAny)
		if len(ids) > 0 {
			availableRefs := MediaImageRefsFromCtx(ctx)
			refImages = resolveRefImageIDs(ctx, ids, availableRefs, maxRefImages)
			slog.Info("create_image: reference images resolved",
				"requested", len(ids), "loaded", len(refImages))

			// LLMs sometimes pass UUIDs from prior turns. Auto-bind to current
			// turn's images instead of silently producing a random face.
			if len(refImages) == 0 && len(availableRefs) > 0 {
				fallback := make([]string, 0, len(availableRefs))
				for _, r := range availableRefs {
					fallback = append(fallback, r.ID)
				}
				refImages = resolveRefImageIDs(ctx, fallback, availableRefs, maxRefImages)
				slog.Warn("create_image: requested IDs not in current turn — auto-bound to current refs",
					"requested_ids", ids, "available_ids", fallback, "loaded", len(refImages))
			}

			if len(refImages) == 0 {
				return ErrorResult(fmt.Sprintf(
					"reference_image_ids %v could not be resolved (looked up by id, path, basename in %d available refs). Ask the user to resend the image before retrying.", ids, len(availableRefs)))
			}
		}
	}

	chain := ResolveMediaProviderChain(ctx, "create_image", "", "",
		imageGenProviderPriority, imageGenModelDefaults, t.registry)

	// Inject prompt, aspect_ratio, and reference_images into each chain entry's params.
	// `reference_images` is always written as a typed `[]providers.ImageContent`
	// (nil when no refs). Per-provider consumers must use
	//   refs, _ := params["reference_images"].([]providers.ImageContent)
	// and treat len(refs)==0 as the text-only path (do NOT range or index).
	for i := range chain {
		if chain[i].Params == nil {
			chain[i].Params = make(map[string]any)
		}
		chain[i].Params["prompt"] = prompt
		chain[i].Params["aspect_ratio"] = aspectRatio
		chain[i].Params["reference_images"] = refImages
	}

	chainResult, err := ExecuteWithChain(ctx, chain, t.registry, t.callProvider)
	if err != nil {
		if len(refImages) > 0 {
			return ErrorResult(fmt.Sprintf(
				"image generation with reference images failed (chain exhausted). "+
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

	result := &Result{ForLLM: fmt.Sprintf("MEDIA:%s\nUse the EXACT filename when referencing: %s", imagePath, filepath.Base(imagePath))}
	result.Media = []bus.MediaFile{{Path: imagePath, MimeType: "image/png", Filename: filepath.Base(imagePath)}}
	result.MediaPrompts = map[int]string{0: prompt}
	result.Deliverable = fmt.Sprintf("[Generated image: %s]\nPrompt: %s", filepath.Base(imagePath), prompt)

	// Register with DeliveredMedia so a follow-up message(MEDIA:path) call sees
	// the file as already-queued and refuses the duplicate send.
	if dm := DeliveredMediaFromCtx(ctx); dm != nil {
		dm.Mark(imagePath)
	}
	if t.vaultIntc != nil {
		go t.vaultIntc.AfterWriteMedia(context.WithoutCancel(ctx), imagePath, prompt, "image/png")
	}
	result.Provider = chainResult.Provider
	result.Model = chainResult.Model
	if chainResult.Usage != nil {
		result.Usage = chainResult.Usage
	}
	return result
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

	// Per-provider caps applied inside each call function. The tool-layer cap
	// (maxRefImages) is the primary safeguard; these are belt-and-suspenders
	// in case future code reaches the providers with more refs than expected.
	geminiRefCap     = 4  // Gemini face-preservation limit (verified 2026-05).
	openRouterRefCap = 4  // Matches Gemini face limit for OR-routed Gemini models.
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

// resolveRefImageIDs looks up image MediaRefs by ID, validates MIME and size,
// loads file bytes, base64-encodes, and returns []ImageContent.
//
// Lookup tries three keys in order: MediaRef.ID, MediaRef.Path,
// filepath.Base(MediaRef.Path). The last covers cases where the LLM lists the
// uploads dir via exec and passes the filename it sees there.
//
// Refs that don't resolve, fail validation, or fail to load are dropped with a
// warn log. Duplicates are deduped (first occurrence wins) and order is preserved.
//
// Tenant scope: `refs` originates from MediaImageRefsFromCtx(ctx), which is
// populated only by the tenant-scoped persistence layer in loop_input_media.
// Cross-tenant lookup is impossible — IDs outside the current-turn upload set
// fall through to the "not found" branch.
//
// Logging hygiene: only logs id/mime/size — never the base64 Data.
// Allocates a fresh slice — does NOT alias the input refs.
func resolveRefImageIDs(_ context.Context, ids []string, refs []providers.MediaRef, maxRefs int) []providers.ImageContent {
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
	var aggregateBytes int64
	for _, id := range ids {
		if seen[id] {
			slog.Warn("create_image: duplicate reference image ID skipped", "id", id)
			continue
		}
		seen[id] = true
		if len(out) >= maxRefs {
			slog.Warn("create_image: reference images truncated at cap", "cap", maxRefs, "requested", len(ids))
			break
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
			continue
		}
		if !allowedRefMIMEs[ref.MimeType] {
			slog.Warn("create_image: skipping reference image with unsupported MIME", "id", id, "mime", ref.MimeType)
			continue
		}
		if ref.Path == "" {
			slog.Warn("create_image: reference image has no path", "id", id)
			continue
		}
		fi, err := os.Stat(ref.Path)
		if err != nil {
			slog.Warn("create_image: failed to stat reference image", "id", id, "error", err)
			continue
		}
		if fi.Size() > maxRefImageBytes {
			slog.Warn("create_image: reference image exceeds per-image byte cap",
				"id", id, "size", fi.Size(), "cap", maxRefImageBytes)
			continue
		}
		if aggregateBytes+fi.Size() > maxRefImagesAggregateBytes {
			slog.Warn("create_image: aggregate reference image bytes exceeded; stopping",
				"accumulated", aggregateBytes, "cap", maxRefImagesAggregateBytes)
			break
		}
		data, err := os.ReadFile(ref.Path)
		if err != nil {
			slog.Warn("create_image: failed to read reference image", "id", id, "error", err)
			continue
		}
		aggregateBytes += int64(len(data))
		out = append(out, providers.ImageContent{
			MimeType: ref.MimeType,
			Data:     base64.StdEncoding.EncodeToString(data),
		})
	}
	return out
}
