package oa

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/store"
)

// flakyStore fails the first N Update calls then succeeds. Other interface
// methods are no-ops (the retry path only touches Update via Persist).
type flakyStore struct {
	store.ChannelInstanceStore
	failsRemaining int32
	calls          int32
}

func (s *flakyStore) Update(ctx context.Context, id uuid.UUID, updates map[string]any) error {
	atomic.AddInt32(&s.calls, 1)
	if atomic.AddInt32(&s.failsRemaining, -1) >= 0 {
		return errors.New("simulated DB blip")
	}
	return nil
}

func TestPersistWithRetry_SucceedsOnSecondAttempt(t *testing.T) {
	s := &flakyStore{failsRemaining: 1}
	creds := &ChannelCreds{AppID: "x", SecretKey: "y", AccessToken: "AT", RefreshToken: "RT"}

	start := time.Now()
	err := persistWithRetry(context.Background(), s, uuid.New(), creds)
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("expected success after 1 retry, got %v", err)
	}
	if got := atomic.LoadInt32(&s.calls); got != 2 {
		t.Errorf("expected 2 calls (1 fail + 1 success), got %d", got)
	}
	// Backoff #2 is 2s; allow a small slack.
	if elapsed < 1900*time.Millisecond || elapsed > 3*time.Second {
		t.Errorf("expected ~2s elapsed for one backoff, got %v", elapsed)
	}
}

func TestPersistWithRetry_GivesUpAfterAllAttempts(t *testing.T) {
	s := &flakyStore{failsRemaining: 10}
	creds := &ChannelCreds{AppID: "x", SecretKey: "y"}

	err := persistWithRetry(context.Background(), s, uuid.New(), creds)
	if err == nil {
		t.Fatal("expected error after all retries exhausted")
	}
	if got := atomic.LoadInt32(&s.calls); got != 3 {
		t.Errorf("expected exactly 3 attempts, got %d", got)
	}
}

func TestPersistWithRetry_SurvivesParentCtxCancel(t *testing.T) {
	s := &flakyStore{failsRemaining: 1}
	creds := &ChannelCreds{AppID: "x", SecretKey: "y"}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// Parent ctx is cancelled, but persistWithRetry uses WithoutCancel for the
	// Update call so the rotated tokens still land in DB.
	err := persistWithRetry(ctx, s, uuid.New(), creds)
	if err != nil {
		t.Fatalf("expected success despite cancelled parent ctx, got %v", err)
	}
}
