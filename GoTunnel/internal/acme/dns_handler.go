package acme

import (
	"context"
	"fmt"
	"strings"

	"gotunnel/internal/dns"
)

type DNSChallengeHandler struct {
	providerName string
	registry     *dns.Registry
}

func NewDNSChallengeHandler(registry *dns.Registry, providerName string) *DNSChallengeHandler {
	return &DNSChallengeHandler{
		providerName: strings.TrimSpace(providerName),
		registry:     registry,
	}
}

func (h *DNSChallengeHandler) Type() string { return "dns-01" }

func (h *DNSChallengeHandler) HandleChallenge(ctx context.Context, domain, challenge string) error {
	if h.registry == nil {
		return fmt.Errorf("dns registry is not configured")
	}
	return h.registry.CreateChallenge(ctx, h.providerName, domain, "", challenge)
}

func (h *DNSChallengeHandler) CleanupChallenge(ctx context.Context, domain, challenge string) error {
	if h.registry == nil {
		return nil
	}
	return h.registry.CleanupChallenge(ctx, h.providerName, domain, challenge)
}
