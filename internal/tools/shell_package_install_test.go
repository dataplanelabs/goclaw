package tools

import (
	"context"
	"strings"
	"testing"
)

func TestRedundantPipInstallImports(t *testing.T) {
	tests := []struct {
		name string
		cmd  string
		want []string
		ok   bool
	}{
		{name: "pip", cmd: "pip install pyzipper", want: []string{"pyzipper"}, ok: true},
		{name: "pip3 flags", cmd: "pip3 install --no-cache-dir --break-system-packages pyzipper==0.4.0", want: []string{"pyzipper"}, ok: true},
		{name: "python module", cmd: "python3 -m pip install pyzipper", want: []string{"pyzipper"}, ok: true},
		{name: "upgrade stays approval gated", cmd: "pip install --upgrade pyzipper", ok: false},
		{name: "requirements stays approval gated", cmd: "pip install -r requirements.txt", ok: false},
		{name: "shell chain stays approval gated", cmd: "pip install pyzipper && python3 script.py", ok: false},
		{name: "url stays approval gated", cmd: "pip install https://example.com/pkg.whl", ok: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := redundantPipInstallImports(tt.cmd)
			if ok != tt.ok {
				t.Fatalf("ok = %v, want %v (got imports %v)", ok, tt.ok, got)
			}
			if strings.Join(got, ",") != strings.Join(tt.want, ",") {
				t.Fatalf("imports = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestInstalledPipInstallResultAlreadyImportable(t *testing.T) {
	orig := pythonImportableForPipInstall
	pythonImportableForPipInstall = func(_ context.Context, importName string) bool {
		return importName == "pyzipper"
	}
	t.Cleanup(func() { pythonImportableForPipInstall = orig })

	result := installedPipInstallResult(context.Background(), "pip install pyzipper")
	if result == nil || result.IsError {
		t.Fatalf("result = %#v, want non-error short circuit", result)
	}
	if !strings.Contains(result.ForLLM, "already installed") {
		t.Fatalf("ForLLM = %q, want already installed message", result.ForLLM)
	}
}

func TestInstalledPipInstallResultMissingPackageFallsThrough(t *testing.T) {
	orig := pythonImportableForPipInstall
	pythonImportableForPipInstall = func(context.Context, string) bool { return false }
	t.Cleanup(func() { pythonImportableForPipInstall = orig })

	if result := installedPipInstallResult(context.Background(), "pip install pyzipper"); result != nil {
		t.Fatalf("result = %#v, want nil so approval flow can handle install", result)
	}
}
