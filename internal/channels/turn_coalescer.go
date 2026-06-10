package channels

import (
	"sync"
	"time"
)

// TurnCoalescer buffers addressed inbound turns for a short trailing-edge
// window, merging later pieces such as follow-up media before dispatch.
type TurnCoalescer[T any] struct {
	delay time.Duration
	merge func(existing, next T) T
	flush func(T)

	mu      sync.Mutex
	entries map[string]*turnCoalescerEntry[T]
}

type turnCoalescerEntry[T any] struct {
	timer      *time.Timer
	generation int
	value      T
}

func NewTurnCoalescer[T any](delay time.Duration, merge func(existing, next T) T, flush func(T)) *TurnCoalescer[T] {
	return &TurnCoalescer[T]{
		delay:   delay,
		merge:   merge,
		flush:   flush,
		entries: make(map[string]*turnCoalescerEntry[T]),
	}
}

func (c *TurnCoalescer[T]) Delay() time.Duration {
	if c == nil {
		return 0
	}
	return c.delay
}

func (c *TurnCoalescer[T]) Enqueue(key string, value T) {
	if c == nil || c.delay <= 0 {
		if c != nil && c.flush != nil {
			c.flush(value)
		}
		return
	}

	c.mu.Lock()
	entry, ok := c.entries[key]
	if !ok {
		entry = &turnCoalescerEntry[T]{value: value}
		c.entries[key] = entry
	} else {
		if entry.timer != nil {
			entry.timer.Stop()
		}
		if c.merge != nil {
			entry.value = c.merge(entry.value, value)
		} else {
			entry.value = value
		}
	}

	entry.generation++
	generation := entry.generation
	delay := c.delay
	entry.timer = time.AfterFunc(delay, func() {
		c.flushGeneration(key, generation)
	})
	c.mu.Unlock()
}

func (c *TurnCoalescer[T]) Flush(key string) {
	if c == nil {
		return
	}
	c.mu.Lock()
	entry, ok := c.entries[key]
	if !ok {
		c.mu.Unlock()
		return
	}
	if entry.timer != nil {
		entry.timer.Stop()
	}
	delete(c.entries, key)
	value := entry.value
	c.mu.Unlock()

	if c.flush != nil {
		c.flush(value)
	}
}

func (c *TurnCoalescer[T]) FlushAll() {
	if c == nil {
		return
	}
	var values []T

	c.mu.Lock()
	for key, entry := range c.entries {
		if entry.timer != nil {
			entry.timer.Stop()
		}
		values = append(values, entry.value)
		delete(c.entries, key)
	}
	c.mu.Unlock()

	if c.flush == nil {
		return
	}
	for _, value := range values {
		c.flush(value)
	}
}

func (c *TurnCoalescer[T]) flushGeneration(key string, generation int) {
	c.mu.Lock()
	entry, ok := c.entries[key]
	if !ok || entry.generation != generation {
		c.mu.Unlock()
		return
	}
	if entry.timer != nil {
		entry.timer.Stop()
	}
	delete(c.entries, key)
	value := entry.value
	c.mu.Unlock()

	if c.flush != nil {
		c.flush(value)
	}
}
