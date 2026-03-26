package broker

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"gotunnel/internal/domains"
	"gotunnel/internal/metrics"
	"gotunnel/internal/nfr"
	"gotunnel/internal/ops"
	"gotunnel/internal/protocol"
	"gotunnel/internal/state"
	"gotunnel/internal/telemetry"
	"gotunnel/internal/transport"
)

type Server struct {
	addr       string
	relayURL   string
	validToken string
	publicBase string
	nodeID     string

	mu           sync.RWMutex
	leases       map[string]protocol.Lease
	relays       map[string]protocol.RelayRegistration
	audits       []string
	store        Store
	metrics      *metrics.Registry
	certFile     string
	keyFile      string
	domains      *domains.Registry
	replicator   *state.Replicator
	nfrValidator *nfr.Validator
	hardener     *ops.Hardener
}

func New(addr, relayURL, validToken, publicBase, certFile, keyFile, postgresDSN, stateDir, nodeID string, clusterNodes []string) *Server {
	store := NewMemoryStore()
	if strings.TrimSpace(postgresDSN) != "" {
		if pgStore, err := NewPostgresStore(postgresDSN); err == nil {
			store = pgStore
		}
	}
	replicationStore := state.Store(state.NewMemoryStore())
	if strings.TrimSpace(stateDir) != "" {
		if fileStore, err := state.NewFileStore(stateDir); err == nil {
			replicationStore = fileStore
		}
	}
	if strings.TrimSpace(nodeID) == "" {
		nodeID = firstNonEmpty(os.Getenv("NODE_ID"), "broker-local")
	}
	replicationConfig := state.Config{NodeID: nodeID, Nodes: clusterNodes}
	if len(clusterNodes) > 0 {
		replicationConfig.PeerTransport = state.NewHTTPPeerTransport(clusterNodes, validToken)
	}
	return &Server{
		addr:         addr,
		relayURL:     strings.TrimRight(relayURL, "/"),
		validToken:   strings.TrimSpace(validToken),
		publicBase:   strings.TrimRight(publicBase, "/"),
		nodeID:       nodeID,
		leases:       make(map[string]protocol.Lease),
		relays:       make(map[string]protocol.RelayRegistration),
		store:        store,
		metrics:      metrics.New(),
		certFile:     certFile,
		keyFile:      keyFile,
		domains:      domains.NewRegistry(),
		replicator:   state.NewReplicator(replicationConfig, replicationStore),
		nfrValidator: newBrokerNFRValidator(),
		hardener:     ops.NewHardener(ops.HardenerConfig{Version: "dev"}),
	}
}

