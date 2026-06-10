package agent

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/nextlevelbuilder/goclaw/internal/bus"
)

// disk name: {YYYYMMDD-HHmmss}_{middle}_{hash8}{ext} where middle is
// "{sender}_{orig-or-kind}", "{orig-or-kind}", or just the kind fallback.
var diskNamePat = regexp.MustCompile(`^(\d{8}-\d{6})_(.+)_([0-9a-f]{8})(\.[a-z0-9]+)$`)

func writeTmpFile(t *testing.T, name, data string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, []byte(data), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	return p
}

// TestPersistMedia_NamingScheme verifies the metadata-rich disk name:
//   - sender (slugified, Vietnamese/CJK aware) when present
//   - original filename slug, or media kind when the name is empty/synthetic
//   - timestamp prefix + content-hash suffix
func TestPersistMedia_NamingScheme(t *testing.T) {
	workspace := t.TempDir()

	type tc struct {
		name       string
		sender     string
		filename   string
		content    string
		mime       string
		wantMiddle string
		wantExtPat string
		wantMime   string
	}

	cases := []tc{
		{
			name: "sender_and_vietnamese_filename", sender: "Nguyễn Nhất Duy",
			filename: "Báo cáo Q4.pdf", content: "%PDF-1.4\nA", mime: "application/pdf",
			wantMiddle: "nguyen-nhat-duy_bao-cao-q4", wantExtPat: `\.pdf`,
		},
		{
			name: "no_sender_uses_original", sender: "",
			filename: "Báo cáo Q4.pdf", content: "%PDF-1.4\nB", mime: "application/pdf",
			wantMiddle: "bao-cao-q4", wantExtPat: `\.pdf`,
		},
		{
			name: "cjk_filename_preserved", sender: "",
			filename: "猫の写真.png", content: "cjk-bytes", mime: "application/octet-stream",
			wantMiddle: "猫の写真", wantExtPat: `\.bin`,
		},
		{
			name: "empty_filename_falls_back_to_kind", sender: "",
			filename: "", content: "audio-bytes", mime: "audio/ogg",
			wantMiddle: "audio", wantExtPat: `\.ogg`,
		},
		{
			name: "synthetic_goclaw_name_falls_back_to_kind", sender: "",
			filename: "goclaw_zca_1104016324.jpg", content: "synthetic-bytes", mime: "application/octet-stream",
			wantMiddle: "document", wantExtPat: `\.bin`,
		},
		{
			name: "all_unsafe_filename_falls_back_to_kind", sender: "",
			filename: "///", content: "unsafe-bytes", mime: "application/pdf",
			wantMiddle: "document", wantExtPat: `\.pdf`,
		},
	}

	var loop Loop
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			src := writeTmpFile(t, "src.bin", c.content)
			refs := loop.persistMedia("session-key-test", []bus.MediaFile{{
				Path:     src,
				MimeType: c.mime,
				Filename: c.filename,
			}}, workspace, c.sender)
			if len(refs) != 1 {
				t.Fatalf("got %d refs, want 1", len(refs))
			}
			if c.wantMime != "" && refs[0].MimeType != c.wantMime {
				t.Fatalf("mime = %q, want %q", refs[0].MimeType, c.wantMime)
			}
			base := filepath.Base(refs[0].Path)
			m := diskNamePat.FindStringSubmatch(base)
			if m == nil {
				t.Fatalf("disk name %q does not match scheme {ts}_{middle}_{hash8}{ext}", base)
			}
			if m[2] != c.wantMiddle {
				t.Fatalf("middle = %q, want %q (full: %q)", m[2], c.wantMiddle, base)
			}
			if !regexp.MustCompile(c.wantExtPat + `$`).MatchString(base) {
				t.Fatalf("disk name %q does not match ext pattern %q", base, c.wantExtPat)
			}
		})
	}
}

// TestPersistMedia_DedupByContent verifies identical content persists once:
// a second upload of the same bytes reuses the existing file.
func TestPersistMedia_DedupByContent(t *testing.T) {
	workspace := t.TempDir()
	var loop Loop

	persist := func(filename string) string {
		src := writeTmpFile(t, "src.bin", "the same exact bytes")
		refs := loop.persistMedia("s", []bus.MediaFile{{
			Path: src, MimeType: "application/pdf", Filename: filename,
		}}, workspace, "Sender One")
		if len(refs) != 1 {
			t.Fatalf("got %d refs, want 1", len(refs))
		}
		return refs[0].Path
	}

	first := persist("report-a.pdf")
	second := persist("report-b.pdf") // different name, same content
	if first != second {
		t.Fatalf("expected dedup to reuse the same file, got %q vs %q", first, second)
	}
	uploads := filepath.Join(workspace, ".uploads")
	files, _ := os.ReadDir(uploads)
	if len(files) != 1 {
		t.Fatalf("expected 1 file in .uploads after dedup, got %d", len(files))
	}
}
