package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/nextlevelbuilder/goclaw/internal/providers"
)

// dashScopeVideoEndpoint derives the video-synthesis submit endpoint from apiBase.
// Strips the OpenAI-compat suffix exactly like dashScopeImageEndpoint does.
func dashScopeVideoEndpoint(apiBase string) string {
	base := strings.TrimRight(apiBase, "/")
	for _, suffix := range []string{
		"/compatible-mode/v1",
		"/compatible-mode",
		"/openai/v1",
		"/openai",
		"/v1",
	} {
		if before, ok := strings.CutSuffix(base, suffix); ok {
			base = before
			break
		}
	}
	return base + "/api/v1/services/aigc/video-generation/video-synthesis"
}

// dashScopeVideoSize maps aspect_ratio to a valid wan2.2-t2v-plus size string.
// Only these values are accepted; a wrong size fails the task with 400.
func dashScopeVideoSize(aspectRatio string) string {
	switch aspectRatio {
	case "9:16":
		return "1080*1920"
	case "1:1":
		return "1440*1440"
	default:
		return "1920*1080"
	}
}

// callDashScopeVideoGen calls the DashScope Wan video-synthesis API (async).
// POST → task_id → poll /api/v1/tasks/<id> until SUCCEEDED/FAILED → download video.
// duration is accepted for API compatibility but wan2.2-t2v-plus outputs a fixed ~5s clip.
func callDashScopeVideoGen(ctx context.Context, apiKey, apiBase, model, prompt string, duration int, aspectRatio string, params map[string]any) ([]byte, *providers.Usage, error) {
	submitURL := dashScopeVideoEndpoint(apiBase)
	size := dashScopeVideoSize(aspectRatio)

	body := map[string]any{
		"model": model,
		"input": map[string]any{
			"prompt": prompt,
		},
		"parameters": map[string]any{
			"size": size,
		},
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal request: %w", err)
	}

	slog.Info("create_video: calling DashScope Wan API",
		"model", model, "size", size, "aspect_ratio", aspectRatio)

	req, err := http.NewRequestWithContext(ctx, "POST", submitURL, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)
	// Mandatory — video-synthesis rejects synchronous calls.
	req.Header.Set("X-DashScope-Async", "enable")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, nil, fmt.Errorf("API error %d: %s", resp.StatusCode, truncateBytes(respBody, 500))
	}

	var initResp struct {
		Output *struct {
			TaskID     string `json:"task_id"`
			TaskStatus string `json:"task_status"`
		} `json:"output"`
		RequestID string `json:"request_id"`
	}
	if err := json.Unmarshal(respBody, &initResp); err != nil {
		return nil, nil, fmt.Errorf("parse init response: %w", err)
	}
	if initResp.Output == nil || initResp.Output.TaskID == "" {
		return nil, nil, fmt.Errorf("no task_id in DashScope video response: %s", truncateBytes(respBody, 300))
	}

	return dashScopeVideoPollTask(ctx, apiKey, apiBase, initResp.Output.TaskID, client)
}

// dashScopeVideoPollTask polls GET /api/v1/tasks/<task_id> until SUCCEEDED or FAILED.
// Typical completion ~30s; allows up to ~5 min (30 polls × 10s).
func dashScopeVideoPollTask(ctx context.Context, apiKey, apiBase, taskID string, client *http.Client) ([]byte, *providers.Usage, error) {
	pollURL := dashScopeTaskEndpoint(apiBase, taskID)
	slog.Info("create_video: DashScope Wan task started, polling", "task_id", taskID)

	const maxPolls = 30
	const pollInterval = 10 * time.Second

	for i := range maxPolls {
		select {
		case <-ctx.Done():
			return nil, nil, ctx.Err()
		case <-time.After(pollInterval):
		}

		pollReq, err := http.NewRequestWithContext(ctx, "GET", pollURL, nil)
		if err != nil {
			return nil, nil, fmt.Errorf("create poll request: %w", err)
		}
		pollReq.Header.Set("Authorization", "Bearer "+apiKey)

		pollResp, err := client.Do(pollReq)
		if err != nil {
			slog.Warn("create_video: DashScope poll error, retrying", "error", err, "attempt", i+1)
			continue
		}

		pollBody, _ := io.ReadAll(pollResp.Body)
		pollResp.Body.Close()

		if pollResp.StatusCode != http.StatusOK {
			return nil, nil, fmt.Errorf("poll API error %d: %s", pollResp.StatusCode, truncateBytes(pollBody, 500))
		}

		var taskResp struct {
			Output *struct {
				TaskStatus string `json:"task_status"`
				VideoURL   string `json:"video_url"`
				Code       string `json:"code"`
				Message    string `json:"message"`
			} `json:"output"`
		}
		if err := json.Unmarshal(pollBody, &taskResp); err != nil {
			return nil, nil, fmt.Errorf("parse poll response: %w", err)
		}
		if taskResp.Output == nil {
			continue
		}

		switch taskResp.Output.TaskStatus {
		case "SUCCEEDED":
			if taskResp.Output.VideoURL == "" {
				return nil, nil, fmt.Errorf("task SUCCEEDED but no video_url")
			}
			return dashScopeDownloadVideo(ctx, taskResp.Output.VideoURL)
		case "FAILED":
			msg := taskResp.Output.Message
			if msg == "" {
				msg = taskResp.Output.Code
			}
			return nil, nil, fmt.Errorf("DashScope video task %s failed: %s", taskID, msg)
		default:
			if (i+1)%3 == 0 {
				slog.Info("create_video: DashScope task pending", "attempt", i+1, "status", taskResp.Output.TaskStatus)
			}
		}
	}

	return nil, nil, fmt.Errorf("DashScope video task %s timed out after %d polls", taskID, maxPolls)
}

// dashScopeDownloadVideo downloads a generated video from a pre-signed URL.
func dashScopeDownloadVideo(ctx context.Context, videoURL string) ([]byte, *providers.Usage, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", videoURL, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("create download request: %w", err)
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("download video: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, nil, fmt.Errorf("download error %d: %s", resp.StatusCode, truncateBytes(body, 300))
	}

	videoBytes, err := limitedReadAll(resp.Body, maxMediaDownloadBytes)
	if err != nil {
		return nil, nil, fmt.Errorf("read video data: %w", err)
	}

	return videoBytes, nil, nil
}
