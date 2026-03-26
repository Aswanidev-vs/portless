package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"gotunnel/internal/acme"
	"gotunnel/internal/auth"
	"gotunnel/internal/dns"
	"gotunnel/internal/security"
	"gotunnel/internal/state"
)

// Example: Using GoTunnel Enterprise Features Programmatically

func main() {
	ctx := context.Background()

	// 1. Initialize Authentication System
	fmt.Println("🔐 Initializing Authentication System...")
	authStore := auth.NewMemoryStore()
	authenticator := auth.NewAuthenticator(auth.Config{
		Store:    authStore,
		Issuer:   "gotunnel-example",
		Audience: "gotunnel-api",
	})

	// Create an admin user
	adminUser, err := authenticator.CreateUser(ctx, auth.CreateUserRequest{
		Username:    "admin",
		Email:       "admin@example.com",
		DisplayName: "Administrator",
		Password:    "secure-password-123",
		Roles:       []auth.Role{auth.RoleAdmin},
		WorkspaceID: "default",
	})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("✅ Created admin user: %s\n", adminUser.Username)

	// Enable MFA for the admin user
	mfaSetup, err := authenticator.EnableMFA(ctx, adminUser.ID, "totp")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("✅ MFA enabled. Secret: %s\n", mfaSetup.Secret)
	fmt.Printf("   Backup codes: %v\n", mfaSetup.BackupCodes)

	// 2. Initialize DNS Providers
	fmt.Println("\n🌐 Initializing DNS Providers...")
	dnsRegistry := dns.NewRegistry()

	// Register Cloudflare provider
	dnsRegistry.RegisterProvider("cloudflare", dns.NewMemoryProvider(), dns.ProviderConfig{
		Name:    "cloudflare",
		Type:    "cloudflare",
		Enabled: true,
		Zones:   []string{"example.com"},
	})

	// Register Route53 provider
	dnsRegistry.RegisterProvider("route53", dns.NewMemoryProvider(), dns.ProviderConfig{
		Name:    "route53",
		Type:    "route53",
		Enabled: true,
		Zones:   []string{"example.com"},
	})

	fmt.Println("✅ DNS providers registered")

	// 3. Initialize ACME Certificate Manager
	fmt.Println("\n🔒 Initializing ACME Certificate Manager...")
	acmeManager, err := acme.NewLifecycleManager(acme.LifecycleConfig{
		Email:         "admin@example.com",
		DirectoryURL:  "https://acme-staging-v02.api.letsencrypt.org/directory",
		RenewalWindow: 30 * 24 * time.Hour,
		MaxRetries:    5,
	})
	if err != nil {
		log.Fatal(err)
	}

	// Register DNS-01 challenge handler
	dnsHandler := &DNSChallengeHandler{
		Registry: dnsRegistry,
		Provider: "cloudflare",
	}
	acmeManager.RegisterChallengeHandler(dnsHandler)

	fmt.Println("✅ ACME manager initialized")

	// 4. Initialize Security Enforcer
	fmt.Println("\n🛡️  Initializing Security Enforcer...")
	securityEnforcer := security.NewEnforcer(security.EnforcerConfig{
		NetworkPolicy: security.NetworkPolicy{
			AllowedCIDRs:   []string{"10.0.0.0/8", "172.16.0.0/12"},
			BlockedCIDRs:   []string{"1.2.3.0/24"},
			MaxConnections: 10000,
		},
		TLSPolicy: security.TLSPolicy{
			MinVersion: 0x0304, // TLS 1.3
			CipherSuites: []uint16{
				0x1301, // TLS_AES_128_GCM_SHA256
				0x1302, // TLS_AES_256_GCM_SHA384
				0x1303, // TLS_CHACHA20_POLY1305_SHA256
			},
		},
		MaxAuditLog: 10000,
	})

	// Add a security policy
	securityEnforcer.AddPolicy(&security.Policy{
		ID:      "policy-1",
		Name:    "Block Suspicious IPs",
		Type:    security.PolicyTypeNetwork,
		Action:  security.PolicyActionDeny,
		Enabled: true,
		Rules: []security.Rule{
			{
				Field:    "source_ip",
				Operator: "prefix",
				Value:    "1.2.3.",
			},
		},
	})

	fmt.Println("✅ Security enforcer initialized")

	// 5. Initialize State Replication
	fmt.Println("\n🔄 Initializing State Replication...")
	stateStore := state.NewMemoryStore()
	replicator := state.NewReplicator(state.Config{
		NodeID:            "node-1",
		Nodes:             []string{"node-1", "node-2", "node-3"},
		HeartbeatInterval: 1 * time.Second,
		ElectionTimeout:   5 * time.Second,
	}, stateStore)

	// Set state change callback
	replicator.OnStateChange(func(key string, value interface{}) {
		fmt.Printf("   State changed: %s = %v\n", key, value)
	})

	// Set failover callback
	replicator.OnFailover(func(newLeader string) {
		fmt.Printf("   Failover occurred. New leader: %s\n", newLeader)
	})

	fmt.Println("✅ State replication initialized")

	// 6. Demonstrate Usage
	fmt.Println("\n📊 Demonstrating Features...")

	// Authenticate a user
	session, err := authenticator.AuthenticateWithPassword(ctx, "admin", "secure-password-123")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("✅ User authenticated. Session: %s\n", session.Token[:16]+"...")

	// Check permissions
	hasPermission := auth.HasPermission(adminUser, auth.PermTunnelCreate)
	fmt.Printf("   Has tunnel:create permission: %v\n", hasPermission)

	// Enforce network policy
	err = securityEnforcer.EnforceNetworkPolicy(ctx, "10.0.1.100:12345")
	if err != nil {
		fmt.Printf("   Network policy blocked: %v\n", err)
	} else {
		fmt.Println("   Network policy allowed connection")
	}

	// Set replicated state
	err = replicator.Set(ctx, "tunnel:web:status", "active", 0)
	if err != nil {
		log.Fatal(err)
	}

	// Get replicated state
	entry, err := replicator.Get(ctx, "tunnel:web:status")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("   Replicated state: %s = %v\n", entry.Key, entry.Value)

	// Get system health
	nodes := replicator.GetNodes()
	fmt.Printf("   Cluster nodes: %d\n", len(nodes))

	// Get security audit log
	auditLog := securityEnforcer.GetAuditLog()
	fmt.Printf("   Security events: %d\n", len(auditLog))

	fmt.Println("\n✅ All enterprise features demonstrated successfully!")
	fmt.Println("\n📚 For more examples, see:")
	fmt.Println("   - examples/basic-http.yaml")
	fmt.Println("   - examples/https-auto-cert.yaml")
	fmt.Println("   - examples/enterprise-auth.yaml")
	fmt.Println("   - examples/docker-compose.yaml")
}

// DNSChallengeHandler implements ACME DNS-01 challenge handling
type DNSChallengeHandler struct {
	Registry *dns.Registry
	Provider string
}

func (h *DNSChallengeHandler) Type() string {
	return "dns-01"
}

func (h *DNSChallengeHandler) HandleChallenge(ctx context.Context, domain, token string) error {
	return h.Registry.CreateChallenge(ctx, h.Provider, domain, token, token)
}

func (h *DNSChallengeHandler) CleanupChallenge(ctx context.Context, domain, token string) error {
	return h.Registry.CleanupChallenge(ctx, h.Provider, domain, token)
}

// MemoryStore implementations for examples
type memoryStore struct {
	data map[string]interface{}
}

func newMemoryStore() *memoryStore {
	return &memoryStore{data: make(map[string]interface{})}
}
