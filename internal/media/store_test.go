package media

import "testing"

func TestExtFromMime(t *testing.T) {
	t.Parallel()
	cases := []struct {
		mime string
		want string
	}{
		{"image/jpeg", ".jpg"},
		{"image/png", ".png"},
		{"image/gif", ".gif"},
		{"image/webp", ".webp"},
		{"image/jxl", ".jxl"},
		{"video/mp4", ".mp4"},
		{"audio/ogg", ".ogg"},
		{"audio/opus", ".ogg"},
		{"audio/mpeg", ".mp3"},
		{"application/pdf", ".pdf"},
		{"application/vnd.openxmlformats-officedocument.wordprocessingml.document", ".docx"},
		{"application/octet-stream", ""},
		{"", ""},
	}
	for _, tc := range cases {
		t.Run(tc.mime, func(t *testing.T) {
			t.Parallel()
			if got := ExtFromMime(tc.mime); got != tc.want {
				t.Errorf("ExtFromMime(%q) = %q, want %q", tc.mime, got, tc.want)
			}
		})
	}
}
