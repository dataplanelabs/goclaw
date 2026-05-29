package agent

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/bootstrap"
	"github.com/nextlevelbuilder/goclaw/internal/store"
)

// fakeConfigPermStore implements the full store.ConfigPermissionStore interface.
// ListFileWriters supplies the exact-scope roster (controls whether the gate
// engages); CheckPermission drives the per-sender writer decision; effective
// supplies the wildcard-aware roster (defaults to writers).
type fakeConfigPermStore struct {
	writers   []store.ConfigPermission
	effective []store.ConfigPermission
	allow     bool
	permErr   error
}

func (f *fakeConfigPermStore) CheckPermission(_ context.Context, _ uuid.UUID, _, _, _ string) (bool, error) {
	return f.allow, f.permErr
}
func (f *fakeConfigPermStore) Grant(_ context.Context, _ *store.ConfigPermission) error { return nil }
func (f *fakeConfigPermStore) Revoke(_ context.Context, _ uuid.UUID, _, _, _ string) error {
	return nil
}
func (f *fakeConfigPermStore) List(_ context.Context, _ uuid.UUID, _, _ string) ([]store.ConfigPermission, error) {
	return nil, nil
}
func (f *fakeConfigPermStore) ListFileWriters(_ context.Context, _ uuid.UUID, _ string) ([]store.ConfigPermission, error) {
	return f.writers, nil
}
func (f *fakeConfigPermStore) ListEffectiveFileWriters(_ context.Context, _ uuid.UUID, _ string) ([]store.ConfigPermission, error) {
	if f.effective != nil {
		return f.effective, nil
	}
	return f.writers, nil
}

// Synthetic fixtures — no real users, ids, or chat content.
const testGroupID = "group:testchan:900000000000000001"
const testSender = "100000000000000001"  // the sender under test
const otherUserID = "999999999999999999" // a non-writer

func oneWriterRoster() []store.ConfigPermission {
	return []store.ConfigPermission{{UserID: "200000000000000002", Metadata: json.RawMessage(`{"displayName":"Writer One"}`)}}
}

func newWriterLoop(f *fakeConfigPermStore) *Loop {
	return &Loop{configPermStore: f, agentUUID: uuid.New()}
}

// Wildcard grantee (file_writer or * @ group:*) is NOT in the exact-scope roster
// but CheckPermission allows → must be treated as a writer, and appended to the
// displayed roster for self-consistency.
func TestBuildGroupWriterPrompt_WildcardGranteeIsWriter(t *testing.T) {
	l := newWriterLoop(&fakeConfigPermStore{writers: oneWriterRoster(), allow: true})
	prompt, _ := l.buildGroupWriterPrompt(context.Background(), testGroupID, testSender, nil)

	if !strings.Contains(prompt, "IS A FILE WRITER") {
		t.Fatalf("wildcard grantee should be a writer; got:\n%s", prompt)
	}
	if strings.Contains(prompt, "IS NOT A FILE WRITER") {
		t.Errorf("must not inject refusal for a wildcard grantee")
	}
	if !strings.Contains(prompt, testSender) {
		t.Errorf("affirmative hint should reference the sender id; got:\n%s", prompt)
	}
}

func TestBuildGroupWriterPrompt_ExactScopeWriterStillRecognized(t *testing.T) {
	// Sender IS in the exact roster AND CheckPermission allows (regression).
	writers := []store.ConfigPermission{{UserID: testSender, Metadata: json.RawMessage(`{"displayName":"Sender Label"}`)}}
	l := newWriterLoop(&fakeConfigPermStore{writers: writers, allow: true})
	prompt, _ := l.buildGroupWriterPrompt(context.Background(), testGroupID, testSender, nil)
	if !strings.Contains(prompt, "IS A FILE WRITER") || !strings.Contains(prompt, "Sender Label") {
		t.Fatalf("exact-scope writer should be recognized with label; got:\n%s", prompt)
	}
}

func TestBuildGroupWriterPrompt_NonWriterRefusedAndFilesFiltered(t *testing.T) {
	l := newWriterLoop(&fakeConfigPermStore{writers: oneWriterRoster(), allow: false})
	files := []bootstrap.ContextFile{
		{Path: bootstrap.SoulFile, Content: "soul"},
		{Path: bootstrap.AgentsFile, Content: "agents"},
		{Path: "NOTES.md", Content: "notes"},
	}
	prompt, out := l.buildGroupWriterPrompt(context.Background(), testGroupID, testSender, files)

	if !strings.Contains(prompt, "IS NOT A FILE WRITER") {
		t.Fatalf("non-writer should get the refusal block; got:\n%s", prompt)
	}
	for _, f := range out {
		if f.Path == bootstrap.SoulFile || f.Path == bootstrap.AgentsFile {
			t.Errorf("SOUL.md/AGENTS.md must be filtered for non-writers; got %q", f.Path)
		}
	}
	if len(out) != 1 || out[0].Path != "NOTES.md" {
		t.Errorf("non-identity files must be preserved; got %+v", out)
	}
}

