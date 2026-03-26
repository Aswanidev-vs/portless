package state

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"
)

// StateEntry represents a state entry
type StateEntry struct {
	Key       string        `json:"key"`
	Value     interface{}   `json:"value"`
	Version   int64         `json:"version"`
	CreatedAt time.Time     `json:"created_at"`
	UpdatedAt time.Time     `json:"updated_at"`
	TTL       time.Duration `json:"ttl,omitempty"`
	Checksum  string        `json:"checksum"`
}

// ReplicationLog represents a state change log entry
type ReplicationLog struct {
	SequenceID int64       `json:"sequence_id"`
	Operation  string      `json:"operation"` // "set", "delete", "update"
	Key        string      `json:"key"`
	Value      interface{} `json:"value,omitempty"`
	Timestamp  time.Time   `json:"timestamp"`
	NodeID     string      `json:"node_id"`
	Checksum   string      `json:"checksum"`
}

// NodeStatus represents the status of a node
type NodeStatus string

const (
	NodeStatusActive     NodeStatus = "active"
	NodeStatusStandby    NodeStatus = "standby"
	NodeStatusSyncing    NodeStatus = "syncing"
	NodeStatusFailed     NodeStatus = "failed"
	NodeStatusRecovering NodeStatus = "recovering"
)

// Node represents a state node
type Node struct {
	ID            string     `json:"id"`
	Address       string     `json:"address"`
	Status        NodeStatus `json:"status"`
	IsLeader      bool       `json:"is_leader"`
	LastHeartbeat time.Time  `json:"last_heartbeat"`
	SequenceID    int64      `json:"sequence_id"`
}

// ReplicationConfig holds replication configuration
type Config struct {
	NodeID            string
	Nodes             []string
	HeartbeatInterval time.Duration
	ElectionTimeout   time.Duration
	SyncTimeout       time.Duration
	MaxLogEntries     int
	SnapshotInterval  int
	PeerTransport     PeerTransport
}

type PeerTransport interface {
	BroadcastHeartbeat(ctx context.Context, self Node) error
	FetchLogs(ctx context.Context, peer Node, fromSequence int64) ([]ReplicationLog, error)
	FetchSnapshot(ctx context.Context, peer Node) ([]StateEntry, error)
}

// Store defines the interface for state storage
type Store interface {
	Get(ctx context.Context, key string) (StateEntry, error)
	Set(ctx context.Context, entry StateEntry) error
	Delete(ctx context.Context, key string) error
	List(ctx context.Context, prefix string) ([]StateEntry, error)
	SaveLog(ctx context.Context, log ReplicationLog) error
	GetLogs(ctx context.Context, fromSequence int64) ([]ReplicationLog, error)
	Snapshot(ctx context.Context, entries []StateEntry) error
	LoadSnapshot(ctx context.Context) ([]StateEntry, error)
}

// Replicator manages state replication across nodes
type Replicator struct {
	mu            sync.RWMutex
	config        Config
	store         Store
	localState    map[string]StateEntry
	nodes         map[string]*Node
	leaderID      string
	sequenceID    int64
	stopCh        chan struct{}
	wg            sync.WaitGroup
	onStateChange func(key string, value interface{})
	onFailover    func(newLeader string)
}

// NewReplicator creates a new state replicator
func NewReplicator(cfg Config, store Store) *Replicator {
	if cfg.HeartbeatInterval == 0 {
		cfg.HeartbeatInterval = 1 * time.Second
	}
	if cfg.ElectionTimeout == 0 {
		cfg.ElectionTimeout = 5 * time.Second
	}
	if cfg.SyncTimeout == 0 {
		cfg.SyncTimeout = 30 * time.Second
	}
	if cfg.MaxLogEntries == 0 {
		cfg.MaxLogEntries = 10000
	}

	r := &Replicator{
		config:     cfg,
		store:      store,
		localState: make(map[string]StateEntry),
		nodes:      make(map[string]*Node),
		stopCh:     make(chan struct{}),
	}

	// Initialize self node
	r.nodes[cfg.NodeID] = &Node{
		ID:            cfg.NodeID,
		Address:       cfg.NodeID,
		Status:        NodeStatusActive,
		LastHeartbeat: time.Now(),
	}

	for _, node := range cfg.Nodes {
		node = strings.TrimSpace(node)
		if node == "" || node == cfg.NodeID {
			continue
		}
		r.nodes[node] = &Node{
			ID:            node,
			Address:       node,
			Status:        NodeStatusSyncing,
			LastHeartbeat: time.Now(),
		}
	}

	return r
}

