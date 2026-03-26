package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadExpandsEnvironmentVariables(t *testing.T) {
	t.Setenv("GOTUNNEL_TOKEN", "secret-token")

	dir := t.TempDir()
	path := filepath.Join(dir, "gotunnel.yaml")
	content := []byte(`
version: 1
auth:
  token: env:GOTUNNEL_TOKEN
relay:
  broker_url: http://127.0.0.1:8090
tunnels:
  - name: app
    protocol: http
    local_url: http://127.0.0.1:3000
`)
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Auth.Token != "secret-token" {
		t.Fatalf("expected expanded token, got %q", cfg.Auth.Token)
	}
}

func TestValidateRejectsUnsupportedProtocol(t *testing.T) {
	cfg := &Config{
		Version: 1,
		Relay: RelayConfig{
			BrokerURL: "http://127.0.0.1:8090",
		},
		Tunnels: []Tunnel{{
			Name:     "db",
			Protocol: "icmp",
			LocalURL: "tcp://127.0.0.1:5432",
		}},
	}

	if err := cfg.Validate(); err == nil {
		t.Fatal("expected validation error for unsupported protocol")
	}
}
