package agent

import (
	"bytes"
	"image"
	"image/jpeg"
	"os"
	"path/filepath"
	"testing"
)

// TestSanitizeImage_LossyJXL feeds a real lossy JXL fixture through SanitizeImage
// and asserts the output is a valid JPEG with reasonable bounds.
// Fixture was produced via: cjxl --lossless_jpeg=0 --effort=7 --distance=1.0 sample.jpg sample.jxl
func TestSanitizeImage_LossyJXL(t *testing.T) {
	t.Parallel()
	out, err := SanitizeImage(filepath.Join("testdata", "sample.jxl"))
	if err != nil {
		t.Fatalf("SanitizeImage(jxl): %v", err)
	}
	defer os.Remove(out)

	info, err := os.Stat(out)
	if err != nil {
		t.Fatalf("stat sanitized output: %v", err)
	}
	if info.Size() == 0 {
		t.Fatalf("sanitized output is empty")
	}
	if info.Size() > imageSanitizeMaxBytes {
		t.Errorf("sanitized output %d bytes exceeds limit %d", info.Size(), imageSanitizeMaxBytes)
	}

	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read sanitized output: %v", err)
	}
	cfg, err := jpeg.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("output is not a valid JPEG: %v", err)
	}
	if cfg.Width <= 0 || cfg.Height <= 0 {
		t.Errorf("invalid JPEG dimensions: %dx%d", cfg.Width, cfg.Height)
	}
	if cfg.Width > imageMaxSide || cfg.Height > imageMaxSide {
		t.Errorf("dimensions %dx%d exceed max side %d", cfg.Width, cfg.Height, imageMaxSide)
	}
}

// TestSanitizeImage_ISOContainerJXL covers the ISOBMFF-container JXL magic-byte
// variant (different first 12 bytes) — proves the gen2brain/jpegxl registration
// catches both codestream and container forms.
func TestSanitizeImage_ISOContainerJXL(t *testing.T) {
	t.Parallel()
	out, err := SanitizeImage(filepath.Join("testdata", "sample-iso.jxl"))
	if err != nil {
		t.Fatalf("SanitizeImage(iso-jxl): %v", err)
	}
	defer os.Remove(out)

	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read sanitized output: %v", err)
	}
	if _, err := jpeg.DecodeConfig(bytes.NewReader(data)); err != nil {
		t.Fatalf("output is not a valid JPEG: %v", err)
	}
}

// TestSanitizeImage_MalformedJXL feeds garbage bytes with a .jxl extension and
// asserts the decoder errors rather than panicking — wazero sandbox should
// contain any malformed-input failure as a Go error.
func TestSanitizeImage_MalformedJXL(t *testing.T) {
	t.Parallel()
	tmp, err := os.CreateTemp("", "goclaw_malformed_*.jxl")
	if err != nil {
		t.Fatalf("create temp: %v", err)
	}
	defer os.Remove(tmp.Name())
	// Write JXL magic byte followed by garbage so decoder dispatches then fails.
	if _, err := tmp.Write([]byte{0xff, 0x0a, 0xde, 0xad, 0xbe, 0xef, 0x00, 0x00, 0x00, 0x00}); err != nil {
		t.Fatalf("write malformed: %v", err)
	}
	tmp.Close()

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("SanitizeImage panicked on malformed JXL: %v", r)
		}
	}()

	out, err := SanitizeImage(tmp.Name())
	if err == nil {
		os.Remove(out)
		t.Fatal("expected error for malformed JXL, got nil")
	}
}

// TestImageDecode_JXLRegistered verifies that gen2brain/jpegxl's init()
// registered with image.RegisterFormat so image.Decode dispatches correctly.
// The format name is "jxl" for both raw and container variants.
func TestImageDecode_JXLRegistered(t *testing.T) {
	t.Parallel()
	for _, fixture := range []string{"sample.jxl", "sample-iso.jxl"} {
		t.Run(fixture, func(t *testing.T) {
			t.Parallel()
			f, err := os.Open(filepath.Join("testdata", fixture))
			if err != nil {
				t.Fatalf("open fixture %s: %v", fixture, err)
			}
			defer f.Close()
			img, format, err := image.Decode(f)
			if err != nil {
				t.Fatalf("image.Decode(%s): %v", fixture, err)
			}
			if format != "jxl" {
				t.Errorf("format = %q, want %q", format, "jxl")
			}
			if img == nil {
				t.Fatalf("decoded image is nil")
			}
		})
	}
}
