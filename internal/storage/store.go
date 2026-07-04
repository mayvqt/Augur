package storage

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type Watch struct {
	RequestID   int       `json:"request_id"`
	DiscordID   string    `json:"discord_id"`
	Title       string    `json:"title"`
	MediaType   string    `json:"media_type"`
	CreatedAt   time.Time `json:"created_at"`
	CompletedAt time.Time `json:"completed_at,omitempty"`
}

type Store struct {
	path string
	mu   sync.Mutex
	data state
}

type state struct {
	Watches []Watch `json:"watches"`
}

func Open(path string) (*Store, error) {
	if err := ensureDir(path); err != nil {
		return nil, err
	}
	store := &Store{path: path}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		store.data = state{Watches: []Watch{}}
		return store, nil
	}
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		store.data = state{Watches: []Watch{}}
		return store, nil
	}
	if err := json.Unmarshal(data, &store.data); err != nil {
		return nil, err
	}
	return store, nil
}

func (s *Store) Close() error {
	return s.save()
}

func (s *Store) AddWatch(ctx context.Context, watch Watch) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, existing := range s.data.Watches {
		if existing.RequestID == watch.RequestID {
			s.data.Watches[i] = watch
			return s.saveLocked()
		}
	}
	s.data.Watches = append(s.data.Watches, watch)
	return s.saveLocked()
}

func (s *Store) OpenWatches(ctx context.Context) ([]Watch, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []Watch
	for _, watch := range s.data.Watches {
		if watch.CompletedAt.IsZero() {
			out = append(out, watch)
		}
	}
	return out, nil
}

func (s *Store) CompleteWatch(ctx context.Context, requestID int, completedAt time.Time) (Watch, bool, error) {
	if err := ctx.Err(); err != nil {
		return Watch{}, false, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, watch := range s.data.Watches {
		if watch.RequestID == requestID && watch.CompletedAt.IsZero() {
			s.data.Watches[i].CompletedAt = completedAt.UTC()
			if err := s.saveLocked(); err != nil {
				return Watch{}, false, err
			}
			return s.data.Watches[i], true, nil
		}
	}
	return Watch{}, false, nil
}

func (s *Store) save() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.saveLocked()
}

func (s *Store) saveLocked() error {
	data, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

func ensureDir(path string) error {
	dir := filepath.Dir(path)
	if dir == "." || dir == "" {
		return nil
	}
	return os.MkdirAll(dir, 0o755)
}
