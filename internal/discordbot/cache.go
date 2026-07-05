package discordbot

import (
	"crypto/rand"
	"encoding/hex"
	"sync"
	"time"

	"github.com/mayvqt/Augur/internal/seer"
)

type selectionCache struct {
	mu    sync.Mutex
	items map[string]cachedSelection
}

type cachedSelection struct {
	result    seer.SearchResult
	expiresAt time.Time
}

func (c *selectionCache) setMany(cacheID string, results map[string]seer.SearchResult) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.initLocked()
	expiresAt := time.Now().Add(15 * time.Minute)
	for key, result := range results {
		c.items[cacheKey(cacheID, key)] = cachedSelection{result: result, expiresAt: expiresAt}
	}
	c.pruneLocked()
}

func (c *selectionCache) get(cacheID, key string) (seer.SearchResult, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.initLocked()
	item, ok := c.items[cacheKey(cacheID, key)]
	if !ok || time.Now().After(item.expiresAt) {
		return seer.SearchResult{}, false
	}
	return item.result, true
}

func (c *selectionCache) initLocked() {
	if c.items == nil {
		c.items = map[string]cachedSelection{}
	}
}

func (c *selectionCache) pruneLocked() {
	now := time.Now()
	for key, item := range c.items {
		if now.After(item.expiresAt) {
			delete(c.items, key)
		}
	}
}

func cacheKey(cacheID, key string) string {
	return cacheID + ":" + key
}

func randomID() string {
	var data [8]byte
	if _, err := rand.Read(data[:]); err != nil {
		return hex.EncodeToString([]byte(time.Now().Format(time.RFC3339Nano)))
	}
	return hex.EncodeToString(data[:])
}
