package tools

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"image/jpeg"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/disintegration/imaging"
	_ "github.com/gen2brain/jpegxl" // register JXL decoder so loadImageFromPath can re-encode .jxl as JPEG
	"github.com/nextlevelbuilder/goclaw/internal/providers"
)

// --- Context helpers for media images ---

const ctxMediaImages toolContextKey = "tool_media_images"

// WithMediaImages stores base64-encoded images in context for read_image tool access.
func WithMediaImages(ctx context.Context, images []providers.ImageContent) context.Context {
	return context.WithValue(ctx, ctxMediaImages, images)
}

// MediaImagesFromCtx retrieves stored images from context.
func MediaImagesFromCtx(ctx context.Context) []providers.ImageContent {
	v, _ := ctx.Value(ctxMediaImages).([]providers.ImageContent)
	return v
}

const ctxMediaImageRefs toolContextKey = "tool_media_image_refs"

// WithMediaImageRefs stores image MediaRefs (id-indexed metadata) for image-gen
// tools to look up bytes on demand. Parallel to WithMediaImages (which stores
// already-loaded bytes for vision tools).
func WithMediaImageRefs(ctx context.Context, refs []providers.MediaRef) context.Context {
	return context.WithValue(ctx, ctxMediaImageRefs, refs)
}

// MediaImageRefsFromCtx retrieves stored image refs from context.
func MediaImageRefsFromCtx(ctx context.Context) []providers.MediaRef {
	v, _ := ctx.Value(ctxMediaImageRefs).([]providers.MediaRef)
	return v
}

const ctxCurrentTurnImageRefs toolContextKey = "tool_current_turn_image_refs"

// WithCurrentTurnUserImageRefs stores MediaRefs for images the USER uploaded
// in the CURRENT turn only (excludes historical refs). Used by create_image
// to auto-inject a reference when the LLM forgets to pass reference_image_ids
// despite the user having just uploaded a photo (common with weaker models).
func WithCurrentTurnUserImageRefs(ctx context.Context, refs []providers.MediaRef) context.Context {
	return context.WithValue(ctx, ctxCurrentTurnImageRefs, refs)
}

// CurrentTurnUserImageRefsFromCtx retrieves current-turn user image refs.
func CurrentTurnUserImageRefsFromCtx(ctx context.Context) []providers.MediaRef {
	v, _ := ctx.Value(ctxCurrentTurnImageRefs).([]providers.MediaRef)
	return v
}

// --- ReadImageTool ---

// visionProviderPriority is the order in which providers are tried for vision.
// claude-cli follows anthropic so installations with a native Anthropic API key
// keep using the faster direct API, while claude-cli-only setups still resolve.
var visionProviderPriority = []string{"openrouter", "gemini", "anthropic", "claude-cli", "dashscope"}

// visionModelDefaults maps provider names to preferred vision models.
// Empty string lets the provider pick its own default model.
var visionModelDefaults = map[string]string{
	"openrouter": "google/gemini-2.5-flash-image",
	"gemini":     "gemini-2.5-flash",
	"anthropic":  "",
	"claude-cli": "",
	"dashscope":  "qwen3-vl",
}

// ReadImageTool uses a vision-capable provider to describe images attached to the current message.
type ReadImageTool struct {
	registry        *providers.Registry
	allowedPrefixes []string // extra allowed path prefixes (e.g. skills-store dirs)
	deniedPrefixes  []string // path prefixes the tool must reject (e.g. memory.db)
}

func NewReadImageTool(registry *providers.Registry) *ReadImageTool {
	return &ReadImageTool{registry: registry}
}

// AllowPaths registers extra read-allowed path prefixes (PathAllowable interface).
// Wired at startup with system/builtin skill dirs alongside read_file and send_file.
// Per-session activated skills flow through ctx automatically — no AllowPaths needed.
func (t *ReadImageTool) AllowPaths(prefixes ...string) {
	t.allowedPrefixes = append(t.allowedPrefixes, prefixes...)
}

// DenyPaths registers path prefixes the tool must reject even when they fall
// inside an allowed scope (e.g. memory.db, config.json). Wired alongside the
// other filesystem tools so read_image cannot be used to exfiltrate sensitive
// in-workspace files via vision-model API roundtrip.
func (t *ReadImageTool) DenyPaths(prefixes ...string) {
	t.deniedPrefixes = append(t.deniedPrefixes, prefixes...)
}

