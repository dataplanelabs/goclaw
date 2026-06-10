// Package refstore is a tenant-scoped file store for VieNeu voice-cloning
// reference WAVs. Layout: <baseDir>/{tenant_id}/{voice_id}.wav.
package refstore

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
)

var (
	ErrInvalidVoiceID = errors.New("refstore: voice_id must be a non-empty filename without separators")
	ErrInvalidTenant  = errors.New("refstore: tenant_id is zero")
)

type Store struct {
	baseDir string
}

// New creates a Store and ensures baseDir exists. Path-traversal guards are
// enforced per-call, not at construction.
func New(baseDir string) (*Store, error) {
	if baseDir == "" {
		return nil, fmt.Errorf("refstore: baseDir is empty")
	}
	abs, err := filepath.Abs(baseDir)
	if err != nil {
		return nil, fmt.Errorf("refstore: resolve baseDir: %w", err)
	}
	if err := os.MkdirAll(abs, 0o755); err != nil {
		return nil, fmt.Errorf("refstore: mkdir baseDir: %w", err)
	}
	return &Store{baseDir: abs}, nil
}

func (s *Store) BaseDir() string { return s.baseDir }

func validateIDs(tenantID uuid.UUID, voiceID string) error {
	if tenantID == uuid.Nil {
		return ErrInvalidTenant
	}
	if voiceID == "" || strings.ContainsAny(voiceID, "/\\") || voiceID == "." || voiceID == ".." {
		return ErrInvalidVoiceID
	}
	return nil
}

func (s *Store) PathFor(tenantID uuid.UUID, voiceID string) (string, error) {
	if err := validateIDs(tenantID, voiceID); err != nil {
		return "", err
	}
	return filepath.Join(s.baseDir, tenantID.String(), voiceID+".wav"), nil
}

func (s *Store) Save(tenantID uuid.UUID, voiceID string, src io.Reader) (string, error) {
	dst, err := s.PathFor(tenantID, voiceID)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return "", fmt.Errorf("refstore: mkdir tenant dir: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(dst), "."+voiceID+".*.tmp")
	if err != nil {
		return "", fmt.Errorf("refstore: open tmp: %w", err)
	}
	tmpPath := tmp.Name()
	cleanup := func() { _ = os.Remove(tmpPath) }
	if _, err := io.Copy(tmp, src); err != nil {
		_ = tmp.Close()
		cleanup()
		return "", fmt.Errorf("refstore: write: %w", err)
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return "", fmt.Errorf("refstore: close: %w", err)
	}
	if err := os.Rename(tmpPath, dst); err != nil {
		cleanup()
		return "", fmt.Errorf("refstore: rename: %w", err)
	}
	return dst, nil
}

func (s *Store) Exists(tenantID uuid.UUID, voiceID string) bool {
	p, err := s.PathFor(tenantID, voiceID)
	if err != nil {
		return false
	}
	_, err = os.Stat(p)
	return err == nil
}

func (s *Store) Delete(tenantID uuid.UUID, voiceID string) error {
	p, err := s.PathFor(tenantID, voiceID)
	if err != nil {
		return err
	}
	if err := os.Remove(p); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("refstore: delete: %w", err)
	}
	return nil
}
