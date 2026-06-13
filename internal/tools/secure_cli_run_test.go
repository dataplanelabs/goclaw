package tools

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

func TestSecureCliRun_Schema(t *testing.T) {
	et := NewExecTool(t.TempDir(), false)
	et.SetSecureCLIStore(newStubSecureCLIStore())
	tool := NewSecureCliRunTool(et)

	if got := tool.Name(); got != "secure_cli_run" {
		t.Fatalf("name=%q, want secure_cli_run", got)
	}
	params := tool.Parameters()
	props, _ := params["properties"].(map[string]any)
	for _, want := range []string{"binary", "args", "cwd"} {
		if _, ok := props[want]; !ok {
			t.Errorf("missing param %q", want)
		}
	}
	req, _ := params["required"].([]string)
	if len(req) != 2 || req[0] != "binary" || req[1] != "args" {
		t.Errorf("required=%v, want [binary args]", req)
	}
}

func TestSecureCliRun_ExecToolNotWired(t *testing.T) {
	tool := NewSecureCliRunTool(nil)
	res := tool.Execute(context.Background(), map[string]any{"binary": "gh", "args": []any{"--version"}})
	if res == nil || !res.IsError || !strings.Contains(res.ForLLM, "exec tool not wired") {
		t.Fatalf("expected 'exec tool not wired' error, got %+v", res)
	}
}

func TestSecureCliRun_SecureStoreMissing(t *testing.T) {
	et := NewExecTool(t.TempDir(), false)
	// no SetSecureCLIStore
	tool := NewSecureCliRunTool(et)
	res := tool.Execute(context.Background(), map[string]any{"binary": "gh", "args": []any{"--version"}})
	if res == nil || !res.IsError || !strings.Contains(res.ForLLM, "secure_cli store not configured") {
		t.Fatalf("expected 'store not configured' error, got %+v", res)
	}
}

func TestSecureCliRun_MissingBinary(t *testing.T) {
	et := NewExecTool(t.TempDir(), false)
	et.SetSecureCLIStore(newStubSecureCLIStore())
	tool := NewSecureCliRunTool(et)
	res := tool.Execute(context.Background(), map[string]any{"binary": "", "args": []any{}})
	if res == nil || !res.IsError {
		t.Fatalf("expected error for empty binary, got %+v", res)
	}
}

func TestSecureCliRun_ArgsNotStringSlice(t *testing.T) {
	et := NewExecTool(t.TempDir(), false)
	et.SetSecureCLIStore(newStubSecureCLIStore())
	tool := NewSecureCliRunTool(et)
	res := tool.Execute(context.Background(), map[string]any{"binary": "gh", "args": []any{42}})
	if res == nil || !res.IsError || !strings.Contains(res.ForLLM, "must be a string") {
		t.Fatalf("expected 'must be a string' error, got %+v", res)
	}
}

func TestSecureCliRun_BinaryNotRegistered(t *testing.T) {
	et := NewExecTool(t.TempDir(), false)
	stub := newStubSecureCLIStore()
	// no entry for "gh", no registration
	et.SetSecureCLIStore(stub)
	tool := NewSecureCliRunTool(et)
	res := tool.Execute(context.Background(), map[string]any{"binary": "gh", "args": []any{"--version"}})
	if res == nil || !res.IsError || !strings.Contains(res.ForLLM, "not registered") {
		t.Fatalf("expected 'not registered' error, got %+v", res)
	}
}

func TestSecureCliRun_NoGrant(t *testing.T) {
	et := NewExecTool(t.TempDir(), false)
	stub := newStubSecureCLIStore()
	// Registered but no grant — LookupByBinary returns nil; IsRegisteredBinary returns true.
	stub.registered["gh"] = true
	et.SetSecureCLIStore(stub)
	tool := NewSecureCliRunTool(et)
	res := tool.Execute(context.Background(), map[string]any{"binary": "gh", "args": []any{"--version"}})
	if res == nil || !res.IsError || !strings.Contains(res.ForLLM, "no grant") {
		t.Fatalf("expected 'no grant' error, got %+v", res)
	}
}

