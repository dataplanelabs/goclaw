// Package skills loads and manages SKILL.md files from multiple source directories.
// Skills are injected into the agent's system prompt to provide specialized knowledge.
//
// Hierarchy (highest priority wins, matching TS loadSkillEntries):
//  1. Workspace skills          — <workspace>/skills/
//  2. Project agent skills      — <workspace>/.agents/skills/
//  3. Personal agent skills     — ~/.agents/skills/
//  4. Global/managed skills     — ~/.goclaw/skills/
//  5. Builtin skills            — bundled with binary
package skills

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Metadata holds parsed SKILL.md frontmatter.
type Metadata struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// Info describes a discovered skill.
type Info struct {
	Name        string `json:"name"`
	Slug        string `json:"slug"`    // directory name (unique identifier)
	Path        string `json:"path"`    // absolute path to SKILL.md
	BaseDir     string `json:"baseDir"` // skill directory (parent of SKILL.md)
	Source      string `json:"source"`  // "workspace", "global", "builtin"
	Description string `json:"description"`
}

// Loader discovers and loads SKILL.md files from multiple directories.
type Loader struct {
	// Skill directories in priority order (highest first).
	// Matches TS loadSkillEntries() 5-tier hierarchy.
	workspaceSkills     string // <workspace>/skills/
	projectAgentSkills  string // <workspace>/.agents/skills/
	personalAgentSkills string // ~/.agents/skills/
	globalSkills        string // ~/.goclaw/skills/
	builtinSkills       string // bundled with binary

	// DB-managed skills directories (set via SetManagedDir / AddManagedDir).
	// Each entry uses versioned subdirectory structure: <dir>/<slug>/<version>/SKILL.md.
	// Multiple dirs let the loader index both the master skills-store and any
	// per-tenant skills-store roots (e.g. /app/data/tenants/<tenant>/skills-store)
	// in a single BM25 index — tenant scoping is enforced downstream by
	// SkillSearchTool.filterByAccess, not by hiding files here.
	managedSkillsDirs []string

	mu    sync.RWMutex
	cache map[string]*Info // name → info (lazily populated)

	// Version tracking for hot-reload (matching TS bumpSkillsSnapshotVersion).
	// Bumped by the watcher on SKILL.md changes; consumers compare to detect staleness.
	version atomic.Int64
}

// NewLoader creates a skills loader.
// workspace: project workspace root (skills dir is workspace/skills/)
// globalSkills: global skills directory (e.g. ~/.goclaw/skills)
// builtinSkills: bundled skills directory
func NewLoader(workspace, globalSkills, builtinSkills string) *Loader {
	wsSkills := ""
	projectAgentSkills := ""
	if workspace != "" {
		wsSkills = filepath.Join(workspace, "skills")
		projectAgentSkills = filepath.Join(workspace, ".agents", "skills")
	}

	// Personal agent skills: ~/.agents/skills/ (matching TS)
	homeDir, _ := os.UserHomeDir()
	personalAgentSkills := ""
	if homeDir != "" && os.Getenv("GOCLAW_DISABLE_PERSONAL_SKILLS") != "1" {
		personalAgentSkills = filepath.Join(homeDir, ".agents", "skills")
	}

	return &Loader{
		workspaceSkills:     wsSkills,
		projectAgentSkills:  projectAgentSkills,
		personalAgentSkills: personalAgentSkills,
		globalSkills:        globalSkills,
		builtinSkills:       builtinSkills,
		cache:               make(map[string]*Info),
	}
}

// SetManagedDir replaces the managed skills directory list with a single dir.
// Managed skills use versioned subdirectories: <dir>/<slug>/<version>/SKILL.md.
// Called after PG stores are created.
func (l *Loader) SetManagedDir(dir string) {
	if dir == "" {
		l.managedSkillsDirs = nil
	} else {
		l.managedSkillsDirs = []string{dir}
	}
	l.BumpVersion() // trigger re-scan
}

// AddManagedDir appends an additional managed skills directory. Used to register
// per-tenant skills-store roots on top of the master one set via SetManagedDir.
// Duplicates are ignored. Empty paths are skipped silently.
func (l *Loader) AddManagedDir(dir string) {
	if dir == "" {
		return
	}
	for _, existing := range l.managedSkillsDirs {
		if existing == dir {
			return
		}
	}
	l.managedSkillsDirs = append(l.managedSkillsDirs, dir)
	l.BumpVersion()
}

