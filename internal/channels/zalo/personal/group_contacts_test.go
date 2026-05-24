package personal

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestGroupCache_FreshLifecycle(t *testing.T) {
	t.Parallel()
	cache := newGroupCache()
	if cache.fresh("g1") {
		t.Fatal("unmarked group should not be fresh")
	}
	cache.mark("g1")
	if !cache.fresh("g1") {
		t.Fatal("just-marked group should be fresh")
	}
}

func TestGroupCache_FreshExpires(t *testing.T) {
	cache := newGroupCache()
	base := time.Now()
	nowFn = func() time.Time { return base }
	t.Cleanup(func() { nowFn = time.Now })

	cache.mark("g1")
	nowFn = func() time.Time { return base.Add(groupCacheTTL + time.Second) }
	if cache.fresh("g1") {
		t.Fatal("group should expire after TTL")
	}
}

func TestGroupCache_ConcurrentAccess(t *testing.T) {
	t.Parallel()
	cache := newGroupCache()
	var wg sync.WaitGroup
	for i := range 100 {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			key := "g" + string(rune('A'+id%26))
			cache.mark(key)
			_ = cache.fresh(key)
		}(i)
	}
	wg.Wait()
}

func TestShouldBootstrap_DebounceWindow(t *testing.T) {
	base := time.Now()
	nowFn = func() time.Time { return base }
	t.Cleanup(func() { nowFn = time.Now })

	var ts atomic.Int64
	if !shouldBootstrap(&ts) {
		t.Fatal("first call should return true")
	}
	if shouldBootstrap(&ts) {
		t.Fatal("immediate second call should return false (debounced)")
	}

	nowFn = func() time.Time { return base.Add(groupBootstrapDebounce + time.Second) }
	if !shouldBootstrap(&ts) {
		t.Fatal("after debounce window, should return true again")
	}
}

func TestEnsureGroupKnown_NilArgsAreNoop(t *testing.T) {
	t.Parallel()
	cache := newGroupCache()
	ensureGroupKnown(context.Background(), nil, nil, cache, "zalo_personal", "inst", "g1")
	ensureGroupKnown(context.Background(), nil, nil, cache, "zalo_personal", "inst", "")
	if cache.fresh("g1") || cache.fresh("") {
		t.Fatal("no-op calls should not mark cache")
	}
}

func TestEnsureGroupKnown_CacheHitSkipsFetch(t *testing.T) {
	t.Parallel()
	cache := newGroupCache()
	cache.mark("g1")
	// Passing nil session would panic if FetchGroups were called; cache hit must short-circuit.
	ensureGroupKnown(context.Background(), nil, nil, cache, "zalo_personal", "inst", "g1")
	// Give the goroutine a chance to (incorrectly) fire.
	time.Sleep(20 * time.Millisecond)
	if !cache.fresh("g1") {
		t.Fatal("cache entry should remain after no-op call")
	}
}

func TestShouldBootstrap_ReconnectStorm(t *testing.T) {
	base := time.Now()
	nowFn = func() time.Time { return base }
	t.Cleanup(func() { nowFn = time.Now })

	var ts atomic.Int64
	var trueCount atomic.Int32
	var wg sync.WaitGroup
	for range 5 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if shouldBootstrap(&ts) {
				trueCount.Add(1)
			}
		}()
	}
	wg.Wait()
	if got := trueCount.Load(); got != 1 {
		t.Fatalf("5 concurrent reconnects: want exactly 1 bootstrap, got %d", got)
	}
}
