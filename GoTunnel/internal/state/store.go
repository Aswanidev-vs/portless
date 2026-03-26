package state

import (
	"context"
	"fmt"
	"strings"
	"sync"
)

type MemoryStore struct {
	mu       sync.RWMutex
	entries  map[string]StateEntry
	logs     []ReplicationLog
	snapshot []StateEntry
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		entries: make(map[string]StateEntry),
	}
}

func (s *MemoryStore) Get(_ context.Context, key string) (StateEntry, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	entry, ok := s.entries[key]
	if !ok {
		return StateEntry{}, fmt.Errorf("key %s not found", key)
	}
	return entry, nil
}

func (s *MemoryStore) Set(_ context.Context, entry StateEntry) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.entries[entry.Key] = entry
	return nil
}

func (s *MemoryStore) Delete(_ context.Context, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.entries, key)
	return nil
}

func (s *MemoryStore) List(_ context.Context, prefix string) ([]StateEntry, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var entries []StateEntry
	for key, entry := range s.entries {
		if prefix == "" || strings.HasPrefix(key, prefix) {
			entries = append(entries, entry)
		}
	}
	return entries, nil
}

func (s *MemoryStore) SaveLog(_ context.Context, log ReplicationLog) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.logs = append(s.logs, log)
	return nil
}

func (s *MemoryStore) GetLogs(_ context.Context, fromSequence int64) ([]ReplicationLog, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var logs []ReplicationLog
	for _, log := range s.logs {
		if log.SequenceID > fromSequence {
			logs = append(logs, log)
		}
	}
	return logs, nil
}

func (s *MemoryStore) Snapshot(_ context.Context, entries []StateEntry) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.snapshot = append([]StateEntry(nil), entries...)
	return nil
}

func (s *MemoryStore) LoadSnapshot(_ context.Context) ([]StateEntry, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]StateEntry(nil), s.snapshot...), nil
}

