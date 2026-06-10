package cmd

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"
)

func TestRunSkillsShowHTTPPrintsDBBackedContent(t *testing.T) {
	oldGet := skillsGatewayHTTPGet
	t.Cleanup(func() { skillsGatewayHTTPGet = oldGet })

	var paths []string
	skillsGatewayHTTPGet = func(path string) (map[string]any, error) {
		paths = append(paths, path)
		switch path {
		case "/v1/skills":
			return map[string]any{
				"skills": []map[string]any{{
					"id":          "11111111-1111-1111-1111-111111111111",
					"slug":        "gh-read",
					"name":        "GitHub Reader",
					"description": "Read GitHub data",
					"source":      "gcplane",
					"version":     "3",
					"visibility":  "private",
				}},
			}, nil
		case "/v1/skills/11111111-1111-1111-1111-111111111111/files/SKILL.md":
			return map[string]any{
				"content": "---\nname: GitHub Reader\nslug: gh-read\n---\nUse GitHub safely.\n",
			}, nil
		default:
			t.Fatalf("unexpected path %q", path)
		}
		return nil, nil
	}

	var ok bool
	out := captureStdout(t, func() {
		ok = runSkillsShowHTTP("gh-read")
	})
	if !ok {
		t.Fatal("runSkillsShowHTTP returned false")
	}

	if len(paths) != 2 {
		t.Fatalf("paths = %v, want list + content fetch", paths)
	}
	if !strings.Contains(out, "Slug:        gh-read") {
		t.Fatalf("output missing skill metadata:\n%s", out)
	}
	if !strings.Contains(out, "--- Content ---\nUse GitHub safely.") {
		t.Fatalf("output missing stripped content:\n%s", out)
	}
	if strings.Contains(out, "slug: gh-read") {
		t.Fatalf("output should strip frontmatter:\n%s", out)
	}
}

func TestHTTPSkillMatchesSlugOrName(t *testing.T) {
	skill := map[string]any{"slug": "gh-read", "name": "GitHub Reader"}
	if !httpSkillMatches(skill, "gh-read") {
		t.Fatal("expected slug match")
	}
	if !httpSkillMatches(skill, "GitHub Reader") {
		t.Fatal("expected name match")
	}
	if httpSkillMatches(skill, "missing") {
		t.Fatal("unexpected match")
	}
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()

	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe stdout: %v", err)
	}
	os.Stdout = w

	fn()

	if err := w.Close(); err != nil {
		t.Fatalf("close stdout writer: %v", err)
	}
	os.Stdout = old

	var buf bytes.Buffer
	if _, err := io.Copy(&buf, r); err != nil {
		t.Fatalf("read stdout: %v", err)
	}
	if err := r.Close(); err != nil {
		t.Fatalf("close stdout reader: %v", err)
	}
	return buf.String()
}
