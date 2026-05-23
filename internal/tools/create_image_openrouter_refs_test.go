package tools

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/nextlevelbuilder/goclaw/internal/providers"
)

// captureOpenRouterBody starts an httptest server that returns a minimal
// chat-completions response with a generated image (data URL) and captures
// the request body for inspection.
func captureOpenRouterBody(t *testing.T) (server *httptest.Server, gotBody *[]byte, gotAuth *string) {
	t.Helper()
	bodyBuf := new([]byte)
	auth := new(string)
	pngBytes := []byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a}
	pngDataURL := "data:image/png;base64," + base64.StdEncoding.EncodeToString(pngBytes)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*auth = r.Header.Get("Authorization")
		b, _ := io.ReadAll(r.Body)
		*bodyBuf = b
		resp := map[string]any{
			"choices": []map[string]any{
				{
					"message": map[string]any{
						"content": "ok",
						"images": []map[string]any{
							{"image_url": map[string]any{"url": pngDataURL}},
						},
					},
				},
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	return srv, bodyBuf, auth
}

// TestORBody_NoRefs_GoldenSnapshot pins the OpenRouter wire body when no refs
// are supplied: content stays as a plain string (regression contract).
func TestORBody_NoRefs_GoldenSnapshot(t *testing.T) {
	srv, body, _ := captureOpenRouterBody(t)
	defer srv.Close()

	tool := &CreateImageTool{}
	_, _, err := tool.callImageGenAPI(context.Background(),
		"k", srv.URL, "google/gemini-2.5-flash-image", "a sunset", "1:1",
		map[string]any{"reference_images": []providers.ImageContent(nil)})
	if err != nil {
		t.Fatalf("call: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(*body, &parsed); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	msgs := parsed["messages"].([]any)
	content := msgs[0].(map[string]any)["content"]
	if s, ok := content.(string); !ok || s != "a sunset" {
		t.Errorf("content should be plain string %q, got %T %v", "a sunset", content, content)
	}
	if parsed["model"] != "google/gemini-2.5-flash-image" {
		t.Errorf("model = %v", parsed["model"])
	}
	mods := parsed["modalities"].([]any)
	if len(mods) != 2 || mods[0] != "image" || mods[1] != "text" {
		t.Errorf("modalities = %v, want [image text]", mods)
	}
}

// TestORBody_WithRefs_ArrayContent verifies that when refs are present, content
// becomes an array starting with a text part and followed by image_url parts.
func TestORBody_WithRefs_ArrayContent(t *testing.T) {
	srv, body, _ := captureOpenRouterBody(t)
	defer srv.Close()

	refs := []providers.ImageContent{
		{MimeType: "image/jpeg", Data: "AAA="},
		{MimeType: "image/png", Data: "BBB="},
	}
	tool := &CreateImageTool{}
	_, _, err := tool.callImageGenAPI(context.Background(),
		"k", srv.URL, "google/gemini-2.5-flash-image", "a portrait", "1:1",
		map[string]any{"reference_images": refs})
	if err != nil {
		t.Fatalf("call: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(*body, &parsed); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	msgs := parsed["messages"].([]any)
	parts, ok := msgs[0].(map[string]any)["content"].([]any)
	if !ok {
		t.Fatalf("content should be array, got %T", msgs[0].(map[string]any)["content"])
	}
	if len(parts) != 3 {
		t.Fatalf("len(parts) = %d, want 3 (1 text + 2 image_url)", len(parts))
	}
	if parts[0].(map[string]any)["type"] != "text" {
		t.Errorf("parts[0] not text-first: %v", parts[0])
	}
	if parts[0].(map[string]any)["text"] != "a portrait" {
		t.Errorf("parts[0].text = %v", parts[0].(map[string]any)["text"])
	}
	for i, want := range []string{
		"data:image/jpeg;base64,AAA=",
		"data:image/png;base64,BBB=",
	} {
		p := parts[i+1].(map[string]any)
		if p["type"] != "image_url" {
			t.Errorf("parts[%d].type = %v, want image_url", i+1, p["type"])
		}
		url := p["image_url"].(map[string]any)["url"]
		if url != want {
			t.Errorf("parts[%d].image_url.url = %q, want %q", i+1, url, want)
		}
	}
}

// TestORModalitiesPreserved ensures modalities array is always present (refs or no).
func TestORModalitiesPreserved(t *testing.T) {
	srv, body, _ := captureOpenRouterBody(t)
	defer srv.Close()

	refs := []providers.ImageContent{{MimeType: "image/jpeg", Data: "AAA="}}
	tool := &CreateImageTool{}
	_, _, err := tool.callImageGenAPI(context.Background(),
		"k", srv.URL, "google/gemini-2.5-flash-image", "p", "1:1",
		map[string]any{"reference_images": refs})
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	var parsed map[string]any
	_ = json.Unmarshal(*body, &parsed)
	if _, ok := parsed["modalities"]; !ok {
		t.Errorf("modalities missing from body with refs")
	}
}

// TestORBody_NoModelChange asserts that supplying refs does NOT swap the
// configured model.
func TestORBody_NoModelChange(t *testing.T) {
	srv, body, _ := captureOpenRouterBody(t)
	defer srv.Close()

	refs := []providers.ImageContent{{MimeType: "image/jpeg", Data: "AAA="}}
	tool := &CreateImageTool{}
	_, _, err := tool.callImageGenAPI(context.Background(),
		"k", srv.URL, "google/gemini-2.5-flash-image", "p", "1:1",
		map[string]any{"reference_images": refs})
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	var parsed map[string]any
	_ = json.Unmarshal(*body, &parsed)
	if parsed["model"] != "google/gemini-2.5-flash-image" {
		t.Errorf("model auto-changed: %v", parsed["model"])
	}
	if strings.Contains(string(*body), "gemini-3") {
		t.Errorf("body should NOT auto-upgrade to gemini-3.x: %s", *body)
	}
}

// TestORRefsTruncation verifies that more than openRouterRefCap refs are
// trimmed in the wire body.
func TestORRefsTruncation(t *testing.T) {
	srv, body, _ := captureOpenRouterBody(t)
	defer srv.Close()

	refs := make([]providers.ImageContent, 7)
	for i := range refs {
		refs[i] = providers.ImageContent{MimeType: "image/jpeg", Data: "AAA="}
	}
	tool := &CreateImageTool{}
	_, _, err := tool.callImageGenAPI(context.Background(),
		"k", srv.URL, "google/gemini-2.5-flash-image", "p", "1:1",
		map[string]any{"reference_images": refs})
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	var parsed map[string]any
	_ = json.Unmarshal(*body, &parsed)
	parts := parsed["messages"].([]any)[0].(map[string]any)["content"].([]any)
	// 1 text + openRouterRefCap image_url
	if len(parts) != 1+openRouterRefCap {
		t.Fatalf("len(parts) = %d, want %d after cap", len(parts), 1+openRouterRefCap)
	}
}
