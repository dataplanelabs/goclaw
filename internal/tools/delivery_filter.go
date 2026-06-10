package tools

import (
	"os"
	"path/filepath"
	"strings"
)

// IsScratchDeliveryPath returns true for paths that look like intermediate
// build artifacts rather than user-facing final files.
func IsScratchDeliveryPath(p string) bool {
	p = strings.TrimSpace(p)
	if p == "" {
		return false
	}

	normalized := filepath.ToSlash(filepath.Clean(p))
	scan := trimTempRoot(normalized)

	for _, part := range strings.Split(scan, "/") {
		name := strings.ToLower(strings.TrimSpace(part))
		if name == "" || name == "." {
			continue
		}
		if isScratchSegment(name) || isScratchFilename(name) {
			return true
		}
	}
	return false
}

func trimTempRoot(p string) string {
	tempRoots := []string{
		filepath.ToSlash(filepath.Clean(os.TempDir())),
		"/tmp",
		"/private/tmp",
		"/var/tmp",
		"/private/var/tmp",
	}
	for _, root := range tempRoots {
		if root == "." || root == "" {
			continue
		}
		if p == root {
			return ""
		}
		if strings.HasPrefix(p, root+"/") {
			return strings.TrimPrefix(p, root+"/")
		}
	}
	return p
}

func isScratchSegment(name string) bool {
	switch name {
	case ".tmp", "_tmp", "tmp", "temp", ".temp", "_temp",
		"staging", ".staging", "_staging",
		"scratch", ".scratch", "_scratch":
		return true
	default:
		return false
	}
}

func isScratchFilename(name string) bool {
	prefixes := []string{
		"_tmp", "tmp_", "tmp-", ".tmp",
		"_temp", "temp_", "temp-", ".temp",
		"_staging", "staging_", "staging-", ".staging",
		"_scratch", "scratch_", "scratch-", ".scratch",
	}
	for _, prefix := range prefixes {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return strings.Contains(name, ".tmp.") ||
		strings.Contains(name, ".temp.") ||
		strings.Contains(name, ".staging.") ||
		strings.Contains(name, ".scratch.")
}
