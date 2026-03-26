package state

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

type FileStore struct {
	mu        sync.RWMutex
	baseDir   string
	entries   map[string]StateEntry
	logs      []ReplicationLog
	snapshot  []StateEntry
	logPath   string
	statePath string
}

type fileStoreData struct {
	Entries  []StateEntry     `json:"entries"`
	Logs     []ReplicationLog `json:"logs"`
	Snapshot []StateEntry     `json:"snapshot"`
}

func NewFileStore(baseDir string) (*FileStore, error) {
	baseDir = filepath.Clean(baseDir)
	if err := os.MkdirAll(baseDir, 0o755); err != nil {
		return nil, fmt.Errorf("create state dir: %w", err)
	}
	s := &FileStore{
		baseDir:   baseDir,
		entries:   make(map[string]StateEntry),
		logPath:   filepath.Join(baseDir, "replication-log.json"),
		statePath: filepath.Join(baseDir, "state.json"),
	}
	if err := s.load(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *FileStore) Get(_ context.Context, key string) (StateEntry, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	entry, ok := s.entries[key]
	if !ok {
		return StateEntry{}, fmt.Errorf("key %s not found", key)
	}
	return entry, nil
}

func (s *FileStore) Set(_ context.Context, entry StateEntry) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.entries[entry.Key] = entry
	return s.persist()
}

func (s *FileStore) Delete(_ context.Context, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.entries, key)
	return s.persist()
}

func (s *FileStore) List(_ context.Context, prefix string) ([]StateEntry, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	entries := make([]StateEntry, 0, len(s.entries))
	for key, entry := range s.entries {
		if prefix == "" || strings.HasPrefix(key, prefix) {
			entries = append(entries, entry)
		}
	}
	return entries, nil
}

func (s *FileStore) SaveLog(_ context.Context, log ReplicationLog) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.logs = append(s.logs, log)
	return s.persist()
}

func (s *FileStore) GetLogs(_ context.Context, fromSequence int64) ([]ReplicationLog, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	logs := make([]ReplicationLog, 0, len(s.logs))
	for _, log := range s.logs {
		if log.SequenceID > fromSequence {
			logs = append(logs, log)
		}
	}
	return logs, nil
}

func (s *FileStore) Snapshot(_ context.Context, entries []StateEntry) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.snapshot = append([]StateEntry(nil), entries...)
	return s.persist()
}

func (s *FileStore) LoadSnapshot(_ context.Context) ([]StateEntry, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]StateEntry(nil), s.snapshot...), nil
}

func (s *FileStore) load() error {
	payload, err := os.ReadFile(s.statePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read state file: %w", err)
	}
	var data fileStoreData
	if err := json.Unmarshal(payload, &data); err != nil {
		return fmt.Errorf("decode state file: %w", err)
	}
	for _, entry := range data.Entries {
		s.entries[entry.Key] = entry
	}
	s.logs = append([]ReplicationLog(nil), data.Logs...)
	s.snapshot = append([]StateEntry(nil), data.Snapshot...)
	return nil
}

func (s *FileStore) persist() error {
	entries := make([]StateEntry, 0, len(s.entries))
	for _, entry := range s.entries {
		entries = append(entries, entry)
	}
	data := fileStoreData{
		Entries:  entries,
		Logs:     append([]ReplicationLog(nil), s.logs...),
		Snapshot: append([]StateEntry(nil), s.snapshot...),
	}
	payload, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("encode state file: %w", err)
	}
	tmp := s.statePath + ".tmp"
	if err := os.WriteFile(tmp, payload, 0o644); err != nil {
		return fmt.Errorf("write state temp file: %w", err)
	}
	if err := os.Rename(tmp, s.statePath); err != nil {
		return fmt.Errorf("replace state file: %w", err)
	}
	if err := os.WriteFile(s.logPath, payload, 0o644); err != nil {
		return fmt.Errorf("write replication log file: %w", err)
	}
	return nil
}
