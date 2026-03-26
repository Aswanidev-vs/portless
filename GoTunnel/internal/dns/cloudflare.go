package dns

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type CloudflareProvider struct {
	apiToken string
	client   *http.Client
	zones    []string
}

func NewCloudflareProvider(apiToken string, zones []string) *CloudflareProvider {
	return &CloudflareProvider{
		apiToken: strings.TrimSpace(apiToken),
		client:   &http.Client{Timeout: 15 * time.Second},
		zones:    append([]string(nil), zones...),
	}
}

func (p *CloudflareProvider) Name() string { return "cloudflare" }

func (p *CloudflareProvider) CreateRecord(ctx context.Context, zone string, record DNSRecord) (DNSRecord, error) {
	zoneID, err := p.resolveZoneID(ctx, zone)
	if err != nil {
		return DNSRecord{}, err
	}
	body := map[string]interface{}{
		"type":    string(record.Type),
		"name":    record.Name,
		"content": record.Value,
		"ttl":     record.TTL,
	}
	var resp struct {
		Success bool `json:"success"`
		Result  struct {
			ID string `json:"id"`
		} `json:"result"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := p.request(ctx, http.MethodPost, "/zones/"+zoneID+"/dns_records", body, &resp); err != nil {
		return DNSRecord{}, err
	}
	if !resp.Success {
		return DNSRecord{}, fmt.Errorf("cloudflare create record failed")
	}
	record.ZoneID = zoneID
	record.RecordID = resp.Result.ID
	record.CreatedAt = time.Now().UTC()
	record.UpdatedAt = record.CreatedAt
	return record, nil
}

func (p *CloudflareProvider) UpdateRecord(ctx context.Context, zone string, record DNSRecord) error {
	zoneID, err := p.resolveZoneID(ctx, zone)
	if err != nil {
		return err
	}
	if record.RecordID == "" {
		return fmt.Errorf("record id is required")
	}
	body := map[string]interface{}{
		"type":    string(record.Type),
		"name":    record.Name,
		"content": record.Value,
		"ttl":     record.TTL,
	}
	var resp struct {
		Success bool `json:"success"`
	}
	return p.request(ctx, http.MethodPut, "/zones/"+zoneID+"/dns_records/"+record.RecordID, body, &resp)
}

func (p *CloudflareProvider) DeleteRecord(ctx context.Context, zone, recordID string) error {
	zoneID, err := p.resolveZoneID(ctx, zone)
	if err != nil {
		return err
	}
	var resp struct {
		Success bool `json:"success"`
	}
	return p.request(ctx, http.MethodDelete, "/zones/"+zoneID+"/dns_records/"+recordID, nil, &resp)
}

func (p *CloudflareProvider) GetRecords(ctx context.Context, zone, name string, recordType RecordType) ([]DNSRecord, error) {
	zoneID, err := p.resolveZoneID(ctx, zone)
	if err != nil {
		return nil, err
	}
	query := url.Values{}
	query.Set("type", string(recordType))
	query.Set("name", name)
	var resp struct {
		Success bool `json:"success"`
		Result  []struct {
			ID      string `json:"id"`
			Type    string `json:"type"`
			Name    string `json:"name"`
			Content string `json:"content"`
			TTL     int    `json:"ttl"`
		} `json:"result"`
	}
	if err := p.request(ctx, http.MethodGet, "/zones/"+zoneID+"/dns_records?"+query.Encode(), nil, &resp); err != nil {
		return nil, err
	}
	records := make([]DNSRecord, 0, len(resp.Result))
	for _, result := range resp.Result {
		records = append(records, DNSRecord{
			Type:     RecordType(result.Type),
			Name:     result.Name,
			Value:    result.Content,
			TTL:      result.TTL,
			ZoneID:   zoneID,
			RecordID: result.ID,
		})
	}
	return records, nil
}

func (p *CloudflareProvider) GetZones(ctx context.Context) ([]DNSZone, error) {
	query := url.Values{}
	if len(p.zones) == 1 {
		query.Set("name", p.zones[0])
	}
	var resp struct {
		Success bool `json:"success"`
		Result  []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"result"`
	}
	path := "/zones"
	if query.Encode() != "" {
		path += "?" + query.Encode()
	}
	if err := p.request(ctx, http.MethodGet, path, nil, &resp); err != nil {
		return nil, err
	}
	zones := make([]DNSZone, 0, len(resp.Result))
	for _, result := range resp.Result {
		if len(p.zones) > 0 && !containsZone(p.zones, result.Name) {
			continue
		}
		zones = append(zones, DNSZone{ID: result.ID, Name: result.Name, Provider: p.Name(), Status: "active", CreatedAt: time.Now().UTC()})
	}
	return zones, nil
}

func (p *CloudflareProvider) CreateChallengeRecord(ctx context.Context, domain, _token, keyAuth string) error {
	zones, err := p.GetZones(ctx)
	if err != nil {
		return err
	}
	zone := FindZone(domain, zones)
	if zone == nil {
		return fmt.Errorf("cloudflare zone not found for %s", domain)
	}
	record := DNSRecord{
		Type:  RecordTypeTXT,
		Name:  "_acme-challenge." + strings.TrimSuffix(domain, "."),
		Value: keyAuth,
		TTL:   60,
	}
	_, err = p.CreateRecord(ctx, zone.Name, record)
	return err
}

func (p *CloudflareProvider) DeleteChallengeRecord(ctx context.Context, domain, _ string) error {
	zones, err := p.GetZones(ctx)
	if err != nil {
		return err
	}
	zone := FindZone(domain, zones)
	if zone == nil {
		return fmt.Errorf("cloudflare zone not found for %s", domain)
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

func (p *CloudflareProvider) ValidateCredentials(ctx context.Context) error {
	_, err := p.GetZones(ctx)
	return err
}

func (p *CloudflareProvider) resolveZoneID(ctx context.Context, zone string) (string, error) {
	zones, err := p.GetZones(ctx)
	if err != nil {
		return "", err
	}
	for _, z := range zones {
		if strings.EqualFold(strings.TrimSuffix(z.Name, "."), strings.TrimSuffix(zone, ".")) {
			return z.ID, nil
		}
	}
	return "", fmt.Errorf("cloudflare zone %q not found", zone)
}

func (p *CloudflareProvider) request(ctx context.Context, method, path string, body interface{}, out interface{}) error {
	var payload io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return err
		}
		payload = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, "https://api.cloudflare.com/client/v4"+path, payload)
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
		return fmt.Errorf("cloudflare api %s %s failed: %s", method, path, strings.TrimSpace(string(data)))
	}
	if out == nil {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func containsZone(zones []string, zone string) bool {
	for _, candidate := range zones {
		if strings.EqualFold(strings.TrimSuffix(candidate, "."), strings.TrimSuffix(zone, ".")) {
			return true
		}
	}
	return false
}