// hasManagedDirs reports whether any managed dir is configured.
func (l *Loader) hasManagedDirs() bool { return len(l.managedSkillsDirs) > 0 }

// ListSkills returns all available skills, respecting the priority hierarchy.
// Higher-priority sources override lower ones by name.
func (l *Loader) ListSkills(_ context.Context) []Info {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.cache = make(map[string]*Info)
	seen := make(map[string]bool)
	var skills []Info

	// Priority: workspace > project-agents > personal-agents > global > managed > builtin
	// Managed (DB-seeded) skills take priority over raw bundled files so agents
	// always receive paths within the skills-store (workspace-accessible), not /app/bundled-skills/.
	for _, src := range []struct {
		dir    string
		source string
	}{
		{l.workspaceSkills, "workspace"},
		{l.projectAgentSkills, "agents-project"},
		{l.personalAgentSkills, "agents-personal"},
		{l.globalSkills, "global"},
	} {
		if src.dir == "" {
			continue
		}
		dirs, err := os.ReadDir(src.dir)
		if err != nil {
			continue
		}
		for _, d := range dirs {
			if !d.IsDir() || seen[d.Name()] {
				continue
			}
			skillFile := filepath.Join(src.dir, d.Name(), "SKILL.md")
			if _, err := os.Stat(skillFile); err != nil {
				continue
			}

			info := Info{
				Name:    d.Name(),
				Slug:    d.Name(),
				Path:    skillFile,
				BaseDir: filepath.Join(src.dir, d.Name()),
				Source:  src.source,
			}
			if meta := parseMetadata(skillFile); meta != nil {
				info.Description = meta.Description
				if meta.Name != "" {
					info.Name = meta.Name
				}
			}
			skills = append(skills, info)
			seen[d.Name()] = true
			l.cacheSkillLocked(&info)
		}
	}

	// Managed skills (versioned, DB-seeded) come before builtin so their workspace paths win.
	// Iterate ALL configured managed dirs (master + any tenant-scoped dirs) so
	// BM25 / system-prompt listing surface tenant-uploaded skills, not just the master ones.
	if l.hasManagedDirs() {
		for _, info := range l.listManagedSkills() {
			if seen[info.Slug] {
				continue
			}
			skills = append(skills, info)
			seen[info.Slug] = true
			l.cacheSkillLocked(&info)
		}
	}

	// Builtin (raw bundled files) — lowest priority fallback.
	if l.builtinSkills != "" {
		dirs, err := os.ReadDir(l.builtinSkills)
		if err == nil {
			for _, d := range dirs {
				if !d.IsDir() || seen[d.Name()] {
					continue
				}
				skillFile := filepath.Join(l.builtinSkills, d.Name(), "SKILL.md")
				if _, err := os.Stat(skillFile); err != nil {
					continue
				}
				info := Info{
					Name:    d.Name(),
					Slug:    d.Name(),
					Path:    skillFile,
					BaseDir: filepath.Join(l.builtinSkills, d.Name()),
					Source:  "builtin",
				}
				if meta := parseMetadata(skillFile); meta != nil {
					info.Description = meta.Description
					if meta.Name != "" {
						info.Name = meta.Name
					}
				}
				skills = append(skills, info)
				seen[d.Name()] = true
				l.cacheSkillLocked(&info)
			}
		}
	}

	return skills
}

func (l *Loader) cacheSkillLocked(info *Info) {
	for _, key := range []string{
		info.Slug,
		info.Name,
		strings.ToLower(info.Name),
		normalizeForSearch(info.Name),
		Slugify(info.Name),
	} {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		if _, exists := l.cache[key]; !exists {
			l.cache[key] = info
		}
	}
}

