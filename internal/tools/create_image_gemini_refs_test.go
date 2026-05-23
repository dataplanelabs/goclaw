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

// captureGeminiBody starts an httptest server that echoes a minimal Gemini
// response and captures the posted body + URL for assertions.
func captureGeminiBody(t *testing.T) (server *httptest.Server, gotBody *[]byte, gotURL *string) {
	t.Helper()
	bodyBuf := new([]byte)
	urlBuf := new(string)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*urlBuf = r.URL.String()
		b, _ := io.ReadAll(r.Body)
		*bodyBuf = b
		// Return a 1×1 PNG inlineData so the parser succeeds.
		pngBytes := []byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a}
		resp := map[string]any{
			"candidates": []map[string]any{
				{
					"content": map[string]any{
						"parts": []map[string]any{
							{
								"inlineData": map[string]any{
									"mimeType": "image/png",
									"data":     base64.StdEncoding.EncodeToString(pngBytes),
								},
							},
						},
					},
				},
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	return srv, bodyBuf, urlBuf
}

// TestGeminiBody_NoRefs_GoldenSnapshot pins the exact body shape sent to Gemini
// when no reference_images are present. This is the regression contract: any
// future change that alters the wire format must break this test deliberately.
func TestGeminiBody_NoRefs_GoldenSnapshot(t *testing.T) {
	srv, body, _ := captureGeminiBody(t)
	defer srv.Close()

	tool := &CreateImageTool{}
	_, _, err := tool.callGeminiNativeImageGen(context.Background(),
		"k", srv.URL, "gemini-2.5-flash-image", "a sunset",
		map[string]any{"reference_images": []providers.ImageContent(nil)})
	if err != nil {
		t.Fatalf("call: %v", err)
	}

	want := `{"contents":[{"parts":[{"text":"a sunset"}]}],"generationConfig":{"responseModalities":["TEXT","IMAGE"]}}`
	if got := string(*body); got != want {
		t.Errorf("body mismatch\n got: %s\nwant: %s", got, want)
	}
}

// TestGeminiBody_WithRefs_InlineDataAppended verifies the parts array gains
// inline_data entries after the text part when refs are supplied.
func TestGeminiBody_WithRefs_InlineDataAppended(t *testing.T) {
	srv, body, _ := captureGeminiBody(t)
	defer srv.Close()

	refs := []providers.ImageContent{
		{MimeType: "image/jpeg", Data: "AAA="},
		{MimeType: "image/png", Data: "BBB="},
	}
	tool := &CreateImageTool{}
	_, _, err := tool.callGeminiNativeImageGen(context.Background(),
		"k", srv.URL, "gemini-2.5-flash-image", "a portrait",
		map[string]any{"reference_images": refs})
	if err != nil {
		t.Fatalf("call: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(*body, &parsed); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	contents := parsed["contents"].([]any)
	parts := contents[0].(map[string]any)["parts"].([]any)
	if len(parts) != 3 {
		t.Fatalf("len(parts) = %d, want 3 (1 text + 2 inline_data)", len(parts))
	}
	if parts[0].(map[string]any)["text"] != "a portrait" {
		t.Errorf("parts[0] not text-first: %v", parts[0])
	}
	for i, want := range []struct {
		mime string
		data string
	}{
		{"image/jpeg", "AAA="},
		{"image/png", "BBB="},
	} {
		inline, ok := parts[i+1].(map[string]any)["inline_data"].(map[string]any)
		if !ok {
			t.Fatalf("parts[%d] missing inline_data: %v", i+1, parts[i+1])
		}
		if inline["mime_type"] != want.mime || inline["data"] != want.data {
			t.Errorf("parts[%d] inline_data = %v, want mime=%q data=%q", i+1, inline, want.mime, want.data)
		}
	}
}

// TestGeminiBody_NoModelChange asserts that supplying refs does NOT swap the
// configured model. The URL still resolves to the caller's model (per codex
// review #5: no silent auto-upgrade).
func TestGeminiBody_NoModelChange(t *testing.T) {
	srv, _, gotURL := captureGeminiBody(t)
	defer srv.Close()

	refs := []providers.ImageContent{{MimeType: "image/jpeg", Data: "AAA="}}
	tool := &CreateImageTool{}
	_, _, err := tool.callGeminiNativeImageGen(context.Background(),
		"k", srv.URL, "gemini-2.5-flash-image", "p",
		map[string]any{"reference_images": refs})
	if err != nil {
		t.Fatalf("call: %v", err)
	}

	if !strings.Contains(*gotURL, "/models/gemini-2.5-flash-image:generateContent") {
		t.Errorf("URL = %q, expected unchanged model gemini-2.5-flash-image", *gotURL)
	}
	if strings.Contains(*gotURL, "gemini-3") {
		t.Errorf("URL should NOT auto-upgrade to gemini-3.x: %q", *gotURL)
	}
}

// TestGeminiRefsTruncation verifies that more than geminiRefCap refs are
// trimmed in the wire body.
func TestGeminiRefsTruncation(t *testing.T) {
	srv, body, _ := captureGeminiBody(t)
	defer srv.Close()

	refs := make([]providers.ImageContent, 7)
	for i := range refs {
		refs[i] = providers.ImageContent{MimeType: "image/jpeg", Data: "AAA="}
	}
	tool := &CreateImageTool{}
	_, _, err := tool.callGeminiNativeImageGen(context.Background(),
		"k", srv.URL, "gemini-2.5-flash-image", "p",
		map[string]any{"reference_images": refs})
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	var parsed map[string]any
	_ = json.Unmarshal(*body, &parsed)
	parts := parsed["contents"].([]any)[0].(map[string]any)["parts"].([]any)
	// 1 text + 4 inline_data (Gemini cap)
	if len(parts) != 1+geminiRefCap {
		t.Fatalf("len(parts) = %d, want %d after cap", len(parts), 1+geminiRefCap)
	}
}
