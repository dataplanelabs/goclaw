package tools

import (
	"os"
	"strconv"
	"time"
)

const defaultToolTimeout = 180 * time.Second

// toolTimeoutFromEnv reads "<NAME>_TIMEOUT_SEC" from env; default 180s.
// Used by per-tool timeouts to bound HTTP/IO so a hung provider call fails
// loud instead of producing zombie agent traces.
func toolTimeoutFromEnv(name string) time.Duration {
	if v := os.Getenv(name + "_TIMEOUT_SEC"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return time.Duration(n) * time.Second
		}
	}
	return defaultToolTimeout
}
