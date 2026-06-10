package refstore

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func newTestStore(t *testing.T) (*Store, string) {
	t.Helper()
	dir := t.TempDir()
	s, err := New(filepath.Join(dir, "vieneu-refs"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return s, dir
}

func TestSaveAndPathFor(t *testing.T) {
	s, _ := newTestStore(t)
	tenant := uuid.New()
	voiceID := uuid.New().String()
	path, err := s.Save(tenant, voiceID, bytes.NewReader([]byte("RIFF\x00\x00\x00\x00WAVE")))
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	if !strings.HasSuffix(path, voiceID+".wav") {
		t.Errorf("path = %q", path)
	}
	if !strings.Contains(path, tenant.String()) {
		t.Errorf("path %q missing tenant subdir", path)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("readback: %v", err)
	}
	if string(got) != "RIFF\x00\x00\x00\x00WAVE" {
		t.Errorf("content mismatch: %q", got)
	}
}

func TestPathFor_RejectsTraversal(t *testing.T) {
	s, _ := newTestStore(t)
	tenant := uuid.New()
	for _, bad := range []string{"", "..", ".", "../escape", "a/b", "a\\b"} {
		if _, err := s.PathFor(tenant, bad); !errors.Is(err, ErrInvalidVoiceID) {
			t.Errorf("PathFor(%q) → %v, want ErrInvalidVoiceID", bad, err)
		}
	}
}

func TestPathFor_RejectsZeroTenant(t *testing.T) {
	s, _ := newTestStore(t)
	if _, err := s.PathFor(uuid.Nil, "voice"); !errors.Is(err, ErrInvalidTenant) {
		t.Errorf("err = %v, want ErrInvalidTenant", err)
	}
}

func TestSave_TenantScoping(t *testing.T) {
	s, _ := newTestStore(t)
	a := uuid.New()
	b := uuid.New()
	voiceID := uuid.New().String()
	pa, _ := s.Save(a, voiceID, bytes.NewReader([]byte("A")))
	pb, _ := s.Save(b, voiceID, bytes.NewReader([]byte("B")))
	if pa == pb {
		t.Error("same path across tenants")
	}
	contentA, _ := os.ReadFile(pa)
	contentB, _ := os.ReadFile(pb)
	if string(contentA) != "A" || string(contentB) != "B" {
		t.Errorf("contents leaked: A=%q B=%q", contentA, contentB)
	}
}

func TestExistsAndDelete(t *testing.T) {
	s, _ := newTestStore(t)
	tenant := uuid.New()
	voiceID := uuid.New().String()
	if s.Exists(tenant, voiceID) {
		t.Error("Exists true before save")
	}
	if _, err := s.Save(tenant, voiceID, bytes.NewReader([]byte("x"))); err != nil {
		t.Fatal(err)
	}
	if !s.Exists(tenant, voiceID) {
		t.Error("Exists false after save")
	}
	if err := s.Delete(tenant, voiceID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if s.Exists(tenant, voiceID) {
		t.Error("Exists true after delete")
	}
	// Delete-missing is a no-op.
	if err := s.Delete(tenant, voiceID); err != nil {
		t.Errorf("delete-missing: %v", err)
	}
}

func TestNew_EmptyBaseDirRejected(t *testing.T) {
	if _, err := New(""); err == nil {
		t.Error("empty baseDir should fail")
	}
}
