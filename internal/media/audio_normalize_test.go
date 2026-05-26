package media

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestIsAudioExt(t *testing.T) {
	cases := []struct {
		path string
		want bool
	}{
		{"foo.mp3", true},
		{"foo.MP3", true},
		{"foo.m4a", true},
		{"foo.ogg", true},
		{"foo.opus", true},
		{"foo.wav", true},
		{"foo.aac", true},
		{"foo.png", false},
		{"foo.jpg", false},
		{"foo", false},
		{"", false},
	}
	for _, c := range cases {
		if got := IsAudioExt(c.path); got != c.want {
			t.Errorf("IsAudioExt(%q) = %v, want %v", c.path, got, c.want)
		}
	}
}

func TestNormalizeAudio_PassthroughSameFormat(t *testing.T) {
	tmp := t.TempDir()
	src := filepath.Join(tmp, "input.mp3")
	if err := os.WriteFile(src, []byte("fake mp3"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := NormalizeAudio(context.Background(), src, "mp3")
	if err != nil {
		t.Fatalf("NormalizeAudio passthrough: %v", err)
	}
	if got != src {
		t.Errorf("passthrough should return src unchanged; got %q want %q", got, src)
	}
}

func TestNormalizeAudio_StripsExtensionDot(t *testing.T) {
	tmp := t.TempDir()
	src := filepath.Join(tmp, "input.m4a")
	if err := os.WriteFile(src, []byte("fake"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := NormalizeAudio(context.Background(), src, ".m4a")
	if err != nil {
		t.Fatalf("expected passthrough with leading-dot extension: %v", err)
	}
	if got != src {
		t.Errorf("expected passthrough; got %q", got)
	}
}

func TestNormalizeAudio_MissingSource(t *testing.T) {
	_, err := NormalizeAudio(context.Background(), "/nonexistent/path.mp3", "m4a")
	if err == nil {
		t.Fatal("expected error for missing source file")
	}
	if !strings.Contains(err.Error(), "stat src") {
		t.Errorf("error should mention stat: %v", err)
	}
}

func TestNormalizeAudio_NoFFmpeg(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err == nil {
		t.Skip("ffmpeg is installed; skipping no-ffmpeg test")
	}
	tmp := t.TempDir()
	src := filepath.Join(tmp, "input.mp3")
	if err := os.WriteFile(src, []byte("fake"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := NormalizeAudio(context.Background(), src, "m4a")
	if err == nil {
		t.Fatal("expected error when ffmpeg missing")
	}
	if !strings.Contains(err.Error(), "ffmpeg not found") {
		t.Errorf("error should mention ffmpeg not found: %v", err)
	}
}

func TestFFmpegArgsFor_TargetExtensions(t *testing.T) {
	cases := []struct {
		target   string
		wantCodec string
	}{
		{"m4a", "aac"},
		{"aac", "aac"},
		{"mp3", "libmp3lame"},
		{"ogg", "libopus"},
		{"opus", "libopus"},
		{"wav", "pcm_s16le"},
	}
	for _, c := range cases {
		args := ffmpegArgsFor(c.target, "src", "dst")
		joined := strings.Join(args, " ")
		if !strings.Contains(joined, c.wantCodec) {
			t.Errorf("ffmpegArgsFor(%q) missing codec %q; got: %s", c.target, c.wantCodec, joined)
		}
	}
}
