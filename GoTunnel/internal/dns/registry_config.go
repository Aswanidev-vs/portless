package dns

import (
	"fmt"
	"os"
	"strings"
)

func NewRegistryFromEnv() (*Registry, string, error) {
	registry := NewRegistry()
	registry.RegisterProvider("manual", NewManualProvider(), ProviderConfig{Name: "manual", Type: "manual", Enabled: true})
	registry.RegisterProvider("memory", NewMemoryProvider(), ProviderConfig{Name: "memory", Type: "memory", Enabled: true})

	defaultProvider := strings.TrimSpace(os.Getenv("GOTUNNEL_DNS_PROVIDER"))
	if defaultProvider == "" {
		defaultProvider = "manual"
	}

	if token := strings.TrimSpace(os.Getenv("CLOUDFLARE_API_TOKEN")); token != "" {
		zones := splitCSVEnv(os.Getenv("CLOUDFLARE_ZONES"))
		registry.RegisterProvider("cloudflare", NewCloudflareProvider(token, zones), ProviderConfig{
			Name:        "cloudflare",
			Type:        "cloudflare",
			Credentials: map[string]string{"api_token": "env:CLOUDFLARE_API_TOKEN"},
			Zones:       zones,
			Enabled:     true,
		})
	}

	if token := strings.TrimSpace(os.Getenv("DIGITALOCEAN_API_TOKEN")); token != "" {
		zones := splitCSVEnv(os.Getenv("DIGITALOCEAN_ZONES"))
		registry.RegisterProvider("digitalocean", NewDigitalOceanProvider(token, zones), ProviderConfig{
			Name:        "digitalocean",
			Type:        "digitalocean",
			Credentials: map[string]string{"api_token": "env:DIGITALOCEAN_API_TOKEN"},
			Zones:       zones,
			Enabled:     true,
		})
	}

	if _, err := registry.GetProvider(defaultProvider); err != nil {
		return nil, "", fmt.Errorf("default dns provider %q is not configured", defaultProvider)
	}
	return registry, defaultProvider, nil
}

func splitCSVEnv(v string) []string {
	if strings.TrimSpace(v) == "" {
		return nil
	}
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}
