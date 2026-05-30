package browser

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
)

// Bound the AX-tree fetch so a huge page fails fast instead of hanging the action.
const snapshotAXTimeout = 45 * time.Second

// Errors meaning the page/connection died under us — safe to retry once after re-resolve.
var staleErrSubstrings = []string{
	"context canceled", "cannot find context", "Inspector.detached",
	"target closed", "Target closed", "no such target",
	"use of closed network connection", "websocket: close",
}

func isStalePageErr(err error) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	for _, sub := range staleErrSubstrings {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

// reResolvePageLocked drops the cached page and re-fetches it live. Must hold m.mu.
func (m *Manager) reResolvePageLocked(targetID, tenantID string) (*rod.Page, error) {
	delete(m.pages, targetID)
	return m.getPageForTenant(targetID, tenantID)
}

// watchPageClose spawns a goroutine that closes page when ctx is cancelled.
// Returns a cancel func that stops the watchdog on normal-path close.
// Uses sync.Once so page.Close() is idempotent if both paths fire concurrently.
func watchPageClose(ctx context.Context, page *rod.Page) (stopWatchdog func()) {
	var once sync.Once
	closeFn := func() { _ = page.Close() }
	stopped := make(chan struct{})

	go func() {
		select {
		case <-ctx.Done():
			once.Do(closeFn)
		case <-stopped:
		}
	}()

	return func() {
		close(stopped)
	}
}

// Snapshot returns the page's accessibility tree, time-bounded and retried once if the page went stale.
func (m *Manager) Snapshot(ctx context.Context, targetID string, opts SnapshotOptions) (*SnapshotResult, error) {
	tenantID := tenantIDFromCtx(ctx)
	sessionKey := sessionKeyFromCtx(ctx)
	m.mu.Lock()
	targetID = m.sessionTargetLocked(sessionKey, targetID)
	page, err := m.getPageForTenant(targetID, tenantID)
	m.mu.Unlock()
	if err != nil {
		return nil, err
	}

	result, err := proto.AccessibilityGetFullAXTree{}.Call(page.Timeout(snapshotAXTimeout))
	if err != nil && isStalePageErr(err) {
		m.mu.Lock()
		page, err = m.reResolvePageLocked(targetID, tenantID)
		m.mu.Unlock()
		if err == nil {
			result, err = proto.AccessibilityGetFullAXTree{}.Call(page.Timeout(snapshotAXTimeout))
		}
	}
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || strings.Contains(err.Error(), "deadline exceeded") {
			return nil, fmt.Errorf("get AX tree: page too large to snapshot in %s — use a screenshot instead", snapshotAXTimeout)
		}
		return nil, fmt.Errorf("get AX tree: %w", err)
	}

	snap := FormatSnapshot(result.Nodes, opts)
	info, _ := page.Info()
	snap.TargetID = targetID
	if info != nil {
		snap.URL = info.URL
		snap.Title = info.Title
	}

	// Cache refs
	m.refs.Store(targetID, snap.Refs)

	return snap, nil
}

// Screenshot captures PNG bytes, retried once if the page went stale.
func (m *Manager) Screenshot(ctx context.Context, targetID string, fullPage bool) ([]byte, error) {
	tenantID := tenantIDFromCtx(ctx)
	sessionKey := sessionKeyFromCtx(ctx)
	m.mu.Lock()
	targetID = m.sessionTargetLocked(sessionKey, targetID)
	page, err := m.getPageForTenant(targetID, tenantID)
	m.mu.Unlock()
	if err != nil {
		return nil, err
	}

	img, err := capturePNG(page, fullPage)
	if err != nil && isStalePageErr(err) {
		m.mu.Lock()
		page, err = m.reResolvePageLocked(targetID, tenantID)
		m.mu.Unlock()
		if err == nil {
			img, err = capturePNG(page, fullPage)
		}
	}
	return img, err
}

func capturePNG(page *rod.Page, fullPage bool) ([]byte, error) {
	if fullPage {
		return page.Screenshot(true, &proto.PageCaptureScreenshot{
			Format: proto.PageCaptureScreenshotFormatPng,
		})
	}
	return page.Screenshot(false, nil)
}

// Navigate navigates a page to a URL.
// A ctx-cancel watchdog closes the page if ctx is done during the blocking WaitStable call.
func (m *Manager) Navigate(ctx context.Context, targetID, url string) error {
	tenantID := tenantIDFromCtx(ctx)
	sessionKey := sessionKeyFromCtx(ctx)
	m.mu.Lock()
	targetID = m.sessionTargetLocked(sessionKey, targetID)
	page, err := m.getPageForTenant(targetID, tenantID)
	m.mu.Unlock()

	if err != nil {
		return err
	}

	// Watchdog: close page on ctx cancel to unblock any pending Rod CDP calls.
	stop := watchPageClose(ctx, page)
	defer stop()

	if err := page.Navigate(url); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("navigate: %w", err)
	}
	if err := page.WaitStable(300 * time.Millisecond); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("wait stable after navigate: %w", err)
	}
	return nil
}

// Close shuts down the browser if running.
func (m *Manager) Close() error {
	return m.Stop(context.Background())
}

// Refs returns the RefStore for external use (e.g. actions).
func (m *Manager) Refs() *RefStore {
	return m.refs
}
