package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"gotunnel/internal/config"
	"gotunnel/internal/protocol"
	"gotunnel/internal/telemetry"
	"gotunnel/internal/transport"
)

type Daemon struct {
	BrokerURL string
	Token     string
	HTTP      *http.Client
	tcpMu     sync.Mutex
	tcpConns  map[string]net.Conn
}

type StartedTunnel struct {
	ClientID string
	Lease    protocol.Lease
}

func New(brokerURL, token string) *Daemon {
	telemetry.Init("gotunnel-client")
	return &Daemon{
		BrokerURL: strings.TrimRight(brokerURL, "/"),
		Token:     token,
		HTTP:      transport.NewHTTPClient(false),
		tcpConns:  make(map[string]net.Conn),
	}
}

func (d *Daemon) StartTunnel(ctx context.Context, tunnel config.Tunnel) (*StartedTunnel, error) {
	lease, err := d.openSession(ctx, tunnel)
	if err != nil {
		return nil, err
	}
	started, err := d.registerWithRelay(ctx, lease)
	if err != nil {
		return nil, err
	}

	if started.QUICAddr != "" {
		if err := d.startQUICSession(ctx, started.QUICAddr, started.ClientID, started.ReconnectToken, tunnel, started.Lease.ID); err == nil {
			return &StartedTunnel{
				ClientID: started.ClientID,
				Lease:    started.Lease,
			}, nil
		}
	}

	if tunnel.Protocol == "tcp" {
		go d.pollTCPLoop(ctx, started.ClientID, started.Lease.RelayURL, tunnel.LocalURL)
	} else if tunnel.Protocol == "udp" {
		go d.pollUDPLoop(ctx, started.ClientID, started.Lease.RelayURL, tunnel.LocalURL)
	} else {
		go d.pollLoop(ctx, started.ClientID, started.Lease.RelayURL, tunnel.LocalURL)
	}

	return &StartedTunnel{
		ClientID: started.ClientID,
		Lease:    started.Lease,
	}, nil
}

