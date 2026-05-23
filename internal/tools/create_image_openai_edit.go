package tools

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"strings"

	"github.com/nextlevelbuilder/goclaw/internal/providers"
)

// openAIEditRefCap is OpenAI's documented cap for `image[]` parts on the
// /v1/images/edits endpoint (16 — verified 2026-05).
const openAIEditRefCap = 16

// callOpenAIImageEdit posts a multipart/form-data request to /v1/images/edits
// for image-to-image editing with reference images. Model-aware:
//   - gpt-image-1, gpt-image-1.5: sends input_fidelity=high
//   - gpt-image-2*:               OMITS input_fidelity (auto high-fidelity; sending it 400s)
//   - gpt-image-1-mini:           OMITS input_fidelity (not supported)
//
// Caps refs at openAIEditRefCap (16). Returns decoded image bytes.
func callOpenAIImageEdit(ctx context.Context, apiKey, apiBase, model, prompt string, params map[string]any) ([]byte, *providers.Usage, error) {
	refImages, _ := params["reference_images"].([]providers.ImageContent)
	if len(refImages) == 0 {
		return nil, nil, fmt.Errorf("openai edit: no reference images provided")
	}
	if len(refImages) > openAIEditRefCap {
		slog.Warn("create_image: truncating reference images for OpenAI",
			"provided", len(refImages), "cap", openAIEditRefCap)
		refImages = refImages[:openAIEditRefCap]
	}

	buf := &bytes.Buffer{}
	w := multipart.NewWriter(buf)

	if err := w.WriteField("model", model); err != nil {
		return nil, nil, fmt.Errorf("write field model: %w", err)
	}
	if err := w.WriteField("prompt", prompt); err != nil {
		return nil, nil, fmt.Errorf("write field prompt: %w", err)
	}
	size := openAIEditSizeFromAspect(GetParamString(params, "aspect_ratio", "1:1"))
	if err := w.WriteField("size", size); err != nil {
		return nil, nil, fmt.Errorf("write field size: %w", err)
	}
	if err := w.WriteField("n", "1"); err != nil {
		return nil, nil, fmt.Errorf("write field n: %w", err)
	}
	// gpt-image-* /edits rejects response_format with HTTP 400; b64_json is the
	// default response shape for these models. Only legacy dall-e-* accepts it.
	if openAISupportsResponseFormat(model) {
		if err := w.WriteField("response_format", "b64_json"); err != nil {
			return nil, nil, fmt.Errorf("write field response_format: %w", err)
		}
	}
	if openAISupportsInputFidelity(model) {
		if err := w.WriteField("input_fidelity", "high"); err != nil {
			return nil, nil, fmt.Errorf("write field input_fidelity: %w", err)
		}
	}

	for i, img := range refImages {
		data, err := base64.StdEncoding.DecodeString(img.Data)
		if err != nil {
			return nil, nil, fmt.Errorf("decode reference image %d: %w", i, err)
		}
		fw, err := w.CreateFormFile("image[]", fmt.Sprintf("ref%d.%s", i, openAIExtFromMIME(img.MimeType)))
		if err != nil {
			return nil, nil, fmt.Errorf("create form file %d: %w", i, err)
		}
		if _, err := fw.Write(data); err != nil {
			return nil, nil, fmt.Errorf("write form file %d: %w", i, err)
		}
	}

	if err := w.Close(); err != nil {
		return nil, nil, fmt.Errorf("close multipart writer: %w", err)
	}

	url := strings.TrimRight(apiBase, "/") + "/images/edits"
	req, err := http.NewRequestWithContext(ctx, "POST", url, buf)
	if err != nil {
		return nil, nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", w.FormDataContentType())
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
		return nil, nil, fmt.Errorf("openai edit API error %d: %s", resp.StatusCode, truncateBytes(respBody, 500))
	}

	// Response shape mirrors /v1/images/generations: {data:[{b64_json}|{url}]}.
	var imgResp struct {
		Data []struct {
			B64JSON string `json:"b64_json"`
			URL     string `json:"url"`
		} `json:"data"`
	}
	if err := json.Unmarshal(respBody, &imgResp); err != nil {
		return nil, nil, fmt.Errorf("parse response: %w", err)
	}
	if len(imgResp.Data) == 0 {
		return nil, nil, fmt.Errorf("no image data in OpenAI edit response")
	}
	if imgResp.Data[0].B64JSON != "" {
		decoded, err := base64.StdEncoding.DecodeString(imgResp.Data[0].B64JSON)
		if err != nil {
			return nil, nil, fmt.Errorf("decode b64_json: %w", err)
		}
		return decoded, nil, nil
	}
	if imgResp.Data[0].URL != "" {
		return downloadImageURL(ctx, imgResp.Data[0].URL)
	}
	return nil, nil, fmt.Errorf("no image bytes or URL in OpenAI edit response")
}

// openAIEditSizeFromAspect maps aspect_ratio to a concrete size string accepted
// by /v1/images/edits. Mirrors providers.SizeFromAspect for consistency; conservative
// dimensions even though gpt-image-2 supports up to 3840x2160.
func openAIEditSizeFromAspect(ar string) string {
	switch ar {
	case "1:1":
		return "1024x1024"
	case "3:4":
		return "1024x1365"
	case "4:3":
		return "1365x1024"
	case "9:16":
		return "1024x1792"
	case "16:9":
		return "1792x1024"
	default:
		return "1024x1024"
	}
}

// openAISupportsInputFidelity returns true for models that accept the
// input_fidelity multipart field. As of 2026-05:
//   - gpt-image-1 / gpt-image-1.5 (and dated variants): accept
//   - gpt-image-2*:                                       reject (auto high-fidelity)
//   - gpt-image-1-mini:                                   reject (not supported)
//
// Future unknown models default to OMIT (safer — avoids 400s on new variants).
//
// Order matters: the gpt-image-1-mini exact match MUST be checked before the
// gpt-image-1* prefix fall-through, otherwise mini would falsely report true.
func openAISupportsInputFidelity(model string) bool {
	if model == "gpt-image-1-mini" {
		return false
	}
	if strings.HasPrefix(model, "gpt-image-2") {
		return false
	}
	return strings.HasPrefix(model, "gpt-image-1")
}

// openAISupportsResponseFormat returns true only for legacy dall-e-* models
// where /v1/images/edits accepts response_format. gpt-image-* models 400 on
// this field (auto-returns b64_json) — verified 2026-05.
func openAISupportsResponseFormat(model string) bool {
	return strings.HasPrefix(model, "dall-e-")
}

// openAIExtFromMIME maps an image MIME to a file extension used in the
// multipart filename. Only inputs already validated by allowedRefMIMEs reach
// this function, so the default is a safe fallback.
func openAIExtFromMIME(mime string) string {
	switch mime {
	case "image/jpeg", "image/jpg":
		return "jpg"
	case "image/png":
		return "png"
	case "image/webp":
		return "webp"
	default:
		return "png"
	}
}
