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

type tcpRuntime struct {
	mu        sync.RWMutex
	queues    map[string]chan protocol.TCPEvent
	listeners map[string]net.Listener
	conns     map[string]net.Conn
}

var tcpRegistry sync.Map

func tcpState(s *Server) *tcpRuntime {
	state, ok := tcpRegistry.Load(s)
	if ok {
		return state.(*tcpRuntime)
	}
	runtime := &tcpRuntime{
		queues:    make(map[string]chan protocol.TCPEvent),
		listeners: make(map[string]net.Listener),
		conns:     make(map[string]net.Conn),
	}
	actual, _ := tcpRegistry.LoadOrStore(s, runtime)
	return actual.(*tcpRuntime)
}

func ensureTCPTunnelListener(s *Server, lease protocol.Lease, clientID string) (int, error) {
	state := tcpState(s)
	state.mu.Lock()
	defer state.mu.Unlock()
	if listener, ok := state.listeners[lease.ID]; ok {
		addr := listener.Addr().String()
		_, port, _ := net.SplitHostPort(addr)
		p, _ := strconv.Atoi(port)
		return p, nil
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	queue := make(chan protocol.TCPEvent, 256)
	state.listeners[lease.ID] = listener
	state.queues[clientID] = queue
	go acceptTCPConnections(s, lease, clientID, queue, listener)
	_, port, _ := net.SplitHostPort(listener.Addr().String())
	p, _ := strconv.Atoi(port)
	return p, nil
}

func acceptTCPConnections(s *Server, lease protocol.Lease, clientID string, queue chan protocol.TCPEvent, listener net.Listener) {
	for {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		connectionID := fmt.Sprintf("tcp-%d", time.Now().UnixNano())
		state := tcpState(s)
		state.mu.Lock()
		state.conns[connectionID] = conn
		state.mu.Unlock()
		sendTCPEventToClient(s, clientID, queue, protocol.TCPEvent{Type: "open", ConnectionID: connectionID, TunnelID: lease.ID})
		go readPublicTCP(s, clientID, connectionID, lease.ID, queue, conn)
	}
}

func readPublicTCP(s *Server, clientID, connectionID, tunnelID string, queue chan protocol.TCPEvent, conn net.Conn) {
	buf := make([]byte, 32*1024)
	for {
		n, err := conn.Read(buf)
		if n > 0 {
			sendTCPEventToClient(s, clientID, queue, protocol.TCPEvent{Type: "data", ConnectionID: connectionID, TunnelID: tunnelID, Payload: append([]byte(nil), buf[:n]...)})
		}
		if err != nil {
			sendTCPEventToClient(s, clientID, queue, protocol.TCPEvent{Type: "close", ConnectionID: connectionID, TunnelID: tunnelID})
			closeTCPPublicConn(s, connectionID)
			return
		}
	}
}

func (s *Server) handleNextTCPEvent(w http.ResponseWriter, r *http.Request) {
	clientID := r.URL.Query().Get("client_id")
	state := tcpState(s)
	state.mu.RLock()
	queue, ok := state.queues[clientID]
	state.mu.RUnlock()
	if !ok {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	select {
	case event := <-queue:
		s.metrics.IncCounter("gotunnel_relay_tcp_events_delivered_total")
		writeJSON(w, http.StatusOK, event)
	case <-time.After(25 * time.Second):
		w.WriteHeader(http.StatusNoContent)
	case <-r.Context().Done():
		w.WriteHeader(http.StatusRequestTimeout)
	}
}

func (s *Server) handleTCPChunk(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var chunk protocol.TCPChunkRequest
	if err := json.NewDecoder(r.Body).Decode(&chunk); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	state := tcpState(s)
	state.mu.RLock()
	conn := state.conns[chunk.ConnectionID]
	state.mu.RUnlock()
	if conn == nil {
		http.Error(w, "connection not found", http.StatusNotFound)
		return
	}
	if len(chunk.Payload) > 0 {
		if _, err := conn.Write(chunk.Payload); err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
	}
	if chunk.Close {
		closeTCPPublicConn(s, chunk.ConnectionID)
	}
	w.WriteHeader(http.StatusAccepted)
}

func closeTCPPublicConn(s *Server, connectionID string) {
	state := tcpState(s)
	state.mu.Lock()
	defer state.mu.Unlock()
	if conn := state.conns[connectionID]; conn != nil {
		_ = conn.Close()
		delete(state.conns, connectionID)
	}
}

func writeTCPChunkToPublic(s *Server, chunk protocol.TCPChunkRequest) {
	state := tcpState(s)
	state.mu.RLock()
	conn := state.conns[chunk.ConnectionID]
	state.mu.RUnlock()
	if conn == nil {
		return
	}
	if len(chunk.Payload) > 0 {
		_, _ = conn.Write(chunk.Payload)
	}
	if chunk.Close {
		closeTCPPublicConn(s, chunk.ConnectionID)
	}
}

func sendTCPEventToClient(s *Server, clientID string, queue chan protocol.TCPEvent, event protocol.TCPEvent) {
	client := s.findClientByID(clientID)
	if client != nil && client.QUICUp {
		select {
		case client.QUICSend <- protocol.ControlMessage{
			Type:     "tcp_event",
			ClientID: clientID,
			TCPEvent: &event,
		}:
			return
		default:
		}
	}
	queue <- event
}