// Gap A: cwd="/etc" must be rejected before reaching cmd.Dir.
func TestSecureCliRun_CwdAbsoluteEscape_Rejected(t *testing.T) {
	ws := t.TempDir()
	et := NewExecTool(ws, false)
	stub := newStubSecureCLIStore()
	et.SetSecureCLIStore(stub)
	tool := NewSecureCliRunTool(et)

	ctx := WithToolWorkspace(context.Background(), ws)
	res := tool.Execute(ctx, map[string]any{
		"binary": "gh",
		"args":   []any{"--version"},
		"cwd":    "/etc",
	})
	if res == nil || !res.IsError {
		t.Fatalf("expected cwd rejection, got %+v", res)
	}
	if !strings.Contains(res.ForLLM, "cwd rejected") {
		t.Fatalf("expected 'cwd rejected' in error, got: %s", res.ForLLM)
	}
}

// Gap A: cwd with ".." path traversal must be clamped/rejected.
func TestSecureCliRun_CwdDotDotEscape_Rejected(t *testing.T) {
	ws := t.TempDir()
	et := NewExecTool(ws, false)
	stub := newStubSecureCLIStore()
	et.SetSecureCLIStore(stub)
	tool := NewSecureCliRunTool(et)

	ctx := WithToolWorkspace(context.Background(), ws)
	res := tool.Execute(ctx, map[string]any{
		"binary": "gh",
		"args":   []any{"--version"},
		"cwd":    filepath.Join(ws, "..", "..", "etc"),
	})
	if res == nil || !res.IsError {
		t.Fatalf("expected cwd rejection for path traversal, got %+v", res)
	}
	if !strings.Contains(res.ForLLM, "cwd rejected") {
		t.Fatalf("expected 'cwd rejected' in error, got: %s", res.ForLLM)
	}
}

// Gap A: cwd within the tenant workspace must be accepted.
func TestSecureCliRun_CwdInsideWorkspace_Accepted(t *testing.T) {
	ws := t.TempDir()
	et := NewExecTool(ws, false)
	stub := newStubSecureCLIStore()
	et.SetSecureCLIStore(stub)
	tool := NewSecureCliRunTool(et)

	ctx := WithToolWorkspace(context.Background(), ws)
	// Provide a cwd inside the workspace. The store returns nil → binary-not-found
	// error (not a cwd error), proving cwd validation passed.
	res := tool.Execute(ctx, map[string]any{
		"binary": "gh",
		"args":   []any{"--version"},
		"cwd":    ws,
	})
	// Expect a binary-not-found or no-grant error, NOT a cwd-rejected error.
	if res != nil && res.IsError && strings.Contains(res.ForLLM, "cwd rejected") {
		t.Fatalf("valid cwd inside workspace was incorrectly rejected: %s", res.ForLLM)
	}
}

// Gap A: omitting cwd defaults to tenant workspace (not global workspace).
func TestSecureCliRun_NoCwd_DefaultsToTenantWorkspace(t *testing.T) {
	globalWs := t.TempDir()
	tenantWs := t.TempDir()

	et := NewExecTool(globalWs, false)
	stub := newStubSecureCLIStore()
	stub.registered["gh"] = true
	et.SetSecureCLIStore(stub)
	tool := NewSecureCliRunTool(et)

	ctx := WithToolWorkspace(context.Background(), tenantWs)
	// No "cwd" key — Execute should default to tenantWs.
	// The call reaches no-grant error, but cwd resolution must not touch globalWs.
	// We verify indirectly: the tool does not panic and reaches the store lookup.
	stub.mu.Lock()
	before := stub.lookupCalls
	stub.mu.Unlock()

	tool.Execute(ctx, map[string]any{"binary": "gh", "args": []any{}})

	stub.mu.Lock()
	after := stub.lookupCalls
	stub.mu.Unlock()
	if after == before {
		t.Fatal("expected LookupByBinary to be called (default cwd should not block execution)")
	}
}