// listManagedSkills scans every configured managed dir for versioned skill directories.
// Structure: <managedDir>/<slug>/<version>/SKILL.md. Returns the latest version of each
// unique slug found. Earlier dirs win on slug collision so callers can prepend the
// authoritative (master) dir before tenant-scoped overlays if they care about precedence.
func (l *Loader) listManagedSkills() []Info {
	seen := make(map[string]struct{})
	var skills []Info
	for _, root := range l.managedSkillsDirs {
		dirs, err := os.ReadDir(root)
		if err != nil {
			continue
		}
		for _, d := range dirs {
			if !d.IsDir() {
				continue
			}
			slug := d.Name()
			if _, dup := seen[slug]; dup {
				continue
			}

			latestVersion, latestDir := l.findLatestVersionIn(root, slug)
			if latestVersion < 0 {
				continue
			}

			skillFile := filepath.Join(latestDir, "SKILL.md")
			if _, err := os.Stat(skillFile); err != nil {
				continue
			}

			info := Info{
				Name:    slug,
				Slug:    slug,
				Path:    skillFile,
				BaseDir: latestDir,
				Source:  "managed",
			}
			if meta := parseMetadata(skillFile); meta != nil {
				info.Description = meta.Description
				if meta.Name != "" {
					info.Name = meta.Name
				}
			}
			skills = append(skills, info)
			seen[slug] = struct{}{}
		}
	}
	return skills
}

// findLatestVersion searches every managed dir for the highest-numbered version of
// the given slug. The first dir holding any version wins (matches the iteration
// order in listManagedSkills, keeping the chosen file consistent across callers).
// Returns (version, path) or (-1, "") if no valid version found.
func (l *Loader) findLatestVersion(slug string) (int, string) {
	for _, root := range l.managedSkillsDirs {
		if v, dir := l.findLatestVersionIn(root, slug); v >= 0 {
			return v, dir
		}
	}
	return -1, ""
}

// findLatestVersionIn looks up the latest version inside a single managed root.
func (l *Loader) findLatestVersionIn(root, slug string) (int, string) {
	slugDir := filepath.Join(root, slug)
	entries, err := os.ReadDir(slugDir)
	if err != nil {
		return -1, ""
	}

	var versions []int
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		v, err := strconv.Atoi(e.Name())
		if err != nil || v < 1 {
			continue
		}
		versions = append(versions, v)
	}
	if len(versions) == 0 {
		return -1, ""
	}

	sort.Sort(sort.Reverse(sort.IntSlice(versions)))
	latestVer := versions[0]
	return latestVer, filepath.Join(slugDir, strconv.Itoa(latestVer))
}

// LoadSkill reads and returns the content of a skill by name (frontmatter stripped).
// The {baseDir} placeholder in SKILL.md is replaced with the skill's absolute directory path.
// Priority: workspace > agents > global > managed > builtin
func (l *Loader) LoadSkill(_ context.Context, name string) (string, bool) {
	// Check flat skill directories (workspace, agents, global) first
	for _, dir := range []string{l.workspaceSkills, l.projectAgentSkills, l.personalAgentSkills, l.globalSkills} {
		if dir == "" {
			continue
		}
		path := filepath.Join(dir, name, "SKILL.md")
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		content := stripFrontmatter(string(data))
		content = strings.ReplaceAll(content, "{baseDir}", filepath.Join(dir, name))
		return content, true
	}

	// Managed skills (DB-seeded, versioned) take priority over raw builtin files.
	if l.hasManagedDirs() {
		latestVer, latestDir := l.findLatestVersion(name)
		if latestVer >= 0 {
			path := filepath.Join(latestDir, "SKILL.md")
			data, err := os.ReadFile(path)
			if err == nil {
				content := stripFrontmatter(string(data))
				content = strings.ReplaceAll(content, "{baseDir}", latestDir)
				return content, true
			}
		}
	}

	// Builtin fallback (only if not in managed)
	if l.builtinSkills != "" {
		path := filepath.Join(l.builtinSkills, name, "SKILL.md")
		data, err := os.ReadFile(path)
		if err == nil {
			content := stripFrontmatter(string(data))
			content = strings.ReplaceAll(content, "{baseDir}", filepath.Join(l.builtinSkills, name))
			return content, true
		}
	}

	return "", false
}

// LoadForContext loads multiple skills and formats them for system prompt injection.
// If allowList is nil, all skills are loaded. If non-nil, only listed skills are loaded.
func (l *Loader) LoadForContext(ctx context.Context, allowList []string) string {
	var names []string

	if allowList == nil {
		// Load all available skills
		for _, s := range l.ListSkills(ctx) {
			names = append(names, s.Name)
		}
	} else {
		names = allowList
	}

	if len(names) == 0 {
		return ""
	}

	var parts []string
	for _, name := range names {
		content, ok := l.LoadSkill(ctx, name)
		if !ok {
			continue
		}
		parts = append(parts, fmt.Sprintf("### Skill: %s\n\n%s", name, content))
	}

	if len(parts) == 0 {
		return ""
	}

	return "## Available Skills\n\n" + strings.Join(parts, "\n\n---\n\n")
}

