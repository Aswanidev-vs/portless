package dns

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
)

// RecordType represents DNS record types
type RecordType string

const (
	RecordTypeA     RecordType = "A"
	RecordTypeAAAA  RecordType = "AAAA"
	RecordTypeCNAME RecordType = "CNAME"
	RecordTypeTXT   RecordType = "TXT"
	RecordTypeMX    RecordType = "MX"
	RecordTypeNS    RecordType = "NS"
)

// DNSRecord represents a DNS record
type DNSRecord struct {
	Type      RecordType `json:"type"`
	Name      string     `json:"name"`
	Value     string     `json:"value"`
	TTL       int        `json:"ttl"`
	Priority  int        `json:"priority,omitempty"`
	ZoneID    string     `json:"zone_id,omitempty"`
	RecordID  string     `json:"record_id,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
}

// ChallengeRecord represents an ACME challenge DNS record
type ChallengeRecord struct {
	Domain    string    `json:"domain"`
	Token     string    `json:"token"`
	KeyAuth   string    `json:"key_auth"`
	FQDN      string    `json:"fqdn"`
	Value     string    `json:"value"`
	CreatedAt time.Time `json:"created_at"`
}

// Provider defines the interface for DNS providers
type Provider interface {
	Name() string
	CreateRecord(ctx context.Context, zone string, record DNSRecord) (DNSRecord, error)
	UpdateRecord(ctx context.Context, zone string, record DNSRecord) error
	DeleteRecord(ctx context.Context, zone, recordID string) error
	GetRecords(ctx context.Context, zone, name string, recordType RecordType) ([]DNSRecord, error)
	GetZones(ctx context.Context) ([]DNSZone, error)
	CreateChallengeRecord(ctx context.Context, domain, token, keyAuth string) error
	DeleteChallengeRecord(ctx context.Context, domain, token string) error
	ValidateCredentials(ctx context.Context) error
}

// DNSZone represents a DNS zone
type DNSZone struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Provider  string    `json:"provider"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
}

// ProviderConfig holds provider-specific configuration
type ProviderConfig struct {
	Name        string            `json:"name"`
	Type        string            `json:"type"`
	Credentials map[string]string `json:"credentials"`
	Zones       []string          `json:"zones,omitempty"`
	Enabled     bool              `json:"enabled"`
}

// Registry manages DNS providers
type Registry struct {
	mu        sync.RWMutex
	providers map[string]Provider
	configs   map[string]ProviderConfig
}

// NewRegistry creates a new DNS provider registry
func NewRegistry() *Registry {
	return &Registry{
		providers: make(map[string]Provider),
		configs:   make(map[string]ProviderConfig),
	}
}

// RegisterProvider registers a DNS provider
func (r *Registry) RegisterProvider(name string, provider Provider, config ProviderConfig) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.providers[name] = provider
	r.configs[name] = config
}

// GetProvider retrieves a DNS provider by name
func (r *Registry) GetProvider(name string) (Provider, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	provider, ok := r.providers[name]
	if !ok {
		return nil, fmt.Errorf("DNS provider %q not found", name)
	}
	return provider, nil
}

// ListProviders lists all registered providers
func (r *Registry) ListProviders() []ProviderConfig {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var configs []ProviderConfig
	for _, config := range r.configs {
		configs = append(configs, config)
	}
	return configs
}

// CreateChallenge creates a DNS challenge for ACME validation
func (r *Registry) CreateChallenge(ctx context.Context, providerName, domain, token, keyAuth string) error {
	provider, err := r.GetProvider(providerName)
	if err != nil {
		return err
	}
	return provider.CreateChallengeRecord(ctx, domain, token, keyAuth)
}

// CleanupChallenge removes a DNS challenge record
func (r *Registry) CleanupChallenge(ctx context.Context, providerName, domain, token string) error {
	provider, err := r.GetProvider(providerName)
	if err != nil {
		return err
	}
	return provider.DeleteChallengeRecord(ctx, domain, token)
}

// ManualProvider implements manual DNS management
type ManualProvider struct {
	records map[string][]DNSRecord
	mu      sync.RWMutex
}

// NewManualProvider creates a new manual DNS provider
func NewManualProvider() *ManualProvider {
	return &ManualProvider{
		records: make(map[string][]DNSRecord),
	}
}

func (p *ManualProvider) Name() string { return "manual" }

func (p *ManualProvider) CreateRecord(_ context.Context, zone string, record DNSRecord) (DNSRecord, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	record.RecordID = fmt.Sprintf("manual-%d", time.Now().UnixNano())
	record.CreatedAt = time.Now()
	record.UpdatedAt = time.Now()
	key := fmt.Sprintf("%s:%s:%s", zone, record.Name, record.Type)
	p.records[key] = append(p.records[key], record)
	return record, nil
}

func (p *ManualProvider) UpdateRecord(_ context.Context, zone string, record DNSRecord) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	record.UpdatedAt = time.Now()
	key := fmt.Sprintf("%s:%s:%s", zone, record.Name, record.Type)
	p.records[key] = []DNSRecord{record}
	return nil
}

