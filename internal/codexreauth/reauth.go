// Package codexreauth triggers `codex login --device-auth` on a remote workstation
// over SSH and returns the verification URL + user-code from its output.
//
// Designed for deterministic, no-LLM paths (e.g. Telegram /login codex command).
// The flow:
//  1. Resolve the workstation by key from the store.
//  2. Background the login on-pod via nohup to avoid blocking (SSH sessions are
//     synchronous; device-auth can take minutes waiting for the user to approve).
//  3. Poll /tmp/codex-login.log for the verification URL + user-code (max 30s).
//  4. Return DeviceAuthInfo for the caller to surface to the user.
package codexreauth

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/nextlevelbuilder/goclaw/internal/store"
	"github.com/nextlevelbuilder/goclaw/internal/workstation"
)

// DeviceAuthInfo holds the parsed output from `codex login --device-auth`.
type DeviceAuthInfo struct {
	VerificationURL string
	UserCode        string
}

// URLPattern matches the device-auth verification URL emitted by codex ≥0.139.
// Example: "Open this URL: https://auth.openai.com/device?code=ABC-123"
var URLPattern = regexp.MustCompile(`https://[^\s]+`)

// UserCodePattern matches the user code line emitted by codex ≥0.139.
// Example: "Enter code: ABC-123" or "User code: ABC-123"
var UserCodePattern = regexp.MustCompile(`(?i)(?:user\s*code|enter\s*code)[:\s]+([A-Z0-9]{4}-[A-Z0-9]{4}(?:-[A-Z0-9]{4})?)`)

const (
	// defaultWorkstationKey is the key for the coding-agent workstation.
	defaultWorkstationKey = "coding-agent"
	// logPath is where nohup writes the device-auth output on-pod.
	logPath = "/tmp/codex-login.log"
	// pollTimeout is the maximum time to wait for the URL+code to appear in the log.
	pollTimeout = 60 * time.Second
	// pollInterval is how often we check the log file.
	pollInterval = 2 * time.Second
)

// Trigger launches `codex login --device-auth` in the background on the pod
// identified by workstationKey (defaults to "coding-agent") within the given tenant,
// then polls the log file until the verification URL + user-code are available.
//
// The caller must supply a WorkstationStore and BackendCache. The function opens its
// own SSH session (bypasses WorkstationExecTool so the argv allowlist is not a gate).
func Trigger(
	ctx context.Context,
	wsStore store.WorkstationStore,
	backendCache *workstation.BackendCache,
	tenantID uuid.UUID,
	workstationKey string,
) (*DeviceAuthInfo, error) {
	if workstationKey == "" {
		workstationKey = defaultWorkstationKey
	}

	storeCtx := store.WithTenantID(ctx, tenantID)
	ws, err := wsStore.GetByKey(storeCtx, workstationKey)
	if err != nil {
		return nil, fmt.Errorf("workstation %q not found: %w", workstationKey, err)
	}
	if !ws.Active {
		return nil, fmt.Errorf("workstation %q is inactive", workstationKey)
	}

	backend, err := backendCache.Get(ctx, ws.ID)
	if err != nil {
		return nil, fmt.Errorf("backend not ready: %w", err)
	}

	// Step 1: launch the login in the background so we don't block on the SSH session.
	// We truncate the log first, then nohup the process so we can safely poll later.
	launchCmd := fmt.Sprintf("echo '' > %s && nohup codex login --device-auth > %s 2>&1 &", logPath, logPath)
	if err := runShort(ctx, backend, launchCmd); err != nil {
		return nil, fmt.Errorf("launch device-auth: %w", err)
	}

	// Step 2: poll log file until URL+code appear.
	deadline := time.Now().Add(pollTimeout)
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		output, err := readLog(ctx, backend, logPath)
		if err == nil {
			if info := parseDeviceAuth(output); info != nil {
				return info, nil
			}
		}

		time.Sleep(pollInterval)
	}

	return nil, fmt.Errorf("timed out waiting for codex device-auth URL (is codex ≥0.139 installed?)")
}

// runShort opens a session, runs cmd (expected to complete quickly), and drains output.
func runShort(ctx context.Context, backend workstation.Backend, cmd string) error {
	sctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	sess, err := backend.OpenSession(sctx, "codex-reauth-launch-"+uuid.New().String())
	if err != nil {
		return err
	}
	defer func() { _ = sess.Close(context.Background()) }()

	req := workstation.ExecRequest{
		Cmd:     "sh",
		Args:    []string{"-c", cmd},
		Timeout: 8 * time.Second,
	}
	stream, err := sess.Exec(sctx, req)
	if err != nil {
		return err
	}
	// drain — we don't care about output for the launch step
	_, _ = io.ReadAll(stream.Stdout())
	_, _ = io.ReadAll(stream.Stderr())
	_, _ = stream.Wait()
	return nil
}

