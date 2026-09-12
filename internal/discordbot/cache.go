package discordbot

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/mayvqt/Augur/internal/seer"
)

const (
	selectionTTL        = 15 * time.Minute
	selectionMaxEntries = 10_000
)

type selectionCache struct {
	mu       sync.Mutex
	searches map[string]*cachedSearch
}

type cachedSearch struct {
	interactionMu sync.Mutex
	ownerID       string
	query         string
	expiresAt     time.Time
	results       []cachedSelection
	quota         *seer.Quota
	quotaKnown    bool
	submitting    bool
	submitted     bool
}

type cachedSelection struct {
	result           seer.SearchResult
	availableSeasons []seer.Season
	selectedSeasons  seer.SeasonSelection
}

type cachedResultOption struct {
	key    string
	result seer.SearchResult
}

func (c *selectionCache) set(cacheID, ownerID, query string, results []seer.SearchResult) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.initLocked()

	selections := make([]cachedSelection, len(results))
	for index, result := range results {
		selections[index].result = result
	}
	c.searches[cacheID] = &cachedSearch{
		ownerID:   ownerID,
		query:     query,
		expiresAt: time.Now().Add(selectionTTL),
		results:   selections,
	}
	c.pruneLocked()
	c.evictOldestLocked()
}

func (c *selectionCache) setResults(cacheID, ownerID string, results []seer.SearchResult) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	search, ok := c.searchLocked(cacheID, ownerID)
	if !ok || search.submitting || search.submitted {
		return false
	}
	search.results = make([]cachedSelection, len(results))
	for index, result := range results {
		search.results[index].result = result
	}
	search.expiresAt = time.Now().Add(selectionTTL)
	return true
}

func (c *selectionCache) query(cacheID, ownerID string) (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	search, ok := c.searchLocked(cacheID, ownerID)
	if !ok {
		return "", false
	}
	return search.query, true
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

func (c *selectionCache) setAvailableSeasons(cacheID, key, ownerID string, seasons []seer.Season) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	selection, ok := c.selectionLocked(cacheID, key, ownerID)
	if !ok {
		return false
	}
	selection.availableSeasons = append([]seer.Season(nil), seasons...)
	return true
}

func (c *selectionCache) selection(cacheID, key, ownerID string) (seer.SearchResult, []seer.Season, seer.SeasonSelection, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	selection, ok := c.selectionLocked(cacheID, key, ownerID)
	if !ok {
		return seer.SearchResult{}, nil, seer.SeasonSelection{}, false
	}
	return selection.result, append([]seer.Season(nil), selection.availableSeasons...), seer.SeasonSelection{
		Numbers: append([]int(nil), selection.selectedSeasons.Numbers...),
		All:     selection.selectedSeasons.All,
	}, true
}

// A search owns its response ordering. Other searches remain independent; a
// concurrent click gets an immediate ephemeral acknowledgement instead of
// waiting behind a network call and later overwriting the completed response.
func (c *selectionCache) beginInteraction(cacheID, ownerID string) (func(), bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	search, ok := c.searchLocked(cacheID, ownerID)
	if !ok || !search.interactionMu.TryLock() {
		return nil, false
	}
	search.expiresAt = time.Now().Add(selectionTTL)
	return search.interactionMu.Unlock, true
}

// submissionState is checked before acknowledging components so stale clicks do
// not replace the response belonging to the request already in progress.
func (c *selectionCache) submissionState(cacheID, ownerID string) string {
	c.mu.Lock()
	defer c.mu.Unlock()
	search, ok := c.searchLocked(cacheID, ownerID)
	if !ok {
		return "expired"
	}
	if search.submitted {
		return "submitted"
	}
	if search.submitting {
		return "submitting"
	}
	return ""
}

func (c *selectionCache) beginSubmission(cacheID, key, ownerID string, all bool) (seer.SearchResult, seer.SeasonSelection, string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	search, ok := c.searchLocked(cacheID, ownerID)
	if !ok {
		return seer.SearchResult{}, seer.SeasonSelection{}, "expired"
	}
	if search.submitted {
		return seer.SearchResult{}, seer.SeasonSelection{}, "submitted"
	}
	if search.submitting {
		return seer.SearchResult{}, seer.SeasonSelection{}, "submitting"
	}
	selection, ok := c.selectionLocked(cacheID, key, ownerID)
	if !ok {
		return seer.SearchResult{}, seer.SeasonSelection{}, "expired"
	}
	if all {
		selection.selectedSeasons = seer.SeasonSelection{All: true}
	}
	search.submitting = true
	return selection.result, seer.SeasonSelection{All: selection.selectedSeasons.All, Numbers: append([]int(nil), selection.selectedSeasons.Numbers...)}, ""
}

func (c *selectionCache) finishSubmission(cacheID, ownerID string, consumed bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	// Retain a tombstone until expiry. Delayed duplicate clicks must leave the
	// successful response intact, including a request accepted before a local error.
	if search, ok := c.searches[cacheID]; ok && search.ownerID == ownerID {
		search.submitting = false
		search.submitted = consumed
	}
}

func (c *selectionCache) selectSeasonPage(cacheID, key, ownerID string, page int, numbers []int) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	selection, ok := c.selectionLocked(cacheID, key, ownerID)
	if !ok {
		return errors.New(expiredSeasonPickerMessage)
	}
	visible, ok := seasonPage(selection.availableSeasons, page)
	if !ok {
		return errors.New("that season page is no longer available")
	}
	allowed := make(map[int]bool, len(visible))
	for _, season := range visible {
		allowed[season.SeasonNumber] = true
	}
	merged := make(map[int]bool)
	for _, number := range selection.selectedSeasons.Numbers {
		if !allowed[number] {
			merged[number] = true
		}
	}
	for _, number := range numbers {
		if !allowed[number] {
			return errors.New("choose seasons from the displayed page")
		}
		merged[number] = true
	}
	next := seer.SeasonSelection{}
	for number := range merged {
		next.Numbers = append(next.Numbers, number)
	}
	sort.Ints(next.Numbers)
	if err := validatePickerSelection(next, c.searches[cacheID].quota); err != nil {
		return err
	}
	selection.selectedSeasons = next
	return nil
}

func (c *selectionCache) selectionLocked(cacheID, key, ownerID string) (*cachedSelection, bool) {
	search, ok := c.searchLocked(cacheID, ownerID)
	if !ok || search.submitting || search.submitted {
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

func (c *selectionCache) evictOldestLocked() {
	for len(c.searches) > selectionMaxEntries {
		var oldestID string
		var oldestExpiry time.Time
		for cacheID, search := range c.searches {
			if oldestID == "" || search.expiresAt.Before(oldestExpiry) {
				oldestID = cacheID
				oldestExpiry = search.expiresAt
			}
		}
		delete(c.searches, oldestID)
	}
}

func randomID() (string, error) {
	var data [16]byte
	if _, err := rand.Read(data[:]); err != nil {
		return "", fmt.Errorf("generate secure selection ID: %w", err)
	}
	return hex.EncodeToString(data[:]), nil
}
