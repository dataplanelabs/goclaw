package skills

import (
	"context"
	"sync"
	"time"
)

// SkillContext holds the set of skills activated during one agent run.
// Attached to ctx at Loop.Run entry; consumed by filesystem tools to merge
// each activated skill's directory into their allow-list. Lives for the
// duration of one inbound message; not persisted across sessions.
type SkillContext struct {
	mu        sync.RWMutex
	activated map[string]*ActivatedSkill
}

// ActivatedSkill records one skill's session-scoped state — what
// directories the agent may now read from filesystem tools, when it was
// activated, and which source it came from.
type ActivatedSkill struct {
	Slug        string
	Source      string
	BaseDir     string
	AssetPaths  []string
	ScriptPaths []string
	ActivatedAt time.Time
}

// NewSkillContext returns an empty session context.
func NewSkillContext() *SkillContext {
	return &SkillContext{activated: make(map[string]*ActivatedSkill)}
}

// Activate records a skill as available for filesystem tool calls in this
// session. Idempotent on slug — re-activating returns true so the caller
// can detect cache hits for metrics.
func (sc *SkillContext) Activate(s *ActivatedSkill) (cached bool) {
	if sc == nil || s == nil || s.Slug == "" {
		return false
	}
	sc.mu.Lock()
	defer sc.mu.Unlock()
	if existing, ok := sc.activated[s.Slug]; ok {
		s.ActivatedAt = existing.ActivatedAt
		return true
	}
	if s.ActivatedAt.IsZero() {
		s.ActivatedAt = time.Now().UTC()
	}
	sc.activated[s.Slug] = s
	return false
}

// IsActivated reports whether the given slug has been activated in this session.
func (sc *SkillContext) IsActivated(slug string) bool {
	if sc == nil {
		return false
	}
	sc.mu.RLock()
	defer sc.mu.RUnlock()
	_, ok := sc.activated[slug]
	return ok
}

// AllowedPrefixes returns a defensive copy of every activated skill's BaseDir.
// Filesystem tools merge this into their resolvePath allow-list so they accept
// paths under any activated skill's directory.
func (sc *SkillContext) AllowedPrefixes() []string {
	if sc == nil {
		return nil
	}
	sc.mu.RLock()
	defer sc.mu.RUnlock()
	if len(sc.activated) == 0 {
		return nil
	}
	out := make([]string, 0, len(sc.activated))
	for _, s := range sc.activated {
		if s.BaseDir != "" {
			out = append(out, s.BaseDir)
		}
	}
	return out
}

// Snapshot returns a copy of every activated skill record (slug-keyed).
// Used for tracing / debugging — never mutate the result.
func (sc *SkillContext) Snapshot() map[string]ActivatedSkill {
	if sc == nil {
		return nil
	}
	sc.mu.RLock()
	defer sc.mu.RUnlock()
	out := make(map[string]ActivatedSkill, len(sc.activated))
	for k, v := range sc.activated {
		out[k] = *v
	}
	return out
}

// ActivatedSkillFromPayload converts an ActivationPayload (returned by
// use_skill) into the lighter ActivatedSkill record stored in session context.
func ActivatedSkillFromPayload(p *ActivationPayload) *ActivatedSkill {
	if p == nil {
		return nil
	}
	return &ActivatedSkill{
		Slug:        p.Slug,
		Source:      p.Source,
		BaseDir:     p.BaseDir,
		AssetPaths:  append([]string(nil), p.AssetPaths...),
		ScriptPaths: append([]string(nil), p.ScriptPaths...),
		ActivatedAt: p.ActivatedAt,
	}
}

type skillContextKey struct{}

// WithSkillContext attaches a SkillContext to ctx. Call once per inbound
// message at Loop.Run entry so all downstream tool handlers inherit it.
func WithSkillContext(ctx context.Context, sc *SkillContext) context.Context {
	return context.WithValue(ctx, skillContextKey{}, sc)
}

// SkillContextFromContext returns the session SkillContext, or nil when none
// was attached (e.g. CLI / one-shot runs). Filesystem tools must treat nil as
// "no activated skills" and fall back to their static allow-list.
func SkillContextFromContext(ctx context.Context) *SkillContext {
	if ctx == nil {
		return nil
	}
	if sc, ok := ctx.Value(skillContextKey{}).(*SkillContext); ok {
		return sc
	}
	return nil
}

// SkillAllowedPrefixesFromContext is a convenience for filesystem tools:
// returns the snapshot of all activated skills' BaseDirs, or nil if no
// SkillContext is attached.
func SkillAllowedPrefixesFromContext(ctx context.Context) []string {
	return SkillContextFromContext(ctx).AllowedPrefixes()
}
