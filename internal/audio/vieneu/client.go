package vieneu

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type client struct {
	httpClient *http.Client
	endpoint   string
}

func newClient(endpoint string, timeoutMs int) *client {
	return &client{
		httpClient: &http.Client{Timeout: time.Duration(timeoutMs) * time.Millisecond},
		endpoint:   endpoint,
	}
}

func (c *client) postJSON(ctx context.Context, path string, body any) ([]byte, http.Header, error) {
	buf, err := json.Marshal(body)
	if err != nil {
		return nil, nil, fmt.Errorf("vieneu: marshal request: %w", err)
	}
	url := c.endpoint + path

	var lastErr error
	for attempt := 1; attempt <= 2; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(buf))
		if err != nil {
			return nil, nil, err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "*/*")

		resp, err := c.httpClient.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("%w: %v", ErrDaemonUnreachable, err)
			if attempt == 1 {
				time.Sleep(200 * time.Millisecond)
				continue
			}
			return nil, nil, lastErr
		}
		respBody, rerr := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if rerr != nil {
			return nil, nil, fmt.Errorf("vieneu: read response: %w", rerr)
		}
		if resp.StatusCode >= 500 && attempt == 1 {
			lastErr = fmt.Errorf("vieneu: %d %s", resp.StatusCode, truncate(respBody, 200))
			time.Sleep(200 * time.Millisecond)
			continue
		}
		if resp.StatusCode >= 400 {
			return respBody, resp.Header, fmt.Errorf("vieneu: %d %s", resp.StatusCode, truncate(respBody, 200))
		}
		return respBody, resp.Header, nil
	}
	return nil, nil, lastErr
}

func (c *client) getJSON(ctx context.Context, path string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.endpoint+path, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrDaemonUnreachable, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("vieneu: read response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("vieneu: GET %s → %d %s", path, resp.StatusCode, truncate(body, 200))
	}
	return body, nil
}

func truncate(b []byte, n int) string {
	if len(b) <= n {
		return string(b)
	}
	return string(b[:n]) + "..."
}
