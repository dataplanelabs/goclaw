package vault

import "strings"

// StripTenantPrefix removes a leading "tenants/<slug>/" segment so a legacy
// tenant-prefixed vault path resolves against the tenant workspace root exactly
// like a bare tenant-root-relative path. A single-segment "tenants/<slug>" (no
// file part) is left unchanged — it has nothing to strip to. The canonical
// stored convention is tenant-root-relative (no prefix); this normalizes any
// older prefixed value at read time and is the runtime twin of the DB migration.
func StripTenantPrefix(p string) string {
	rest, ok := strings.CutPrefix(p, "tenants/")
	if !ok {
		return p
	}
	if i := strings.IndexByte(rest, '/'); i >= 0 {
		return rest[i+1:]
	}
	return p
}
