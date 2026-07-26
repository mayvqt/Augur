package discordbot

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
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
	ownerID   string
	expiresAt time.Time
}

func (c *selectionCache) setMany(cacheID, ownerID string, results map[string]seer.SearchResult) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.initLocked()
	expiresAt := time.Now().Add(15 * time.Minute)
	for key, result := range results {
		c.items[cacheKey(cacheID, key)] = cachedSelection{result: result, ownerID: ownerID, expiresAt: expiresAt}
	}
	c.pruneLocked()
}

func (c *selectionCache) get(cacheID, key, ownerID string) (seer.SearchResult, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.initLocked()
	item, ok := c.items[cacheKey(cacheID, key)]
	if !ok || item.ownerID != ownerID || !time.Now().Before(item.expiresAt) {
		if ok && item.ownerID == ownerID {
			delete(c.items, cacheKey(cacheID, key))
		}
		return seer.SearchResult{}, false
	}
	return item.result, true
}

func (c *selectionCache) take(cacheID, key, ownerID string) (seer.SearchResult, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.initLocked()
	item, ok := c.items[cacheKey(cacheID, key)]
	if !ok || item.ownerID != ownerID || !time.Now().Before(item.expiresAt) {
		return seer.SearchResult{}, false
	}
	c.discardLocked(cacheID, ownerID)
	return item.result, true
}

func (c *selectionCache) discard(cacheID, ownerID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.initLocked()
	c.discardLocked(cacheID, ownerID)
}

func (c *selectionCache) discardLocked(cacheID, ownerID string) {
	for itemKey, candidate := range c.items {
		if candidate.ownerID == ownerID && strings.HasPrefix(itemKey, cacheID+":") {
			delete(c.items, itemKey)
		}
	}
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

func randomID() (string, error) {
	var data [16]byte
	if _, err := rand.Read(data[:]); err != nil {
		return "", fmt.Errorf("generate secure selection ID: %w", err)
	}
	return hex.EncodeToString(data[:]), nil
}
