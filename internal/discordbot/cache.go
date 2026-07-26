package discordbot

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strconv"
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
	seasons   seer.SeasonSelection
	quota     *seer.Quota
	ownerID   string
	expiresAt time.Time
}

type cachedResultOption struct {
	key    string
	result seer.SearchResult
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

func (c *selectionCache) options(cacheID, ownerID string) []cachedResultOption {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.initLocked()

	now := time.Now()
	options := make([]cachedResultOption, 0, 25)
	for index := 0; index < 25; index++ {
		key := strconv.Itoa(index)
		item, ok := c.items[cacheKey(cacheID, key)]
		if !ok {
			continue
		}
		if item.ownerID != ownerID || !now.Before(item.expiresAt) {
			if item.ownerID == ownerID {
				delete(c.items, cacheKey(cacheID, key))
			}
			continue
		}
		options = append(options, cachedResultOption{key: key, result: item.result})
	}
	return options
}

func (c *selectionCache) setQuota(cacheID, key, ownerID string, quota *seer.Quota) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.initLocked()
	itemKey := cacheKey(cacheID, key)
	item, ok := c.items[itemKey]
	if !ok || item.ownerID != ownerID || !time.Now().Before(item.expiresAt) {
		return false
	}
	if quota == nil {
		item.quota = nil
	} else {
		quotaCopy := *quota
		item.quota = &quotaCopy
	}
	c.items[itemKey] = item
	return true
}

func (c *selectionCache) getQuota(cacheID, key, ownerID string) (*seer.Quota, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.initLocked()
	item, ok := c.items[cacheKey(cacheID, key)]
	if !ok || item.ownerID != ownerID || !time.Now().Before(item.expiresAt) {
		return nil, false
	}
	if item.quota == nil {
		return nil, true
	}
	quotaCopy := *item.quota
	return &quotaCopy, true
}

func (c *selectionCache) setSeasons(cacheID, key, ownerID string, seasons seer.SeasonSelection) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.initLocked()
	itemKey := cacheKey(cacheID, key)
	item, ok := c.items[itemKey]
	if !ok || item.ownerID != ownerID || !time.Now().Before(item.expiresAt) {
		return false
	}
	item.seasons = seer.SeasonSelection{
		Numbers: append([]int(nil), seasons.Numbers...),
		All:     seasons.All,
	}
	c.items[itemKey] = item
	return true
}

func (c *selectionCache) take(cacheID, key, ownerID string) (seer.SearchResult, seer.SeasonSelection, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.initLocked()
	item, ok := c.items[cacheKey(cacheID, key)]
	if !ok || item.ownerID != ownerID || !time.Now().Before(item.expiresAt) {
		return seer.SearchResult{}, seer.SeasonSelection{}, false
	}
	c.discardLocked(cacheID, ownerID)
	return item.result, item.seasons, true
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
