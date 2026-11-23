package project

import (
	"errors"
	iofs "io/fs"
	"sync"
	"sync/atomic"
)

type cacheStatistics struct {
	Hits     uint64
	Misses   uint64
	Resident uint64
}

type cacheEntryState uint8

const (
	cacheEntryEmpty cacheEntryState = iota
	cacheEntryLoading
	cacheEntryReady
	cacheEntryClosed
)

type cacheEntry[T any] struct {
	value   T
	err     error
	wait    chan struct{}
	load    func() (T, error)
	dispose func(T) error
	mu      sync.Mutex
	state   cacheEntryState
}

type resourceCache[K comparable, T any] struct {
	entries  map[K]*cacheEntry[T]
	counters cacheCounters
	mu       sync.RWMutex
	closed   bool
}

type cacheCounters struct {
	hits     atomic.Uint64
	misses   atomic.Uint64
	resident atomic.Uint64
}

func newResourceCache[K comparable, T any](capacity int) *resourceCache[K, T] {
	return &resourceCache[K, T]{entries: make(map[K]*cacheEntry[T], capacity)}
}

func (c *resourceCache[K, T]) register(key K, load func() (T, error), dispose func(T) error) error {
	if c == nil || load == nil {
		return iofs.ErrInvalid
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return iofs.ErrClosed
	}
	if _, exists := c.entries[key]; exists {
		return iofs.ErrExist
	}
	c.entries[key] = &cacheEntry[T]{load: load, dispose: dispose}
	return nil
}

func (c *resourceCache[K, T]) get(key K) (T, bool, error) {
	if c == nil {
		var zero T
		return zero, false, iofs.ErrClosed
	}
	c.mu.RLock()
	if c.closed {
		c.mu.RUnlock()
		var zero T
		return zero, false, iofs.ErrClosed
	}
	entry, ok := c.entries[key]
	c.mu.RUnlock()
	if !ok {
		var zero T
		return zero, false, nil
	}
	value, err := entry.resolve(&c.counters)
	return value, true, err
}

func (c *resourceCache[K, T]) load(key K, load func() (T, error), dispose func(T) error) (T, error) {
	if c == nil || load == nil {
		var zero T
		return zero, iofs.ErrInvalid
	}
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		var zero T
		return zero, iofs.ErrClosed
	}
	entry := c.entries[key]
	if entry == nil {
		entry = &cacheEntry[T]{load: load, dispose: dispose}
		c.entries[key] = entry
	}
	c.mu.Unlock()
	return entry.resolve(&c.counters)
}

func (c *resourceCache[K, T]) replace(key K, value T, dispose func(T) error) error {
	if c == nil {
		return iofs.ErrClosed
	}
	entry := &cacheEntry[T]{state: cacheEntryReady, value: value, dispose: dispose}
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		if dispose != nil {
			return errors.Join(iofs.ErrClosed, dispose(value))
		}
		return iofs.ErrClosed
	}
	previous := c.entries[key]
	c.entries[key] = entry
	c.counters.resident.Add(1)
	c.mu.Unlock()
	if previous == nil {
		return nil
	}
	return previous.close(&c.counters)
}

func (c *resourceCache[K, T]) remove(key K) error {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	entry := c.entries[key]
	delete(c.entries, key)
	c.mu.Unlock()
	if entry == nil {
		return nil
	}
	return entry.close(&c.counters)
}

func (c *resourceCache[K, T]) loaded(key K) (T, bool) {
	if c == nil {
		var zero T
		return zero, false
	}
	c.mu.RLock()
	entry := c.entries[key]
	c.mu.RUnlock()
	if entry == nil {
		var zero T
		return zero, false
	}
	return entry.loaded()
}

