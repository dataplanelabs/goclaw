package upgrade

// RequiredSchemaVersion is the schema migration version this binary requires.
// Bump this whenever adding a new SQL migration file.
//
// A guard test in version_test.go asserts this constant matches the highest
// migration file in ./migrations — so the next person who adds a migration
// without bumping the constant gets a CI failure instead of a silent skip in
// prod (see commit history around 000059_cron_write_only_hash).
const RequiredSchemaVersion uint = 80
