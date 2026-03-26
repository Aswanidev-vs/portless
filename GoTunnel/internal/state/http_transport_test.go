package state

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestHTTPPeerTransportReplicatesLogsAndSnapshot(t *testing.T) {
	store := NewMemoryStore()
	replicator := NewReplicator(Config{NodeID: "node-a"}, store)
	ctx := context.Background()
	if err := replicator.Set(ctx, "leases/lease-1", map[string]string{"id": "lease-1"}, 0); err != nil {
		t.Fatalf("set: %v", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/state/peer/heartbeat", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/v1/state/peer/logs", func(w http.ResponseWriter, r *http.Request) {
		logs, err := replicator.ExportLogs(r.Context(), ParsePeerSequence(r.URL.Query().Get("from")))
		if err != nil {
			t.Fatalf("export logs: %v", err)
		}
		writeJSONTest(w, logs)
	})
	mux.HandleFunc("/v1/state/peer/snapshot", func(w http.ResponseWriter, r *http.Request) {
		entries, err := replicator.ExportSnapshot(r.Context())
		if err != nil {
			t.Fatalf("export snapshot: %v", err)
		}
		writeJSONTest(w, entries)
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	transport := NewHTTPPeerTransport([]string{"node-a=" + server.URL}, "")
	peer := Node{ID: "node-a", Address: server.URL, IsLeader: true, LastHeartbeat: time.Now()}
	logs, err := transport.FetchLogs(ctx, peer, 0)
	if err != nil {
		t.Fatalf("fetch logs: %v", err)
	}
	if len(logs) != 1 || logs[0].Key != "leases/lease-1" {
		t.Fatalf("unexpected logs: %+v", logs)
	}

	snapshot, err := transport.FetchSnapshot(ctx, peer)
	if err != nil {
		t.Fatalf("fetch snapshot: %v", err)
	}
	if len(snapshot) != 1 || snapshot[0].Key != "leases/lease-1" {
		t.Fatalf("unexpected snapshot: %+v", snapshot)
	}
}

func writeJSONTest(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(v)
}
