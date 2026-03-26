package dns

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type DigitalOceanProvider struct {
	apiToken string
	client   *http.Client
	zones    []string
}

func NewDigitalOceanProvider(apiToken string, zones []string) *DigitalOceanProvider {
	return &DigitalOceanProvider{
		apiToken: strings.TrimSpace(apiToken),
		client:   &http.Client{Timeout: 15 * time.Second},
		zones:    append([]string(nil), zones...),
	}
}

func (p *DigitalOceanProvider) Name() string { return "digitalocean" }

func (p *DigitalOceanProvider) CreateRecord(ctx context.Context, zone string, record DNSRecord) (DNSRecord, error) {
	body := map[string]interface{}{
		"type": record.Type,
		"name": record.Name,
		"data": record.Value,
		"ttl":  record.TTL,
	}
	var resp struct {
		DomainRecord struct {
			ID   int    `json:"id"`
			Type string `json:"type"`
			Name string `json:"name"`
			Data string `json:"data"`
			TTL  int    `json:"ttl"`
		} `json:"domain_record"`
	}
	if err := p.request(ctx, http.MethodPost, "/domains/"+zone+"/records", body, &resp); err != nil {
		return DNSRecord{}, err
	}
	record.ZoneID = zone
	record.RecordID = fmt.Sprintf("%d", resp.DomainRecord.ID)
	record.CreatedAt = time.Now().UTC()
	record.UpdatedAt = record.CreatedAt
	return record, nil
}

func (p *DigitalOceanProvider) UpdateRecord(ctx context.Context, zone string, record DNSRecord) error {
	if record.RecordID == "" {
		return fmt.Errorf("record id is required")
	}
	body := map[string]interface{}{
		"type": record.Type,
		"name": record.Name,
		"data": record.Value,
		"ttl":  record.TTL,
	}
	return p.request(ctx, http.MethodPut, "/domains/"+zone+"/records/"+record.RecordID, body, nil)
}

func (p *DigitalOceanProvider) DeleteRecord(ctx context.Context, zone, recordID string) error {
	return p.request(ctx, http.MethodDelete, "/domains/"+zone+"/records/"+recordID, nil, nil)
}

func (p *DigitalOceanProvider) GetRecords(ctx context.Context, zone, name string, recordType RecordType) ([]DNSRecord, error) {
	var resp struct {
		DomainRecords []struct {
			ID   int    `json:"id"`
			Type string `json:"type"`
			Name string `json:"name"`
			Data string `json:"data"`
			TTL  int    `json:"ttl"`
		} `json:"domain_records"`
	}
	if err := p.request(ctx, http.MethodGet, "/domains/"+zone+"/records", nil, &resp); err != nil {
		return nil, err
	}
	records := make([]DNSRecord, 0, len(resp.DomainRecords))
	for _, result := range resp.DomainRecords {
		fqdn := result.Name
		if result.Name == "@" {
			fqdn = zone
		} else if !strings.HasSuffix(result.Name, zone) {
			fqdn = result.Name + "." + zone
		}
		if name != "" && !strings.EqualFold(strings.TrimSuffix(fqdn, "."), strings.TrimSuffix(name, ".")) {
			continue
		}
		if recordType != "" && !strings.EqualFold(result.Type, string(recordType)) {
			continue
		}
		records = append(records, DNSRecord{
			Type:     RecordType(result.Type),
			Name:     fqdn,
			Value:    result.Data,
			TTL:      result.TTL,
			ZoneID:   zone,
			RecordID: fmt.Sprintf("%d", result.ID),
		})
	}
	return records, nil
}

func (p *DigitalOceanProvider) GetZones(ctx context.Context) ([]DNSZone, error) {
	var resp struct {
		Domains []struct {
			Name string `json:"name"`
		} `json:"domains"`
	}
	if err := p.request(ctx, http.MethodGet, "/domains", nil, &resp); err != nil {
		return nil, err
	}
	zones := make([]DNSZone, 0, len(resp.Domains))
	for _, domain := range resp.Domains {
		if len(p.zones) > 0 && !containsZone(p.zones, domain.Name) {
			continue
		}
		zones = append(zones, DNSZone{ID: domain.Name, Name: domain.Name, Provider: p.Name(), Status: "active", CreatedAt: time.Now().UTC()})
	}
	return zones, nil
}

func (p *DigitalOceanProvider) CreateChallengeRecord(ctx context.Context, domain, _token, keyAuth string) error {
	zones, err := p.GetZones(ctx)
	if err != nil {
		return err
	}
	zone := FindZone(domain, zones)
	if zone == nil {
		return fmt.Errorf("digitalocean zone not found for %s", domain)
	}
	fqdn := "_acme-challenge." + strings.TrimSuffix(domain, ".")
	name := ExtractSubdomain(fqdn, zone.Name)
	_, err = p.CreateRecord(ctx, zone.Name, DNSRecord{Type: RecordTypeTXT, Name: name, Value: keyAuth, TTL: 60})
	return err
}

func (p *DigitalOceanProvider) DeleteChallengeRecord(ctx context.Context, domain, _ string) error {
	zones, err := p.GetZones(ctx)
	if err != nil {
		return err
	}
	zone := FindZone(domain, zones)
	if zone == nil {
		return fmt.Errorf("digitalocean zone not found for %s", domain)
	}
	fqdn := "_acme-challenge." + strings.TrimSuffix(domain, ".")
	records, err := p.GetRecords(ctx, zone.Name, fqdn, RecordTypeTXT)
	if err != nil {
		return err
	}
	for _, record := range records {
		if err := p.DeleteRecord(ctx, zone.Name, record.RecordID); err != nil {
			return err
		}
	}
	return nil
}

func (p *DigitalOceanProvider) ValidateCredentials(ctx context.Context) error {
	_, err := p.GetZones(ctx)
	return err
}

func (p *DigitalOceanProvider) request(ctx context.Context, method, path string, body interface{}, out interface{}) error {
	var payload io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return err
		}
		payload = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, "https://api.digitalocean.com/v2"+path, payload)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+p.apiToken)
	req.Header.Set("Content-Type", "application/json")
	resp, err := p.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return fmt.Errorf("digitalocean api %s %s failed: %s", method, path, strings.TrimSpace(string(data)))
	}
	if out == nil {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}
