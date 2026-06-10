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

// writeSSEImageStream emits a minimal DashScope multimodal-generation SSE stream with two
// progressive image parts (the LAST is the final result) + a terminal empty/stop event,
// matching the real intl-region wire shape.
func writeSSEImageStream(w http.ResponseWriter, firstURL, lastURL string) {
	w.Header().Set("Content-Type", "text/event-stream;charset=UTF-8")
	imgEvent := func(url string) string {
		return fmt.Sprintf(`{"output":{"choices":[{"message":{"role":"assistant","content":[{"type":"image","image":%q}]},"finish_reason":"null"}],"finished":false}}`, url)
	}
	fmt.Fprintf(w, "id:1\nevent:result\ndata:%s\n\n", imgEvent(firstURL))
	fmt.Fprintf(w, "id:2\nevent:result\ndata:%s\n\n", imgEvent(lastURL))
	fmt.Fprint(w, "id:3\nevent:result\ndata:{\"output\":{\"choices\":[{\"message\":{\"role\":\"assistant\",\"content\":[{\"type\":\"text\",\"text\":\"\"}]},\"finish_reason\":\"stop\"}],\"finished\":true},\"usage\":{\"image_count\":2}}\n\n")
}

// callDashScopeImageGen must POST with X-DashScope-SSE:enable + parameters.stream=true, then
// pick the LAST image URL from the SSE stream and download it.
func TestCallDashScopeImageGen_SSE(t *testing.T) {
	const finalPNG = "FINALPNG"
	imgSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		if strings.Contains(r.URL.Path, "final") {
			_, _ = w.Write([]byte(finalPNG))
		} else {
			_, _ = w.Write([]byte("INTERMEDIATE"))
		}
	}))
	defer imgSrv.Close()

	var sseHeader string
	var captured map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sseHeader = r.Header.Get("X-DashScope-SSE")
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &captured)
		writeSSEImageStream(w, imgSrv.URL+"/intermediate.png", imgSrv.URL+"/final.png")
	}))
	defer srv.Close()

	imgBytes, _, err := callDashScopeImageGen(t.Context(), "synthetic-key", srv.URL+"/compatible-mode/v1", "wan2.6-image", "city at dusk", nil)
	if err != nil {
		t.Fatalf("callDashScopeImageGen: %v", err)
	}
	if string(imgBytes) != finalPNG {
		t.Errorf("image bytes = %q, want the LAST image %q", imgBytes, finalPNG)
	}
	if sseHeader != "enable" {
		t.Errorf("X-DashScope-SSE = %q, want \"enable\"", sseHeader)
	}
	params, _ := captured["parameters"].(map[string]any)
	if stream, _ := params["stream"].(bool); !stream {
		t.Errorf("parameters.stream must be true, got %v", params["stream"])
	}
}

// An error delivered inside the SSE stream (HTTP 200) must surface as an error.
func TestDashScopeStreamImageURL_ErrorEvent(t *testing.T) {
	stream := "id:1\nevent:error\ndata:{\"code\":\"InvalidParameter\",\"message\":\"bad\"}\n\n"
	if _, err := dashScopeStreamImageURL(strings.NewReader(stream)); err == nil {
		t.Fatal("want error from in-stream error event, got nil")
	}
}

// Regression: wan2.x multimodal-generation requires content as a list of parts.
// Old shape `{"role":"user","content":"<prompt>"}` returned 400.
func TestCallDashScopeImageGen_RequestShape(t *testing.T) {
	var captured map[string]any
	imgSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte("PNG"))
	}))
	defer imgSrv.Close()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &captured)
		writeSSEImageStream(w, imgSrv.URL+"/a.png", imgSrv.URL+"/b.png")
	}))
	defer srv.Close()

	_, _, _ = callDashScopeImageGen(t.Context(), "k", srv.URL+"/api/v1/services/aigc/multimodal-generation", "wan2.6-image", "hello prompt", map[string]any{"aspect_ratio": "3:4"})

	input, ok := captured["input"].(map[string]any)
	if !ok {
		t.Fatalf("input missing or wrong type: %v", captured)
	}
	msgs, ok := input["messages"].([]any)
	if !ok || len(msgs) == 0 {
		t.Fatalf("messages missing/empty: %v", input)
	}
	content, ok := msgs[0].(map[string]any)["content"].([]any)
	if !ok || len(content) != 1 {
		t.Fatalf("content must be a 1-element list, got %v", msgs[0])
	}
	if got, _ := content[0].(map[string]any)["text"].(string); !strings.Contains(got, "hello prompt") {
		t.Errorf("text part missing prompt: %v", content[0])
	}
}

// dashScopePollTask is the China-region async fallback (unused by the intl SSE path but kept):
// PENDING-less SUCCEEDED → image bytes.
func TestDashScopePollTask(t *testing.T) {
	const png = "POLLPNG"
	imgSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte(png))
	}))
	defer imgSrv.Close()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(fmt.Sprintf(`{"output":{"task_status":"SUCCEEDED","results":[{"url":%q}]}}`, imgSrv.URL+"/img.png")))
	}))
	defer srv.Close()

	imgBytes, _, err := dashScopePollTask(t.Context(), "k", srv.URL+"/compatible-mode/v1", "task-1", &http.Client{})
	if err != nil {
		t.Fatalf("dashScopePollTask: %v", err)
	}
	if string(imgBytes) != png {
		t.Errorf("image bytes = %q, want %q", imgBytes, png)
	}
}
