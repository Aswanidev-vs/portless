package relay

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"time"

	"gotunnel/internal/auth"
	"gotunnel/internal/plugins"
	"gotunnel/internal/protocol"
)

type phase3State struct {
	mu      sync.RWMutex
	relayID string
	plugins *plugins.Runtime
}

var phase3Registry sync.Map

func initPhase3State(s *Server, pluginConfig string) {
	runtime, err := plugins.Load(pluginConfig)
	if err != nil {
		runtime = &plugins.Runtime{}
	}
	phase3Registry.Store(s, &phase3State{plugins: runtime})
}

func p3(s *Server) *phase3State {
	state, _ := phase3Registry.Load(s)
	return state.(*phase3State)
}

func (s *Server) registrationLoop(ctx context.Context) {
	s.registerRelay()
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			s.registerRelay()
			s.sendHeartbeat()
		case <-ctx.Done():
			return
		}
	}
}

func (s *Server) registerRelay() {
	if s.brokerURL == "" {
		return
	}
	state := p3(s)
	reqBody := protocol.RelayRegistrationRequest{
		ID:         state.relayID,
		Name:       localFirstNonEmpty(strings.TrimSpace(s.relayName), "relay-"+strings.TrimLeft(s.addr, ":")),
		Region:     localFirstNonEmpty(strings.TrimSpace(s.relayRegion), "local"),
		RelayURL:   inferRelayURL(s),
		PublicBase: s.publicBase,
		Capacity:   s.capacity,
		Features:   state.plugins.FeatureNames(),
	}
	data, _ := json.Marshal(reqBody)
	resp, err := s.client.Post(strings.TrimRight(s.brokerURL, "/")+"/v1/relays/register", "application/json", bytes.NewReader(data))
	if err != nil {
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		return
	}
	var reg protocol.RelayRegistration
	if err := json.NewDecoder(resp.Body).Decode(&reg); err != nil {
		return
	}
	state.mu.Lock()
	state.relayID = reg.ID
	state.mu.Unlock()
}

func (s *Server) sendHeartbeat() {
	state := p3(s)
	state.mu.RLock()
	relayID := state.relayID
	state.mu.RUnlock()
	if relayID == "" || s.brokerURL == "" {
		return
	}
	s.mu.RLock()
	assigned := len(s.tunnels)
	s.mu.RUnlock()
	_ = s.coordinator.Heartbeat(context.Background(), relayID, localFirstNonEmpty(strings.TrimSpace(s.relayRegion), "local"), assigned)
	data, _ := json.Marshal(protocol.RelayHeartbeatRequest{
		ID:              relayID,
		AssignedTunnels: assigned,
	})
	_, _ = s.client.Post(strings.TrimRight(s.brokerURL, "/")+"/v1/relays/heartbeat", "application/json", bytes.NewReader(data))
}

func inferRelayURL(s *Server) string {
	if s.publicBase != "" {
		return strings.TrimRight(s.publicBase, "/")
	}
	if strings.HasPrefix(s.addr, ":") {
		return "http://127.0.0.1" + s.addr
	}
	return "http://" + s.addr
}

func localFirstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func (s *Server) handleRelayAuditExport(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.authorizePermission(w, r, auth.PermAuditExport); !ok {
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, protocol.AuditExport{
		GeneratedAt: time.Now().UTC(),
		Tunnels:     s.snapshot().Tunnels,
		Requests:    s.snapshot().Requests,
		Sessions:    s.snapshot().Sessions,
		Breakpoints: s.snapshot().Breakpoints,
		Events:      s.snapshot().Events,
	})
}
