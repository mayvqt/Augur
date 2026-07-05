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
	mu   sync.RWMutex
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
	store.data.normalize()
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
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Watch, 0, len(s.data.Watches))
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
	s.data.normalize()
	return writeStateFile(s.path, s.data)
}

func ensureDir(path string) error {
	dir := filepath.Dir(path)
	if dir == "." || dir == "" {
		return nil
	}
	return os.MkdirAll(dir, 0o755)
}

func (s *state) normalize() {
	if s.Watches == nil {
		s.Watches = []Watch{}
	}
}

func writeStateFile(path string, data state) error {
	dir := filepath.Dir(path)
	if dir == "." || dir == "" {
		dir = "."
	}
	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() {
		_ = os.Remove(tmpName)
	}()
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return err
	}
	encoder := json.NewEncoder(tmp)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}
