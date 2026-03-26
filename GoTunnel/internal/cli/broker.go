package cli

import (
	"context"
	"flag"
	"fmt"
	"os/signal"
	"strings"
	"syscall"

	"gotunnel/internal/broker"
)

func runBroker(args []string) error {
	fs := flag.NewFlagSet("broker", flag.ContinueOnError)
	listenAddr := fs.String("listen", ":8090", "Broker listen address")
	relayURL := fs.String("relay-url", "http://127.0.0.1:8091", "Relay base URL returned to clients")
	publicBase := fs.String("public-base", "http://127.0.0.1:8091", "Debug ingress base URL")
	token := fs.String("token", "dev-token", "Static access token accepted by the broker")
	certFile := fs.String("tls-cert", "", "TLS certificate file for TLS 1.3 + HTTP/2")
	keyFile := fs.String("tls-key", "", "TLS key file for TLS 1.3 + HTTP/2")
	postgresDSN := fs.String("postgres-dsn", "", "Optional PostgreSQL DSN for durable broker state")
	stateDir := fs.String("state-dir", "", "Optional durable state directory for broker replication state")
	nodeID := fs.String("node-id", "", "Optional broker node identifier for replicated state")
	clusterNodes := fs.String("cluster-nodes", "", "Optional comma-separated broker peers as node-id=http://host:port")
	if err := fs.Parse(args); err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	server := broker.New(*listenAddr, *relayURL, *token, *publicBase, *certFile, *keyFile, *postgresDSN, *stateDir, *nodeID, splitCSV(*clusterNodes))
	fmt.Printf("broker listening on %s\n", *listenAddr)
	return server.ListenAndServe(ctx)
}

func splitCSV(value string) []string {
	var out []string
	for _, item := range strings.Split(value, ",") {
		if trimmed := strings.TrimSpace(item); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}
