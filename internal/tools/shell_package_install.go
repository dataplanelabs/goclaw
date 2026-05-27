package tools

import (
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

var pipImportNameRE = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*(\.[A-Za-z_][A-Za-z0-9_]*)*$`)

var pythonImportableForPipInstall = defaultPythonImportableForPipInstall

// installedPipInstallResult avoids asking for package-install approval when a
// model redundantly runs "pip install <pkg>" for packages already baked into the
// runtime image. It is deliberately narrow: shell chains, upgrades, requirements
// files, URLs, paths, and unparseable package specs still go through approval.
func installedPipInstallResult(ctx context.Context, command string) *Result {
	imports, ok := redundantPipInstallImports(command)
	if !ok || len(imports) == 0 {
		return nil
	}
	for _, importName := range imports {
		if !pythonImportableForPipInstall(ctx, importName) {
			return nil
		}
	}
	return NewResult(fmt.Sprintf(
		"Python package(s) already installed and importable: %s. No install was run; import and use them directly.",
		strings.Join(imports, ", "),
	))
}

func redundantPipInstallImports(command string) ([]string, bool) {
	segments := nonEmptyShellSegments(command)
	if len(segments) != 1 {
		return nil, false
	}
	words := parseExecCommandWords(segments[0])
	if len(words) < 3 {
		return nil, false
	}

	i := 0
	switch words[0] {
	case "pip", "pip3":
		i = 1
	case "python", "python3":
		if len(words) < 5 || words[1] != "-m" || words[2] != "pip" {
			return nil, false
		}
		i = 3
	default:
		return nil, false
	}
	if words[i] != "install" {
		return nil, false
	}
	i++

	imports := make([]string, 0, len(words)-i)
	for ; i < len(words); i++ {
		word := strings.TrimSpace(words[i])
		if word == "" {
			continue
		}
		switch word {
		case "--no-cache-dir", "--break-system-packages", "--disable-pip-version-check", "--user", "-q", "-qq", "-qqq":
			continue
		}
		if strings.HasPrefix(word, "-") {
			return nil, false
		}
		importName, ok := pipSpecImportName(word)
		if !ok {
			return nil, false
		}
		imports = append(imports, importName)
	}
	return imports, len(imports) > 0
}

func nonEmptyShellSegments(command string) []string {
	raw := splitExecCommandSegments(command)
	segments := make([]string, 0, len(raw))
	for _, segment := range raw {
		segment = strings.TrimSpace(segment)
		if segment != "" {
			segments = append(segments, segment)
		}
	}
	return segments
}

func pipSpecImportName(spec string) (string, bool) {
	name := spec
	for _, op := range []string{"==", ">=", "<=", "~=", "!=", ">", "<"} {
		if idx := strings.Index(name, op); idx >= 0 {
			if idx == 0 {
				return "", false
			}
			name = name[:idx]
			break
		}
	}
	if idx := strings.IndexByte(name, '['); idx >= 0 {
		if idx == 0 {
			return "", false
		}
		name = name[:idx]
	}
	name = strings.TrimSpace(name)
	if !pipImportNameRE.MatchString(name) {
		return "", false
	}
	return name, true
}

func defaultPythonImportableForPipInstall(ctx context.Context, importName string) bool {
	if !pipImportNameRE.MatchString(importName) {
		return false
	}
	checkCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(checkCtx, "python3", "-c", "import "+importName)
	return cmd.Run() == nil
}
