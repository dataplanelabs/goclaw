package tools

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/i18n"
	"github.com/nextlevelbuilder/goclaw/internal/store"
)

// SecureCliRunTool is the typed agent-tool surface for skills that want to
// invoke registered CLI binaries (gh, kubectl, codex, gcplane, ...) through
// the secure_cli credentialed exec path. Calls flow into the same
// (*ExecTool).executeCredentialed() that backs the `exec` tool — same three
// gates (shell-operator reject, deny_args, env scrub) plus the same audit log.
//
// Construction order matters: depends on a fully-initialized *ExecTool with
// secureCLIStore already wired. Register in cmd/gateway_tools_wiring.go AFTER
// the exec tool.
type SecureCliRunTool struct {
	execTool *ExecTool
}

func NewSecureCliRunTool(execTool *ExecTool) *SecureCliRunTool {
	return &SecureCliRunTool{execTool: execTool}
}

func (t *SecureCliRunTool) Name() string { return "secure_cli_run" }

func (t *SecureCliRunTool) Description() string {
	return "Invoke a registered CLI binary (gh, kubectl, etc.) under the secure_cli credentialed exec gate. " +
		"Credentials are injected per-grant; shell operators and deny_args are blocked. " +
		"Use this for skills that wrap CLIs instead of shelling out directly."
}

func (t *SecureCliRunTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"binary": map[string]any{
				"type":        "string",
				"description": "Registered binary name (e.g. \"gh\", \"kubectl\").",
			},
			"args": map[string]any{
				"type":        "array",
				"items":       map[string]any{"type": "string"},
				"description": "Argument list, NO shell operators. Each element is a separate argv slot.",
			},
			"cwd": map[string]any{
				"type":        "string",
				"description": "Working directory (defaults to workspace root).",
			},
		},
		"required": []string{"binary", "args"},
	}
}

func (t *SecureCliRunTool) Execute(ctx context.Context, args map[string]any) *Result {
	locale := store.LocaleFromContext(ctx)
	if t.execTool == nil {
		return ErrorResult("secure_cli_run: exec tool not wired")
	}
	if t.execTool.secureCLIStore == nil {
		return ErrorResult("secure_cli_run: secure_cli store not configured")
	}

	binary, _ := args["binary"].(string)
	binary = strings.TrimSpace(binary)
	if binary == "" {
		return ErrorResult(i18n.T(locale, i18n.MsgRequired, "binary"))
	}
	normBinary := normalizeBinaryName(binary)

	rawArgs, _ := args["args"].([]any)
	cmdArgs := make([]string, 0, len(rawArgs))
	for _, a := range rawArgs {
		s, ok := a.(string)
		if !ok {
			return ErrorResult(fmt.Sprintf("secure_cli_run: args[%d] must be a string", len(cmdArgs)))
		}
		cmdArgs = append(cmdArgs, s)
	}

	// Default to tenant-scoped workspace; fall back to global only if context has none.
	tenantWs := ToolWorkspaceFromCtx(ctx)
	if tenantWs == "" {
		tenantWs = t.execTool.workspace
	}
	cwd := tenantWs
	if rawCwd, _ := args["cwd"].(string); rawCwd != "" {
		// Mirror shell.go's effectiveRestrict path: clamp agent-supplied cwd to
		// the tenant workspace so gh/codex cannot escape via a rogue working dir.
		allowed := allowedWriteWithTeamWorkspace(ctx, nil)
		resolved, err := resolvePathWithAllowed(rawCwd, tenantWs, true, allowed)
		if err != nil {
			return ErrorResult(fmt.Sprintf("secure_cli_run: cwd rejected: %v", err))
		}
		cwd = resolved
	}

	agentID := store.AgentIDFromContext(ctx)
	var agentIDPtr *uuid.UUID
	if agentID != uuid.Nil {
		agentIDPtr = &agentID
	}
	userID := store.CredentialUserIDFromContext(ctx)

	cred, err := t.execTool.secureCLIStore.LookupByBinary(ctx, normBinary, agentIDPtr, userID)
	if err != nil {
		slog.Warn("secure_cli_run.lookup_failed", "binary", binary, "agent_id", agentID, "error", err)
		return ErrorResult(i18n.T(locale, i18n.MsgInternalError, err.Error()))
	}
	if cred == nil {
		// Distinguish "not registered" vs "no grant" via IsRegisteredBinary
		// (cheap second query; only on the error path).
		registered, regErr := t.execTool.secureCLIStore.IsRegisteredBinary(ctx, normBinary)
		if regErr == nil && registered {
			slog.Warn("secure_cli_run.no_grant", "binary", binary, "agent_id", agentID)
			return ErrorResult(i18n.T(locale, i18n.MsgSecureCliNoGrant, binary))
		}
		slog.Warn("secure_cli_run.binary_not_found", "binary", binary, "agent_id", agentID)
		return ErrorResult(i18n.T(locale, i18n.MsgSecureCliBinaryNotFound, binary))
	}

	// Reconstruct rawCommand for shell-operator detection (in executeCredentialed Step 1).
	// Skill authors pass args as an array so quoting is preserved; we re-join only
	// for the detector — args[] is what actually reaches exec.Command.
	rawCommand := binary
	if len(cmdArgs) > 0 {
		rawCommand = binary + " " + strings.Join(cmdArgs, " ")
	}

	sandboxKey := ToolSandboxKeyFromCtx(ctx)

	slog.Info("secure_cli_run.exec",
		"binary", binary,
		"args_count", len(cmdArgs),
		"agent_id", agentID,
		"tenant_id", store.TenantIDFromContext(ctx),
		"cred_id", cred.ID,
	)

	return t.execTool.executeCredentialed(ctx, cred, normBinary, cmdArgs, cwd, sandboxKey, rawCommand)
}