// Start starts the replicator
func (r *Replicator) Start(ctx context.Context) error {
	// Load snapshot
	entries, err := r.store.LoadSnapshot(ctx)
	if err == nil {
		for _, entry := range entries {
			r.localState[entry.Key] = entry
			if entry.Version > r.sequenceID {
				r.sequenceID = entry.Version
			}
		}
	}

	// Start heartbeat
	r.wg.Add(1)
	go r.heartbeatWorker(ctx)

	// Start leader election
	r.wg.Add(1)
	go r.leaderElectionWorker(ctx)

	// Start sync worker
	r.wg.Add(1)
	go r.syncWorker(ctx)

	// Start cleanup worker
	r.wg.Add(1)
	go r.cleanupWorker(ctx)

	return nil
}

// Stop stops the replicator
func (r *Replicator) Stop() {
	close(r.stopCh)
	r.wg.Wait()
}

// Get retrieves a state entry
func (r *Replicator) Get(ctx context.Context, key string) (StateEntry, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Check local cache first
	if entry, ok := r.localState[key]; ok {
		if entry.TTL > 0 && time.Since(entry.UpdatedAt) > entry.TTL {
			return StateEntry{}, fmt.Errorf("key %s expired", key)
		}
		return entry, nil
	}

	// Try store
	return r.store.Get(ctx, key)
}

// Set sets a state entry and replicates to other nodes
func (r *Replicator) Set(ctx context.Context, key string, value interface{}, ttl time.Duration) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.sequenceID++
	now := time.Now()

	entry := StateEntry{
		Key:       key,
		Value:     value,
		Version:   r.sequenceID,
		CreatedAt: now,
		UpdatedAt: now,
		TTL:       ttl,
		Checksum:  calculateChecksum(value),
	}

	// Save to store
	if err := r.store.Set(ctx, entry); err != nil {
		return fmt.Errorf("save state: %w", err)
	}

	// Create replication log
	log := ReplicationLog{
		SequenceID: r.sequenceID,
		Operation:  "set",
		Key:        key,
		Value:      value,
		Timestamp:  now,
		NodeID:     r.config.NodeID,
		Checksum:   entry.Checksum,
	}

	if err := r.store.SaveLog(ctx, log); err != nil {
		return fmt.Errorf("save log: %w", err)
	}

	// Update local state
	r.localState[key] = entry

	// Notify listeners
	if r.onStateChange != nil {
		r.onStateChange(key, value)
	}

	return nil
}

// Delete deletes a state entry
func (r *Replicator) Delete(ctx context.Context, key string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.sequenceID++
	now := time.Now()

	// Delete from store
	if err := r.store.Delete(ctx, key); err != nil {
		return fmt.Errorf("delete state: %w", err)
	}

	// Create replication log
	log := ReplicationLog{
		SequenceID: r.sequenceID,
		Operation:  "delete",
		Key:        key,
		Timestamp:  now,
		NodeID:     r.config.NodeID,
	}

	if err := r.store.SaveLog(ctx, log); err != nil {
		return fmt.Errorf("save log: %w", err)
	}

	// Update local state
	delete(r.localState, key)

	// Notify listeners
	if r.onStateChange != nil {
		r.onStateChange(key, nil)
	}

	return nil
}

// List lists state entries with a prefix
func (r *Replicator) List(ctx context.Context, prefix string) ([]StateEntry, error) {
	return r.store.List(ctx, prefix)
}

// OnStateChange sets a callback for state changes
func (r *Replicator) OnStateChange(fn func(key string, value interface{})) {
	r.onStateChange = fn
}

// OnFailover sets a callback for failover events
func (r *Replicator) OnFailover(fn func(newLeader string)) {
	r.onFailover = fn
}

// GetLeader returns the current leader
func (r *Replicator) GetLeader() string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.leaderID
}

// IsLeader returns true if this node is the leader
func (r *Replicator) IsLeader() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.leaderID == r.config.NodeID
}

