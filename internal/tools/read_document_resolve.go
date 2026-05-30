package tools

import (
	"context"
	"encoding/base64"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/nextlevelbuilder/goclaw/internal/providers"
)

// resolveDocumentFile finds the document file path from context MediaRefs.
func (t *ReadDocumentTool) resolveDocumentFile(ctx context.Context, mediaID string) (path, mime string, err error) {
	refs := MediaDocRefsFromCtx(ctx)

	// Find specific media_id, or use the most recent document ref.
	var ref *providers.MediaRef
	if mediaID != "" {
		for i := range refs {
			if refs[i].ID == mediaID {
				ref = &refs[i]
				break
			}
		}
	} else if len(refs) > 0 {
		ref = &refs[len(refs)-1]
	}

	// Not in the current conversation window — fall back to the session .uploads/ on disk.
	// A document sent in an earlier turn persists as .uploads/<media_id>.bin even after it
	// ages out of the conversation refs, so resolving by media_id still works (trace 019e79ef).
	if ref == nil {
		if p, m, ok := resolveUploadedDoc(ctx, mediaID); ok {
			return p, m, nil
		}
		if mediaID != "" {
			return "", "", fmt.Errorf("document with media_id %q not found in conversation or uploads", mediaID)
		}
		return "", "", fmt.Errorf("no documents available in this conversation. The user may not have sent a document.")
	}

	// Prefer persisted workspace path; fall back to legacy .media/ lookup.
	p := ref.Path
	if p == "" {
		var err error
		if t.mediaLoader == nil {
			return "", "", fmt.Errorf("no media storage configured")
		}
		p, err = t.mediaLoader.LoadPath(ref.ID)
		if err != nil {
			return "", "", fmt.Errorf("document file not found: %v", err)
		}
	}

	// Determine MIME type: prefer ref's stored MIME, fall back to extension.
	mime = ref.MimeType
	if mime == "" || mime == "application/octet-stream" {
		mime = mimeFromDocExt(filepath.Ext(p))
	}

	return p, mime, nil
}

// callProvider dispatches document analysis to the appropriate provider API.
// For Gemini: uses native generateContent API (supports PDF natively).
// For others: uses standard Chat API with base64 document.
func (t *ReadDocumentTool) callProvider(ctx context.Context, cp credentialProvider, providerName, model string, params map[string]any) ([]byte, *providers.Usage, error) {
	prompt := GetParamString(params, "prompt", "Analyze this document and describe its contents.")
	data, _ := params["data"].([]byte)
	mime := GetParamString(params, "mime", "application/octet-stream")

	// Gemini: use native API (requires credentials; OpenAI-compat endpoint doesn't support non-image MIME types).
	ptype := GetParamString(params, "_provider_type", providerTypeFromName(providerName))
	if cp != nil && ptype == "gemini" {
		slog.Info("read_document: using gemini native API",
			"provider", providerName, "model", model,
			"doc_size", len(data), "mime", mime)
		resp, err := geminiNativeDocumentCall(ctx, cp.APIKey(), model, prompt, data, mime)
		if err != nil {
			return nil, nil, fmt.Errorf("gemini native call: %w", err)
		}
		return []byte(resp.Content), resp.Usage, nil
	}

	// Other providers: use standard Chat API with document as base64 image_url.
	p, err := t.registry.Get(ctx, providerName)
	if err != nil {
		return nil, nil, fmt.Errorf("provider %q not available: %w", providerName, err)
	}

	slog.Info("read_document: using chat API", "provider", providerName, "model", model, "doc_size", len(data))

	opts := map[string]any{
		"max_tokens":  16384,
		"temperature": 0.2,
	}
	// Scope disable_tools to claude-cli only — it's a CLI-bridge-specific
	// option that skips loading the built-in MCP toolset for one-shot calls.
	// Other providers silently ignore unknown keys today, but leaking
	// provider-specific flags into the shared Options map couples layers.
	if providerName == "claude-cli" {
		opts["disable_tools"] = true
	}

	resp, err := p.Chat(ctx, providers.ChatRequest{
		Messages: []providers.Message{
			{
				Role:    "user",
				Content: prompt,
				Images:  []providers.ImageContent{{MimeType: mime, Data: base64.StdEncoding.EncodeToString(data)}},
			},
		},
		Model:   model,
		Options: opts,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("chat call: %w", err)
	}
	return []byte(resp.Content), resp.Usage, nil
}

// resolveUploadedDoc finds a document in the session .uploads/ folder by media_id when it's
// no longer in the conversation refs. Uploads are stored as <media_id>.bin (the conversation
// tag's id carries no extension), so match the full name OR the stem. Path-bounded +
// hardlink-checked like uploadsImageRefs; MIME sniffed from content since .bin has no ext.
func resolveUploadedDoc(ctx context.Context, mediaID string) (path, mime string, ok bool) {
	if mediaID == "" || strings.ContainsAny(mediaID, `/\`) || strings.Contains(mediaID, "..") {
		return "", "", false
	}
	workspace := ToolWorkspaceFromCtx(ctx)
	if workspace == "" {
		return "", "", false
	}
	uploadsReal, err := filepath.EvalSymlinks(filepath.Join(workspace, ".uploads"))
	if err != nil {
		return "", "", false
	}
	entries, err := os.ReadDir(uploadsReal)
	if err != nil {
		return "", "", false
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		stem := strings.TrimSuffix(name, filepath.Ext(name))
		if name != mediaID && stem != mediaID {
			continue
		}
		real, err := filepath.EvalSymlinks(filepath.Join(uploadsReal, name))
		if err != nil || !isPathInside(real, uploadsReal) || checkHardlink(real) != nil {
			continue
		}
		m := mimeFromDocExt(filepath.Ext(name))
		if m == "application/octet-stream" {
			m = sniffDocMIME(real)
		}
		return real, m, true
	}
	return "", "", false
}

// sniffDocMIME detects a document MIME from its leading bytes (e.g. %PDF -> application/pdf)
// for extensionless .bin uploads; falls back to application/octet-stream.
func sniffDocMIME(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return "application/octet-stream"
	}
	defer f.Close()
	buf := make([]byte, 512)
	n, _ := f.Read(buf)
	return http.DetectContentType(buf[:n])
}

// mimeFromDocExt returns MIME type for document file extensions.
func mimeFromDocExt(ext string) string {
	switch strings.ToLower(ext) {
	case ".pdf":
		return "application/pdf"
	case ".doc":
		return "application/msword"
	case ".docx":
		return "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
	case ".xls", ".xlsx":
		return "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
	case ".ppt", ".pptx":
		return "application/vnd.openxmlformats-officedocument.presentationml.presentation"
	case ".csv":
		return "text/csv"
	default:
		return "application/octet-stream"
	}
}