// skillDescMaxLen is the max character length for skill descriptions
// in the system prompt inline XML. ~200 chars ≈ ~50 tokens, balancing
// discoverability with prompt budget. Full SKILL.md is read on actual use.
const skillDescMaxLen = 200

// BuildSummary returns an XML summary of skills for context injection.
// If allowList is nil, all skills are included. If non-nil, only listed skills are included.
// The format matches the TS <available_skills> XML used in system prompts.
func (l *Loader) BuildSummary(ctx context.Context, allowList []string) string {
	allSkills := l.ListSkills(ctx)
	if len(allSkills) == 0 {
		return ""
	}

	// Filter by allowList if provided
	var filtered []Info
	if allowList == nil {
		filtered = allSkills
	} else {
		allowed := make(map[string]bool, len(allowList))
		for _, name := range allowList {
			allowed[name] = true
		}
		for _, s := range allSkills {
			if allowed[s.Slug] {
				filtered = append(filtered, s)
			}
		}
	}

	if len(filtered) == 0 {
		return ""
	}

	var lines []string
	lines = append(lines, "<available_skills>")
	for _, s := range filtered {
		lines = append(lines, "  <skill>")
		lines = append(lines, fmt.Sprintf("    <name>%s</name>", escapeXML(s.Name)))
		lines = append(lines, fmt.Sprintf("    <slug>%s</slug>", escapeXML(s.Slug)))
		desc := s.Description
		if len([]rune(desc)) > skillDescMaxLen {
			desc = string([]rune(desc)[:skillDescMaxLen]) + "…"
		}
		lines = append(lines, fmt.Sprintf("    <description>%s</description>", escapeXML(desc)))
		lines = append(lines, fmt.Sprintf("    <location>%s</location>", escapeXML(s.Path)))
		lines = append(lines, "  </skill>")
	}
	lines = append(lines, "</available_skills>")

	return strings.Join(lines, "\n")
}

// BuildPinnedSummary generates XML summary for only the pinned skill names.
// Delegates to BuildSummary with pinned names as allowlist.
// Returns empty string if none match.
func (l *Loader) BuildPinnedSummary(ctx context.Context, pinnedNames []string) string {
	if len(pinnedNames) == 0 {
		return ""
	}
	return l.BuildSummary(ctx, pinnedNames)
}

// Version returns the current skill snapshot version.
// Consumers compare this to their cached version to detect changes.
func (l *Loader) Version() int64 {
	return l.version.Load()
}

// BumpVersion increments the version counter (called by watcher on changes).
func (l *Loader) BumpVersion() {
	l.version.Store(time.Now().UnixMilli())
}

// Dirs returns all non-empty skill directories (for the watcher to monitor).
func (l *Loader) Dirs() []string {
	var dirs []string
	for _, d := range []string{l.workspaceSkills, l.projectAgentSkills, l.personalAgentSkills, l.globalSkills, l.builtinSkills} {
		if d != "" {
			dirs = append(dirs, d)
		}
	}
	dirs = append(dirs, l.managedSkillsDirs...)
	return dirs
}

// FilterSkills returns skills filtered by an allowlist.
// If allowList is nil, all skills are returned. If empty slice, none are returned.
func (l *Loader) FilterSkills(ctx context.Context, allowList []string) []Info {
	all := l.ListSkills(ctx)
	if allowList == nil {
		return all
	}
	if len(allowList) == 0 {
		return nil
	}
	allowed := make(map[string]bool, len(allowList))
	for _, name := range allowList {
		allowed[name] = true
	}
	var filtered []Info
	for _, s := range all {
		if allowed[s.Slug] {
			filtered = append(filtered, s)
		}
	}
	return filtered
}

// GetSkill returns info about a specific skill.
func (l *Loader) GetSkill(ctx context.Context, name string) (*Info, bool) {
	// Ensure cache is populated
	l.ListSkills(ctx)

	l.mu.RLock()
	defer l.mu.RUnlock()
	for _, key := range []string{name, strings.ToLower(name), normalizeForSearch(name), Slugify(name)} {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		if info, ok := l.cache[key]; ok {
			return info, true
		}
	}
	return nil, false
}

