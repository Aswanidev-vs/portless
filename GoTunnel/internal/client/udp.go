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

func (d *Daemon) pollUDPLoop(ctx context.Context, clientID, relayURL, localAddr string) {
	local := strings.TrimPrefix(localAddr, "udp://")
	conn, err := net.ListenPacket("udp", "")
	if err != nil {
		return
	}
	defer conn.Close()

	backendAddr, err := net.ResolveUDPAddr("udp", local)
	if err != nil {
		return
	}

	for {
		if ctx.Err() != nil {
			return
		}
		event, ok, err := d.nextUDPEvent(ctx, relayURL, clientID)
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
		d.handleUDPEvent(ctx, relayURL, clientID, conn.(*net.UDPConn), backendAddr, event, false, nil)
	}
}

func (d *Daemon) handleUDPEvent(ctx context.Context, relayURL, clientID string, conn *net.UDPConn, backendAddr *net.UDPAddr, event protocol.UDPEvent, viaQUIC bool, send func(protocol.ControlMessage) error) {
	if event.Type != "data" {
		return
	}
	_, _ = conn.WriteToUDP(event.Payload, backendAddr)
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	buf := make([]byte, 64*1024)
	n, _, err := conn.ReadFrom(buf)
	if err == nil && n > 0 {
		chunk := protocol.UDPChunkRequest{
			ClientID:     clientID,
			ConnectionID: event.ConnectionID,
			PeerAddr:     event.PeerAddr,
			Payload:      append([]byte(nil), buf[:n]...),
		}
		if viaQUIC && send != nil {
			_ = send(protocol.ControlMessage{Type: "udp_chunk", ClientID: clientID, UDPChunk: &chunk})
		} else {
			_ = d.sendUDPChunk(ctx, relayURL, chunk)
		}
	}
}

func (d *Daemon) nextUDPEvent(ctx context.Context, relayURL, clientID string) (protocol.UDPEvent, bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(relayURL, "/")+"/v1/client/udp/next?client_id="+urlEncode(clientID), nil)
	if err != nil {
		return protocol.UDPEvent{}, false, err
	}
	resp, err := d.HTTP.Do(req)
	if err != nil {
		return protocol.UDPEvent{}, false, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNoContent {
		return protocol.UDPEvent{}, false, nil
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return protocol.UDPEvent{}, false, fmt.Errorf("poll udp relay failed: %s", strings.TrimSpace(string(body)))
	}
	var event protocol.UDPEvent
	if err := json.NewDecoder(resp.Body).Decode(&event); err != nil {
		return protocol.UDPEvent{}, false, err
	}
	return event, true, nil
}

func (d *Daemon) sendUDPChunk(ctx context.Context, relayURL string, chunk protocol.UDPChunkRequest) error {
	data, _ := json.Marshal(chunk)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(relayURL, "/")+"/v1/client/udp/chunk", bytes.NewReader(data))
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
