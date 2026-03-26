package domains

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"gotunnel/internal/dns"
	"gotunnel/internal/protocol"
)

type Provider interface {
	Name() string
	Ensure(ctx context.Context, domain string, metadata map[string]string) (protocol.DomainRecord, error)
}

type Registry struct {
	mu        sync.RWMutex
	providers map[string]Provider
}

func NewRegistry() *Registry {
	r := &Registry{providers: make(map[string]Provider)}
	r.Register(ManualProvider{})
	r.Register(MemoryProvider{})
	if dnsRegistry, _, err := dns.NewRegistryFromEnv(); err == nil {
		if provider, err := dnsRegistry.GetProvider("cloudflare"); err == nil {
			r.Register(DNSProviderAdapter{name: "cloudflare", provider: provider})
		}
		if provider, err := dnsRegistry.GetProvider("digitalocean"); err == nil {
			r.Register(DNSProviderAdapter{name: "digitalocean", provider: provider})
		}
	}
	return r
}

func (r *Registry) Register(p Provider) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.providers[p.Name()] = p
}

func (r *Registry) Ensure(ctx context.Context, req protocol.DomainRequest) (protocol.DomainRecord, error) {
	providerName := strings.TrimSpace(req.Provider)
	if providerName == "" {
		providerName = "manual"
	}
	r.mu.RLock()
	p := r.providers[providerName]
	r.mu.RUnlock()
	if p == nil {
		return protocol.DomainRecord{}, fmt.Errorf("unknown domain provider %q", providerName)
	}
	return p.Ensure(ctx, req.Domain, req.Metadata)
}

type ManualProvider struct{}

func (ManualProvider) Name() string { return "manual" }
func (ManualProvider) Ensure(_ context.Context, domain string, metadata map[string]string) (protocol.DomainRecord, error) {
	return protocol.DomainRecord{
		Domain:    domain,
		Provider:  "manual",
		Status:    "pending_dns_validation",
		Challenge: "_acme-challenge." + domain,
		Metadata:  metadata,
		UpdatedAt: time.Now().UTC(),
	}, nil
}

type MemoryProvider struct{}

func (MemoryProvider) Name() string { return "memory" }
func (MemoryProvider) Ensure(_ context.Context, domain string, metadata map[string]string) (protocol.DomainRecord, error) {
	return protocol.DomainRecord{
		Domain:    domain,
		Provider:  "memory",
		Status:    "verified",
		Metadata:  metadata,
		UpdatedAt: time.Now().UTC(),
	}, nil
}

type DNSProviderAdapter struct {
	name     string
	provider dns.Provider
}

func (p DNSProviderAdapter) Name() string { return p.name }

func (p DNSProviderAdapter) Ensure(ctx context.Context, domain string, metadata map[string]string) (protocol.DomainRecord, error) {
	if p.provider == nil {
		return protocol.DomainRecord{}, fmt.Errorf("dns provider is not configured")
	}
	zones, err := p.provider.GetZones(ctx)
	if err != nil {
		return protocol.DomainRecord{}, err
	}
	zone := dns.FindZone(domain, zones)
	if zone == nil {
		return protocol.DomainRecord{}, fmt.Errorf("no DNS zone found for %s", domain)
	}

	target := strings.TrimSpace(metadata["target"])
	status := "pending_dns_validation"
	if target != "" {
		recordName := dns.ExtractSubdomain(domain, zone.Name)
		recordType := dns.RecordTypeCNAME
		recordValue := target
		if recordName == "@" {
			recordType = dns.RecordTypeTXT
			recordValue = metadata["validation"]
			if strings.TrimSpace(recordValue) == "" {
				recordValue = "managed-by-gotunnel"
			}
		}
		record := dns.DNSRecord{
			Type:  recordType,
			Name:  recordName,
			Value: recordValue,
			TTL:   60,
		}
		if _, err := p.provider.CreateRecord(ctx, zone.Name, record); err != nil {
			return protocol.DomainRecord{}, err
		}
		status = "verified"
	}

	outMetadata := make(map[string]string, len(metadata)+1)
	for key, value := range metadata {
		outMetadata[key] = value
	}
	outMetadata["zone"] = zone.Name
	if token := strings.TrimSpace(os.Getenv(strings.ToUpper(p.name) + "_API_TOKEN")); token != "" {
		outMetadata["credential_source"] = "env"
	}
	return protocol.DomainRecord{
		Domain:    domain,
		Provider:  p.name,
		Status:    status,
		Challenge: "_acme-challenge." + domain,
		Metadata:  outMetadata,
		UpdatedAt: time.Now().UTC(),
	}, nil
}
