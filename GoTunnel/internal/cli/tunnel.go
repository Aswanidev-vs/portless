package cli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"gotunnel/internal/client"
	"gotunnel/internal/config"
	"gotunnel/internal/protocol"
)

func runTunnel(args []string) error {
	if len(args) == 0 {
		return errors.New("usage: gotunnel tunnel <start|share> ...")
	}

	switch args[0] {
	case "start":
		return runTunnelStart(args[1:])
	case "share":
		return runTunnelShare(args[1:])
	default:
		return errors.New("usage: gotunnel tunnel <start|share> ...")
	}
}

func runTunnelStart(args []string) error {
	fs := flag.NewFlagSet("tunnel start", flag.ContinueOnError)
	configPath := fs.String("file", "", "Load tunnel settings from GoTunnel YAML config")
	name := fs.String("name", "app", "Tunnel name")
	protocolName := fs.String("protocol", "http", "Tunnel protocol: http or tcp")
	localURL := fs.String("local-url", "http://127.0.0.1:3000", "Local HTTP service URL")
	region := fs.String("region", "", "Preferred relay region")
	subdomain := fs.String("subdomain", "", "Requested subdomain")
	brokerURL := fs.String("broker-url", "http://127.0.0.1:8090", "Broker base URL")
	token := fs.String("token", "", "Access token; defaults to saved token or GOTUNNEL_TOKEN")
	inspect := fs.Bool("inspect", true, "Enable request inspection")
	httpsMode := fs.String("https", "auto", "HTTPS mode")
	production := fs.Bool("production", false, "Mark the tunnel as production-sensitive")
	allowSharing := fs.Bool("allow-sharing", true, "Allow collaborative sharing for this tunnel")
	if err := fs.Parse(args); err != nil {
		return err
	}

	cfgTunnel, resolvedBrokerURL, resolvedToken, err := resolveTunnelConfig(*configPath, *name, *protocolName, *localURL, *region, *subdomain, *brokerURL, *token, *inspect, *httpsMode, *production, *allowSharing)
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	daemon := client.New(resolvedBrokerURL, resolvedToken)
	started, err := daemon.StartTunnel(ctx, cfgTunnel)
	if err != nil {
		return err
	}

	fmt.Printf("Tunnel %s active\n", cfgTunnel.Name)
	fmt.Printf("Public URL: %s\n", started.Lease.PublicURL)
	fmt.Printf("Debug URL: %s\n", started.Lease.DebugURL)
	fmt.Printf("Public Host: %s\n", started.Lease.PublicHost)
	fmt.Printf("Tunnel ID: %s\n", started.Lease.ID)
	if started.Lease.Protocol == "tcp" {
		fmt.Printf("TCP Endpoint: %s\n", started.Lease.PublicURL)
	}
	<-ctx.Done()
	return nil
}

func runTunnelShare(args []string) error {
	fs := flag.NewFlagSet("tunnel share", flag.ContinueOnError)
	relayURL := fs.String("relay-url", "http://127.0.0.1:8091", "Relay base URL")
	tunnelID := fs.String("tunnel-id", "", "Tunnel ID to share")
	subdomain := fs.String("subdomain", "", "Tunnel subdomain to share if tunnel ID is unknown")
	owner := fs.String("owner", "developer", "Owner name for the collaborative session")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*tunnelID) == "" && strings.TrimSpace(*subdomain) == "" {
		return errors.New("--tunnel-id or --subdomain is required")
	}

	body, _ := json.Marshal(protocol.CreateSessionRequest{
		TunnelID:  strings.TrimSpace(*tunnelID),
		Subdomain: strings.TrimSpace(*subdomain),
		Owner:     strings.TrimSpace(*owner),
	})
	resp, err := http.Post(strings.TrimRight(*relayURL, "/")+"/v1/sessions", "application/json", strings.NewReader(string(body)))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("share session failed: %s", resp.Status)
	}

	var session protocol.CollaborationSession
	if err := json.NewDecoder(resp.Body).Decode(&session); err != nil {
		return err
	}
	fmt.Printf("Session ID: %s\n", session.ID)
	fmt.Printf("Invite Token: %s\n", session.InviteToken)
	fmt.Printf("Invite URL: %s\n", session.InviteURL)
	return nil
}

func resolveTunnelConfig(configPath, name, protocolName, localURL, region, subdomain, brokerURL, token string, inspect bool, httpsMode string, production bool, allowSharing bool) (config.Tunnel, string, string, error) {
	if configPath != "" {
		cfg, err := config.Load(configPath)
		if err != nil {
			return config.Tunnel{}, "", "", err
		}
		resolvedToken := firstNonEmpty(token, cfg.Auth.Token, tokenFromEnv(), loadSavedToken())
		if strings.TrimSpace(resolvedToken) == "" {
			return config.Tunnel{}, "", "", fmt.Errorf("no token available; use --token, GOTUNNEL_TOKEN, or gotunnel login")
		}
		return cfg.Tunnels[0], cfg.Relay.BrokerURL, resolvedToken, nil
	}

	resolvedToken := firstNonEmpty(token, tokenFromEnv(), loadSavedToken())
	if strings.TrimSpace(resolvedToken) == "" {
		return config.Tunnel{}, "", "", fmt.Errorf("no token available; use --token, GOTUNNEL_TOKEN, or gotunnel login")
	}
	return config.Tunnel{
		Name:      name,
		Protocol:  protocolName,
		LocalURL:  localURL,
		Region:    region,
		Subdomain: subdomain,
		Inspect:   inspect,
		HTTPS:     httpsMode,
		Production: production,
		AllowSharing: &allowSharing,
	}, brokerURL, resolvedToken, nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func tokenFromEnv() string {
	return strings.TrimSpace(os.Getenv("GOTUNNEL_TOKEN"))
}
