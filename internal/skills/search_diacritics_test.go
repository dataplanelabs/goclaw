package skills

import (
	"testing"
)

func TestFoldDiacritics(t *testing.T) {
	cases := []struct{ in, want string }{
		{"Tạo", "Tao"},
		{"An Nhiên", "An Nhien"},
		{"Thiết kế", "Thiet ke"},
		{"café", "cafe"},
		{"plain", "plain"},
		{"", ""},
		{"sài gòn", "sai gon"},
	}
	for _, c := range cases {
		got := FoldDiacritics(c.in)
		if got != c.want {
			t.Errorf("FoldDiacritics(%q): got %q, want %q", c.in, got, c.want)
		}
	}
}

func TestTokenize_DiacriticsFolded(t *testing.T) {
	tokens := tokenize("Tạo poster cho An Nhiên Safety")
	got := map[string]bool{}
	for _, tk := range tokens {
		got[tk] = true
	}
	want := []string{"tao", "poster", "cho", "an", "nhien", "safety"}
	for _, w := range want {
		if !got[w] {
			t.Errorf("tokenize missing %q; got tokens=%v", w, tokens)
		}
	}
}

func TestTokenizeSlug_WholeAndComponents(t *testing.T) {
	got := tokenizeSlug("design-annhien")
	want := map[string]bool{"design-annhien": false, "design": false, "annhien": false}
	for _, tk := range got {
		if _, ok := want[tk]; ok {
			want[tk] = true
		}
	}
	for tk, found := range want {
		if !found {
			t.Errorf("tokenizeSlug missing %q; got %v", tk, got)
		}
	}
}

// TestSearch_VietnameseQueryMatchesDiacriticDoc — the failure mode from trace
// 019e62ff: query "design poster annhien" (no diacritics) should find a skill
// whose description is "Tạo... An Nhiên Safety..." (with diacritics).
func TestSearch_VietnameseQueryMatchesDiacriticDoc(t *testing.T) {
	idx := NewIndex()
	idx.Build([]Info{
		{
			Name: "Thiết kế An Nhiên", Slug: "design-annhien",
			Description: "Tạo mọi loại ấn phẩm thiết kế cho An Nhiên Safety — poster, banner, tờ rơi.",
			Path:        "/skills/design-annhien/1/SKILL.md", BaseDir: "/skills/design-annhien/1", Source: "managed",
		},
		{
			Name: "Sales of Day", Slug: "sales-of-day",
			Description: "Báo cáo doanh số cuối ngày",
			Path:        "/skills/sales-of-day/1/SKILL.md", BaseDir: "/skills/sales-of-day/1", Source: "managed",
		},
	})

	cases := []struct {
		name, query, wantTopSlug string
	}{
		{"romanized_query", "design poster annhien", "design-annhien"},
		{"vietnamese_query", "thiết kế poster an nhiên", "design-annhien"},
		{"vietnamese_no_diacritics", "thiet ke poster an nhien", "design-annhien"},
		{"exact_slug", "design-annhien", "design-annhien"},
		{"single_term_slug_match", "annhien", "design-annhien"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			results := idx.Search(c.query, 3)
			if len(results) == 0 {
				t.Fatalf("expected results for query %q, got none", c.query)
			}
			if results[0].Slug != c.wantTopSlug {
				t.Errorf("query %q: top=%q want %q; full results: %+v",
					c.query, results[0].Slug, c.wantTopSlug, results)
			}
		})
	}
}

// TestSearch_SlugExactBonus_OutranksDescriptionMatch — even when another skill
// has the query keywords in its description, the slug-exact match wins.
func TestSearch_SlugExactBonus_OutranksDescriptionMatch(t *testing.T) {
	idx := NewIndex()
	idx.Build([]Info{
		{
			Name: "Hire", Slug: "hire-process",
			Description: "Workflow for design poster reviews and annhien onboarding tasks",
			Path:        "/a/SKILL.md", Source: "managed",
		},
		{
			Name: "Design Annhien", Slug: "design-annhien",
			Description: "Posters for the brand",
			Path:        "/b/SKILL.md", Source: "managed",
		},
	})
	results := idx.Search("design poster annhien", 3)
	if len(results) == 0 {
		t.Fatalf("expected results")
	}
	if results[0].Slug != "design-annhien" {
		t.Errorf("slug-exact match should outrank description coincidence; got top=%q", results[0].Slug)
	}
}
