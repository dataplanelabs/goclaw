package tools

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Regression: X-DashScope-Async:enable must be set — wan2.x+ reject synchronous calls.
// Exercises the full async path: POST returns task_id → poll → SUCCEEDED → image bytes.
func TestCallDashScopeImageGen_AsyncHeader(t *testing.T) {
	const syntheticPNG = "SYNTHETICPNG"
	var asyncHeader string

	imgSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte(syntheticPNG))
	}))
	defer imgSrv.Close()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPost {
			asyncHeader = r.Header.Get("X-DashScope-Async")
			_, _ = w.Write([]byte(`{"output":{"task_id":"synthetic-task-001","task_status":"PENDING"}}`))
			return
		}
		// Poll GET: return SUCCEEDED immediately.
		_, _ = w.Write([]byte(fmt.Sprintf(`{"output":{"task_status":"SUCCEEDED","results":[{"url":%q}]}}`, imgSrv.URL+"/img.png")))
	}))
	defer srv.Close()

	// Test the poll path directly (dashScopePollTask sleeps 10 s per iteration).
	imgBytes, _, err := dashScopePollTask(t.Context(), "synthetic-key", srv.URL+"/compatible-mode/v1", "synthetic-task-001", &http.Client{})
	if err != nil {
		t.Fatalf("dashScopePollTask unexpected error: %v", err)
	}
	if string(imgBytes) != syntheticPNG {
		t.Errorf("image bytes = %q, want %q", imgBytes, syntheticPNG)
	}

	// Verify callDashScopeImageGen sends X-DashScope-Async:enable on the initial POST.
	_, _, _ = callDashScopeImageGen(t.Context(), "synthetic-key", srv.URL+"/compatible-mode/v1", "wan2.6-image", "city at dusk", nil)
	if asyncHeader != "enable" {
		t.Errorf("X-DashScope-Async = %q, want \"enable\"", asyncHeader)
	}
}

// Regression: wan2.x multimodal-generation requires content as a list of parts.
// Old shape `{"role":"user","content":"<prompt>"}` returned 400.
func TestCallDashScopeImageGen_RequestShape(t *testing.T) {
	var captured map[string]any
	var asyncHeader string

	imgSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte("SYNTHETICPNG2"))
	}))
	defer imgSrv.Close()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPost {
			asyncHeader = r.Header.Get("X-DashScope-Async")
			body, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(body, &captured)
			_, _ = w.Write([]byte(`{"output":{"task_id":"shape-test-task","task_status":"PENDING"}}`))
			return
		}
		// Poll GET: return SUCCEEDED immediately so no 10 s sleep needed.
		_, _ = w.Write([]byte(fmt.Sprintf(`{"output":{"task_status":"SUCCEEDED","results":[{"url":%q}]}}`, imgSrv.URL+"/img.png")))
	}))
	defer srv.Close()

	_, _, _ = callDashScopeImageGen(t.Context(), "k", srv.URL+"/api/v1/services/aigc/multimodal-generation", "wan2.6-image", "hello prompt", map[string]any{"aspect_ratio": "3:4"})

	if asyncHeader != "enable" {
		t.Errorf("X-DashScope-Async header = %q, want \"enable\"", asyncHeader)
	}

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
