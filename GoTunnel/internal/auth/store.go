package auth

import (
	"context"
	"fmt"
	"strings"
)

type DevSSOProvider struct {
	name string
}

func NewDevSSOProvider(name string) *DevSSOProvider {
	if strings.TrimSpace(name) == "" {
		name = "dev-sso"
	}
	return &DevSSOProvider{name: name}
}

func (p *DevSSOProvider) Name() string { return p.name }

func (p *DevSSOProvider) GetAuthorizationURL(state string) string {
	return "https://example.invalid/" + p.name + "?state=" + state
}

func (p *DevSSOProvider) ExchangeCode(_ context.Context, code string) (SSOUser, error) {
	if strings.TrimSpace(code) == "" {
		return SSOUser{}, fmt.Errorf("code is required")
	}
	normalized := strings.ToLower(strings.ReplaceAll(code, " ", "-"))
	return SSOUser{
		ExternalID:  normalized,
		Email:       normalized + "@gotunnel.local",
		DisplayName: code,
		Username:    normalized,
		Groups:      []string{"developers"},
	}, nil
}

func (p *DevSSOProvider) ValidateToken(_ context.Context, token string) (SSOUser, error) {
	return p.ExchangeCode(context.Background(), token)
}