func (t *ReadImageTool) Name() string { return "read_image" }

func (t *ReadImageTool) Description() string {
	return "Analyze images using vision AI. Without `path`: analyzes only the images attached in the current user turn (<media:image> tags). With `path`: loads a specific image from disk — use this for older images from earlier turns or generated outputs in workspace."
}

func (t *ReadImageTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"prompt": map[string]any{
				"type":        "string",
				"description": "What you want to know about the image(s). E.g. 'Describe this image in detail' or 'What text is in this image?'",
			},
			"path": map[string]any{
				"type":        "string",
				"description": "Optional file path to an image in the workspace. Use this for older images from earlier turns (e.g. a previously generated image the user references later). If omitted, analyzes only the images attached in the current turn.",
			},
		},
		"required": []string{"prompt"},
	}
}

// maxImageFileBytes is the max size for loading workspace images (10MB).
const maxImageFileBytes = 10 * 1024 * 1024

func (t *ReadImageTool) Execute(ctx context.Context, args map[string]any) (result *Result) {
	timeout := toolTimeoutFromEnv("READ_IMAGE")
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	defer func() {
		if result != nil && result.IsError && errors.Is(ctx.Err(), context.DeadlineExceeded) {
			result = ErrorResult(fmt.Sprintf("read_image timed out after %s (set READ_IMAGE_TIMEOUT_SEC to adjust).", timeout))
		}
	}()

	prompt, _ := args["prompt"].(string)
	if prompt == "" {
		prompt = "Describe this image in detail."
	}

	// If path is provided, load image from workspace file
	images := MediaImagesFromCtx(ctx)
	if imgPath, _ := args["path"].(string); imgPath != "" {
		fileImages, err := t.loadImageFromPath(ctx, imgPath)
		if err != nil {
			return ErrorResult(err.Error())
		}
		images = fileImages
	}

	if len(images) == 0 {
		return ErrorResult("No images available. Either send an image in the chat or provide a file path with the 'path' parameter.")
	}

	chain := ResolveMediaProviderChain(ctx, "read_image", "", "",
		visionProviderPriority, visionModelDefaults, t.registry)

	// Inject prompt and images into each chain entry's params
	for i := range chain {
		if chain[i].Params == nil {
			chain[i].Params = make(map[string]any)
		}
		chain[i].Params["prompt"] = prompt
		chain[i].Params["images"] = images
	}

	if len(chain) == 0 {
		return ErrorResult("No vision provider configured. Ask the user to add a vision-capable provider (e.g. Gemini, Anthropic, OpenRouter) in the system settings.")
	}

	chainResult, err := ExecuteWithChain(ctx, chain, t.registry, t.callProvider)
	if err != nil {
		return ErrorResult(fmt.Sprintf("Image analysis failed — all vision providers returned errors: %v. The user may need to check their provider API keys or configuration.", err))
	}

	out := NewResult(string(chainResult.Data))
	out.Usage = chainResult.Usage
	out.Provider = chainResult.Provider
	out.Model = chainResult.Model
	return out
}

