package broker

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"gotunnel/internal/protocol"
)

func TestOpenSessionCreatesLease(t *testing.T) {
	srv := New(":0", "http://relay.local", "token-123", "http://127.0.0.1:8091", "", "", "", "", "", nil)

	reqBody := protocol.OpenSessionRequest{
		Token:              "token-123",
		Name:               "app",
		Protocol:           "http",
		LocalURL:           "http://127.0.0.1:3000",
		RequestedSubdomain: "app",
		InspectionEnabled:  true,
		HTTPSMode:          "auto",
	}
	data, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPost, "/v1/sessions/open", bytes.NewReader(data))
	rec := httptest.NewRecorder()

	srv.handleOpenSession(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusCreated)
	}

	var lease protocol.Lease
	if err := json.Unmarshal(rec.Body.Bytes(), &lease); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if lease.Subdomain != "app" {
		t.Fatalf("subdomain = %q, want app", lease.Subdomain)
	}
	if lease.DebugURL == "" {
		t.Fatal("expected debug URL to be populated")
	}
}

func TestSelectRelayPrefersRequestedRegion(t *testing.T) {
	srv := New(":0", "http://relay.local", "token-123", "http://127.0.0.1:8091", "", "", "", "", "", nil)
	srv.relays["relay-a"] = protocol.RelayRegistration{
		ID:              "relay-a",
		Region:          "us-east-1",
		RelayURL:        "http://east",
		LastHeartbeat:   time.Now().UTC(),
		AssignedTunnels: 3,
	}
	srv.relays["relay-b"] = protocol.RelayRegistration{
		ID:              "relay-b",
		Region:          "eu-west-1",
		RelayURL:        "http://eu",
		LastHeartbeat:   time.Now().UTC(),
		AssignedTunnels: 1,
	}

	selected := srv.selectRelay("eu-west-1")
	if selected == nil {
		t.Fatal("expected relay selection")
	}
	if selected.ID != "relay-b" {
		t.Fatalf("selected relay = %q, want relay-b", selected.ID)
	}
}