// readLog reads the contents of logPath on the pod.
func readLog(ctx context.Context, backend workstation.Backend, path string) (string, error) {
	sctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	sess, err := backend.OpenSession(sctx, "codex-reauth-poll-"+uuid.New().String())
	if err != nil {
		return "", err
	}
	defer func() { _ = sess.Close(context.Background()) }()

	req := workstation.ExecRequest{
		Cmd:     "cat",
		Args:    []string{path},
		Timeout: 4 * time.Second,
	}
	stream, err := sess.Exec(sctx, req)
	if err != nil {
		return "", err
	}
	out, _ := io.ReadAll(stream.Stdout())
	_, _ = io.ReadAll(stream.Stderr())
	_, _ = stream.Wait()
	return string(out), nil
}

// authJSONPath is the location codex writes its auth token on successful login.
const authJSONPath = "/root/.codex/auth.json"

// StatusResult is returned by Status.
type StatusResult struct {
	Authenticated bool      `json:"authenticated"`
	AuthAt        time.Time `json:"auth_at,omitempty"`
}

// Status checks whether codex auth.json exists and is fresh (written within maxAge).
// A missing or old file means the pod needs re-auth.
func Status(
	ctx context.Context,
	wsStore store.WorkstationStore,
	backendCache *workstation.BackendCache,
	tenantID uuid.UUID,
	workstationKey string,
	maxAge time.Duration,
) (*StatusResult, error) {
	if workstationKey == "" {
		workstationKey = defaultWorkstationKey
	}

	storeCtx := store.WithTenantID(ctx, tenantID)
	ws, err := wsStore.GetByKey(storeCtx, workstationKey)
	if err != nil {
		return nil, fmt.Errorf("workstation %q not found: %w", workstationKey, err)
	}
	if !ws.Active {
		return &StatusResult{Authenticated: false}, nil
	}

	backend, err := backendCache.Get(ctx, ws.ID)
	if err != nil {
		return nil, fmt.Errorf("backend not ready: %w", err)
	}

	// stat the auth.json file on the pod to read its mtime
	out, err := readLog(ctx, backend, authJSONPath)
	if err != nil || strings.TrimSpace(out) == "" {
		return &StatusResult{Authenticated: false}, nil
	}

	// Get mtime via stat
	mtimeOut, err := statMtime(ctx, backend, authJSONPath)
	if err != nil {
		return &StatusResult{Authenticated: false}, nil
	}

	authAt, err := time.Parse(time.RFC3339, strings.TrimSpace(mtimeOut))
	if err != nil {
		return &StatusResult{Authenticated: false}, nil
	}

	fresh := time.Since(authAt) <= maxAge
	return &StatusResult{Authenticated: fresh, AuthAt: authAt}, nil
}

// statMtime reads the modification time of a file on the pod via `date -r`.
func statMtime(ctx context.Context, backend workstation.Backend, path string) (string, error) {
	sctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	sess, err := backend.OpenSession(sctx, "codex-reauth-stat-"+uuid.New().String())
	if err != nil {
		return "", err
	}
	defer func() { _ = sess.Close(context.Background()) }()

	req := workstation.ExecRequest{
		// Use `date -r <path> +%Y-%m-%dT%H:%M:%SZ` on Linux (GNU coreutils)
		Cmd:     "date",
		Args:    []string{"-r", path, "+%Y-%m-%dT%H:%M:%SZ"},
		Timeout: 4 * time.Second,
	}
	stream, err := sess.Exec(sctx, req)
	if err != nil {
		return "", err
	}
	out, _ := io.ReadAll(stream.Stdout())
	_, _ = io.ReadAll(stream.Stderr())
	_, _ = stream.Wait()
	return string(out), nil
}

// parseDeviceAuth scans the log output for the verification URL and user code.
// Returns nil if both have not appeared yet.
func parseDeviceAuth(output string) *DeviceAuthInfo {
	var info DeviceAuthInfo

	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		line := scanner.Text()

		if info.VerificationURL == "" {
			if m := URLPattern.FindString(line); m != "" {
				// exclude codex update-check URLs that may appear at startup
				if strings.Contains(m, "openai.com") || strings.Contains(m, "auth") {
					info.VerificationURL = m
				}
			}
		}

		if info.UserCode == "" {
			if m := UserCodePattern.FindStringSubmatch(line); len(m) >= 2 {
				info.UserCode = m[1]
			}
		}
	}

	if info.VerificationURL == "" || info.UserCode == "" {
		return nil
	}
	return &info
}
