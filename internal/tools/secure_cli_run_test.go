package tools

import (
	"context"
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