func (d *Daemon) openSession(ctx context.Context, tunnel config.Tunnel) (protocol.Lease, error) {
	ctx, span := telemetry.Start(ctx, "gotunnel/client", "open_session")
	defer span.End()
	reqBody := protocol.OpenSessionRequest{
		Token:              d.Token,
		Name:               tunnel.Name,
		Protocol:           tunnel.Protocol,
		RequestedSubdomain: tunnel.Subdomain,
		RequestedRegion:    tunnel.Region,
		LocalURL:           tunnel.LocalURL,
		InspectionEnabled:  tunnel.Inspect,
		HTTPSMode:          tunnel.HTTPS,
		WebhookURL:         tunnel.WebhookURL,
		CustomDomain:       tunnel.CustomDomain,
		Production:         tunnel.Production,
		Labels:             append([]string(nil), tunnel.Labels...),
	}
	if tunnel.AllowSharing != nil {
		reqBody.AllowSharing = *tunnel.AllowSharing
	}
	data, _ := json.Marshal(reqBody)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, d.BrokerURL+"/v1/sessions/open", bytes.NewReader(data))
	if err != nil {
		return protocol.Lease{}, fmt.Errorf("build broker request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := d.HTTP.Do(req)
	if err != nil {
		return protocol.Lease{}, fmt.Errorf("open session: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return protocol.Lease{}, fmt.Errorf("open session failed: %s", strings.TrimSpace(string(body)))
	}

	var lease protocol.Lease
	if err := json.NewDecoder(resp.Body).Decode(&lease); err != nil {
		return protocol.Lease{}, fmt.Errorf("decode lease: %w", err)
	}
	return lease, nil
}

func (d *Daemon) registerWithRelay(ctx context.Context, lease protocol.Lease) (*protocol.RegisterTunnelResponse, error) {
	ctx, span := telemetry.Start(ctx, "gotunnel/client", "register_with_relay")
	defer span.End()
	data, _ := json.Marshal(protocol.RegisterTunnelRequest{LeaseID: lease.ID})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(lease.RelayURL, "/")+"/v1/client/register", bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("build relay registration request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := d.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("register with relay: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("register with relay failed: %s", strings.TrimSpace(string(body)))
	}

	var out protocol.RegisterTunnelResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("decode relay response: %w", err)
	}
	if out.Lease.Protocol == "tcp" && out.TCPPort > 0 {
		out.Lease.PublicURL = fmt.Sprintf("tcp://127.0.0.1:%d", out.TCPPort)
	}
	if out.Lease.Protocol == "udp" && out.TCPPort > 0 {
		out.Lease.PublicURL = fmt.Sprintf("udp://127.0.0.1:%d", out.TCPPort)
	}
	return &out, nil
}

func (d *Daemon) pollLoop(ctx context.Context, clientID, relayURL, localBase string) {
	for {
		if ctx.Err() != nil {
			return
		}

		pending, ok, err := d.nextRequest(ctx, relayURL, clientID)
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

		result := d.forwardToLocal(ctx, clientID, localBase, pending)
		_ = d.submitResponse(ctx, relayURL, result)
	}
}

func (d *Daemon) nextRequest(ctx context.Context, relayURL, clientID string) (protocol.PendingRequest, bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(relayURL, "/")+"/v1/client/next?client_id="+url.QueryEscape(clientID), nil)
	if err != nil {
		return protocol.PendingRequest{}, false, err
	}

	resp, err := d.HTTP.Do(req)
	if err != nil {
		return protocol.PendingRequest{}, false, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNoContent {
		return protocol.PendingRequest{}, false, nil
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return protocol.PendingRequest{}, false, fmt.Errorf("poll relay failed: %s", strings.TrimSpace(string(body)))
	}

	var pending protocol.PendingRequest
	if err := json.NewDecoder(resp.Body).Decode(&pending); err != nil {
		return protocol.PendingRequest{}, false, err
	}
	return pending, true, nil
}

func (d *Daemon) forwardToLocal(ctx context.Context, clientID, localBase string, pending protocol.PendingRequest) protocol.SubmitResponseRequest {
	result := protocol.SubmitResponseRequest{
		ClientID:  clientID,
		RequestID: pending.ID,
		Headers:   make(map[string][]string),
	}

	target, err := buildTargetURL(localBase, pending.Path, pending.Query)
	if err != nil {
		result.StatusCode = http.StatusBadGateway
		result.Error = err.Error()
		return result
	}

	req, err := http.NewRequestWithContext(ctx, pending.Method, target, bytes.NewReader(pending.Body))
	if err != nil {
		result.StatusCode = http.StatusBadGateway
		result.Error = err.Error()
		return result
	}
	for key, values := range pending.Headers {
		if strings.EqualFold(key, "Host") {
			continue
		}
		for _, value := range values {
			req.Header.Add(key, value)
		}
	}

	resp, err := d.HTTP.Do(req)
	if err != nil {
		result.StatusCode = http.StatusBadGateway
		result.Error = err.Error()
		return result
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	result.StatusCode = resp.StatusCode
	result.Headers = cloneHeaders(resp.Header)
	result.Body = body
	return result
}

func (d *Daemon) submitResponse(ctx context.Context, relayURL string, result protocol.SubmitResponseRequest) error {
	data, _ := json.Marshal(result)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(relayURL, "/")+"/v1/client/respond", bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := d.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("submit response failed: %s", strings.TrimSpace(string(body)))
	}
	return nil
}

func buildTargetURL(localBase, path, query string) (string, error) {
	base, err := url.Parse(localBase)
	if err != nil {
		return "", fmt.Errorf("parse local url: %w", err)
	}
	if path == "" {
		path = "/"
	}
	base.Path = strings.TrimRight(base.Path, "/") + path
	base.RawQuery = query
	return base.String(), nil
}

func cloneHeaders(h http.Header) map[string][]string {
	out := make(map[string][]string, len(h))
	for key, values := range h {
		out[key] = append([]string(nil), values...)
	}
	return out
}
