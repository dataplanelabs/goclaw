package schedule

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

type fakeSrc struct {
	resolveID   func(ctx context.Context, tenantID, channelName string) (string, error)
	loadInst    func(ctx context.Context, id string) (*Schedule, error)
	loadThr     func(ctx context.Context, id, key string) (*Schedule, *time.Time, error)
	resolveHits int32
	instHits    int32
	thrHits     int32
}

func (f *fakeSrc) src() ScheduleSource {
	return ScheduleSource{
		ResolveInstanceID: func(ctx context.Context, t, c string) (string, error) {
			atomic.AddInt32(&f.resolveHits, 1)
			return f.resolveID(ctx, t, c)
		},
		LoadInstance: func(ctx context.Context, id string) (*Schedule, error) {
			atomic.AddInt32(&f.instHits, 1)
			return f.loadInst(ctx, id)
		},
		LoadThreadOverride: func(ctx context.Context, id, key string) (*Schedule, *time.Time, error) {
			atomic.AddInt32(&f.thrHits, 1)
			return f.loadThr(ctx, id, key)
		},
	}
}

func TestRegistry_NoInstanceShortCircuit(t *testing.T) {
	f := &fakeSrc{
		resolveID: func(_ context.Context, _, _ string) (string, error) { return "", nil },
		loadInst:  func(_ context.Context, _ string) (*Schedule, error) { return nil, nil },
		loadThr:   func(_ context.Context, _, _ string) (*Schedule, *time.Time, error) { return nil, nil, nil },
	}
	r := NewRegistry(f.src(), time.Minute)
	got := r.ResolveMode(context.Background(), "t1", "telegram", "direct:peer1", time.Now())
	if got != ModeActive {
		t.Fatalf("want active, got %v", got)
	}
	if f.instHits != 0 || f.thrHits != 0 {
		t.Fatalf("expected no schedule loads on empty instance, got inst=%d thr=%d", f.instHits, f.thrHits)
	}
}

func TestRegistry_InstanceCacheHit(t *testing.T) {
	f := &fakeSrc{
		resolveID: func(_ context.Context, _, _ string) (string, error) { return "inst-1", nil },
		loadInst:  func(_ context.Context, _ string) (*Schedule, error) { return &Schedule{DefaultMode: ModeStandby}, nil },
		loadThr:   func(_ context.Context, _, _ string) (*Schedule, *time.Time, error) { return nil, nil, nil },
	}
	r := NewRegistry(f.src(), time.Minute)
	now := time.Now()
	for i := 0; i < 3; i++ {
		if got := r.ResolveMode(context.Background(), "t1", "tg", "direct:p1", now); got != ModeStandby {
			t.Fatalf("iter %d: want standby, got %v", i, got)
		}
	}
	if f.resolveHits != 1 {
		t.Fatalf("resolveID called %d times, want 1 (cache miss only on first)", f.resolveHits)
	}
	if f.instHits != 1 {
		t.Fatalf("loadInst called %d times, want 1", f.instHits)
	}
}

func TestRegistry_ThreadBeatsInstance(t *testing.T) {
	f := &fakeSrc{
		resolveID: func(_ context.Context, _, _ string) (string, error) { return "inst-1", nil },
		loadInst:  func(_ context.Context, _ string) (*Schedule, error) { return &Schedule{DefaultMode: ModeStandby}, nil },
		loadThr: func(_ context.Context, _, _ string) (*Schedule, *time.Time, error) {
			return &Schedule{DefaultMode: ModeActive}, nil, nil
		},
	}
	r := NewRegistry(f.src(), time.Minute)
	if got := r.ResolveMode(context.Background(), "t1", "tg", "direct:p1", time.Now()); got != ModeActive {
		t.Fatalf("thread override should beat instance: want active, got %v", got)
	}
	if f.instHits != 0 {
		t.Fatalf("instance loader should not run when thread override resolves: got %d", f.instHits)
	}
}

func TestRegistry_ExpiredThreadFallsThrough(t *testing.T) {
	past := time.Now().Add(-time.Hour)
	f := &fakeSrc{
		resolveID: func(_ context.Context, _, _ string) (string, error) { return "inst-1", nil },
		loadInst:  func(_ context.Context, _ string) (*Schedule, error) { return &Schedule{DefaultMode: ModeActive}, nil },
		loadThr: func(_ context.Context, _, _ string) (*Schedule, *time.Time, error) {
			return &Schedule{DefaultMode: ModeStandby}, &past, nil
		},
	}
	r := NewRegistry(f.src(), time.Minute)
	if got := r.ResolveMode(context.Background(), "t1", "tg", "direct:p1", time.Now()); got != ModeActive {
		t.Fatalf("expired thread override: want fall-through to instance active, got %v", got)
	}
}

func TestRegistry_InvalidateInstanceForcesRefetch(t *testing.T) {
	calls := int32(0)
	f := &fakeSrc{
		resolveID: func(_ context.Context, _, _ string) (string, error) {
			n := atomic.AddInt32(&calls, 1)
			if n == 1 {
				return "inst-1", nil
			}
			return "inst-2", nil
		},
		loadInst: func(_ context.Context, id string) (*Schedule, error) {
			if id == "inst-2" {
				return &Schedule{DefaultMode: ModeStandby}, nil
			}
			return &Schedule{DefaultMode: ModeActive}, nil
		},
		loadThr: func(_ context.Context, _, _ string) (*Schedule, *time.Time, error) { return nil, nil, nil },
	}
	r := NewRegistry(f.src(), time.Minute)
	now := time.Now()
	if got := r.ResolveMode(context.Background(), "t1", "tg", "direct:p1", now); got != ModeActive {
		t.Fatalf("pre-invalidate: %v", got)
	}
	r.InvalidateInstance("t1", "tg")
	if got := r.ResolveMode(context.Background(), "t1", "tg", "direct:p1", now); got != ModeStandby {
		t.Fatalf("post-invalidate: %v", got)
	}
}

func TestRegistry_ReloadClearsThreadCache(t *testing.T) {
	thrLoads := int32(0)
	f := &fakeSrc{
		resolveID: func(_ context.Context, _, _ string) (string, error) { return "inst-1", nil },
		loadInst:  func(_ context.Context, _ string) (*Schedule, error) { return nil, nil },
		loadThr: func(_ context.Context, _, _ string) (*Schedule, *time.Time, error) {
			atomic.AddInt32(&thrLoads, 1)
			return nil, nil, nil
		},
	}
	r := NewRegistry(f.src(), time.Minute)
	r.ResolveMode(context.Background(), "t1", "tg", "direct:p1", time.Now())
	r.ResolveMode(context.Background(), "t1", "tg", "direct:p1", time.Now())
	if thrLoads != 1 {
		t.Fatalf("expected 1 thread load before reload, got %d", thrLoads)
	}
	r.Reload("inst-1")
	r.ResolveMode(context.Background(), "t1", "tg", "direct:p1", time.Now())
	if thrLoads != 2 {
		t.Fatalf("expected 2 thread loads after reload, got %d", thrLoads)
	}
}
