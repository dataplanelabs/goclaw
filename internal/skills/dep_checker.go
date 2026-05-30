package skills

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

const depCheckTimeout = 5 * time.Second

// CheckSkillDeps verifies all dependencies in a manifest are available.
// Returns (ok, missing) where missing lists unavailable dependencies.
// For Python packages, sets PYTHONPATH=ScriptsDir so local modules and stdlib
// are resolved natively — only truly missing pip packages are reported.
func CheckSkillDeps(m *SkillManifest) (bool, []string) {
	if m == nil || m.IsEmpty() {
		return true, nil
	}
	ensureNpmGlobalEnv()

	var missing []string

	// Check system binaries
	for _, bin := range m.Requires {
		if _, err := exec.LookPath(bin); err != nil {
			missing = append(missing, bin)
		}
	}

	// Check Python packages via import with PYTHONPATH set.
	// If python3 binary is absent, skip per-package listing — the "python3" binary
	// is already reported via m.Requires and is the root cause.
	if len(m.RequiresPython) > 0 {
		if _, err := exec.LookPath("python3"); err == nil {
			missing = append(missing, checkPythonPackages(m.RequiresPython, m.ScriptsDir)...)
		}
	}

	// Check Node packages.
	// If node binary is absent, skip per-package listing for the same reason.
	if len(m.RequiresNode) > 0 {
		if _, err := exec.LookPath("node"); err == nil {
			missing = append(missing, checkNodePackages(m.RequiresNode, m.ScriptsDir)...)
		}
	}

	return len(missing) == 0, missing
}

// checkPythonPackages checks which Python import names are importable.
// Probes twice: with the inherited PYTHONPATH overlay (catches agent-installed packages
// in /app/data/.runtime/pip), then without (catches system site-packages that the overlay
// may shadow with ABI-incompatible stale .so files). Missing only when BOTH probes fail.
func checkPythonPackages(importNames []string, scriptsDir string) []string {
	if len(importNames) == 0 {
		return nil
	}
	missing := pythonProbeMissing(importNames, scriptsDir, true)
	if len(missing) == 0 {
		return nil
	}
	missing = pythonProbeMissing(missing, scriptsDir, false)
	out := make([]string, 0, len(missing))
	for _, name := range missing {
		out = append(out, "pip:"+importToPipName(name))
	}
	return out
}

func pythonProbeMissing(importNames []string, scriptsDir string, includeOverlay bool) []string {
	var sb strings.Builder
	sb.WriteString("import importlib\n")
	for _, name := range importNames {
		sb.WriteString(fmt.Sprintf("try:\n importlib.import_module(%q)\nexcept ImportError:\n print(%q)\n", name, name))
	}

	ctx, cancel := context.WithTimeout(context.Background(), depCheckTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "python3", "-c", sb.String())

	baseEnv := make([]string, 0, len(os.Environ()))
	for _, e := range os.Environ() {
		if !strings.HasPrefix(e, "PYTHONPATH=") {
			baseEnv = append(baseEnv, e)
		}
	}
	var pythonpath string
	if includeOverlay {
		pythonpath = os.Getenv("PYTHONPATH")
	}
	if scriptsDir != "" {
		if pythonpath != "" {
			pythonpath += ":" + scriptsDir
		} else {
			pythonpath = scriptsDir
		}
	}
	if pythonpath != "" {
		cmd.Env = append(baseEnv, "PYTHONPATH="+pythonpath)
	} else {
		cmd.Env = baseEnv
	}

	out, err := cmd.Output()
	if err != nil {
		var missing []string
		for _, name := range importNames {
			missing = append(missing, name)
		}
		return missing
	}

	var missing []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			missing = append(missing, line)
		}
	}
	return missing
}

// checkNodePackages checks which Node packages are resolvable.
// Sets CWD to scriptsDir so local require() calls resolve correctly.
func checkNodePackages(packages []string, scriptsDir string) []string {
	if len(packages) == 0 {
		return nil
	}

	var sb strings.Builder
	for _, pkg := range packages {
		sb.WriteString(fmt.Sprintf("try{require.resolve('%s')}catch(e){console.log(%q)}\n", pkg, pkg))
	}

	ctx, cancel := context.WithTimeout(context.Background(), depCheckTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "node", "-e", sb.String())
	cmd.Env = npmCommandEnv()
	if scriptsDir != "" {
		cmd.Dir = scriptsDir
	}

	out, err := cmd.Output()
	if err != nil {
		var missing []string
		for _, pkg := range packages {
			missing = append(missing, "npm:"+pkg)
		}
		return missing
	}

	var missing []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			missing = append(missing, "npm:"+line)
		}
	}
	return missing
}

// importToPipName maps Python import names to their pip package names when they differ.
var importToPipName = func(importName string) string {
	m := map[string]string{
		"cv2":         "opencv-python",
		"PIL":         "Pillow",
		"yaml":        "pyyaml",
		"sklearn":     "scikit-learn",
		"bs4":         "beautifulsoup4",
		"dateutil":    "python-dateutil",
		"dotenv":      "python-dotenv",
		"pptx":        "python-pptx",
		"docx":        "python-docx",
		"attr":        "attrs",
		"gi":          "PyGObject",
		"psycopg2":    "psycopg2-binary",
		"psycopg":     "psycopg[binary]",
		"MySQLdb":     "mysqlclient",
		"Crypto":      "pycryptodome",
		"serial":      "pyserial",
		"skimage":     "scikit-image",
		"Levenshtein": "python-Levenshtein",
	}
	if pip, ok := m[importName]; ok {
		return pip
	}
	return importName
}

// pipToImportName maps pip package names from explicit skill manifests to the
// top-level Python module that should be imported during dependency checks.
var pipToImportName = func(pipName string) string {
	m := map[string]string{
		"opencv-python":      "cv2",
		"pillow":             "PIL",
		"pyyaml":             "yaml",
		"scikit-learn":       "sklearn",
		"beautifulsoup4":     "bs4",
		"python-dateutil":    "dateutil",
		"python-dotenv":      "dotenv",
		"python-pptx":        "pptx",
		"python-docx":        "docx",
		"attrs":              "attr",
		"pygobject":          "gi",
		"psycopg2-binary":    "psycopg2",
		"psycopg[binary]":    "psycopg",
		"mysqlclient":        "MySQLdb",
		"pycryptodome":       "Crypto",
		"pyserial":           "serial",
		"scikit-image":       "skimage",
		"python-levenshtein": "Levenshtein",
	}
	if importName, ok := m[strings.ToLower(pipName)]; ok {
		return importName
	}
	return pipName
}

// FormatMissing formats a missing deps list into a human-readable string.
func FormatMissing(missing []string) string {
	return strings.Join(missing, ", ")
}
