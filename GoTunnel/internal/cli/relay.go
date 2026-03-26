package cli

import (
	"context"
	"flag"
	"fmt"
	"os/signal"
	"syscall"

	"gotunnel/internal/relay"
)

func runRelay(args []string) error {
	fs := flag.NewFlagSet("relay", flag.ContinueOnError)
	listenAddr := fs.String("listen", ":8091", "Relay listen address")
	brokerURL := fs.String("broker-url", "http://127.0.0.1:8090", "Broker base URL")
	relayName := fs.String("name", "", "Relay name")
	relayRegion := fs.String("region", "local", "Relay region")
	capacity := fs.Int("capacity", 1000, "Relay tunnel capacity hint")
	dashboardUser := fs.String("dashboard-user", "admin", "Dashboard basic auth username")
	dashboardPass := fs.String("dashboard-pass", "admin", "Dashboard basic auth password")
	webhookURL := fs.String("webhook-url", "", "Optional webhook sink for lifecycle events")
	webhookSecret := fs.String("webhook-secret", "", "Optional shared secret for webhook signatures")
	pluginConfig := fs.String("plugin-config", "", "Optional JSON plugin manifest path")
	redisAddr := fs.String("redis-addr", "", "Optional Redis address for relay coordination")
	redisPassword := fs.String("redis-password", "", "Optional Redis password")
	redisDB := fs.Int("redis-db", 0, "Redis database index")
	certFile := fs.String("tls-cert", "", "TLS certificate file for TLS 1.3 + HTTP/2")
	keyFile := fs.String("tls-key", "", "TLS key file for TLS 1.3 + HTTP/2")
	quicAddr := fs.String("quic-listen", "", "Optional QUIC listener address")
	acmeCache := fs.String("acme-cache", "", "Enable Let's Encrypt autocert with a cache directory")
	acmeHosts := fs.String("acme-hosts", "", "Comma-separated hostnames allowed for autocert")
	acmeEmail := fs.String("acme-email", "", "Contact email for Let's Encrypt ACME registration")
	acmeStorage := fs.String("acme-storage", "", "Enable ACME lifecycle manager with a durable certificate storage directory")
	dnsProvider := fs.String("dns-provider", "", "DNS provider for ACME dns-01 challenge and managed custom domains")
	insecureSkipVerify := fs.Bool("insecure-skip-verify", false, "Skip certificate verification for broker/relay HTTP clients")
	publicBase := fs.String("public-base", "http://127.0.0.1:8091", "Public debug ingress base URL")
	allowMutation := fs.Bool("allow-mutation", false, "Allow replay requests to override path, headers, or body")
	if err := fs.Parse(args); err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	server := relay.New(*listenAddr, *brokerURL, *relayName, *relayRegion, *capacity, *dashboardUser, *dashboardPass, *webhookURL, *webhookSecret, *pluginConfig, *redisAddr, *redisPassword, *redisDB, *certFile, *keyFile, *quicAddr, *acmeCache, *acmeHosts, *acmeEmail, *acmeStorage, *dnsProvider, *publicBase, *allowMutation, *insecureSkipVerify)
	fmt.Printf("relay listening on %s\n", *listenAddr)
	return server.ListenAndServe(ctx)
}
