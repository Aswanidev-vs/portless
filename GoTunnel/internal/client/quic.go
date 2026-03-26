package client

import (
	"bufio"
	"context"
	"encoding/json"
	"net"
	"strings"
	"time"

	"gotunnel/internal/config"
	"gotunnel/internal/protocol"
	"gotunnel/internal/transport"
)

func (d *Daemon) startQUICSession(ctx context.Context, relayAddr, clientID, reconnectToken string, tunnel config.Tunnel, tunnelID string) error {
	conn, err := transport.DialQUIC(ctx, transport.QUICOptions{
		Addr:               relayAddr,
		ServerName:         "localhost",
		InsecureSkipVerify: true,
	})
	if err != nil {
		return err
	}
	stream, err := conn.OpenStreamSync(ctx)
	if err != nil {
		return err
	}

	encoder := json.NewEncoder(stream)
	reader := bufio.NewReader(stream)
	send := make(chan protocol.ControlMessage, 256)

	if err := encoder.Encode(protocol.ControlMessage{
		Type:           "session_open",
		ClientID:       clientID,
		ReconnectToken: reconnectToken,
		Session: &protocol.TransportSession{
			ID:         "quic-" + clientID,
			ClientID:   clientID,
			TunnelID:   tunnelID,
			Mode:       protocol.SessionModeQUIC,
			Kind:       protocol.StreamKind(tunnel.Protocol),
			CreatedAt:  time.Now().UTC(),
			LastSeenAt: time.Now().UTC(),
		},
	}); err != nil {
		return err
	}

	go func() {
		for {
			select {
			case msg := <-send:
				_ = encoder.Encode(msg)
			case <-ctx.Done():
				_ = conn.CloseWithError(0, "")
				return
			}
		}
	}()

	if tunnel.Protocol == "udp" {
		go d.handleQUICUDP(ctx, relayAddr, clientID, tunnel, reader, send)
		return nil
	}
	go d.handleQUICControl(ctx, relayAddr, clientID, tunnel, reader, send)
	return nil
}

func (d *Daemon) handleQUICControl(ctx context.Context, relayAddr, clientID string, tunnel config.Tunnel, reader *bufio.Reader, send chan protocol.ControlMessage) {
	decoder := json.NewDecoder(reader)
	for {
		var msg protocol.ControlMessage
		if err := decoder.Decode(&msg); err != nil {
			return
		}
		switch msg.Type {
		case "http_request":
			if msg.Request == nil {
				continue
			}
			result := d.forwardToLocal(ctx, clientID, tunnel.LocalURL, *msg.Request)
			send <- protocol.ControlMessage{
				Type:     "http_response",
				ClientID: clientID,
				Response: &result,
			}
		case "tcp_event":
			if msg.TCPEvent == nil {
				continue
			}
			d.handleTCPEvent(ctx, relayAddr, clientID, tunnel.LocalURL, *msg.TCPEvent, true, func(m protocol.ControlMessage) error {
				send <- m
				return nil
			})
		}
	}
}

func (d *Daemon) handleQUICUDP(ctx context.Context, relayAddr, clientID string, tunnel config.Tunnel, reader *bufio.Reader, send chan protocol.ControlMessage) {
	conn, err := net.ListenPacket("udp", "")
	if err != nil {
		return
	}
	defer conn.Close()
	backendAddr, err := net.ResolveUDPAddr("udp", strings.TrimPrefix(tunnel.LocalURL, "udp://"))
	if err != nil {
		return
	}

	decoder := json.NewDecoder(reader)
	for {
		var msg protocol.ControlMessage
		if err := decoder.Decode(&msg); err != nil {
			return
		}
		if msg.Type == "udp_event" && msg.UDPEvent != nil {
			d.handleUDPEvent(ctx, relayAddr, clientID, conn.(*net.UDPConn), backendAddr, *msg.UDPEvent, true, func(m protocol.ControlMessage) error {
				send <- m
				return nil
			})
		}
	}
}
