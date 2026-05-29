package audio_test

import (
	"context"
	"testing"
	"time"

	"github.com/nextlevelbuilder/goclaw/internal/audio"
)

// blockingTTS blocks until its attempt context is cancelled, then returns that
// context's error — modelling a slow provider (e.g. CPU VieNeu) that overruns.
type blockingTTS struct{ name string }

func (b *blockingTTS) Name() string { return b.name }
func (b *blockingTTS) Synthesize(ctx context.Context, _ string, _ audio.TTSOptions) (*audio.SynthResult, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

// instantTTS returns a result immediately — modelling a fast provider (Edge).
type instantTTS struct{ name string }

func (i *instantTTS) Name() string { return i.name }
func (i *instantTTS) Synthesize(_ context.Context, _ string, _ audio.TTSOptions) (*audio.SynthResult, error) {
	return &audio.SynthResult{}, nil
}

// A slow primary must not consume the whole deadline: each attempt gets its own
// slice, so the fast fallback still runs and succeeds.
func TestSynthesizeWithFallback_SlowPrimaryDoesNotStarveFallback(t *testing.T) {
	mgr := audio.NewManager(audio.ManagerConfig{Primary: "vieneu"})
	mgr.RegisterTTS(&blockingTTS{name: "vieneu"})
	mgr.RegisterTTS(&instantTTS{name: "edge"})

	ctx, cancel := context.WithTimeout(context.Background(), 400*time.Millisecond)
	defer cancel()

	start := time.Now()
	res, err := mgr.SynthesizeWithFallbackAdapted(ctx, "hello", audio.TTSOptions{}, nil)
	if err != nil {
		t.Fatalf("expected fallback to succeed, got error: %v", err)
	}
	if res == nil {
		t.Fatal("expected non-nil result from fallback")
	}
	// Primary slice is ~half the budget; the whole call must finish well before
	// the full deadline (proving the primary was bounded, not allowed to run out
	// the clock).
	if elapsed := time.Since(start); elapsed > 350*time.Millisecond {
		t.Errorf("call took %v; primary was not bounded to its slice", elapsed)
	}
}
