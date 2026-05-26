package skills

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestSkillContext_ActivateAndQuery(t *testing.T) {
	sc := NewSkillContext()
	cached := sc.Activate(&ActivatedSkill{Slug: "design-annhien", BaseDir: "/a/b"})
	if cached {
		t.Errorf("first activation should not be cached")
	}
	if !sc.IsActivated("design-annhien") {
		t.Errorf("expected design-annhien to be activated")
	}
	if sc.IsActivated("missing") {
		t.Errorf("missing should not be activated")
	}
	prefixes := sc.AllowedPrefixes()
	if len(prefixes) != 1 || prefixes[0] != "/a/b" {
		t.Errorf("AllowedPrefixes: got %v, want [/a/b]", prefixes)
	}
}

func TestSkillContext_Idempotent(t *testing.T) {
	sc := NewSkillContext()
	first := &ActivatedSkill{Slug: "x", BaseDir: "/a", ActivatedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
	sc.Activate(first)
	second := &ActivatedSkill{Slug: "x", BaseDir: "/a"}
	cached := sc.Activate(second)
	if !cached {
		t.Errorf("re-activating same slug should report cached=true")
	}
	if !second.ActivatedAt.Equal(first.ActivatedAt) {
		t.Errorf("re-activation should inherit original ActivatedAt; got %v want %v", second.ActivatedAt, first.ActivatedAt)
	}
	if len(sc.AllowedPrefixes()) != 1 {
		t.Errorf("idempotent activation should not duplicate prefix")
	}
}

func TestSkillContext_MultipleSkills(t *testing.T) {
	sc := NewSkillContext()
	sc.Activate(&ActivatedSkill{Slug: "a", BaseDir: "/x"})
	sc.Activate(&ActivatedSkill{Slug: "b", BaseDir: "/y"})
	prefixes := sc.AllowedPrefixes()
	if len(prefixes) != 2 {
		t.Fatalf("got %d prefixes, want 2", len(prefixes))
	}
	got := map[string]bool{}
	for _, p := range prefixes {
		got[p] = true
	}
	if !got["/x"] || !got["/y"] {
		t.Errorf("missing prefix: %v", prefixes)
	}
}

func TestSkillContext_NilSafe(t *testing.T) {
	var sc *SkillContext
	if sc.IsActivated("x") {
		t.Errorf("nil receiver should report false")
	}
	if got := sc.AllowedPrefixes(); got != nil {
		t.Errorf("nil receiver should return nil prefixes, got %v", got)
	}
	if cached := sc.Activate(&ActivatedSkill{Slug: "x"}); cached {
		t.Errorf("nil receiver Activate should noop and return false")
	}
}

func TestSkillContext_EmptyOrNilSkillRejected(t *testing.T) {
	sc := NewSkillContext()
	if cached := sc.Activate(nil); cached {
		t.Errorf("nil skill should noop")
	}
	if cached := sc.Activate(&ActivatedSkill{Slug: ""}); cached {
		t.Errorf("empty slug should noop")
	}
	if len(sc.AllowedPrefixes()) != 0 {
		t.Errorf("expected no prefixes after rejected activations")
	}
}

func TestSkillContext_ConcurrentSafe(t *testing.T) {
	sc := NewSkillContext()
	var wg sync.WaitGroup
	const goroutines = 50
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			slug := "skill-" + string(rune('a'+n%26))
			sc.Activate(&ActivatedSkill{Slug: slug, BaseDir: "/dir/" + slug})
			_ = sc.IsActivated(slug)
			_ = sc.AllowedPrefixes()
		}(i)
	}
	wg.Wait()
	if len(sc.AllowedPrefixes()) == 0 {
		t.Errorf("expected at least one activation after concurrent run")
	}
}

func TestSkillContext_FromContext(t *testing.T) {
	ctx := context.Background()
	if SkillContextFromContext(ctx) != nil {
		t.Errorf("bare context should return nil SkillContext")
	}
	if SkillAllowedPrefixesFromContext(ctx) != nil {
		t.Errorf("bare context should return nil prefixes")
	}
	sc := NewSkillContext()
	sc.Activate(&ActivatedSkill{Slug: "x", BaseDir: "/p"})
	ctx2 := WithSkillContext(ctx, sc)
	if SkillContextFromContext(ctx2) != sc {
		t.Errorf("ctx should return attached SkillContext")
	}
	prefixes := SkillAllowedPrefixesFromContext(ctx2)
	if len(prefixes) != 1 || prefixes[0] != "/p" {
		t.Errorf("ctx prefixes: got %v want [/p]", prefixes)
	}
}

func TestActivatedSkillFromPayload(t *testing.T) {
	p := &ActivationPayload{
		Slug:       "design-annhien",
		Source:     "managed",
		BaseDir:    "/skills/design-annhien/1",
		AssetPaths: []string{"/skills/design-annhien/1/assets/logo.jpg"},
	}
	a := ActivatedSkillFromPayload(p)
	if a.Slug != p.Slug || a.BaseDir != p.BaseDir || a.Source != p.Source {
		t.Errorf("payload→activated mismatch: %+v", a)
	}
	if len(a.AssetPaths) != 1 || a.AssetPaths[0] != p.AssetPaths[0] {
		t.Errorf("AssetPaths not copied correctly: %v", a.AssetPaths)
	}
	// Defensive copy — mutating source must not affect target.
	p.AssetPaths[0] = "MUTATED"
	if a.AssetPaths[0] == "MUTATED" {
		t.Errorf("AssetPaths slice shared with payload (not defensively copied)")
	}
	if ActivatedSkillFromPayload(nil) != nil {
		t.Errorf("nil payload should return nil")
	}
}
