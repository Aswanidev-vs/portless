package relay

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"strings"
	"sync"

	"github.com/quic-go/quic-go"

	"gotunnel/internal/protocol"
	"gotunnel/internal/transport"
)

func (s *Server) runQUICListener(ctx context.Context) {
	if s.quicAddr == "" || s.certFile == "" || s.keyFile == "" {
		return
	}
	listener, err := transport.ListenQUIC(transport.QUICOptions{
		Addr:     s.quicAddr,
		CertFile: s.certFile,
		KeyFile:  s.keyFile,
	})
	if err != nil {
		return
	}
	go func() {
		<-ctx.Done()
		_ = listener.Close()
	}()

	for {
		conn, err := listener.Accept(ctx)
		if err != nil {
			return
		}
		s.metrics.IncCounter("gotunnel_relay_quic_sessions_total")
		go s.handleQUICConn(ctx, conn)
	}
}

func (s *Server) handleQUICConn(ctx context.Context, conn *quic.Conn) {
	defer conn.CloseWithError(0, "")
	stream, err := conn.AcceptStream(ctx)
	if err != nil {
		return
	}
	s.metrics.IncCounter("gotunnel_relay_quic_streams_total")

	reader := bufio.NewReader(stream)
	line, err := reader.ReadBytes('\n')
	if err != nil {
		return
	}
	var hello protocol.ControlMessage
	if err := json.Unmarshal(line, &hello); err != nil || hello.Type != "session_open" || hello.Session == nil {
		return
	}

	clientID := hello.Session.ClientID
	if hello.ReconnectToken != "" {
		if resumed := s.resolveReconnectToken(hello.ReconnectToken); resumed != "" {
			clientID = resumed
		}
	}
	clientState := s.findClientByID(clientID)
	if clientState == nil {
		return
	}
	clientState.QUICUp = true
	var once sync.Once
	defer once.Do(func() { clientState.QUICUp = false })

	done := make(chan struct{})
	go func() {
		defer close(done)
		encoder := json.NewEncoder(stream)
		for {
			select {
			case msg := <-clientState.QUICSend:
				if err := encoder.Encode(msg); err != nil {
					return
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	decoder := json.NewDecoder(reader)
	for {
		var msg protocol.ControlMessage
		if err := decoder.Decode(&msg); err != nil {
			if err != io.EOF {
			}
			<-done
			return
		}
		s.handleQUICMessage(msg)
	}
}

func (s *Server) handleQUICMessage(msg protocol.ControlMessage) {
	switch msg.Type {
	case "http_response":
		if msg.Response == nil {
			return
		}
		s.mu.RLock()
		state, ok := s.requests[msg.Response.RequestID]
		s.mu.RUnlock()
		if ok {
			select {
			case state.ResponseC <- *msg.Response:
			default:
			}
		}
	case "tcp_chunk":
		if msg.TCPChunk != nil {
			writeTCPChunkToPublic(s, *msg.TCPChunk)
		}
	case "udp_chunk":
		if msg.UDPChunk != nil {
			writeUDPChunkToPublic(s, *msg.UDPChunk)
		}
	}
}

func (s *Server) findClientByID(clientID string) *clientState {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.clients[clientID]
}

func (s *Server) resolveReconnectToken(token string) string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.reconnectTokens[token]
}

func splitCSV(in string) []string {
	parts := strings.Split(in, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}
