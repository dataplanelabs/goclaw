package protocol

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"time"
)

// FileUploadResult holds the response from uploading a file to Zalo's file service.
type FileUploadResult struct {
	FileID       string      `json:"fileId"`
	FileURL      string      `json:"fileUrl"`      // populated from WS callback
	ClientFileID json.Number `json:"clientFileId"`  // Zalo may return string or number
	ChunkID      int         `json:"chunkId"`
	Finished     int         `json:"finished"`

	// Set by caller.
	TotalSize int    `json:"-"`
	FileName  string `json:"-"`
	Checksum  string `json:"-"` // MD5 hex
}

// UploadFile uploads a non-image file. fileUrl arrives via WebSocket callback
// after all chunks land; the caller must pass the Listener to register for it.
func UploadFile(ctx context.Context, sess *Session, ln *Listener, threadID string, threadType ThreadType, filePath string) (*FileUploadResult, error) {
	chunks, err := openChunkedUpload(filePath)
	if err != nil {
		return nil, err
	}
	endpoint, typeParam := uploadEndpoint(sess, threadType, "asyncfile/upload")
	if endpoint == "" {
		return nil, fmt.Errorf("zalo_personal: no file service URL")
	}

	var final *FileUploadResult
	err = chunks.run(ctx, sess, endpoint, typeParam, threadID, threadType, func(plain []byte) error {
		var r FileUploadResult
		if err := json.Unmarshal(plain, &r); err != nil {
			return err
		}
		if r.FileID != "" && r.FileID != "-1" {
			final = &r
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if final == nil {
		return nil, fmt.Errorf("zalo_personal: upload completed %d chunks but no final result", chunks.totalChunk)
	}
	final.TotalSize = chunks.totalSize
	final.FileName = chunks.fileName
	final.Checksum = md5Hash(chunks.data)

	if ln != nil {
		urlCh := ln.RegisterUploadCallback(final.FileID)
		select {
		case fileURL := <-urlCh:
			final.FileURL = fileURL
		case <-time.After(30 * time.Second):
			ln.CancelUploadCallback(final.FileID)
			return nil, fmt.Errorf("zalo_personal: timeout waiting for file upload callback (fileId=%s)", final.FileID)
		case <-ctx.Done():
			ln.CancelUploadCallback(final.FileID)
			return nil, ctx.Err()
		}
	}
	return final, nil
}

// SendFile sends a previously uploaded file as a message.
func SendFile(ctx context.Context, sess *Session, threadID string, threadType ThreadType, upload *FileUploadResult) (string, error) {
	fileURL := getServiceURL(sess, "file")
	if fileURL == "" {
		return "", fmt.Errorf("zalo_personal: no file service URL")
	}

	ext := strings.TrimPrefix(filepath.Ext(upload.FileName), ".")

	params := map[string]any{
		"fileId":      upload.FileID,
		"checksum":    upload.Checksum,
		"checksumSha": "",
		"extention":   ext, // Zalo typo: "extention" not "extension"
		"totalSize":   upload.TotalSize,
		"fileName":    upload.FileName,
		"clientId":    upload.ClientFileID.String(),
		"fType":       1,
		"fileCount":   0,
		"fdata":       "{}",
		"fileUrl":     upload.FileURL,
		"zsource":     -1,
		"ttl":         0,
	}
	if threadType == ThreadTypeGroup {
		params["grid"] = threadID
	} else {
		params["toid"] = threadID
	}

	encData, err := encryptPayload(sess, params)
	if err != nil {
		return "", fmt.Errorf("zalo_personal: encrypt file send params: %w", err)
	}

	pathPrefix := "/api/message/"
	if threadType == ThreadTypeGroup {
		pathPrefix = "/api/group/"
	}

	sendURL := makeURL(sess, fileURL+pathPrefix+"asyncfile/msg", map[string]any{"nretry": 0}, true)
	form := buildFormBody(map[string]string{"params": encData})

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, sendURL, form)
	if err != nil {
		return "", err
	}
	setDefaultHeaders(req, sess)

	resp, err := sess.Client.Do(req)
	if err != nil {
		return "", fmt.Errorf("zalo_personal: send file: %w", err)
	}
	defer resp.Body.Close()

	var respEnvelope Response[*string]
	if err := readJSON(resp, &respEnvelope); err != nil {
		return "", fmt.Errorf("zalo_personal: parse file send response: %w", err)
	}
	if respEnvelope.ErrorCode != 0 {
		return "", fmt.Errorf("zalo_personal: file send error code %d", respEnvelope.ErrorCode)
	}
	if respEnvelope.Data == nil {
		return "", nil
	}

	plain, err := decryptDataField(sess, *respEnvelope.Data)
	if err != nil {
		return "", fmt.Errorf("zalo_personal: decrypt file send response: %w", err)
	}

	var result struct {
		MsgID json.Number `json:"msgId"`
	}
	if err := json.Unmarshal(plain, &result); err != nil {
		return "", fmt.Errorf("zalo_personal: parse file send result: %w", err)
	}
	return result.MsgID.String(), nil
}

// md5Hash returns the MD5 hex digest. Required by Zalo's file upload API checksum field.
func md5Hash(data []byte) string {
	h := md5.Sum(data)
	return hex.EncodeToString(h[:])
}
