package tools

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/nextlevelbuilder/goclaw/internal/providers"
)

type capturedEdit struct {
	path        string
	contentType string
	rawBody     []byte
	fields      map[string][]string
	imageFiles  [][]byte
	auth        string
}

// captureOpenAIEdit starts a server that parses the multipart body and returns
// a 1×1 PNG as b64_json.
func captureOpenAIEdit(t *testing.T) (server *httptest.Server, got *capturedEdit) {
	t.Helper()
	c := &capturedEdit{fields: make(map[string][]string)}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c.path = r.URL.Path
		c.contentType = r.Header.Get("Content-Type")
		c.auth = r.Header.Get("Authorization")
		raw, _ := io.ReadAll(r.Body)
		c.rawBody = raw

		boundary := ""
		const prefix = "boundary="
		if idx := strings.Index(c.contentType, prefix); idx >= 0 {
			boundary = c.contentType[idx+len(prefix):]
		}
		mr := multipart.NewReader(bytes.NewReader(raw), boundary)
		for {
			part, err := mr.NextPart()
			if err != nil {
				break
			}
			b, _ := io.ReadAll(part)
			if part.FileName() != "" {
				c.imageFiles = append(c.imageFiles, b)
			} else {
				c.fields[part.FormName()] = append(c.fields[part.FormName()], string(b))
			}
			_ = part.Close()
		}

		png := []byte{0x89, 0x50, 0x4e, 0x47}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{
				{"b64_json": base64.StdEncoding.EncodeToString(png)},
			},
		})
	}))
	return srv, c
}

func TestOpenAIEdit_MultipartShape_GPTImage15(t *testing.T) {
	srv, got := captureOpenAIEdit(t)
	defer srv.Close()

	pngA := []byte{0x89, 0x50, 0x4e, 0x47, 0x01}
	pngB := []byte{0x89, 0x50, 0x4e, 0x47, 0x02}
	refs := []providers.ImageContent{
		{MimeType: "image/png", Data: base64.StdEncoding.EncodeToString(pngA)},
		{MimeType: "image/png", Data: base64.StdEncoding.EncodeToString(pngB)},
	}
	out, _, err := callOpenAIImageEdit(context.Background(), "k", srv.URL,
		"gpt-image-1.5", "make it cyberpunk",
		map[string]any{"aspect_ratio": "1:1", "reference_images": refs})
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if !bytes.Equal(out, []byte{0x89, 0x50, 0x4e, 0x47}) {
		t.Errorf("output bytes mismatch: %x", out)
	}

	if got.path != "/images/edits" {
		t.Errorf("path = %q, want /images/edits", got.path)
	}
	if !strings.HasPrefix(got.contentType, "multipart/form-data") {
		t.Errorf("Content-Type = %q, want multipart/form-data", got.contentType)
	}
	if got.auth != "Bearer k" {
		t.Errorf("Authorization = %q", got.auth)
	}
	check := map[string]string{
		"model":          "gpt-image-1.5",
		"prompt":         "make it cyberpunk",
		"size":           "1024x1024",
		"n":              "1",
		"input_fidelity": "high",
	}
	for k, want := range check {
		if vs := got.fields[k]; len(vs) != 1 || vs[0] != want {
			t.Errorf("field %s = %v, want %q", k, vs, want)
		}
	}
	// response_format must be omitted — gpt-image-* 400s on it.
	if _, ok := got.fields["response_format"]; ok {
		t.Errorf("response_format must be omitted for gpt-image-1.5, got %v", got.fields["response_format"])
	}
	if len(got.imageFiles) != 2 {
		t.Fatalf("len(image files) = %d, want 2", len(got.imageFiles))
	}
	if !bytes.Equal(got.imageFiles[0], pngA) || !bytes.Equal(got.imageFiles[1], pngB) {
		t.Errorf("image files content mismatch")
	}
}

func TestOpenAIEdit_NoInputFidelity_GPTImage2(t *testing.T) {
	srv, got := captureOpenAIEdit(t)
	defer srv.Close()

	refs := []providers.ImageContent{{MimeType: "image/png", Data: "AAA="}}
	_, _, err := callOpenAIImageEdit(context.Background(), "k", srv.URL,
		"gpt-image-2", "p",
		map[string]any{"aspect_ratio": "1:1", "reference_images": refs})
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if _, ok := got.fields["input_fidelity"]; ok {
		t.Errorf("input_fidelity must be omitted for gpt-image-2, got %v", got.fields["input_fidelity"])
	}
	// model field still set
	if vs := got.fields["model"]; len(vs) != 1 || vs[0] != "gpt-image-2" {
		t.Errorf("model field = %v", vs)
	}
}

func TestOpenAIEdit_NoInputFidelity_Mini(t *testing.T) {
	srv, got := captureOpenAIEdit(t)
	defer srv.Close()

	refs := []providers.ImageContent{{MimeType: "image/png", Data: "AAA="}}
	_, _, err := callOpenAIImageEdit(context.Background(), "k", srv.URL,
		"gpt-image-1-mini", "p",
		map[string]any{"aspect_ratio": "1:1", "reference_images": refs})
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if _, ok := got.fields["input_fidelity"]; ok {
		t.Errorf("input_fidelity must be omitted for gpt-image-1-mini, got %v", got.fields["input_fidelity"])
	}
}