// GetNodes returns all known nodes
func (r *Replicator) GetNodes() map[string]Node {
	r.mu.RLock()
	defer r.mu.RUnlock()
	nodes := make(map[string]Node)
	for id, node := range r.nodes {
		nodes[id] = *node
	}
	return nodes
}

func (r *Replicator) heartbeatWorker(ctx context.Context) {
	defer r.wg.Done()

	ticker := time.NewTicker(r.config.HeartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-r.stopCh:
			return
		case <-ticker.C:
			r.sendHeartbeat()
			r.checkNodeHealth()
		}
	}
}

func (r *Replicator) sendHeartbeat() {
	r.mu.Lock()
	var self Node
	if node, ok := r.nodes[r.config.NodeID]; ok {
		node.LastHeartbeat = time.Now()
		node.SequenceID = r.sequenceID
		node.IsLeader = r.leaderID == r.config.NodeID
		node.Address = r.config.NodeID
		self = *node
	}
	r.mu.Unlock()

	if r.config.PeerTransport != nil {
		_ = r.config.PeerTransport.BroadcastHeartbeat(context.Background(), self)
	}
}

func (r *Replicator) checkNodeHealth() {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()
	for id, node := range r.nodes {
		if id == r.config.NodeID {
			continue
		}

		if now.Sub(node.LastHeartbeat) > r.config.ElectionTimeout {
			node.Status = NodeStatusFailed

			// If leader failed, trigger election
			if node.IsLeader {
				r.leaderID = ""
				go r.triggerElection()
			}
		}
	}
}

func (r *Replicator) leaderElectionWorker(ctx context.Context) {
	defer r.wg.Done()

	// Initial election
	r.triggerElection()

	ticker := time.NewTicker(r.config.ElectionTimeout)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-r.stopCh:
			return
		case <-ticker.C:
			if r.leaderID == "" {
				r.triggerElection()
			}
		}
	}
}

func (r *Replicator) triggerElection() {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Simple leader election: node with lowest ID among active nodes wins
	var candidates []string
	for id, node := range r.nodes {
		if node.Status == NodeStatusActive {
			candidates = append(candidates, id)
		}
	}

	if len(candidates) == 0 {
		return
	}

	// Sort candidates and select first
	// In production, use Raft or Paxos
	selectedID := candidates[0]
	for _, id := range candidates {
		if id < selectedID {
			selectedID = id
		}
	}

	if r.leaderID != selectedID {
		oldLeader := r.leaderID
		r.leaderID = selectedID

		// Update node status
		for id, node := range r.nodes {
			node.IsLeader = (id == selectedID)
			if id == selectedID {
				node.Status = NodeStatusActive
			} else {
				node.Status = NodeStatusStandby
			}
		}

		// Notify failover
		if oldLeader != "" && r.onFailover != nil {
			r.onFailover(selectedID)
		}
	}
}

func (r *Replicator) syncWorker(ctx context.Context) {
	defer r.wg.Done()

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-r.stopCh:
			return
		case <-ticker.C:
			r.syncState(ctx)
		}
	}
}

func (r *Replicator) syncState(ctx context.Context) {
	if r.IsLeader() {
		return
	}
	if r.config.PeerTransport == nil {
		return
	}
	leader := r.getLeaderNode()
	if leader == nil {
		return
	}

	logs, err := r.config.PeerTransport.FetchLogs(ctx, *leader, r.sequenceID)
	if err != nil {
		snapshot, snapErr := r.config.PeerTransport.FetchSnapshot(ctx, *leader)
		if snapErr != nil {
			return
		}
		r.applySnapshot(ctx, snapshot)
		return
	}

	// Apply logs
	r.ApplyLogs(ctx, logs)
}

func (r *Replicator) cleanupWorker(ctx context.Context) {
	defer r.wg.Done()

	ticker := time.NewTicker(10 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-r.stopCh:
			return
		case <-ticker.C:
			r.cleanupExpiredEntries(ctx)
		}
	}
}

func (r *Replicator) cleanupExpiredEntries(ctx context.Context) {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()
	for key, entry := range r.localState {
		if entry.TTL > 0 && now.Sub(entry.UpdatedAt) > entry.TTL {
			delete(r.localState, key)
			r.store.Delete(ctx, key)
		}
	}
}