// callProvider dispatches the vision call using provider.Chat().
func (t *ReadImageTool) callProvider(ctx context.Context, cp credentialProvider, providerName, model string, params map[string]any) ([]byte, *providers.Usage, error) {
	prompt := GetParamString(params, "prompt", "Describe this image in detail.")
	images, _ := params["images"].([]providers.ImageContent)

	// Use the provider resolved by ExecuteWithChain when present so wrapped
	// providers (e.g. ChatGPT OAuth pools) are preserved for this media call.
	p, err := providerFromChainParams(ctx, t.registry, providerName, params)
	if err != nil {
		return nil, nil, fmt.Errorf("provider %q not available: %w", providerName, err)
	}
	model = normalizeCodexOnlyModelForProvider("read_image", p, model)

	slog.Info("read_image: calling vision provider", "provider", providerName, "model", model, "images", len(images))

	opts := map[string]any{
		"max_tokens":  1024,
		"temperature": 0.3,
	}
	// claude-cli spawns the Claude CLI binary; loading its built-in MCP
	// toolset costs latency we don't need for a one-shot vision call. Keep
	// this flag scoped to claude-cli so other providers don't receive
	// options they ignore (or worse, choke on in the future).
	if providerName == "claude-cli" {
		opts["disable_tools"] = true
	}

	resp, err := p.Chat(ctx, providers.ChatRequest{
		Messages: []providers.Message{
			{
				Role:    "user",
				Content: prompt,
				Images:  images,
			},
		},
		Model:   model,
		Options: opts,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("vision provider error: %w", err)
	}

	return []byte(resp.Content), resp.Usage, nil
}

// loadImageFromPath reads an image file from the workspace and returns it as ImageContent.
// JPEG XL inputs are transparently decoded and re-encoded as JPEG so the vision
// providers (Anthropic / OpenAI / Gemini), which don't accept image/jxl, see a
// supported format.
func (t *ReadImageTool) loadImageFromPath(ctx context.Context, path string) ([]providers.ImageContent, error) {
	ext := strings.ToLower(filepath.Ext(path))
	mimeTypes := map[string]string{
		".jpg": "image/jpeg", ".jpeg": "image/jpeg",
		".png": "image/png", ".gif": "image/gif",
		".webp": "image/webp", ".bmp": "image/bmp",
		".jxl": "image/jxl",
	}
	mime, ok := mimeTypes[ext]
	if !ok {
		return nil, fmt.Errorf("unsupported image format: %s (supported: jpg, png, gif, webp, bmp, jxl)", ext)
	}

	workspace := ToolWorkspaceFromCtx(ctx)
	resolved, err := resolvePathWithAllowed(path, workspace, effectiveRestrict(ctx, true), allowedWithTeamWorkspace(ctx, t.allowedPrefixes))
	if err != nil {
		return nil, fmt.Errorf("invalid image path: %w", err)
	}
	if err := checkDeniedPath(resolved, workspace, t.deniedPrefixes); err != nil {
		return nil, err
	}

	resolved, fi, err := statImagePathWithSiblingFallback(resolved)
	if err != nil {
		return nil, fmt.Errorf("failed to stat image file: %w", err)
	}
	if fi.Size() > maxImageFileBytes {
		return nil, fmt.Errorf("image file too large (%d bytes, max %d)", fi.Size(), maxImageFileBytes)
	}

	// JXL → JPEG re-encode so providers accept it. image/jxl is not in any
	// major vision provider's supported list (Anthropic/OpenAI/Gemini all reject).
	if mime == "image/jxl" {
		img, err := imaging.Open(resolved, imaging.AutoOrientation(true))
		if err != nil {
			return nil, fmt.Errorf("decode jxl: %w", err)
		}
		var buf bytes.Buffer
		if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 85}); err != nil {
			return nil, fmt.Errorf("encode jpeg: %w", err)
		}
		return []providers.ImageContent{{
			MimeType: "image/jpeg",
			Data:     base64.StdEncoding.EncodeToString(buf.Bytes()),
		}}, nil
	}

	data, err := os.ReadFile(resolved)
	if err != nil {
		return nil, fmt.Errorf("failed to read image file: %w", err)
	}

	return []providers.ImageContent{{
		MimeType: mime,
		Data:     base64.StdEncoding.EncodeToString(data),
	}}, nil
}

func statImagePathWithSiblingFallback(path string) (string, os.FileInfo, error) {
	fi, err := os.Stat(path)
	if err == nil || !os.IsNotExist(err) {
		return path, fi, err
	}

	alt, ok := findNormalizedSiblingImage(path)
	if !ok {
		return path, nil, err
	}
	altFI, altErr := os.Stat(alt)
	if altErr != nil {
		return path, nil, err
	}
	slog.Warn("read_image: corrected missing image path to generated sibling",
		"requested", path,
		"resolved", alt)
	return alt, altFI, nil
}

func findNormalizedSiblingImage(path string) (string, bool) {
	dir := filepath.Dir(path)
	target := normalizedGeneratedImageName(filepath.Base(path))
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", false
	}
	var matches []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.EqualFold(filepath.Ext(name), filepath.Ext(path)) &&
			normalizedGeneratedImageName(name) == target {
			matches = append(matches, filepath.Join(dir, name))
		}
	}
	if len(matches) != 1 {
		return "", false
	}
	return matches[0], true
}

func normalizedGeneratedImageName(name string) string {
	return strings.ReplaceAll(strings.ToLower(name), "-", "_")
}
