package cache

import (
	"context"
	"sync"
	"time"
)

type item struct {
	value   []byte
	expires time.Time // zero: never
	tags    []string
}

// Memory is a Cache in process memory, with TTL and tag invalidation. It is
// the backend for tests and for a single-process deployment; it enforces the
// real constraints, so a test that passes against it is not passing against
// a mock.
type Memory struct {
	// Now is the clock; nil means time.Now. Tests set it to advance time.
	Now   func() time.Time
	mu    sync.Mutex
	items map[string]item
	byTag map[string]map[string]struct{}
}

func (m *Memory) now() time.Time {
	if m.Now != nil {
		return m.Now()
	}
	return time.Now()
}

func (m *Memory) init() {
	if m.items == nil {
		m.items = map[string]item{}
		m.byTag = map[string]map[string]struct{}{}
	}
}

// Get returns a live entry.
func (m *Memory) Get(_ context.Context, key string) ([]byte, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.init()
	it, ok := m.items[key]
	if !ok {
		return nil, false, nil
	}
	if !it.expires.IsZero() && !m.now().Before(it.expires) {
		m.remove(key)
		return nil, false, nil
	}
	return append([]byte(nil), it.value...), true, nil
}

// Set stores a copy of value with its tags.
func (m *Memory) Set(_ context.Context, key string, value []byte, ttl time.Duration, tags []string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.init()
	m.remove(key)
	it := item{value: append([]byte(nil), value...), tags: append([]string(nil), tags...)}
	if ttl > 0 {
		it.expires = m.now().Add(ttl)
	}
	m.items[key] = it
	for _, t := range tags {
		if m.byTag[t] == nil {
			m.byTag[t] = map[string]struct{}{}
		}
		m.byTag[t][key] = struct{}{}
	}
	return nil
}

// Delete drops one entry.
func (m *Memory) Delete(_ context.Context, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.init()
	m.remove(key)
	return nil
}

// InvalidateTags drops every entry carrying any of the tags.
func (m *Memory) InvalidateTags(_ context.Context, tags ...string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.init()
	for _, t := range tags {
		for key := range m.byTag[t] {
			m.remove(key)
		}
	}
	return nil
}

// Len is the number of live entries, for tests.
func (m *Memory) Len() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.items)
}

// remove must be called with the lock held.
func (m *Memory) remove(key string) {
	it, ok := m.items[key]
	if !ok {
		return
	}
	for _, t := range it.tags {
		delete(m.byTag[t], key)
		if len(m.byTag[t]) == 0 {
			delete(m.byTag, t)
		}
	}
	delete(m.items, key)
}