// readyValue is the allocation-free fast path for hot cache reads. It only
// reports entries that have already completed loading; callers can fall back
// to get when another goroutine is still resolving the entry.
func (c *resourceCache[K, T]) readyValue(key K) (T, bool, error) {
	if c == nil {
		var zero T
		return zero, false, nil
	}
	c.mu.RLock()
	entry := c.entries[key]
	c.mu.RUnlock()
	if entry == nil {
		var zero T
		return zero, false, nil
	}
	entry.mu.Lock()
	if entry.state != cacheEntryReady && entry.state != cacheEntryClosed {
		entry.mu.Unlock()
		var zero T
		return zero, false, nil
	}
	value, err := entry.value, entry.err
	entry.mu.Unlock()
	return value, true, err
}

func (c *resourceCache[K, T]) ready(key K) bool {
	if c == nil {
		return false
	}
	c.mu.RLock()
	entry := c.entries[key]
	c.mu.RUnlock()
	if entry == nil {
		return false
	}
	entry.mu.Lock()
	ready := entry.state == cacheEntryReady
	entry.mu.Unlock()
	return ready
}

func (c *resourceCache[K, T]) len() int {
	if c == nil {
		return 0
	}
	c.mu.RLock()
	length := len(c.entries)
	c.mu.RUnlock()
	return length
}

func (c *resourceCache[K, T]) statistics() cacheStatistics {
	if c == nil {
		return cacheStatistics{}
	}
	return cacheStatistics{Hits: c.counters.hits.Load(), Misses: c.counters.misses.Load(), Resident: c.counters.resident.Load()}
}

func (c *resourceCache[K, T]) disposeResources() error {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil
	}
	c.closed = true
	entries := c.entries
	c.entries = nil
	c.mu.Unlock()

	errList := make([]error, 0)
	for _, entry := range entries {
		if err := entry.close(&c.counters); err != nil {
			errList = append(errList, err)
		}
	}
	return errors.Join(errList...)
}

func (e *cacheEntry[T]) resolve(counters *cacheCounters) (T, error) {
	counted := false
	for {
		e.mu.Lock()
		switch e.state {
		case cacheEntryReady:
			if !counted {
				counters.hits.Add(1)
			}
			value, err := e.value, e.err
			e.mu.Unlock()
			return value, err
		case cacheEntryLoading:
			if !counted {
				counters.hits.Add(1)
				counted = true
			}
			wait := e.wait
			e.mu.Unlock()
			<-wait
			continue
		case cacheEntryClosed:
			var zero T
			e.mu.Unlock()
			return zero, iofs.ErrClosed
		case cacheEntryEmpty:
			counters.misses.Add(1)
			counted = true
			load := e.load
			if load == nil {
				var zero T
				e.mu.Unlock()
				return zero, iofs.ErrClosed
			}
			e.state = cacheEntryLoading
			e.wait = make(chan struct{})
			e.mu.Unlock()

			value, err := load()
			e.mu.Lock()
			if retryableResourceError(err) {
				e.state = cacheEntryEmpty
			} else {
				e.value, e.err = value, err
				e.load = nil
				e.state = cacheEntryReady
				if err == nil {
					counters.resident.Add(1)
				}
			}
			close(e.wait)
			e.wait = nil
			e.mu.Unlock()
			return value, err
		}
	}
}

func (e *cacheEntry[T]) loaded() (T, bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.state != cacheEntryReady || e.err != nil {
		var zero T
		return zero, false
	}
	return e.value, true
}

func (e *cacheEntry[T]) close(counters *cacheCounters) error {
	for {
		e.mu.Lock()
		if e.state == cacheEntryLoading {
			wait := e.wait
			e.mu.Unlock()
			<-wait
			continue
		}
		if e.state == cacheEntryClosed {
			e.mu.Unlock()
			return nil
		}
		loaded := e.state == cacheEntryReady && e.err == nil
		value, dispose := e.value, e.dispose
		var zero T
		e.value = zero
		e.err = nil
		e.load = nil
		e.dispose = nil
		e.state = cacheEntryClosed
		e.mu.Unlock()
		if loaded {
			counters.resident.Add(^uint64(0))
		}
		if loaded && dispose != nil {
			return dispose(value)
		}
		return nil
	}
}