// ActivationPayload is the full skill bundle returned by use_skill activation.
// Inlines SKILL.md content when under SkillMDByteBudget; otherwise returns
// paths-only and sets SkillMDTruncatedReason so the agent reads chunks manually.
// AssetPaths and ScriptPaths are flat — files in `assets/` and `scripts/`
// directly, not recursive. Skills bundling nested directories should list
// them in SKILL.md prose.
type ActivationPayload struct {
	Slug                   string    `json:"slug"`
	Name                   string    `json:"name"`
	Source                 string    `json:"source"`
	BaseDir                string    `json:"base_dir"`
	SkillMDPath            string    `json:"skill_md_path"`
	AssetPaths             []string  `json:"asset_paths,omitempty"`
	ScriptPaths            []string  `json:"script_paths,omitempty"`
	SkillMDContent         string    `json:"skill_md_content,omitempty"`
	SkillMDTruncatedReason string    `json:"skill_md_truncated_reason,omitempty"`
	ActivatedAt            time.Time `json:"activated_at"`
}

// SkillMDByteBudget caps inline SKILL.md content. Byte-based (provider-agnostic)
// to avoid tokenizer drift across GLM / Anthropic / OpenAI.
const SkillMDByteBudget = 200 * 1024

// LoadActivationPayload resolves the skill by slug/name and returns the full
// activation bundle: structured paths + inline SKILL.md when under the byte budget.
// Returns an error wrapping "skill_not_found" when the slug isn't in the loader cache.
// Caller is responsible for grant validation (via SkillAccessStore on managed skills).
func (l *Loader) LoadActivationPayload(ctx context.Context, slug string) (*ActivationPayload, error) {
	info, ok := l.GetSkill(ctx, slug)
	if !ok {
		return nil, fmt.Errorf("skill_not_found: %q", slug)
	}
	return BuildActivationPayload(info.Slug, info.Name, info.Source, info.BaseDir, info.Path)
}

// BuildActivationPayload assembles the activation bundle (structured paths +
// inline SKILL.md under the byte budget) for an already-resolved skill. Callers
// that hold an authoritative path — e.g. use_skill resolving a managed skill
// from the DB to avoid stale duplicate filesystem roots (#218) — use this to
// bypass the loader's global filesystem slug scan.
func BuildActivationPayload(slug, name, source, baseDir, skillMDPath string) (*ActivationPayload, error) {
	payload := &ActivationPayload{
		Slug:        slug,
		Name:        name,
		Source:      source,
		BaseDir:     baseDir,
		SkillMDPath: skillMDPath,
		AssetPaths:  listFilesInSubdir(baseDir, "assets"),
		ScriptPaths: listFilesInSubdir(baseDir, "scripts"),
		ActivatedAt: time.Now().UTC(),
	}

	data, err := os.ReadFile(skillMDPath)
	if err != nil {
		return nil, fmt.Errorf("read SKILL.md: %w", err)
	}
	if len(data) > SkillMDByteBudget {
		payload.SkillMDTruncatedReason = "exceeds_200kb_budget"
	} else {
		payload.SkillMDContent = string(data)
	}
	return payload, nil
}

func listFilesInSubdir(baseDir, sub string) []string {
	dir := filepath.Join(baseDir, sub)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	paths := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		paths = append(paths, filepath.Join(dir, e.Name()))
	}
	if len(paths) == 0 {
		return nil
	}
	return paths
}

// --- Frontmatter parsing ---

var frontmatterRe = regexp.MustCompile(`(?s)^---\n(.*?)\n---\n?`)

func parseMetadata(path string) *Metadata {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}

	fm := extractFrontmatter(string(data))
	if fm == "" {
		return &Metadata{Name: filepath.Base(filepath.Dir(path))}
	}

	// Try JSON first
	var jm Metadata
	if json.Unmarshal([]byte(fm), &jm) == nil && jm.Name != "" {
		return &jm
	}

	// Fall back to simple YAML key: value
	kv := parseSimpleYAML(fm)
	return &Metadata{
		Name:        kv["name"],
		Description: kv["description"],
	}
}

// normalizeLineEndings converts \r\n and bare \r to \n so frontmatter regex matches
// files created on Windows or uploaded via ZIP with CRLF line endings.
func normalizeLineEndings(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	return s
}

func extractFrontmatter(content string) string {
	match := frontmatterRe.FindStringSubmatch(normalizeLineEndings(content))
	if len(match) > 1 {
		return match[1]
	}
	return ""
}

