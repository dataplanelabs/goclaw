package tools

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/nextlevelbuilder/goclaw/internal/providers"
)

// captureMinimaxBody starts a server that returns a 1-byte PNG so the parser
// succeeds, and captures the request body.
func captureMinimaxBody(t *testing.T) (server *httptest.Server, gotBody *[]byte) {
	t.Helper()
	bodyBuf := new([]byte)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		*bodyBuf = b
		b64 := base64.StdEncoding.EncodeToString([]byte{0x89, 0x50})
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{"image_base64": []string{b64}},
		})
	}))
	return srv, bodyBuf
}

// TestMinimax_NoRefs_NoSubjectReference verifies the body has no
// subject_reference field when refs are absent (regression contract).
func TestMinimax_NoRefs_NoSubjectReference(t *testing.T) {
	srv, body := captureMinimaxBody(t)
	defer srv.Close()

	_, _, err := callMinimaxImageGen(context.Background(), "k", srv.URL+"/v1",
		"image-01", "p",
		map[string]any{"aspect_ratio": "1:1", "reference_images": []providers.ImageContent(nil)})
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	var parsed map[string]any
	_ = json.Unmarshal(*body, &parsed)
	if _, ok := parsed["subject_reference"]; ok {
		t.Errorf("subject_reference must not appear without refs, got %v", parsed["subject_reference"])
	}
}

// TestMinimax_OneRef_SubjectReferencePopulated asserts that 1 ref produces
// a single character-type entry with the data URL.
func TestMinimax_OneRef_SubjectReferencePopulated(t *testing.T) {
	srv, body := captureMinimaxBody(t)
	defer srv.Close()

	refs := []providers.ImageContent{{MimeType: "image/jpeg", Data: "AAA="}}
	_, _, err := callMinimaxImageGen(context.Background(), "k", srv.URL+"/v1",
		"image-01", "p",
		map[string]any{"aspect_ratio": "1:1", "reference_images": refs})
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	var parsed map[string]any
	_ = json.Unmarshal(*body, &parsed)
	subj, ok := parsed["subject_reference"].([]any)
	if !ok || len(subj) != 1 {
		t.Fatalf("subject_reference = %v, want 1-element array", parsed["subject_reference"])
	}
	entry := subj[0].(map[string]any)
	if entry["type"] != "character" {
		t.Errorf("type = %v, want character", entry["type"])
	}
	if entry["image_file"] != "data:image/jpeg;base64,AAA=" {
		t.Errorf("image_file = %v", entry["image_file"])
	}
}

// TestMinimax_ThreeRefs_TruncatedToOne verifies the cap is enforced and only
// the first ref ends up in the body.
func TestMinimax_ThreeRefs_TruncatedToOne(t *testing.T) {
	srv, body := captureMinimaxBody(t)
	defer srv.Close()

	refs := []providers.ImageContent{
		{MimeType: "image/jpeg", Data: "AAA="},
		{MimeType: "image/png", Data: "BBB="},
		{MimeType: "image/webp", Data: "CCC="},
	}
	_, _, err := callMinimaxImageGen(context.Background(), "k", srv.URL+"/v1",
		"image-01", "p",
		map[string]any{"aspect_ratio": "1:1", "reference_images": refs})
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	var parsed map[string]any
	_ = json.Unmarshal(*body, &parsed)
	subj, _ := parsed["subject_reference"].([]any)
	if len(subj) != 1 {
		t.Fatalf("len(subject_reference) = %d, want 1 (truncated)", len(subj))
	}
	if subj[0].(map[string]any)["image_file"] != "data:image/jpeg;base64,AAA=" {
		t.Errorf("first ref should win after truncation")
	}
	if bytes.Contains(*body, []byte("BBB=")) || bytes.Contains(*body, []byte("CCC=")) {
		t.Errorf("body should not contain truncated refs: %s", *body)
	}
}
