package protocol

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

func TestOpenChunkedUpload_ChunkCount(t *testing.T) {
	cases := []struct {
		name      string
		size      int
		wantCount int
	}{
		{"empty becomes one chunk", 0, 1},
		{"exact one chunk", uploadChunkSize, 1},
		{"one byte over → two", uploadChunkSize + 1, 2},
		{"3 chunks", uploadChunkSize*2 + 1, 3},
		{"poster 2K size 1.5MB", 1_500_000, 3},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "f.bin")
			if err := os.WriteFile(path, make([]byte, tc.size), 0o600); err != nil {
				t.Fatal(err)
			}
			c, err := openChunkedUpload(path)
			if err != nil {
				t.Fatalf("openChunkedUpload: %v", err)
			}
			if c.totalChunk != tc.wantCount {
				t.Errorf("totalChunk=%d, want %d (size=%d)", c.totalChunk, tc.wantCount, tc.size)
			}
			if c.totalSize != tc.size {
				t.Errorf("totalSize=%d, want %d", c.totalSize, tc.size)
			}
		})
	}
}

func TestIsRetryableNetErr(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"ctx canceled", context.Canceled, false},
		{"ctx deadline", context.DeadlineExceeded, false},
		{"ENETUNREACH", syscall.ENETUNREACH, true},
		{"EHOSTUNREACH", syscall.EHOSTUNREACH, true},
		{"ECONNREFUSED", syscall.ECONNREFUSED, true},
		{"ECONNRESET", syscall.ECONNRESET, true},
		{"io.EOF", io.EOF, true},
		{"io.ErrUnexpectedEOF", io.ErrUnexpectedEOF, true},
		{"plain error", errors.New("nope"), false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := isRetryableNetErr(c.err); got != c.want {
				t.Errorf("isRetryableNetErr(%v) = %v, want %v", c.err, got, c.want)
			}
		})
	}
}
