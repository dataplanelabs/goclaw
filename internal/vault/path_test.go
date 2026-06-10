package vault

import "testing"

func TestStripTenantPrefix(t *testing.T) {
	cases := map[string]string{
		"tenants/shtp/a/b.md":          "a/b.md",
		"tenants/shtp/teams/u/x.md":    "teams/u/x.md",
		"tenants/shtp/y":               "y",
		"tenants/shtp":                 "tenants/shtp", // single segment, no file part → unchanged
		"agents/bot/file.md":           "agents/bot/file.md",
		"telegram/group/123/report.md": "telegram/group/123/report.md",
		"README.md":                    "README.md",
		"":                             "",
	}
	for in, want := range cases {
		if got := StripTenantPrefix(in); got != want {
			t.Errorf("StripTenantPrefix(%q) = %q, want %q", in, got, want)
		}
	}
}
