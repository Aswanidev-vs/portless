package relay

import (
	"testing"
	"time"

	"gotunnel/internal/protocol"
)

func TestCreateSessionBindsToTunnel(t *testing.T) {
	srv := New(":0", "http://broker.local", "relay", "local", 100, "admin", "admin", "", "", "", "", "", 0, "", "", "", "", "", "", "", "", "http://127.0.0.1:8091", false, false)
	srv.tunnels["app"] = &tunnelState{
		Lease: protocol.Lease{
			ID:           "lease-1",
			Name:         "app",
			Subdomain:    "app",
			AllowSharing: true,
			CreatedAt:    time.Now().UTC(),
		},
	}

	session, err := createSession(srv, protocol.CreateSessionRequest{
		Subdomain: "app",
		Owner:     "alice",
	})
	if err != nil {
		t.Fatalf("createSession() error = %v", err)
	}
	if session.TunnelID != "lease-1" {
		t.Fatalf("session tunnel id = %q, want lease-1", session.TunnelID)
	}
	if len(session.Participants) != 1 || session.Participants[0].Role != "owner" {
		t.Fatalf("unexpected participants: %+v", session.Participants)
	}
}

func TestMatchBreakpointMatchesMethodAndPath(t *testing.T) {
	srv := New(":0", "http://broker.local", "relay", "local", 100, "admin", "admin", "", "", "", "", "", 0, "", "", "", "", "", "", "", "", "http://127.0.0.1:8091", false, false)
	if _, err := addBreakpoint(srv, protocol.CreateBreakpointRequest{
		TunnelID:   "lease-1",
		Method:     "POST",
		PathPrefix: "/hooks",
		CreatedBy:  "alice",
	}); err != nil {
		t.Fatalf("addBreakpoint() error = %v", err)
	}

	breakpoint, ok := srv.matchBreakpoint(protocol.PendingRequest{
		TunnelID: "lease-1",
		Method:   "POST",
		Path:     "/hooks/github",
	})
	if !ok {
		t.Fatal("expected breakpoint to match")
	}
	if breakpoint.Method != "POST" {
		t.Fatalf("breakpoint method = %q, want POST", breakpoint.Method)
	}
}
