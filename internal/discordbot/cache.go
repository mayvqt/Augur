package discordbot

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/mayvqt/Augur/internal/seer"
)

const selectionTTL = 15 * time.Minute

type selectionCache struct {
	mu       sync.Mutex
	searches map[string]*cachedSearch
}

type cachedSearch struct {
	ownerID    string
	expiresAt  time.Time
	results    []cachedSelection
	quota      *seer.Quota
	quotaKnown bool
}

type cachedSelection struct {
	result  seer.SearchResult
	seasons seer.SeasonSelection
}

type cachedResultOption struct {
	key    string
	result seer.SearchResult
}

func (c *selectionCache) set(cacheID, ownerID string, results []seer.SearchResult) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.initLocked()

	selections := make([]cachedSelection, len(results))
	for index, result := range results {
		selections[index].result = result
	}
	c.searches[cacheID] = &cachedSearch{
		ownerID:   ownerID,
		expiresAt: time.Now().Add(selectionTTL),
		results:   selections,
	}
	c.pruneLocked()
}

func (c *selectionCache) get(cacheID, key, ownerID string) (seer.SearchResult, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	selection, ok := c.selectionLocked(cacheID, key, ownerID)
	if !ok {
		return seer.SearchResult{}, false
	}
	return selection.result, true
}

func (c *selectionCache) options(cacheID, ownerID string) []cachedResultOption {
	c.mu.Lock()
	defer c.mu.Unlock()

	search, ok := c.searchLocked(cacheID, ownerID)
	if !ok {
		return nil
	}
	options := make([]cachedResultOption, len(search.results))
	for index, selection := range search.results {
		options[index] = cachedResultOption{
			key:    strconv.Itoa(index),
			result: selection.result,
		}
	}
	return options
}

func (c *selectionCache) setQuota(cacheID, ownerID string, quota *seer.Quota) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	search, ok := c.searchLocked(cacheID, ownerID)
	if !ok {
		return false
	}
	if quota == nil {
		search.quota = nil
	} else {
		quotaCopy := *quota
		search.quota = &quotaCopy
	}
	search.quotaKnown = true
	return true
}

func (c *selectionCache) getQuota(cacheID, ownerID string) (*seer.Quota, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	search, ok := c.searchLocked(cacheID, ownerID)
	if !ok || !search.quotaKnown {
		return nil, false
	}
	if search.quota == nil {
		return nil, true
	}
	quotaCopy := *search.quota
	return &quotaCopy, true
}

func (c *selectionCache) setSeasons(cacheID, key, ownerID string, seasons seer.SeasonSelection) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	selection, ok := c.selectionLocked(cacheID, key, ownerID)
	if !ok {
		return false
	}
	selection.seasons = seer.SeasonSelection{
		Numbers: append([]int(nil), seasons.Numbers...),
		All:     seasons.All,
	}
	return true
}

func (c *selectionCache) take(cacheID, key, ownerID string) (seer.SearchResult, seer.SeasonSelection, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	selection, ok := c.selectionLocked(cacheID, key, ownerID)
	if !ok {
		return seer.SearchResult{}, seer.SeasonSelection{}, false
	}
	result := selection.result
	seasons := seer.SeasonSelection{
		Numbers: append([]int(nil), selection.seasons.Numbers...),
		All:     selection.seasons.All,
	}
	delete(c.searches, cacheID)
	return result, seasons, true
}

func (c *selectionCache) discard(cacheID, ownerID string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if _, ok := c.searchLocked(cacheID, ownerID); ok {
		delete(c.searches, cacheID)
	}
}

func (c *selectionCache) selectionLocked(cacheID, key, ownerID string) (*cachedSelection, bool) {
	search, ok := c.searchLocked(cacheID, ownerID)
	if !ok {
		return nil, false
	}
	index, err := strconv.Atoi(key)
	if err != nil || index < 0 || index >= len(search.results) {
		return nil, false
	}
	return &search.results[index], true
}

func (c *selectionCache) searchLocked(cacheID, ownerID string) (*cachedSearch, bool) {
	c.initLocked()
	search, ok := c.searches[cacheID]
	if !ok || search.ownerID != ownerID {
		return nil, false
	}
	if !time.Now().Before(search.expiresAt) {
		delete(c.searches, cacheID)
		return nil, false
	}
	return search, true
}

func (c *selectionCache) initLocked() {
	if c.searches == nil {
		c.searches = make(map[string]*cachedSearch)
	}
}

func (c *selectionCache) pruneLocked() {
	now := time.Now()
	for cacheID, search := range c.searches {
		if !now.Before(search.expiresAt) {
			delete(c.searches, cacheID)
		}
	}
}

func randomID() (string, error) {
	var data [16]byte
	if _, err := rand.Read(data[:]); err != nil {
		return "", fmt.Errorf("generate secure selection ID: %w", err)
	}
	return hex.EncodeToString(data[:]), nil
}
