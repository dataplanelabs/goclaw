package browser

import (
	"context"
	"fmt"
	"time"

	"github.com/go-rod/rod/lib/proto"
)

// ListTabs returns open tabs filtered by the caller's tenant context.
func (m *Manager) ListTabs(ctx context.Context) ([]TabInfo, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.browser == nil {
		return nil, fmt.Errorf("browser not running")
	}

	tenantID := tenantIDFromCtx(ctx)

	// Use tenant-scoped browser context for page listing
	b, err := m.tenantBrowserLocked(tenantID)
	if err != nil {
		return nil, err
	}

	pages, err := b.Pages()
	if err != nil {
		if m.remoteURL != "" {
			if reconnErr := m.reconnectLocked(); reconnErr != nil {
				return nil, fmt.Errorf("list pages: %w (reconnect also failed: %v)", err, reconnErr)
			}
			m.logger.Info("auto-reconnected to remote Chrome")
			// Re-acquire tenant browser after reconnect (incognito contexts were reset)
			b, err = m.tenantBrowserLocked(tenantID)
			if err != nil {
				return nil, err
			}
			pages, err = b.Pages()
			if err != nil {
				return nil, fmt.Errorf("list pages after reconnect: %w", err)
			}
		} else {
			return nil, fmt.Errorf("list pages: %w", err)
		}
	}

	tabs := make([]TabInfo, 0, len(pages))
	for _, p := range pages {
		info, err := p.Info()
		if err != nil || info == nil {
			continue
		}
		tid := string(p.TargetID)
		m.pages[tid] = p
		if tenantID != "" {
			m.pageTenants[tid] = tenantID
		}
		tabs = append(tabs, TabInfo{
			TargetID: tid,
			URL:      info.URL,
			Title:    info.Title,
		})
	}
	return tabs, nil
}

// OpenTab opens a new tab with the given URL.
// Pages are created within the tenant's incognito browser context for isolation.
// If the tenant already has maxPages open, the oldest idle page is closed first.
func (m *Manager) OpenTab(ctx context.Context, url string) (*TabInfo, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	tenantID := tenantIDFromCtx(ctx)

	// Enforce max pages per tenant
	if m.maxPages > 0 {
		m.evictOldestIfOverLimitLocked(tenantID)
	}

	b, err := m.tenantBrowserLocked(tenantID)
	if err != nil {
		return nil, err
	}

	page, err := b.Page(proto.TargetCreateTarget{URL: url})
	if err != nil {
		return nil, fmt.Errorf("open tab: %w", err)
	}

	// Watchdog: close page on ctx cancel to unblock WaitStable CDP call.
	stopWatchdog := watchPageClose(ctx, page)
	if err := page.WaitStable(300 * time.Millisecond); err != nil {
		stopWatchdog()
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("wait stable: %w", err)
	}
	stopWatchdog()
	info, _ := page.Info()
	tid := string(page.TargetID)
	m.pages[tid] = page
	m.touchPageLocked(tid)
	if tenantID != "" {
		m.pageTenants[tid] = tenantID
	}

	// Set up console listener
	m.setupConsoleListener(page, tid)

	tab := &TabInfo{TargetID: tid, URL: url}
	if info != nil {
		tab.URL = info.URL
		tab.Title = info.Title
	}
	return tab, nil
}

// oldestEvictableLocked returns the oldest goclaw-opened tab for the tenant when
// at/over maxPages, else "". Tabs a human opened via noVNC (no pageLastUsed entry,
// never touched by goclaw) are not counted or evicted. Must be called with mu held.
func (m *Manager) oldestEvictableLocked(tenantID string) string {
	isMaster := tenantID == "" || tenantID == MasterTenantID

	var oldestID string
	var oldestTime time.Time
	var count int
	for tid := range m.pages {
		lu, tracked := m.pageLastUsed[tid]
		if !tracked {
			continue // human/manual noVNC tab — never evict
		}
		if isMaster {
			if _, hasOwner := m.pageTenants[tid]; hasOwner {
				continue
			}
		} else if m.pageTenants[tid] != tenantID {
			continue
		}
		count++
		if oldestID == "" || lu.Before(oldestTime) {
			oldestID, oldestTime = tid, lu
		}
	}

	if count < m.maxPages {
		return ""
	}
	return oldestID
}

// evictOldestIfOverLimitLocked closes the oldest goclaw-opened tab for a tenant if
// at or over maxPages. Never closes human noVNC tabs. Must be called with mu held.
func (m *Manager) evictOldestIfOverLimitLocked(tenantID string) {
	oldestID := m.oldestEvictableLocked(tenantID)
	if oldestID == "" {
		return
	}
	if page, ok := m.pages[oldestID]; ok {
		_ = page.Close()
	}
	delete(m.pages, oldestID)
	delete(m.console, oldestID)
	delete(m.pageTenants, oldestID)
	delete(m.pageLastUsed, oldestID)
	m.refs.Remove(oldestID)
	m.logger.Info("evicted oldest page (max pages reached)", "targetId", oldestID, "tenant", tenantID)
}

// FocusTab activates a tab.
func (m *Manager) FocusTab(ctx context.Context, targetID string) error {
	tenantID := tenantIDFromCtx(ctx)
	m.mu.Lock()
	defer m.mu.Unlock()

	page, err := m.getPageForTenant(targetID, tenantID)
	if err != nil {
		return err
	}

	_, err = page.Activate()
	return err
}

// CloseTab closes a tab.
func (m *Manager) CloseTab(ctx context.Context, targetID string) error {
	tenantID := tenantIDFromCtx(ctx)
	m.mu.Lock()
	defer m.mu.Unlock()

	page, err := m.getPageForTenant(targetID, tenantID)
	if err != nil {
		return err
	}

	delete(m.pages, targetID)
	delete(m.console, targetID)
	delete(m.pageTenants, targetID)
	delete(m.pageLastUsed, targetID)
	m.refs.Remove(targetID)
	return page.Close()
}

// ConsoleMessages returns captured console messages for a tab.
func (m *Manager) ConsoleMessages(ctx context.Context, targetID string) []ConsoleMessage {
	tenantID := tenantIDFromCtx(ctx)
	m.mu.Lock()
	defer m.mu.Unlock()

	// Validate tenant ownership
	if tenantID != "" && tenantID != MasterTenantID {
		if owner, ok := m.pageTenants[targetID]; ok && owner != tenantID {
			return []ConsoleMessage{}
		}
	}

	msgs := m.console[targetID]
	if msgs == nil {
		return []ConsoleMessage{}
	}

	// Return copy and clear
	result := make([]ConsoleMessage, len(msgs))
	copy(result, msgs)
	m.console[targetID] = nil
	return result
}
