package state

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type HTTPPeerTransport struct {
	client      *http.Client
	peerBaseURL map[string]string
	token       string
}

func NewHTTPPeerTransport(peers []string, token string) *HTTPPeerTransport {
	baseURLs := make(map[string]string)
	for _, peer := range peers {
		trimmed := strings.TrimSpace(peer)
		if trimmed == "" {
			continue
		}
		id := peerNodeID(trimmed)
		baseURL := peerBaseURL(trimmed)
		if id == "" || baseURL == "" {
			continue
		}
		baseURLs[id] = baseURL
	}
	return &HTTPPeerTransport{
		client:      &http.Client{Timeout: 5 * time.Second},
		peerBaseURL: baseURLs,
		token:       strings.TrimSpace(token),
	}
}

func (t *HTTPPeerTransport) BroadcastHeartbeat(ctx context.Context, self Node) error {
	var firstErr error
	for id, baseURL := range t.peerBaseURL {
		if id == self.ID {
			continue
		}
		if err := t.postJSON(ctx, baseURL+"/v1/state/peer/heartbeat", self, nil); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func (t *HTTPPeerTransport) FetchLogs(ctx context.Context, peer Node, fromSequence int64) ([]ReplicationLog, error) {
	var logs []ReplicationLog
	err := t.getJSON(ctx, peer, fmt.Sprintf("/v1/state/peer/logs?from=%d", fromSequence), &logs)
	return logs, err
}

func (t *HTTPPeerTransport) FetchSnapshot(ctx context.Context, peer Node) ([]StateEntry, error) {
	var snapshot []StateEntry
	err := t.getJSON(ctx, peer, "/v1/state/peer/snapshot", &snapshot)
	return snapshot, err
}

func (t *HTTPPeerTransport) getJSON(ctx context.Context, peer Node, path string, dst interface{}) error {
	baseURL := strings.TrimRight(peer.Address, "/")
	if mapped, ok := t.peerBaseURL[peer.ID]; ok && strings.TrimSpace(mapped) != "" {
		baseURL = mapped
	}
	if baseURL == "" {
		return fmt.Errorf("peer %s address not configured", peer.ID)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+path, nil)
	if err != nil {
		return err
	}
	if t.token != "" {
		req.Header.Set("Authorization", "Bearer "+t.token)
	}
	resp, err := t.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("peer %s returned %d", peer.ID, resp.StatusCode)
	}
	if err := json.NewDecoder(resp.Body).Decode(dst); err != nil {
		return err
	}
	return nil
}

func (t *HTTPPeerTransport) postJSON(ctx context.Context, endpoint string, body interface{}, dst interface{}) error {
	payload, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(string(payload)))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if t.token != "" {
		req.Header.Set("Authorization", "Bearer "+t.token)
	}
	resp, err := t.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("peer endpoint returned %d", resp.StatusCode)
	}
	if dst != nil {
		if err := json.NewDecoder(resp.Body).Decode(dst); err != nil {
			return err
		}
	}
	return nil
}

func peerNodeID(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}
	if strings.Contains(trimmed, "=") {
		parts := strings.SplitN(trimmed, "=", 2)
		return strings.TrimSpace(parts[0])
	}
	if parsed, err := url.Parse(trimmed); err == nil && parsed.Host != "" {
		return parsed.Host
	}
	return trimmed
}

func peerBaseURL(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}
	if strings.Contains(trimmed, "=") {
		parts := strings.SplitN(trimmed, "=", 2)
		return strings.TrimRight(strings.TrimSpace(parts[1]), "/")
	}
	if strings.Contains(trimmed, "://") {
		return strings.TrimRight(trimmed, "/")
	}
	return ""
}

func ParsePeerSequence(raw string) int64 {
	value, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	if err != nil {
		return 0
	}
	return value
}
