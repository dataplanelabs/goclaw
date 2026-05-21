package upgrade

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"testing"
)

// TestRequiredSchemaVersion_MatchesHighestMigrationFile is a guard against the
// failure mode where a new ./migrations/NNNNNN_*.up.sql file ships in the
// image but the runtime upgrade check skips it because RequiredSchemaVersion
// wasn't bumped in lockstep.
//
// Real-world incident this catches: shipping 000059_cron_write_only_hash with
// RequiredSchemaVersion=58 caused goclaw to log "Schema current: 58, Schema
// required: 58" and never apply the migration in prod — gcplane callers then
// hit "column write_only_hash does not exist" at runtime.
func TestRequiredSchemaVersion_MatchesHighestMigrationFile(t *testing.T) {
	// Walk up from this test file to the repo root, then look for ./migrations.
	repoRoot, err := findRepoRoot()
	if err != nil {
		t.Fatalf("locate repo root: %v", err)
	}
	migrationsDir := filepath.Join(repoRoot, "migrations")

	entries, err := os.ReadDir(migrationsDir)
	if err != nil {
		t.Fatalf("read migrations dir %s: %v", migrationsDir, err)
	}

	re := regexp.MustCompile(`^(\d+)_.*\.up\.sql$`)
	var highest uint
	for _, e := range entries {
		m := re.FindStringSubmatch(e.Name())
		if m == nil {
			continue
		}
		n, err := strconv.ParseUint(m[1], 10, 64)
		if err != nil {
			continue
		}
		if uint(n) > highest {
			highest = uint(n)
		}
	}

	if highest == 0 {
		t.Fatal("no migration files found — expected at least one NNNNNN_*.up.sql in ./migrations")
	}

	if RequiredSchemaVersion != highest {
		t.Errorf("RequiredSchemaVersion = %d, but highest migration file is %06d.\n"+
			"Either bump internal/upgrade/version.go to %d, or remove the stray migration file.",
			RequiredSchemaVersion, highest, highest)
	}
}

// findRepoRoot walks up from the current working directory looking for go.mod.
func findRepoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", os.ErrNotExist
		}
		dir = parent
	}
}