func TestBuildGroupWriterPrompt_NoWritersFailsOpen(t *testing.T) {
	// Group has zero writer rows → no gate at all (unrestricted), regardless of CheckPermission.
	l := newWriterLoop(&fakeConfigPermStore{writers: nil, allow: false})
	files := []bootstrap.ContextFile{{Path: bootstrap.SoulFile, Content: "soul"}}
	prompt, out := l.buildGroupWriterPrompt(context.Background(), testGroupID, testSender, files)
	if prompt != "" {
		t.Errorf("unrestricted group should produce no prompt section; got:\n%s", prompt)
	}
	if len(out) != 1 {
		t.Errorf("files must be untouched when unrestricted; got %+v", out)
	}
}

func TestBuildGroupWriterPrompt_CheckPermissionErrorFailsOpen(t *testing.T) {
	// Restricted group, but CheckPermission errors → must NOT inject a false
	// refusal; skip the gate this turn and leave files untouched.
	l := newWriterLoop(&fakeConfigPermStore{writers: oneWriterRoster(), allow: false, permErr: errors.New("store unavailable")})
	files := []bootstrap.ContextFile{{Path: bootstrap.SoulFile, Content: "soul"}}
	prompt, out := l.buildGroupWriterPrompt(context.Background(), testGroupID, testSender, files)
	if prompt != "" {
		t.Errorf("CheckPermission error should skip the gate (fail-open), not inject refusal; got:\n%s", prompt)
	}
	if len(out) != 1 {
		t.Errorf("files must be untouched on fail-open; got %+v", out)
	}
}

// Phase 2: the displayed roster lists wildcard grantees (from ListEffectiveFileWriters),
// not just exact-scope writers — even when they aren't the current sender.
func TestBuildGroupWriterPrompt_RosterIncludesWildcardGrantees(t *testing.T) {
	exact := oneWriterRoster()
	effective := []store.ConfigPermission{
		exact[0],
		{UserID: testSender, Metadata: json.RawMessage(`{"displayName":"Writer Two"}`)}, // wildcard grantee
	}
	// Current sender is someone else and not a writer.
	l := newWriterLoop(&fakeConfigPermStore{writers: exact, effective: effective, allow: false})
	prompt, _ := l.buildGroupWriterPrompt(context.Background(), testGroupID, otherUserID, nil)

	if !strings.Contains(prompt, "Writer One") || !strings.Contains(prompt, "Writer Two") {
		t.Fatalf("roster should list exact + wildcard grantees; got:\n%s", prompt)
	}
	if !strings.Contains(prompt, "IS NOT A FILE WRITER") {
		t.Errorf("non-writer sender should still be refused")
	}
}

func TestBuildGroupWriterPrompt_RosterDedupesAcrossLists(t *testing.T) {
	exact := oneWriterRoster()
	effective := []store.ConfigPermission{exact[0], exact[0]} // duplicate of the exact writer
	l := newWriterLoop(&fakeConfigPermStore{writers: exact, effective: effective, allow: true})
	prompt, _ := l.buildGroupWriterPrompt(context.Background(), testGroupID, testSender, nil)
	if n := strings.Count(prompt, "Writer One"); n != 1 {
		t.Errorf("writer should appear once in roster, got %d:\n%s", n, prompt)
	}
}

func TestBuildGroupWriterPrompt_SystemInitiatedUnchanged(t *testing.T) {
	// senderID == "" → system-initiated branch (no per-sender gating).
	l := newWriterLoop(&fakeConfigPermStore{writers: oneWriterRoster(), allow: false})
	prompt, _ := l.buildGroupWriterPrompt(context.Background(), testGroupID, "", nil)
	if !strings.Contains(strings.ToLower(prompt), "system-initiated") {
		t.Fatalf("empty sender should hit the system-initiated branch; got:\n%s", prompt)
	}
	if strings.Contains(prompt, "IS NOT A FILE WRITER") {
		t.Errorf("system-initiated run must not be refused")
	}
}
