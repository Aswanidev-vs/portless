package state

import (
	"context"
	"testing"
	"time"
)

func TestFileStorePersistsEntriesAndLogs(t *testing.T) {
	dir := t.TempDir()
	store, err := NewFileStore(dir)
	if err != nil {
		t.Fatalf("new file store: %v", err)
	}

	entry := StateEntry{
		Key:       "leases/test",
		Value:     map[string]string{"id": "test"},
		Version:   1,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
		Checksum:  "abc123",
	}
	if err := store.Set(context.Background(), entry); err != nil {
		t.Fatalf("set: %v", err)
	}
	if err := store.SaveLog(context.Background(), ReplicationLog{SequenceID: 1, Key: entry.Key, Operation: "set"}); err != nil {
		t.Fatalf("save log: %v", err)
	}

	reloaded, err := NewFileStore(dir)
	if err != nil {
		t.Fatalf("reload file store: %v", err)
	}
	got, err := reloaded.Get(context.Background(), entry.Key)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Key != entry.Key || got.Version != entry.Version {
		t.Fatalf("unexpected persisted entry: %#v", got)
	}
	logs, err := reloaded.GetLogs(context.Background(), 0)
	if err != nil {
		t.Fatalf("get logs: %v", err)
	}
	if len(logs) != 1 {
		t.Fatalf("expected 1 log entry, got %d", len(logs))
	}
}
