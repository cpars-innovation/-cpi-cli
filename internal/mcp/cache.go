package mcp

import (
	"sync"
	"time"
)

// readCache keeps the results of listing tools (list_packages,
// list_artifacts) for a short time: agents call them again and again. Every
// call of a tool that changes the tenant clears it; refresh=true bypasses it.
type readCache struct {
	ttl     time.Duration
	mu      sync.Mutex
	entries map[string]cacheEntry
	// keys serialises loads of the same key: concurrent calls share one
	// tenant request
	keys map[string]*sync.Mutex
}

type cacheEntry struct {
	value any
	at    time.Time
}

func newReadCache(ttl time.Duration) *readCache {
	return &readCache{ttl: ttl, entries: map[string]cacheEntry{}, keys: map[string]*sync.Mutex{}}
}

// get returns the cached value of key, or calls load and caches a
// successful result. cached reports whether the value came from the cache.
func (c *readCache) get(key string, refresh bool, load func() (any, error)) (value any, cached bool, err error) {
	if c.ttl <= 0 {
		v, err := load()
		return v, false, err
	}
	c.mu.Lock()
	km, ok := c.keys[key]
	if !ok {
		km = &sync.Mutex{}
		c.keys[key] = km
	}
	c.mu.Unlock()
	km.Lock()
	defer km.Unlock()

	c.mu.Lock()
	e, ok := c.entries[key]
	c.mu.Unlock()
	if ok && !refresh && time.Since(e.at) < c.ttl {
		return e.value, true, nil
	}
	v, err := load()
	if err == nil {
		c.mu.Lock()
		c.entries[key] = cacheEntry{value: v, at: time.Now()}
		c.mu.Unlock()
	}
	return v, false, err
}

func (c *readCache) clear() {
	c.mu.Lock()
	c.entries = map[string]cacheEntry{}
	c.mu.Unlock()
}
