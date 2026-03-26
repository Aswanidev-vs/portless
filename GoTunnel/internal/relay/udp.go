package relay

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"

	"gotunnel/internal/protocol"
)

type udpRuntime struct {
	mu        sync.RWMutex
	queues    map[string]chan protocol.UDPEvent
	sockets   map[string]net.PacketConn
	peers     map[string]net.Addr
}

var udpRegistry sync.Map

func udpState(s *Server) *udpRuntime {
	state, ok := udpRegistry.Load(s)
	if ok {
		return state.(*udpRuntime)
	}
	runtime := &udpRuntime{
		queues:  make(map[string]chan protocol.UDPEvent),
		sockets: make(map[string]net.PacketConn),
		peers:   make(map[string]net.Addr),
	}
	actual, _ := udpRegistry.LoadOrStore(s, runtime)
	return actual.(*udpRuntime)
}

func ensureUDPTunnelListener(s *Server, lease protocol.Lease, clientID string) (int, error) {
	state := udpState(s)
	state.mu.Lock()
	defer state.mu.Unlock()
	if socket, ok := state.sockets[lease.ID]; ok {
		_, port, _ := net.SplitHostPort(socket.LocalAddr().String())
		p, _ := strconv.Atoi(port)
		return p, nil
	}
	socket, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	queue := make(chan protocol.UDPEvent, 256)
	state.sockets[lease.ID] = socket
	state.queues[clientID] = queue
	go readPublicUDP(s, lease, clientID, queue, socket)
	_, port, _ := net.SplitHostPort(socket.LocalAddr().String())
	p, _ := strconv.Atoi(port)
	return p, nil
}

func readPublicUDP(s *Server, lease protocol.Lease, clientID string, queue chan protocol.UDPEvent, socket net.PacketConn) {
	buf := make([]byte, 64*1024)
	for {
		n, addr, err := socket.ReadFrom(buf)
		if err != nil {
			return
		}
		connectionID := fmt.Sprintf("udp-%s-%d", lease.ID, time.Now().UnixNano())
		state := udpState(s)
		state.mu.Lock()
		state.peers[connectionID] = addr
		state.mu.Unlock()
		sendUDPEventToClient(s, clientID, queue, protocol.UDPEvent{
			Type:         "data",
			ConnectionID: connectionID,
			TunnelID:     lease.ID,
			PeerAddr:     addr.String(),
			Payload:      append([]byte(nil), buf[:n]...),
		})
	}
}

func (s *Server) handleNextUDPEvent(w http.ResponseWriter, r *http.Request) {
	clientID := r.URL.Query().Get("client_id")
	state := udpState(s)
	state.mu.RLock()
	queue, ok := state.queues[clientID]
	state.mu.RUnlock()
	if !ok {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	select {
	case event := <-queue:
		s.metrics.IncCounter("gotunnel_relay_udp_events_delivered_total")
		writeJSON(w, http.StatusOK, event)
	case <-time.After(25 * time.Second):
		w.WriteHeader(http.StatusNoContent)
	case <-r.Context().Done():
		w.WriteHeader(http.StatusRequestTimeout)
	}
}

func (s *Server) handleUDPChunk(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var chunk protocol.UDPChunkRequest
	if err := json.NewDecoder(r.Body).Decode(&chunk); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	state := udpState(s)
	state.mu.RLock()
	peer := state.peers[chunk.ConnectionID]
	var socket net.PacketConn
	for _, candidate := range state.sockets {
		socket = candidate
		break
	}
	state.mu.RUnlock()
	if socket == nil || peer == nil {
		http.Error(w, "udp connection not found", http.StatusNotFound)
		return
	}
	if len(chunk.Payload) > 0 {
		if _, err := socket.WriteTo(chunk.Payload, peer); err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
	}
	w.WriteHeader(http.StatusAccepted)
}

func writeUDPChunkToPublic(s *Server, chunk protocol.UDPChunkRequest) {
	state := udpState(s)
	state.mu.RLock()
	peer := state.peers[chunk.ConnectionID]
	var socket net.PacketConn
	for _, candidate := range state.sockets {
		socket = candidate
		break
	}
	state.mu.RUnlock()
	if socket == nil || peer == nil {
		return
	}
	if len(chunk.Payload) > 0 {
		_, _ = socket.WriteTo(chunk.Payload, peer)
	}
}

func sendUDPEventToClient(s *Server, clientID string, queue chan protocol.UDPEvent, event protocol.UDPEvent) {
	client := s.findClientByID(clientID)
	if client != nil && client.QUICUp {
		select {
		case client.QUICSend <- protocol.ControlMessage{
			Type:     "udp_event",
			ClientID: clientID,
			UDPEvent: &event,
		}:
			return
		default:
		}
	}
	queue <- event
}
