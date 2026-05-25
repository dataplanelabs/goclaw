package protocol

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

// Zalo's per-chunk cap; over this the file_service returns inner error code 201
// "Dung lượng chunk upload không được vượt quá 512K".
const uploadChunkSize = 512 * 1024

type chunkedUpload struct {
	data       []byte
	fileName   string
	totalSize  int
	totalChunk int
	clientID   int64
}

func openChunkedUpload(filePath string) (*chunkedUpload, error) {
	if err := checkFileSize(filePath); err != nil {
		return nil, err
	}
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("zalo_personal: read file: %w", err)
	}
	total := len(data)
	n := (total + uploadChunkSize - 1) / uploadChunkSize
	if n == 0 {
		n = 1
	}
	return &chunkedUpload{
		data:       data,
		fileName:   filepath.Base(filePath),
		totalSize:  total,
		totalChunk: n,
		clientID:   time.Now().UnixMilli(),
	}, nil
}

func uploadEndpoint(sess *Session, threadType ThreadType, action string) (endpoint, typeParam string) {
	base := getServiceURL(sess, "file")
	if base == "" {
		return "", ""
	}
	if threadType == ThreadTypeGroup {
		return base + "/api/group/" + action, "11"
	}
	return base + "/api/message/" + action, "2"
}

// run uploads every chunk sequentially. parseResult is invoked with the
// decrypted JSON body of each chunk that returned a non-empty envelope;
// the caller decides which response carries the final metadata.
func (c *chunkedUpload) run(ctx context.Context, sess *Session, endpoint, typeParam, threadID string, threadType ThreadType, parseResult func(plain []byte) error) error {
	for i := range c.totalChunk {
		start := i * uploadChunkSize
		end := min(start+uploadChunkSize, c.totalSize)

		params := map[string]any{
			"totalChunk": c.totalChunk,
			"fileName":   c.fileName,
			"clientId":   c.clientID,
			"totalSize":  c.totalSize,
			"imei":       sess.IMEI,
			"isE2EE":     0,
			"jxl":        0,
			"chunkId":    i + 1,
		}
		if threadType == ThreadTypeGroup {
			params["grid"] = threadID
		} else {
			params["toid"] = threadID
		}

		plain, err := uploadOneChunk(ctx, sess, endpoint, typeParam, params, c.fileName, c.data[start:end])
		if err != nil {
			return fmt.Errorf("zalo_personal: chunk %d/%d: %w", i+1, c.totalChunk, err)
		}
		if plain == nil {
			continue
		}
		if err := parseResult(plain); err != nil {
			return fmt.Errorf("zalo_personal: parse chunk %d/%d: %w", i+1, c.totalChunk, err)
		}
	}
	return nil
}

// postChunkWithRetry POSTs the chunk with up to 3 attempts on transient
// network errors (dial / reset / timeout). Body is rebuilt each attempt.
func postChunkWithRetry(ctx context.Context, sess *Session, uploadURL, fileName string, chunk []byte) (*http.Response, error) {
	const maxAttempts = 3
	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		body, contentType, err := buildMultipartBody("chunkContent", fileName, chunk)
		if err != nil {
			return nil, fmt.Errorf("build multipart: %w", err)
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, uploadURL, body)
		if err != nil {
			return nil, err
		}
		setDefaultHeaders(req, sess)
		req.Header.Set("Content-Type", contentType)

		resp, err := sess.Client.Do(req)
		if err == nil {
			return resp, nil
		}
		lastErr = err
		if !isRetryableNetErr(err) || attempt == maxAttempts {
			return nil, err
		}
		backoff := time.Duration(200*attempt*attempt) * time.Millisecond
		slog.Warn("zalo_personal.upload: transient error, retrying", "attempt", attempt, "backoff", backoff, "err", err)
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(backoff):
		}
	}
	return nil, lastErr
}

func isRetryableNetErr(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	if errors.Is(err, syscall.ENETUNREACH) || errors.Is(err, syscall.EHOSTUNREACH) ||
		errors.Is(err, syscall.ECONNREFUSED) || errors.Is(err, syscall.ECONNRESET) ||
		errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return true
	}
	var ne net.Error
	if errors.As(err, &ne) && ne.Timeout() {
		return true
	}
	return false
}

// uploadOneChunk POSTs one chunk and returns the decrypted JSON of the data
// field, or nil if the envelope had no data (intermediate chunks may).
func uploadOneChunk(ctx context.Context, sess *Session, endpoint, typeParam string, params map[string]any, fileName string, chunk []byte) ([]byte, error) {
	encParams, err := encryptPayload(sess, params)
	if err != nil {
		return nil, fmt.Errorf("encrypt params: %w", err)
	}
	uploadURL := makeURL(sess, endpoint, map[string]any{
		"type":   typeParam,
		"params": encParams,
	}, true)

	resp, err := postChunkWithRetry(ctx, sess, uploadURL, fileName, chunk)
	if err != nil {
		return nil, fmt.Errorf("http: %w", err)
	}
	defer resp.Body.Close()

	var envelope Response[*string]
	if err := readJSON(resp, &envelope); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}
	if envelope.ErrorCode != 0 {
		return nil, fmt.Errorf("upload error code %d", envelope.ErrorCode)
	}
	if envelope.Data == nil {
		return nil, nil
	}
	plain, err := decryptDataField(sess, *envelope.Data)
	if err != nil {
		return nil, fmt.Errorf("decrypt: %w", err)
	}
	return plain, nil
}
