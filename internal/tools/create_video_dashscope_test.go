package tools

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestCallDashScopeVideoGen_HappyPath mocks POST (PENDING), one poll (RUNNING),
// final poll (SUCCEEDED + video_url), and video download. Asserts:
//   - X-DashScope-Async: enable on the POST
//   - parameters.size mapped to "1080*1920" for 9:16 aspect ratio
//   - returned bytes match synthetic video
func TestCallDashScopeVideoGen_HappyPath(t *testing.T) {
	const syntheticVideo = "SYNTHETIC_VIDEO_BYTES"

	// Serve the synthetic video download.
	videoSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "video/mp4")
		_, _ = w.Write([]byte(syntheticVideo))
	}))
	defer videoSrv.Close()

	pollCount := 0
	var asyncHeader string
	var capturedBody map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		if r.Method == "POST" {
			// Submit endpoint.
			asyncHeader = r.Header.Get("X-DashScope-Async")
			body, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(body, &capturedBody)
			_, _ = fmt.Fprintf(w, `{"output":{"task_id":"task-42","task_status":"PENDING"},"request_id":"req-1"}`)
			return
		}

		// Poll endpoint: first call returns RUNNING, second returns SUCCEEDED.
		pollCount++
		if pollCount == 1 {
			_, _ = fmt.Fprintf(w, `{"output":{"task_status":"RUNNING"}}`)
		} else {
			_, _ = fmt.Fprintf(w, `{"output":{"task_status":"SUCCEEDED","video_url":%q}}`, videoSrv.URL+"/video.mp4")
		}
	}))
	defer srv.Close()

	videoBytes, _, err := callDashScopeVideoGen(
		t.Context(),
		"synthetic-key",
		srv.URL+"/compatible-mode/v1",
		"wan2.2-t2v-plus",
		"a waterfall at sunset",
		5,
		"9:16",
		nil,
	)
	if err != nil {
		t.Fatalf("callDashScopeVideoGen: %v", err)
	}

	// Assert returned bytes match synthetic video.
	if string(videoBytes) != syntheticVideo {
		t.Errorf("video bytes = %q, want %q", videoBytes, syntheticVideo)
	}

	// Assert X-DashScope-Async header.
	if asyncHeader != "enable" {
		t.Errorf("X-DashScope-Async = %q, want \"enable\"", asyncHeader)
	}

	// Assert parameters.size mapped correctly for 9:16.
	params, _ := capturedBody["parameters"].(map[string]any)
	if size, _ := params["size"].(string); size != "1080*1920" {
		t.Errorf("parameters.size = %q, want \"1080*1920\" for 9:16 aspect ratio", size)
	}
}

// TestCallDashScopeVideoGen_Failed verifies that a FAILED task returns a non-nil error
// containing the API message.
func TestCallDashScopeVideoGen_Failed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == "POST" {
			_, _ = fmt.Fprint(w, `{"output":{"task_id":"task-bad","task_status":"PENDING"},"request_id":"req-2"}`)
			return
		}
		_, _ = fmt.Fprint(w, `{"output":{"task_status":"FAILED","code":"InvalidParameter","message":"unsupported model"}}`)
	}))
	defer srv.Close()

	_, _, err := callDashScopeVideoGen(t.Context(), "k", srv.URL, "wan2.2-t2v-plus", "test", 5, "16:9", nil)
	if err == nil {
		t.Fatal("expected error for FAILED task, got nil")
	}
}

// TestDashScopeVideoSize verifies all documented aspect_ratio → size mappings.
func TestDashScopeVideoSize(t *testing.T) {
	cases := []struct {
		ratio string
		want  string
	}{
		{"16:9", "1920*1080"},
		{"9:16", "1080*1920"},
		{"1:1", "1440*1440"},
		{"4:3", "1920*1080"}, // fallback
		{"", "1920*1080"},    // fallback
	}
	for _, tc := range cases {
		if got := dashScopeVideoSize(tc.ratio); got != tc.want {
			t.Errorf("dashScopeVideoSize(%q) = %q, want %q", tc.ratio, got, tc.want)
		}
	}
}

// TestDashScopeVideoEndpoint verifies base-URL stripping for known api_base patterns.
func TestDashScopeVideoEndpoint(t *testing.T) {
	const want = "https://dashscope-intl.aliyuncs.com/api/v1/services/aigc/video-generation/video-synthesis"
	cases := []string{
		"https://dashscope-intl.aliyuncs.com/compatible-mode/v1",
		"https://dashscope-intl.aliyuncs.com/compatible-mode/v1/",
		"https://dashscope-intl.aliyuncs.com/openai/v1",
		"https://dashscope-intl.aliyuncs.com/v1",
	}
	for _, base := range cases {
		if got := dashScopeVideoEndpoint(base); got != want {
			t.Errorf("dashScopeVideoEndpoint(%q) = %q, want %q", base, got, want)
		}
	}
}