func (p *ManualProvider) DeleteRecord(_ context.Context, zone, recordID string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	for key, records := range p.records {
		var filtered []DNSRecord
		for _, r := range records {
			if r.RecordID != recordID {
				filtered = append(filtered, r)
			}
		}
		if len(filtered) == 0 {
			delete(p.records, key)
		} else {
			p.records[key] = filtered
		}
	}
	return nil
}

func (p *ManualProvider) GetRecords(_ context.Context, zone, name string, recordType RecordType) ([]DNSRecord, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	key := fmt.Sprintf("%s:%s:%s", zone, name, recordType)
	return p.records[key], nil
}

func (p *ManualProvider) GetZones(_ context.Context) ([]DNSZone, error) {
	return []DNSZone{{ID: "manual", Name: "manual", Provider: "manual", Status: "active", CreatedAt: time.Now()}}, nil
}

func (p *ManualProvider) CreateChallengeRecord(_ context.Context, domain, token, keyAuth string) error {
	fqdn := "_acme-challenge." + domain
	fmt.Printf("Manual DNS Challenge Required:\n")
	fmt.Printf("  Domain: %s\n", domain)
	fmt.Printf("  FQDN: %s\n", fqdn)
	fmt.Printf("  TXT Record: %s\n", keyAuth)
	fmt.Printf("Please create this TXT record manually.\n")
	return nil
}

func (p *ManualProvider) DeleteChallengeRecord(_ context.Context, _, _ string) error {
	return nil
}

func (p *ManualProvider) ValidateCredentials(_ context.Context) error {
	return nil
}

// MemoryProvider implements in-memory DNS management for testing
type MemoryProvider struct {
	records map[string][]DNSRecord
	mu      sync.RWMutex
}

// NewMemoryProvider creates a new memory DNS provider
func NewMemoryProvider() *MemoryProvider {
	return &MemoryProvider{
		records: make(map[string][]DNSRecord),
	}
}

func (p *MemoryProvider) Name() string { return "memory" }

func (p *MemoryProvider) CreateRecord(_ context.Context, zone string, record DNSRecord) (DNSRecord, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	record.RecordID = fmt.Sprintf("mem-%d", time.Now().UnixNano())
	record.CreatedAt = time.Now()
	record.UpdatedAt = time.Now()
	key := fmt.Sprintf("%s:%s:%s", zone, record.Name, record.Type)
	p.records[key] = append(p.records[key], record)
	return record, nil
}

func (p *MemoryProvider) UpdateRecord(_ context.Context, zone string, record DNSRecord) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	record.UpdatedAt = time.Now()
	key := fmt.Sprintf("%s:%s:%s", zone, record.Name, record.Type)
	p.records[key] = []DNSRecord{record}
	return nil
}

func (p *MemoryProvider) DeleteRecord(_ context.Context, zone, recordID string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	for key, records := range p.records {
		var filtered []DNSRecord
		for _, r := range records {
			if r.RecordID != recordID {
				filtered = append(filtered, r)
			}
		}
		if len(filtered) == 0 {
			delete(p.records, key)
		} else {
			p.records[key] = filtered
		}
	}
	return nil
}

func (p *MemoryProvider) GetRecords(_ context.Context, zone, name string, recordType RecordType) ([]DNSRecord, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	key := fmt.Sprintf("%s:%s:%s", zone, name, recordType)
	return p.records[key], nil
}

func (p *MemoryProvider) GetZones(_ context.Context) ([]DNSZone, error) {
	return []DNSZone{{ID: "memory", Name: "memory.local", Provider: "memory", Status: "active", CreatedAt: time.Now()}}, nil
}

func (p *MemoryProvider) CreateChallengeRecord(_ context.Context, domain, token, keyAuth string) error {
	fqdn := "_acme-challenge." + domain
	record := DNSRecord{
		Type:      RecordTypeTXT,
		Name:      fqdn,
		Value:     keyAuth,
		TTL:       60,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	_, err := p.CreateRecord(context.Background(), "memory.local", record)
	return err
}

func (p *MemoryProvider) DeleteChallengeRecord(_ context.Context, domain, _ string) error {
	fqdn := "_acme-challenge." + domain
	p.mu.Lock()
	defer p.mu.Unlock()
	key := fmt.Sprintf("%s:%s:%s", "memory.local", fqdn, RecordTypeTXT)
	delete(p.records, key)
	return nil
}

func (p *MemoryProvider) ValidateCredentials(_ context.Context) error {
	return nil
}

// FindZone finds the zone for a given domain
func FindZone(domain string, zones []DNSZone) *DNSZone {
	domain = strings.TrimSuffix(domain, ".")
	var bestMatch *DNSZone
	for i := range zones {
		zone := &zones[i]
		zoneName := strings.TrimSuffix(zone.Name, ".")
		if strings.HasSuffix(domain, "."+zoneName) || domain == zoneName {
			if bestMatch == nil || len(zoneName) > len(bestMatch.Name) {
				bestMatch = zone
			}
		}
	}
	return bestMatch
}

// ExtractSubdomain extracts the subdomain portion
func ExtractSubdomain(domain, zone string) string {
	domain = strings.TrimSuffix(domain, ".")
	zone = strings.TrimSuffix(zone, ".")
	if domain == zone {
		return "@"
	}
	suffix := "." + zone
	if strings.HasSuffix(domain, suffix) {
		return strings.TrimSuffix(domain, suffix)
	}
	return domain
}