func stripFrontmatter(content string) string {
	return frontmatterRe.ReplaceAllString(normalizeLineEndings(content), "")
}

// parseSimpleYAMLLists parses YAML list fields into separate []string values keyed
// by top-level key. Scalars and block scalars are ignored. Complements
// parseSimpleYAML (which joins list items with spaces). Used by dep_manifest.go.
//
// Supported grammar (subset):
//   - Flat list:           key:\n  - item1\n  - item2
//   - Quoted items:        key:\n  - "value"
//   - CRLF line endings are normalized
//
// Not supported — misuse is logged at debug level and the key is skipped:
//   - Nested maps (key:\n  subkey:\n    - item) — values would lose prefix semantics
//   - Flow-style lists (key: [a, b]) — silently returns empty
//   - Dash without space (-item) — silently returns empty
//
// Example:
//
//	deps:
//	  - pip:psycopg2-binary
//	  - system:ffmpeg
//
// Returns: {"deps": ["pip:psycopg2-binary", "system:ffmpeg"]}
func parseSimpleYAMLLists(content string) map[string][]string {
	result := make(map[string][]string)
	lines := strings.Split(normalizeLineEndings(content), "\n")
	var currentKey string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		// Indented line — could be a list item for currentKey.
		if len(line) > 0 && (line[0] == ' ' || line[0] == '\t') {
			if currentKey == "" {
				continue
			}
			if strings.HasPrefix(trimmed, "- ") {
				val := strings.TrimSpace(trimmed[2:])
				val = strings.Trim(val, "\"'")
				if val != "" {
					result[currentKey] = append(result[currentKey], val)
				}
				continue
			}
			// Indented non-list line under a tracked key — e.g. nested map:
			//   deps:\n  pip:\n    - requests
			// Silent flatten would drop the "pip:" prefix and miscategorize. Skip + clear key.
			if strings.Contains(trimmed, ":") {
				slog.Debug("skills: parseSimpleYAMLLists skipped nested map",
					"key", currentKey, "nested", trimmed)
				delete(result, currentKey)
				currentKey = ""
			}
			continue
		}
		// Top-level key — reset list tracking.
		idx := strings.IndexByte(trimmed, ':')
		if idx < 0 {
			currentKey = ""
			continue
		}
		key := strings.TrimSpace(trimmed[:idx])
		val := strings.TrimSpace(trimmed[idx+1:])
		if val == "" {
			currentKey = key
		} else {
			currentKey = ""
		}
	}
	return result
}

// parseSimpleYAML parses a subset of YAML: simple key: value pairs,
// multiline block scalars (| and >), and list values (- item).
func parseSimpleYAML(content string) map[string]string {
	result := make(map[string]string)
	lines := strings.Split(content, "\n")

	var currentKey string
	var blockLines []string
	var inBlock bool

	flushBlock := func() {
		if currentKey != "" {
			if len(blockLines) > 0 {
				result[currentKey] = strings.Join(blockLines, " ")
			} else {
				// Empty value (e.g. "slug:" with no indented continuation).
				result[currentKey] = ""
			}
		}
		currentKey = ""
		blockLines = nil
		inBlock = false
	}

	for _, line := range lines {
		// Indented continuation line (block scalar or list item)
		if inBlock && len(line) > 0 && (line[0] == ' ' || line[0] == '\t') {
			trimmed := strings.TrimSpace(line)
			if trimmed == "" {
				continue
			}
			// List item: "  - value"
			if strings.HasPrefix(trimmed, "- ") {
				blockLines = append(blockLines, strings.TrimSpace(trimmed[2:]))
			} else if trimmed != "-" {
				blockLines = append(blockLines, trimmed)
			}
			continue
		}

		// Not indented — flush any pending block and parse as top-level key
		flushBlock()

		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		parts := strings.SplitN(trimmed, ":", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])
		val = strings.Trim(val, "\"'")

		if val == "|" || val == ">" || val == "|-" || val == ">-" {
			// Start of a multiline block — collect subsequent indented lines
			currentKey = key
			inBlock = true
			continue
		}
		if val == "" {
			// Could be start of a list block (e.g. "allowed-tools:\n  - Bash")
			currentKey = key
			inBlock = true
			continue
		}
		result[key] = val
	}
	flushBlock()
	return result
}

func escapeXML(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	return s
}