func (s *Server) ListenAndServe(ctx context.Context) error {
	telemetry.Init("gotunnel-broker")
	_ = s.replicator.Start(ctx)
	s.replicator.OnStateChange(s.applyReplicatedStateChange)
	s.hydrateFromReplicatedState(ctx)
	_ = s.nfrValidator.Start(ctx)
	s.registerOperationalChecks()
	_ = s.hardener.Start(ctx)
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, s.hardener.GetSystemHealth(r.Context()))
	})
	mux.HandleFunc("/ready", func(w http.ResponseWriter, r *http.Request) {
		if !s.hardener.IsReady(r.Context()) {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "not_ready"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
	})
	mux.HandleFunc("/alive", func(w http.ResponseWriter, r *http.Request) {
		if !s.hardener.IsAlive(r.Context()) {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "not_alive"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "alive"})
	})
	mux.Handle("/metrics", s.metrics.Handler())
	mux.HandleFunc("/v1/sessions/open", s.handleOpenSession)
	mux.HandleFunc("/v1/leases/", s.handleGetLease)
	mux.HandleFunc("/v1/relays/register", s.handleRelayRegister)
	mux.HandleFunc("/v1/relays/heartbeat", s.handleRelayHeartbeat)
	mux.HandleFunc("/v1/relays", s.handleRelays)
	mux.HandleFunc("/v1/audit/export", s.handleAuditExport)
	mux.HandleFunc("/v1/domains", s.handleDomains)
	mux.HandleFunc("/v1/state/status", s.handleStateStatus)
	mux.HandleFunc("/v1/nfr/status", s.handleNFRStatus)
	mux.HandleFunc("/v1/state/peer/heartbeat", s.handlePeerHeartbeat)
	mux.HandleFunc("/v1/state/peer/logs", s.handlePeerLogs)
	mux.HandleFunc("/v1/state/peer/snapshot", s.handlePeerSnapshot)

	server := transport.NewServer(transport.ServerOptions{Addr: s.addr, Handler: mux, CertFile: s.certFile, KeyFile: s.keyFile})

	go func() {
		<-ctx.Done()
		s.hardener.Stop()
		s.nfrValidator.Stop()
		s.replicator.Stop()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()

	if s.certFile != "" && s.keyFile != "" {
		if err := server.ListenAndServeTLS(s.certFile, s.keyFile); err != nil && err != http.ErrServerClosed {
			return err
		}
		return nil
	}
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

func (s *Server) registerOperationalChecks() {
	s.hardener.RegisterHealthCheck("broker_runtime", func(context.Context) ops.ComponentHealth {
		s.mu.RLock()
		defer s.mu.RUnlock()
		return ops.ComponentHealth{
			Name:    "broker_runtime",
			Status:  ops.HealthStatusHealthy,
			Message: "broker runtime healthy",
			Metrics: map[string]float64{
				"leases": float64(len(s.leases)),
				"relays": float64(len(s.relays)),
				"audits": float64(len(s.audits)),
			},
		}
	})
	s.hardener.RegisterReadinessProbe(ops.ReadinessProbe{
		Name:    "broker_store",
		Timeout: 2 * time.Second,
		Check: func(context.Context) error {
			if s.store == nil {
				return fmt.Errorf("store not configured")
			}
			return nil
		},
	})
	s.hardener.RegisterLivenessProbe(ops.LivenessProbe{
		Name:    "broker_process",
		Timeout: 2 * time.Second,
		Check: func(context.Context) error {
			return nil
		},
	})
}

func (s *Server) handleOpenSession(w http.ResponseWriter, r *http.Request) {
	ctx, span := telemetry.Start(r.Context(), "gotunnel/broker", "open_session")
	defer span.End()
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req protocol.OpenSessionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("decode request: %v", err), http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(req.Token) == "" {
		http.Error(w, "token is required", http.StatusUnauthorized)
		return
	}
	if s.validToken != "" && req.Token != s.validToken {
		s.metrics.IncCounter("gotunnel_broker_auth_failures_total")
		http.Error(w, "invalid token", http.StatusUnauthorized)
		return
	}
	if req.Protocol != "http" && req.Protocol != "tcp" && req.Protocol != "udp" {
		http.Error(w, "only http, tcp, and udp tunnels are currently supported", http.StatusBadRequest)
		return
	}

	relay := s.selectRelay(req.RequestedRegion)
	subdomain := req.RequestedSubdomain
	if subdomain == "" {
		subdomain = "tunnel-" + shortID()
	}
	publicHost := subdomain + ".gotunnel.local"
	publicURL := "https://" + publicHost
	debugURL := s.publicBase + "/t/" + subdomain
	relayURL := s.relayURL
	region := ""
	if relay != nil {
		relayURL = relay.RelayURL
		if relay.PublicBase != "" {
			debugURL = strings.TrimRight(relay.PublicBase, "/") + "/t/" + subdomain
		}
		region = relay.Region
	}
	if req.Protocol == "tcp" {
		publicURL = "tcp://pending-relay-assignment"
		debugURL = ""
	} else if req.Protocol == "udp" {
		publicURL = "udp://pending-relay-assignment"
		debugURL = ""
	} else if req.CustomDomain != "" {
		publicHost = req.CustomDomain
		publicURL = "https://" + req.CustomDomain
		if record, err := s.domains.Ensure(ctx, protocol.DomainRequest{
			Domain:   req.CustomDomain,
			Provider: strings.TrimSpace(os.Getenv("GOTUNNEL_DOMAIN_PROVIDER")),
			Metadata: map[string]string{"target": publicCNAMEHost(debugURL, relayURL, s.publicBase)},
		}); err == nil {
			if record.Status == "verified" {
				publicURL = "https://" + req.CustomDomain
			}
		}
	} else if debugURL != "" {
		publicURL = debugURL
	}

	lease := protocol.Lease{
		ID:           randomID(),
		Name:         req.Name,
		Protocol:     req.Protocol,
		Subdomain:    subdomain,
		PublicHost:   publicHost,
		PublicURL:    publicURL,
		DebugURL:     debugURL,
		RelayURL:     relayURL,
		Inspection:   req.InspectionEnabled,
		HTTPSMode:    req.HTTPSMode,
		WebhookURL:   req.WebhookURL,
		IssuedTLS:    req.HTTPSMode == "auto",
		Region:       region,
		CustomDomain: req.CustomDomain,
		Production:   req.Production,
		AllowSharing: req.AllowSharing,
		Labels:       append([]string(nil), req.Labels...),
		CreatedAt:    time.Now().UTC(),
	}

	s.mu.Lock()
	s.leases[lease.ID] = lease
	if relay != nil {
		reg := s.relays[relay.ID]
		reg.AssignedTunnels++
		s.relays[relay.ID] = reg
	}
	s.auditLocked(fmt.Sprintf("lease_created id=%s region=%s subdomain=%s production=%t", lease.ID, lease.Region, lease.Subdomain, lease.Production))
	s.mu.Unlock()
	_ = s.store.SaveLease(ctx, lease)
	_ = s.replicator.Set(ctx, "leases/"+lease.ID, lease, 0)
	s.metrics.IncCounter("gotunnel_broker_leases_created_total")
	_ = s.nfrValidator.ValidateScalability("broker_concurrent_tunnels", len(s.leases))

	writeJSON(w, http.StatusCreated, lease)
}

func publicCNAMEHost(debugURL, relayURL, publicBase string) string {
	candidates := []string{publicBase, debugURL, relayURL}
	for _, candidate := range candidates {
		if strings.TrimSpace(candidate) == "" {
			continue
		}
		if parsed, err := url.Parse(candidate); err == nil && parsed.Hostname() != "" {
			return parsed.Hostname()
		}
	}
	return ""
}

func (s *Server) handleGetLease(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	id := strings.TrimPrefix(r.URL.Path, "/v1/leases/")
	s.mu.RLock()
	lease, ok := s.leases[id]
	s.mu.RUnlock()
	if !ok {
		if persisted, err := s.store.GetLease(r.Context(), id); err == nil {
			lease = persisted
			ok = true
		}
	}
	if !ok {
		http.Error(w, "lease not found", http.StatusNotFound)
		return
	}

	writeJSON(w, http.StatusOK, lease)
}

func (s *Server) handleDomains(w http.ResponseWriter, r *http.Request) {
	ctx, span := telemetry.Start(r.Context(), "gotunnel/broker", "domains")
	defer span.End()
	if !s.requireBrokerPermission(w, r) {
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req protocol.DomainRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	record, err := s.domains.Ensure(ctx, req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, http.StatusCreated, record)
}

func (s *Server) handleRelayRegister(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req protocol.RelayRegistrationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(req.Region) == "" || strings.TrimSpace(req.RelayURL) == "" {
		http.Error(w, "region and relay_url are required", http.StatusBadRequest)
		return
	}
	id := req.ID
	if id == "" {
		id = "relay-" + shortID()
	}
	reg := protocol.RelayRegistration{
		ID:            id,
		Name:          firstNonEmpty(req.Name, id),
		Region:        req.Region,
		RelayURL:      strings.TrimRight(req.RelayURL, "/"),
		PublicBase:    strings.TrimRight(req.PublicBase, "/"),
		Capacity:      req.Capacity,
		Features:      append([]string(nil), req.Features...),
		RegisteredAt:  time.Now().UTC(),
		LastHeartbeat: time.Now().UTC(),
		Status:        "online",
	}
	s.mu.Lock()
	if existing, ok := s.relays[id]; ok {
		reg.AssignedTunnels = existing.AssignedTunnels
		reg.RegisteredAt = existing.RegisteredAt
	}
	s.relays[id] = reg
	s.auditLocked(fmt.Sprintf("relay_registered id=%s region=%s", reg.ID, reg.Region))
	s.mu.Unlock()
	_ = s.store.SaveRelay(r.Context(), reg)
	_ = s.replicator.Set(r.Context(), "relays/"+reg.ID, reg, 0)
	s.metrics.IncCounter("gotunnel_broker_relays_registered_total")
	writeJSON(w, http.StatusCreated, reg)
}

func (s *Server) handleRelayHeartbeat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req protocol.RelayHeartbeatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	s.mu.Lock()
	reg, ok := s.relays[req.ID]
	if !ok {
		s.mu.Unlock()
		http.Error(w, "relay not found", http.StatusNotFound)
		return
	}
	reg.LastHeartbeat = time.Now().UTC()
	reg.AssignedTunnels = req.AssignedTunnels
	reg.Status = "online"
	s.relays[req.ID] = reg
	s.mu.Unlock()
	_ = s.store.SaveRelay(r.Context(), reg)
	_ = s.replicator.Set(r.Context(), "relays/"+reg.ID, reg, 0)
	writeJSON(w, http.StatusOK, reg)
}

func (s *Server) handleRelays(w http.ResponseWriter, r *http.Request) {
	if !s.requireBrokerPermission(w, r) {
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	s.mu.RLock()
	out := make([]protocol.RelayRegistration, 0, len(s.relays))
	for _, relay := range s.relays {
		if time.Since(relay.LastHeartbeat) > 45*time.Second {
			relay.Status = "stale"
		}
		out = append(out, relay)
	}
	s.mu.RUnlock()
	if len(out) == 0 {
		if persisted, err := s.store.ListRelays(r.Context()); err == nil {
			out = persisted
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Region == out[j].Region {
			return out[i].AssignedTunnels < out[j].AssignedTunnels
		}
		return out[i].Region < out[j].Region
	})
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleAuditExport(w http.ResponseWriter, r *http.Request) {
	if !s.requireBrokerPermission(w, r) {
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	s.mu.RLock()
	leases := make([]protocol.Lease, 0, len(s.leases))
	for _, lease := range s.leases {
		leases = append(leases, lease)
	}
	relays := make([]protocol.RelayRegistration, 0, len(s.relays))
	for _, relay := range s.relays {
		relays = append(relays, relay)
	}
	audits := append([]string(nil), s.audits...)
	s.mu.RUnlock()
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"generated_at": time.Now().UTC(),
		"leases":       leases,
		"relays":       relays,
		"entries":      audits,
	})
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func randomID() string {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("lease-%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(buf)
}

func shortID() string {
	return randomID()[:6]
}

func (s *Server) selectRelay(region string) *protocol.RelayRegistration {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var candidates []protocol.RelayRegistration
	for _, relay := range s.relays {
		if time.Since(relay.LastHeartbeat) > 45*time.Second {
			continue
		}
		if region != "" && relay.Region != region {
			continue
		}
		candidates = append(candidates, relay)
	}
	if len(candidates) == 0 && region != "" {
		for _, relay := range s.relays {
			if time.Since(relay.LastHeartbeat) <= 45*time.Second {
				candidates = append(candidates, relay)
			}
		}
	}
	if len(candidates) == 0 {
		return nil
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].AssignedTunnels == candidates[j].AssignedTunnels {
			return candidates[i].Region < candidates[j].Region
		}
		return candidates[i].AssignedTunnels < candidates[j].AssignedTunnels
	})
	selected := candidates[0]
	return &selected
}

func (s *Server) auditLocked(entry string) {
	s.audits = append([]string{fmt.Sprintf("%s %s", time.Now().UTC().Format(time.RFC3339), entry)}, s.audits...)
	if len(s.audits) > 200 {
		s.audits = s.audits[:200]
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func (s *Server) handleStateStatus(w http.ResponseWriter, r *http.Request) {
	if !s.requireBrokerPermission(w, r) {
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"leader":    s.replicator.GetLeader(),
		"is_leader": s.replicator.IsLeader(),
		"nodes":     s.replicator.GetNodes(),
	})
}

func (s *Server) handleNFRStatus(w http.ResponseWriter, r *http.Request) {
	if !s.requireBrokerPermission(w, r) {
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"requirements": s.nfrValidator.ListRequirements(),
		"summary":      s.nfrValidator.GetStatusSummary(),
		"failing":      s.nfrValidator.GetFailingRequirements(),
	})
}

func newBrokerNFRValidator() *nfr.Validator {
	v := nfr.NewValidator(nfr.ValidatorConfig{})
	v.AddRequirement(&nfr.NFRRequirement{ID: "broker_concurrent_tunnels", Type: nfr.NFRScalability, Name: "Concurrent tunnels", Threshold: 10000, Unit: "tunnels", Status: nfr.NFRStatusUnknown})
	v.AddRequirement(&nfr.NFRRequirement{ID: "broker_uptime", Type: nfr.NFRUptime, Name: "Broker uptime", Threshold: 99.9, Unit: "percent", Status: nfr.NFRStatusUnknown})
	return v
}

func (s *Server) handlePeerHeartbeat(w http.ResponseWriter, r *http.Request) {
	if !s.requireBrokerPermission(w, r) {
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var node state.Node
	if err := json.NewDecoder(r.Body).Decode(&node); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(node.ID) == "" {
		http.Error(w, "node id is required", http.StatusBadRequest)
		return
	}
	s.replicator.ApplyHeartbeat(node)
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handlePeerLogs(w http.ResponseWriter, r *http.Request) {
	if !s.requireBrokerPermission(w, r) {
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	logs, err := s.replicator.ExportLogs(r.Context(), state.ParsePeerSequence(r.URL.Query().Get("from")))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, logs)
}

func (s *Server) handlePeerSnapshot(w http.ResponseWriter, r *http.Request) {
	if !s.requireBrokerPermission(w, r) {
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	entries, err := s.replicator.ExportSnapshot(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, entries)
}

func (s *Server) requireBrokerPermission(w http.ResponseWriter, r *http.Request) bool {
	if strings.TrimSpace(s.validToken) == "" {
		return true
	}
	if header := strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")); header == s.validToken {
		return true
	}
	http.Error(w, "unauthorized", http.StatusUnauthorized)
	return false
}

func (s *Server) applyReplicatedStateChange(key string, value interface{}) {
	s.mu.Lock()
	defer s.mu.Unlock()
	switch {
	case strings.HasPrefix(key, "leases/"):
		if lease, ok := toLease(value); ok {
			s.leases[lease.ID] = lease
		}
	case strings.HasPrefix(key, "relays/"):
		if relay, ok := toRelay(value); ok {
			s.relays[relay.ID] = relay
		}
	}
}

func (s *Server) hydrateFromReplicatedState(ctx context.Context) {
	entries, err := s.replicator.List(ctx, "")
	if err != nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, entry := range entries {
		switch {
		case strings.HasPrefix(entry.Key, "leases/"):
			if lease, ok := toLease(entry.Value); ok {
				s.leases[lease.ID] = lease
			}
		case strings.HasPrefix(entry.Key, "relays/"):
			if relay, ok := toRelay(entry.Value); ok {
				s.relays[relay.ID] = relay
			}
		}
	}
}

func toLease(v interface{}) (protocol.Lease, bool) {
	var lease protocol.Lease
	if !decodeInto(v, &lease) || lease.ID == "" {
		return protocol.Lease{}, false
	}
	return lease, true
}

func toRelay(v interface{}) (protocol.RelayRegistration, bool) {
	var relay protocol.RelayRegistration
	if !decodeInto(v, &relay) || relay.ID == "" {
		return protocol.RelayRegistration{}, false
	}
	return relay, true
}

func decodeInto(src interface{}, dst interface{}) bool {
	data, err := json.Marshal(src)
	if err != nil {
		return false
	}
	return json.Unmarshal(data, dst) == nil
}
