package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"gotunnel/internal/protocol"
)

func (d *Daemon) pollTCPLoop(ctx context.Context, clientID, relayURL, localAddr string) {
	for {
		if ctx.Err() != nil {
			return
		}
		event, ok, err := d.nextTCPEvent(ctx, relayURL, clientID)
		if err != nil {
			select {
			case <-time.After(2 * time.Second):
				continue
			case <-ctx.Done():
				return
			}
		}
		if !ok {
			continue
		}
		d.handleTCPEvent(ctx, relayURL, clientID, localAddr, event, false, nil)
	}
}

func (d *Daemon) handleTCPEvent(ctx context.Context, relayURL, clientID, localAddr string, event protocol.TCPEvent, viaQUIC bool, send func(protocol.ControlMessage) error) {
	switch event.Type {
	case "open":
		conn, err := net.Dial("tcp", strings.TrimPrefix(localAddr, "tcp://"))
		if err != nil {
			if viaQUIC && send != nil {
				_ = send(protocol.ControlMessage{Type: "tcp_chunk", ClientID: clientID, TCPChunk: &protocol.TCPChunkRequest{ClientID: clientID, ConnectionID: event.ConnectionID, Close: true}})
			} else {
				_ = d.sendTCPChunk(ctx, relayURL, protocol.TCPChunkRequest{ClientID: clientID, ConnectionID: event.ConnectionID, Close: true})
			}
			return
		}
		d.tcpMu.Lock()
		d.tcpConns[event.ConnectionID] = conn
		d.tcpMu.Unlock()
		go d.readLocalTCP(ctx, relayURL, clientID, event.ConnectionID, conn, viaQUIC, send)
	case "data":
		d.tcpMu.Lock()
		conn := d.tcpConns[event.ConnectionID]
		d.tcpMu.Unlock()
		if conn != nil {
			_, _ = conn.Write(event.Payload)
		}
	case "close":
		d.closeTCPConn(event.ConnectionID)
	}
}

func (d *Daemon) nextTCPEvent(ctx context.Context, relayURL, clientID string) (protocol.TCPEvent, bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(relayURL, "/")+"/v1/client/tcp/next?client_id="+urlEncode(clientID), nil)
	if err != nil {
		return protocol.TCPEvent{}, false, err
	}
	resp, err := d.HTTP.Do(req)
	if err != nil {
		return protocol.TCPEvent{}, false, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNoContent {
		return protocol.TCPEvent{}, false, nil
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return protocol.TCPEvent{}, false, fmt.Errorf("poll tcp relay failed: %s", strings.TrimSpace(string(body)))
	}
	var event protocol.TCPEvent
	if err := json.NewDecoder(resp.Body).Decode(&event); err != nil {
		return protocol.TCPEvent{}, false, err
	}
	return event, true, nil
}

func (d *Daemon) readLocalTCP(ctx context.Context, relayURL, clientID, connectionID string, conn net.Conn, viaQUIC bool, send func(protocol.ControlMessage) error) {
	buf := make([]byte, 32*1024)
	for {
		n, err := conn.Read(buf)
		if n > 0 {
			chunk := protocol.TCPChunkRequest{
				ClientID:     clientID,
				ConnectionID: connectionID,
				Payload:      append([]byte(nil), buf[:n]...),
			}
			if viaQUIC && send != nil {
				_ = send(protocol.ControlMessage{Type: "tcp_chunk", ClientID: clientID, TCPChunk: &chunk})
			} else {
				_ = d.sendTCPChunk(ctx, relayURL, chunk)
			}
		}
		if err != nil {
			chunk := protocol.TCPChunkRequest{
				ClientID:     clientID,
				ConnectionID: connectionID,
				Close:        true,
			}
			if viaQUIC && send != nil {
				_ = send(protocol.ControlMessage{Type: "tcp_chunk", ClientID: clientID, TCPChunk: &chunk})
			} else {
				_ = d.sendTCPChunk(ctx, relayURL, chunk)
			}
			d.closeTCPConn(connectionID)
			return
		}
	}
}

func (d *Daemon) sendTCPChunk(ctx context.Context, relayURL string, chunk protocol.TCPChunkRequest) error {
	data, _ := json.Marshal(chunk)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(relayURL, "/")+"/v1/client/tcp/chunk", bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := d.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}

func (d *Daemon) closeTCPConn(connectionID string) {
	d.tcpMu.Lock()
	defer d.tcpMu.Unlock()
	if conn := d.tcpConns[connectionID]; conn != nil {
		_ = conn.Close()
		delete(d.tcpConns, connectionID)
	}
}

func urlEncode(in string) string {
	replacer := strings.NewReplacer(":", "%3A", "/", "%2F", "?", "%3F")
	return replacer.Replace(in)
}
