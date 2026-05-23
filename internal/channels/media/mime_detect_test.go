package media

import "testing"

func TestDetectMIMEType(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		path string
		want string
	}{
		{"jpeg lowercase", "photo.jpg", "image/jpeg"},
		{"jpeg uppercase", "PHOTO.JPG", "image/jpeg"},
		{"png", "image.png", "image/png"},
		{"gif", "anim.gif", "image/gif"},
		{"webp", "sticker.webp", "image/webp"},
		{"jxl", "hd-photo.jxl", "image/jxl"},
		{"opus voice note", "voice.opus", "audio/ogg"},
		{"flac", "track.flac", "audio/flac"},
		{"unknown extension", "blob.xyz", "application/octet-stream"},
		{"no extension", "blob", "application/octet-stream"},
		{"empty", "", "application/octet-stream"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := DetectMIMEType(tc.path); got != tc.want {
				t.Errorf("DetectMIMEType(%q) = %q, want %q", tc.path, got, tc.want)
			}
		})
	}
}

func TestMediaKindFromMime(t *testing.T) {
	t.Parallel()
	cases := []struct {
		mime string
		want string
	}{
		{"image/jpeg", "image"},
		{"image/png", "image"},
		{"image/webp", "image"},
		{"image/jxl", "image"},
		{"video/mp4", "video"},
		{"audio/ogg", "audio"},
		{"application/pdf", "document"},
		{"application/octet-stream", "document"},
		{"", "document"},
	}
	for _, tc := range cases {
		t.Run(tc.mime, func(t *testing.T) {
			t.Parallel()
			if got := MediaKindFromMime(tc.mime); got != tc.want {
				t.Errorf("MediaKindFromMime(%q) = %q, want %q", tc.mime, got, tc.want)
			}
		})
	}
}
