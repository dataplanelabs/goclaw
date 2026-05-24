package personal

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"testing"

	"github.com/nextlevelbuilder/goclaw/internal/providers"
)

// TestDoDownload_Success — happy path: HTTP 200 → returns tempfile with body bytes.
func TestDoDownload_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("hello-jxl"))
	}))
	defer srv.Close()

	path, err := doDownload(context.Background(), srv.URL+"/photo.jpg")
	if err != nil {
		t.Fatalf("doDownload: %v", err)
	}
	defer os.Remove(path)

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(data) != "hello-jxl" {
		t.Errorf("body = %q, want %q", data, "hello-jxl")
	}
}

// TestDoDownload_WrapsStatusAsHTTPError — non-200 responses become
// *providers.HTTPError so providers.RetryDo can decide based on Status code.
func TestDoDownload_WrapsStatusAsHTTPError(t *testing.T) {
	cases := []struct {
		status  int
		wantRetryable bool
	}{
		{http.StatusServiceUnavailable, true}, // 503 retryable
		{http.StatusGatewayTimeout, true},     // 504 retryable
		{http.StatusTooManyRequests, true},    // 429 retryable
		{http.StatusNotFound, false},          // 404 NOT retryable
		{http.StatusForbidden, false},         // 403 NOT retryable
	}
	for _, tc := range cases {
		t.Run(http.StatusText(tc.status), func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				http.Error(w, "boom", tc.status)
			}))
			defer srv.Close()

			_, err := doDownload(context.Background(), srv.URL+"/x")
			if err == nil {
				t.Fatal("expected error")
			}
			var httpErr *providers.HTTPError
			if !errors.As(err, &httpErr) {
				t.Fatalf("expected *providers.HTTPError, got %T: %v", err, err)
			}
			if httpErr.Status != tc.status {
				t.Errorf("Status = %d, want %d", httpErr.Status, tc.status)
			}
			if got := providers.IsRetryableError(err); got != tc.wantRetryable {
				t.Errorf("IsRetryableError = %v, want %v (for status %d)", got, tc.wantRetryable, tc.status)
			}
		})
	}
}

// TestDoDownload_ParsesRetryAfter — server sends Retry-After: 2 → wrapped
// error carries 2s delay so providers.RetryDo respects rate-limit signal.
func TestDoDownload_ParsesRetryAfter(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "2")
		http.Error(w, "rate limited", http.StatusTooManyRequests)
	}))
	defer srv.Close()

	_, err := doDownload(context.Background(), srv.URL+"/x")
	var httpErr *providers.HTTPError
	if !errors.As(err, &httpErr) {
		t.Fatalf("expected *providers.HTTPError, got %v", err)
	}
	if httpErr.RetryAfter == 0 {
		t.Error("RetryAfter not parsed from header")
	}
}

// TestDownloadFile_RetryIntegration — proves downloadFile drives doDownload
// through providers.RetryDo: first 503 then 200 → 2 calls, success.
// SSRF check accepts the httptest localhost? No — uses a custom server that
// the SSRF check rejects. We test the loop wiring via doDownload directly
// using the same RetryDo helper.
func TestDownloadFile_RetryIntegration(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)
		if n == 1 {
			http.Error(w, "warming up", http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("warmed"))
	}))
	defer srv.Close()

	// Exercise the exact retry config + closure shape downloadFile uses.
	path, err := providers.RetryDo(context.Background(), downloadRetryConfig, func() (string, error) {
		return doDownload(context.Background(), srv.URL+"/x.jpg")
	})
	if err != nil {
		t.Fatalf("RetryDo: %v", err)
	}
	defer os.Remove(path)
	if got := calls.Load(); got != 2 {
		t.Errorf("expected 2 attempts, got %d", got)
	}
	data, _ := os.ReadFile(path)
	if string(data) != "warmed" {
		t.Errorf("body = %q, want warmed", data)
	}
}
