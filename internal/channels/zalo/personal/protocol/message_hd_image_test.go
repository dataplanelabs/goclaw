package protocol

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// TestAttachment_HDImage_EmptyHref characterizes the regression from issue #22 F-14:
// HD photos arrive with href="" and the real URL nested inside params.hdUrl. Pre-fix
// code (Attachment{Title, Href} only) drops the frame because the gate checks Href.
// Post-fix: BestMediaURL() resolves params.hdUrl, IsImage() reports true via type/url.
func TestAttachment_HDImage_EmptyHref(t *testing.T) {
	tests := []struct {
		name        string
		fixture     string
		wantURL     string
		wantIsImage bool
	}{
		{
			name:        "params as object",
			fixture:     "hd_image_empty_href.json",
			wantURL:     "https://f20-zpc.zdn.vn/jpg/hd_abc123.jpg",
			wantIsImage: true,
		},
		{
			name:        "params as stringified JSON (zca-js typed shape)",
			fixture:     "hd_image_params_as_string.json",
			wantURL:     "https://f20-zpc.zdn.vn/jpg/hd_xyz.jpg",
			wantIsImage: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			raw, err := os.ReadFile(filepath.Join("testdata", tt.fixture))
			if err != nil {
				t.Fatalf("read fixture: %v", err)
			}
			var att Attachment
			if err := json.Unmarshal(raw, &att); err != nil {
				t.Fatalf("unmarshal Attachment: %v", err)
			}
			if att.Href != "" {
				t.Fatalf("fixture invariant: Href must be empty (got %q)", att.Href)
			}
			if got := att.BestMediaURL(); got != tt.wantURL {
				t.Errorf("BestMediaURL() = %q, want %q", got, tt.wantURL)
			}
			if got := att.IsImage(); got != tt.wantIsImage {
				t.Errorf("IsImage() = %v, want %v", got, tt.wantIsImage)
			}
		})
	}
}

// TestAttachment_BestMediaURL covers the fallback chain explicitly.
func TestAttachment_BestMediaURL(t *testing.T) {
	tests := []struct {
		name string
		att  *Attachment
		want string
	}{
		{"nil", nil, ""},
		{"empty attachment", &Attachment{}, ""},
		{"href fast path", &Attachment{Href: "https://h"}, "https://h"},
		{"href beats everything", &Attachment{Href: "https://h", Thumb: "https://t", Params: json.RawMessage(`{"hdUrl":"https://hd"}`)}, "https://h"},
		{"params object hdUrl", &Attachment{Params: json.RawMessage(`{"hdUrl":"https://hd"}`)}, "https://hd"},
		{"params object oriUrl when no hdUrl", &Attachment{Params: json.RawMessage(`{"oriUrl":"https://ori"}`)}, "https://ori"},
		{"params object normalUrl when no hdUrl/oriUrl", &Attachment{Params: json.RawMessage(`{"normalUrl":"https://n"}`)}, "https://n"},
		{"params object url fallback", &Attachment{Params: json.RawMessage(`{"url":"https://u"}`)}, "https://u"},
		{"params string shape", &Attachment{Params: json.RawMessage(`"{\"hdUrl\":\"https://hd\"}"`)}, "https://hd"},
		{"thumb only with type=photo", &Attachment{Thumb: "https://t", Type: "photo"}, "https://t"},
		{"thumb only WITHOUT type=photo (sticker/file) drops", &Attachment{Thumb: "https://t"}, ""},
		{"thumb only with type=sticker drops", &Attachment{Thumb: "https://t", Type: "sticker"}, ""},
		{"all empty", &Attachment{Title: "x"}, ""},
		// Hardening: malformed params must not panic. Thumb fallback requires Type=="photo".
		{"params malformed garbage with photo type", &Attachment{Type: "photo", Params: json.RawMessage(`!!not-json!!`), Thumb: "https://t"}, "https://t"},
		{"params malformed truncated", &Attachment{Params: json.RawMessage(`{"hdUrl":"abc"`)}, ""},
		{"params is array (unexpected) with photo type", &Attachment{Type: "photo", Params: json.RawMessage(`["a","b"]`), Thumb: "https://t"}, "https://t"},
		{"params is null literal with photo type", &Attachment{Type: "photo", Params: json.RawMessage(`null`), Thumb: "https://t"}, "https://t"},
		{"params malformed without photo type drops", &Attachment{Params: json.RawMessage(`!!not-json!!`), Thumb: "https://t"}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("BestMediaURL panicked on %q: %v", tt.name, r)
				}
			}()
			if got := tt.att.BestMediaURL(); got != tt.want {
				t.Errorf("BestMediaURL() = %q, want %q", got, tt.want)
			}
		})
	}
}
