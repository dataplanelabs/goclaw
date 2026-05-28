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
	// mp3 (non-MP4 container) passes through untouched, so this exercises the
	// leading-dot stripping without invoking ffmpeg. m4a is intentionally not
	// used here because m4a same-ext now triggers a faststart remux.
	src := filepath.Join(tmp, "input.mp3")
	if err := os.WriteFile(src, []byte("fake"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := NormalizeAudio(context.Background(), src, ".mp3")
	if err != nil {
		t.Fatalf("expected passthrough with leading-dot extension: %v", err)
	}
	if got != src {
		t.Errorf("expected passthrough; got %q", got)
	}
}

func TestFFmpegArgsFor_M4AHasFaststart(t *testing.T) {
	joined := strings.Join(ffmpegArgsFor("m4a", "src", "dst"), " ")
	if !strings.Contains(joined, "-movflags +faststart") {
		t.Errorf("m4a args must include +faststart for Zalo mobile playback; got: %s", joined)
	}
	// ADTS .aac has no moov atom — movflags would error, so it must be absent.
	if aac := strings.Join(ffmpegArgsFor("aac", "src", "dst"), " "); strings.Contains(aac, "movflags") {
		t.Errorf("aac (ADTS) args must not include movflags; got: %s", aac)
	}
}

func TestNormalizeAudio_M4APassthroughRemuxesFaststart(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not installed; skipping faststart remux test")
	}
	tmp := t.TempDir()
	// Build a real (tiny) m4a so the -c copy remux succeeds.
	src := filepath.Join(tmp, "input.m4a")
	gen := exec.Command("ffmpeg", "-y", "-loglevel", "error", "-f", "lavfi",
		"-i", "anullsrc=r=16000:cl=mono", "-t", "1", "-c:a", "aac", src)
	if out, err := gen.CombinedOutput(); err != nil {
		t.Skipf("could not synthesize test m4a: %v (%s)", err, out)
	}
	got, err := NormalizeAudio(context.Background(), src, "m4a")
	if err != nil {
		t.Fatalf("m4a passthrough remux: %v", err)
	}
	if got == src {
		t.Fatal("m4a same-ext must remux (faststart), not return src unchanged")
	}
	defer os.Remove(got)
	if _, err := os.Stat(got); err != nil {
		t.Fatalf("remux output missing: %v", err)
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
