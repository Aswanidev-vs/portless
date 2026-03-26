package relay

import (
	"testing"
	"time"

	"gotunnel/internal/protocol"
)

func TestProductionPolicyBlocksSharingWhenDisabled(t *testing.T) {
	srv := New(":0", "http://broker.local", "relay", "local", 100, "admin", "admin", "", "", "", "", "", 0, "", "", "", "", "", "", "", "", "http://127.0.0.1:8091", false, false)
	srv.tunnels["prod"] = &tunnelState{
		Lease: protocol.Lease{
			ID:           "lease-prod",
			Name:         "prod",
			Subdomain:    "prod",
			Production:   true,
			AllowSharing: false,
			CreatedAt:    time.Now().UTC(),
		},
	}

	_, err := createSession(srv, protocol.CreateSessionRequest{
		Subdomain: "prod",
		Owner:     "alice",
	})
	if err == nil {
		t.Fatal("expected sharing policy rejection")
	}
}
