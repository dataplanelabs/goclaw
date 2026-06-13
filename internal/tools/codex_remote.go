package tools

import (
	"context"
)

// CodexRemoteTool runs codex exec on a remote workstation (the sandbox-codex pod)
// by composing a workstation_exec call. Session continuity is file-based on the PVC:
// the default path uses `resume --last`; pass fresh:true to start a new session.
//
// Permission enforcement is fully delegated to WorkstationExecTool.permCheck.
// No separate permission layer; allowlist must include bare "codex".
type CodexRemoteTool struct {
	inner *WorkstationExecTool
}

// NewCodexRemoteTool creates a CodexRemoteTool backed by the given WorkstationExecTool.
func NewCodexRemoteTool(exec *WorkstationExecTool) *CodexRemoteTool {
	return &CodexRemoteTool{inner: exec}
}

func (t *CodexRemoteTool) Name() string { return "codex_remote" }

func (t *CodexRemoteTool) Description() string {
	return "Run codex exec on a remote sandbox pod over SSH. Requires codex authenticated on the pod. " +
		"By default resumes the last session (file-based continuity on PVC); set fresh:true to start a new session. " +
		"Streams output as workstation.exec.chunk events."
}

func (t *CodexRemoteTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"prompt": map[string]any{
				"type":        "string",
				"description": "Prompt to pass to codex exec",
			},
			"repo": map[string]any{
				"type":        "string",
				"description": "Absolute path to the repository on the pod (sets -C <repo>). Falls back to workstation defaultCwd if omitted.",
			},
			"fresh": map[string]any{
				"type":        "boolean",
				"description": "Start a fresh codex session instead of resuming the last one (default: false)",
			},
			"workstation_id": map[string]any{
				"type":        "string",
				"description": "Workstation UUID or key (optional if agent has a default binding)",
			},
		},
		"required": []string{"prompt"},
	}
}

// Execute composes a `codex exec --dangerously-bypass-approvals-and-sandbox [-C <repo>] [resume --last] <prompt>`
// invocation and delegates to WorkstationExecTool.Execute.
// The pod boundary is the isolation layer; --dangerously-bypass-approvals-and-sandbox is safe inside the hardened pod.
func (t *CodexRemoteTool) Execute(ctx context.Context, args map[string]any) *Result {
	prompt, _ := args["prompt"].(string)
	if prompt == "" {
		return ErrorResult("prompt is required")
	}

	repo, _ := args["repo"].(string)
	fresh, _ := args["fresh"].(bool)

	// Build codex argv: exec --dangerously-bypass-approvals-and-sandbox [-C <repo>] [resume --last] <prompt>
	cmdArgs := []string{"exec", "--dangerously-bypass-approvals-and-sandbox"}
	if repo != "" {
		cmdArgs = append(cmdArgs, "-C", repo)
	}
	if !fresh {
		cmdArgs = append(cmdArgs, "resume", "--last")
	}
	cmdArgs = append(cmdArgs, prompt)

	passthrough := map[string]any{
		"command":     "codex",
		"args":        cmdArgs,
		"timeout_sec": float64(3600),
	}
	if repo != "" {
		passthrough["cwd"] = repo
	}
	if wsID, ok := args["workstation_id"]; ok && wsID != nil {
		passthrough["workstation_id"] = wsID
	}

	return t.inner.Execute(ctx, passthrough)
}