// RegisterNode registers a new node
func (r *Replicator) RegisterNode(id, address string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.nodes[id]; !exists {
		r.nodes[id] = &Node{
			ID:            id,
			Address:       address,
			Status:        NodeStatusSyncing,
			LastHeartbeat: time.Now(),
		}
	}
}

func (r *Replicator) ApplyHeartbeat(node Node) {
	r.mu.Lock()
	defer r.mu.Unlock()
	current, ok := r.nodes[node.ID]
	if !ok {
		current = &Node{ID: node.ID}
		r.nodes[node.ID] = current
	}
	current.Address = node.Address
	current.LastHeartbeat = node.LastHeartbeat
	current.SequenceID = node.SequenceID
	current.IsLeader = node.IsLeader
	current.Status = NodeStatusActive
	if node.IsLeader {
		r.leaderID = node.ID
	}
}

func (r *Replicator) ApplyLogs(ctx context.Context, logs []ReplicationLog) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, log := range logs {
		switch log.Operation {
		case "set":
			createdAt := log.Timestamp
			if existing, ok := r.localState[log.Key]; ok && !existing.CreatedAt.IsZero() {
				createdAt = existing.CreatedAt
			}
			entry := StateEntry{
				Key:       log.Key,
				Value:     log.Value,
				Version:   log.SequenceID,
				CreatedAt: createdAt,
				UpdatedAt: log.Timestamp,
				Checksum:  log.Checksum,
			}
			_ = r.store.Set(ctx, entry)
			r.localState[log.Key] = entry
		case "delete":
			_ = r.store.Delete(ctx, log.Key)
			delete(r.localState, log.Key)
		}
		if log.SequenceID > r.sequenceID {
			r.sequenceID = log.SequenceID
		}
	}
}

func (r *Replicator) ExportLogs(ctx context.Context, fromSequence int64) ([]ReplicationLog, error) {
	return r.store.GetLogs(ctx, fromSequence)
}

func (r *Replicator) ExportSnapshot(ctx context.Context) ([]StateEntry, error) {
	return r.store.List(ctx, "")
}

// RemoveNode removes a node
func (r *Replicator) RemoveNode(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.nodes, id)

	if r.leaderID == id {
		r.leaderID = ""
	}
}

// TakeSnapshot creates a snapshot of current state
func (r *Replicator) TakeSnapshot(ctx context.Context) error {
	r.mu.RLock()
	entries := make([]StateEntry, 0, len(r.localState))
	for _, entry := range r.localState {
		entries = append(entries, entry)
	}
	r.mu.RUnlock()

	return r.store.Snapshot(ctx, entries)
}

// RecoverFromFailure recovers state after a failure
func (r *Replicator) RecoverFromFailure(ctx context.Context) error {
	r.mu.Lock()
	r.nodes[r.config.NodeID].Status = NodeStatusRecovering
	r.mu.Unlock()

	// Load snapshot
	entries, err := r.store.LoadSnapshot(ctx)
	if err != nil {
		return fmt.Errorf("load snapshot: %w", err)
	}

	r.mu.Lock()
	for _, entry := range entries {
		r.localState[entry.Key] = entry
		if entry.Version > r.sequenceID {
			r.sequenceID = entry.Version
		}
	}
	r.nodes[r.config.NodeID].Status = NodeStatusActive
	r.mu.Unlock()

	// Sync with leader
	if !r.IsLeader() {
		r.syncState(ctx)
	}

	return nil
}

func (r *Replicator) getLeaderNode() *Node {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.leaderID == "" {
		return nil
	}
	if node, ok := r.nodes[r.leaderID]; ok {
		copy := *node
		return &copy
	}
	return nil
}

func (r *Replicator) applySnapshot(ctx context.Context, entries []StateEntry) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for key := range r.localState {
		delete(r.localState, key)
	}
	_ = r.store.Snapshot(ctx, entries)
	for _, entry := range entries {
		r.localState[entry.Key] = entry
		_ = r.store.Set(ctx, entry)
		if entry.Version > r.sequenceID {
			r.sequenceID = entry.Version
		}
	}
}

func calculateChecksum(value interface{}) string {
	data, _ := json.Marshal(value)
	hash := fmt.Sprintf("%x", data)
	if len(hash) > 32 {
		return hash[:32]
	}
	return hash
}
