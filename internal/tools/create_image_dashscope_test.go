package tools

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Regression: wan2.x multimodal-generation requires content as a list of parts.
// Old shape `{"role":"user","content":"<prompt>"}` returned 400.
func TestCallDashScopeImageGen_RequestShape(t *testing.T) {
	var captured map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &captured)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"output":{"results":[{"url":"http://invalid.test/x.png"}]}}`))
	}))
	defer srv.Close()

	// The handler returns a URL that downloadImageURL will fail to fetch — that's
	// fine, we only care about the captured request body shape.
	_, _, _ = callDashScopeImageGen(t.Context(), "k", srv.URL+"/api/v1/services/aigc/multimodal-generation", "wan2.6-image", "hello prompt", map[string]any{"aspect_ratio": "3:4"})

	input, ok := captured["input"].(map[string]any)
	if !ok {
		t.Fatalf("input missing or wrong type: %v", captured)
	}
	msgs, ok := input["messages"].([]any)
	if !ok || len(msgs) == 0 {
		t.Fatalf("messages missing/empty: %v", input)
	}
	first := msgs[0].(map[string]any)
	content, ok := first["content"].([]any)
	if !ok {
		t.Fatalf("content must be a list, got %T (%v)", first["content"], first["content"])
	}
	if len(content) != 1 {
		t.Fatalf("content len = %d, want 1", len(content))
	}
	part := content[0].(map[string]any)
	if got, _ := part["text"].(string); !strings.Contains(got, "hello prompt") {
		t.Errorf("text part missing prompt: %v", part)
	}
}
