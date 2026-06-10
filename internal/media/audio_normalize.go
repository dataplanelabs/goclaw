package media

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// IsAudioExt reports whether the file extension is a recognized audio format.
func IsAudioExt(filePath string) bool {
	switch strings.ToLower(filepath.Ext(filePath)) {
	case ".mp3", ".m4a", ".aac", ".ogg", ".opus", ".wav":
		return true
	}
	return false
}

// NormalizeAudio re-encodes srcPath to targetExt (e.g. "aac") via ffmpeg,
// returning a temp file the caller must remove; same-ext input passes through
// as srcPath unchanged.
func NormalizeAudio(ctx context.Context, srcPath, targetExt string) (string, error) {
	srcExt := strings.ToLower(strings.TrimPrefix(filepath.Ext(srcPath), "."))
	tgtExt := strings.ToLower(strings.TrimPrefix(targetExt, "."))
	if srcExt == tgtExt {
		return srcPath, nil
	}
	if _, err := os.Stat(srcPath); err != nil {
		return "", fmt.Errorf("media: normalize audio: stat src: %w", err)
	}
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		return "", fmt.Errorf("media: normalize audio: ffmpeg not found in PATH (install ffmpeg or skip transcode)")
	}

	dst, err := os.CreateTemp("", "audio-norm-*."+tgtExt)
	if err != nil {
		return "", fmt.Errorf("media: normalize audio: create tmp: %w", err)
	}
	dstPath := dst.Name()
	_ = dst.Close()

	args := ffmpegArgsFor(tgtExt, srcPath, dstPath)
	cmd := exec.CommandContext(ctx, "ffmpeg", args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		_ = os.Remove(dstPath)
		return "", fmt.Errorf("media: normalize audio: ffmpeg %s -> %s: %w (stderr: %s)", srcExt, tgtExt, err, truncateStderr(out))
	}
	return dstPath, nil
}

// ffmpegArgsFor builds the ffmpeg argv for the target extension. AAC/M4A use
// AAC-LC mono 44.1kHz to match Zalo's own voice messages (raw ADTS AAC), which
// is what plays on both Zalo mobile and desktop.
func ffmpegArgsFor(targetExt, src, dst string) []string {
	common := []string{"-y", "-loglevel", "error", "-i", src}
	switch targetExt {
	case "aac", "m4a":
		return append(common, "-c:a", "aac", "-b:a", "64k", "-ar", "44100", "-ac", "1", dst)
	case "mp3":
		return append(common, "-c:a", "libmp3lame", "-b:a", "64k", "-ar", "16000", "-ac", "1", dst)
	case "ogg", "opus":
		return append(common, "-c:a", "libopus", "-b:a", "32k", "-ar", "16000", "-ac", "1", dst)
	case "wav":
		return append(common, "-c:a", "pcm_s16le", "-ar", "16000", "-ac", "1", dst)
	default:
		return append(common, dst)
	}
}

func truncateStderr(out []byte) string {
	s := strings.TrimSpace(string(out))
	if len(s) > 200 {
		return s[:200] + "…"
	}
	return s
}