func TestOpenAIEdit_DecodesB64(t *testing.T) {
	srv, _ := captureOpenAIEdit(t)
	defer srv.Close()

	refs := []providers.ImageContent{{MimeType: "image/png", Data: "AAA="}}
	out, _, err := callOpenAIImageEdit(context.Background(), "k", srv.URL,
		"gpt-image-2", "p",
		map[string]any{"aspect_ratio": "1:1", "reference_images": refs})
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	want := []byte{0x89, 0x50, 0x4e, 0x47}
	if !bytes.Equal(out, want) {
		t.Errorf("decoded bytes mismatch: %x", out)
	}
}

func TestOpenAIEdit_EmptyRefs_Errors(t *testing.T) {
	_, _, err := callOpenAIImageEdit(context.Background(), "k", "http://example",
		"gpt-image-2", "p", map[string]any{})
	if err == nil {
		t.Fatal("expected error when no refs supplied")
	}
}

func TestOpenAIEdit_AspectRatioToSize(t *testing.T) {
	tests := map[string]string{
		"1:1":     "1024x1024",
		"3:4":     "1024x1365",
		"4:3":     "1365x1024",
		"9:16":    "1024x1792",
		"16:9":    "1792x1024",
		"unknown": "1024x1024",
	}
	for ar, want := range tests {
		t.Run(ar, func(t *testing.T) {
			if got := openAIEditSizeFromAspect(ar); got != want {
				t.Errorf("%q → %q, want %q", ar, got, want)
			}
		})
	}
}

// TestOpenAIEdit_NoResponseFormat_GPTImage verifies gpt-image-* models do NOT
// receive response_format (OpenAI 400s on it for these models — verified 2026-05).
func TestOpenAIEdit_NoResponseFormat_GPTImage(t *testing.T) {
	for _, model := range []string{"gpt-image-2", "gpt-image-1.5", "gpt-image-1", "gpt-image-1-mini"} {
		t.Run(model, func(t *testing.T) {
			srv, got := captureOpenAIEdit(t)
			defer srv.Close()
			refs := []providers.ImageContent{{MimeType: "image/png", Data: "AAA="}}
			_, _, err := callOpenAIImageEdit(context.Background(), "k", srv.URL,
				model, "p",
				map[string]any{"aspect_ratio": "1:1", "reference_images": refs})
			if err != nil {
				t.Fatalf("call: %v", err)
			}
			if _, ok := got.fields["response_format"]; ok {
				t.Errorf("response_format must be omitted for %s, got %v", model, got.fields["response_format"])
			}
		})
	}
}

// TestOpenAIEdit_ResponseFormatDallE verifies legacy dall-e-* models still
// receive response_format (where the param is required and accepted).
func TestOpenAIEdit_ResponseFormatDallE(t *testing.T) {
	srv, got := captureOpenAIEdit(t)
	defer srv.Close()
	refs := []providers.ImageContent{{MimeType: "image/png", Data: "AAA="}}
	_, _, err := callOpenAIImageEdit(context.Background(), "k", srv.URL,
		"dall-e-2", "p",
		map[string]any{"aspect_ratio": "1:1", "reference_images": refs})
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if vs := got.fields["response_format"]; len(vs) != 1 || vs[0] != "b64_json" {
		t.Errorf("response_format = %v, want [b64_json] for dall-e-2", vs)
	}
}

func TestOpenAISupportsResponseFormat(t *testing.T) {
	tests := map[string]bool{
		"gpt-image-2":   false,
		"gpt-image-1.5": false,
		"gpt-image-1":   false,
		"gpt-image-1-mini": false,
		"dall-e-2":      true,
		"dall-e-3":      true,
		"unknown":       false,
	}
	for m, want := range tests {
		t.Run(m, func(t *testing.T) {
			if got := openAISupportsResponseFormat(m); got != want {
				t.Errorf("%q → %v, want %v", m, got, want)
			}
		})
	}
}

func TestOpenAISupportsInputFidelity(t *testing.T) {
	tests := map[string]bool{
		"gpt-image-1":             true,
		"gpt-image-1.5":           true,
		"gpt-image-1.5-2025-12-16": true,
		"gpt-image-1-mini":        false,
		"gpt-image-2":             false,
		"gpt-image-2-2026-04-21":  false,
		"gpt-image-3":             false, // hypothetical future, omit by default
		"dall-e-3":                false,
	}
	for model, want := range tests {
		t.Run(model, func(t *testing.T) {
			if got := openAISupportsInputFidelity(model); got != want {
				t.Errorf("%q → %v, want %v", model, got, want)
			}
		})
	}
}

func TestOpenAIEdit_URLFallback(t *testing.T) {
	// Server returns url instead of b64_json — exercises the downloadImageURL fallback.
	pngBody := []byte{0x89, 0x50, 0x4e, 0x47, 0x03}
	imgSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(pngBody)
	}))
	defer imgSrv.Close()

	editSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{
				{"url": imgSrv.URL + "/img.png"},
			},
		})
	}))
	defer editSrv.Close()

	refs := []providers.ImageContent{{MimeType: "image/png", Data: "AAA="}}
	out, _, err := callOpenAIImageEdit(context.Background(), "k", editSrv.URL,
		"gpt-image-2", "p",
		map[string]any{"aspect_ratio": "1:1", "reference_images": refs})
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if !bytes.Equal(out, pngBody) {
		t.Errorf("downloaded bytes mismatch: %x", out)
	}
}
