package api

import (
	"net/http"
	"sync"
	"time"
)

type responseCache struct {
	ttl time.Duration
	mu  sync.RWMutex
	m   map[string]cacheEntry
}

type cacheEntry struct {
	data      []byte
	status    int
	header    http.Header
	expiresAt time.Time
}

func (c *responseCache) get(key string, now time.Time) (cacheEntry, bool) {
	if c == nil {
		return cacheEntry{}, false
	}
	c.mu.RLock()
	entry, ok := c.m[key]
	c.mu.RUnlock()
	if !ok {
		return cacheEntry{}, false
	}
	if now.After(entry.expiresAt) {
		c.mu.Lock()
		delete(c.m, key)
		c.mu.Unlock()
		return cacheEntry{}, false
	}
	return entry, true
}

func (c *responseCache) set(key string, entry cacheEntry) {
	if c == nil {
		return
	}
	c.mu.Lock()
	if c.m == nil {
		c.m = make(map[string]cacheEntry)
	}
	c.m[key] = entry
	c.mu.Unlock()
}
