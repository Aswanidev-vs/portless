package security

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"
)

// PolicyType represents the type of security policy
type PolicyType string

const (
	PolicyTypeNetwork   PolicyType = "network"
	PolicyTypeAccess    PolicyType = "access"
	PolicyTypeTLS       PolicyType = "tls"
	PolicyTypeRateLimit PolicyType = "rate_limit"
	PolicyTypeAudit     PolicyType = "audit"
)

// PolicyAction represents the action to take when a policy is violated
type PolicyAction string

const (
	PolicyActionAllow PolicyAction = "allow"
	PolicyActionDeny  PolicyAction = "deny"
	PolicyActionLog   PolicyAction = "log"
	PolicyActionRate  PolicyAction = "rate_limit"
)

// Policy represents a security policy
type Policy struct {
	ID          string       `json:"id"`
	Name        string       `json:"name"`
	Type        PolicyType   `json:"type"`
	Action      PolicyAction `json:"action"`
	Priority    int          `json:"priority"`
	Enabled     bool         `json:"enabled"`
	Rules       []Rule       `json:"rules"`
	CreatedAt   time.Time    `json:"created_at"`
	UpdatedAt   time.Time    `json:"updated_at"`
	Description string       `json:"description,omitempty"`
}

// Rule represents a policy rule
type Rule struct {
	Field    string      `json:"field"`
	Operator string      `json:"operator"`
	Value    interface{} `json:"value"`
	Negate   bool        `json:"negate,omitempty"`
}

// NetworkPolicy defines network-level security policies
type NetworkPolicy struct {
	AllowedCIDRs     []string      `json:"allowed_cidrs"`
	BlockedCIDRs     []string      `json:"blocked_cidrs"`
	AllowedPorts     []int         `json:"allowed_ports"`
	BlockedPorts     []int         `json:"blocked_ports"`
	AllowedProtocols []string      `json:"allowed_protocols"`
	MaxConnections   int           `json:"max_connections"`
	Timeout          time.Duration `json:"timeout"`
}

// TLSPolicy defines TLS security policies
type TLSPolicy struct {
	MinVersion        uint16   `json:"min_version"`
	MaxVersion        uint16   `json:"max_version"`
	CipherSuites      []uint16 `json:"cipher_suites"`
	RequireClientCert bool     `json:"require_client_cert"`
	AllowedCNs        []string `json:"allowed_cns,omitempty"`
	AllowedSANs       []string `json:"allowed_sans,omitempty"`
}

// RateLimitPolicy defines rate limiting policies
type RateLimitPolicy struct {
	RequestsPerSecond int           `json:"requests_per_second"`
	BurstSize         int           `json:"burst_size"`
	WindowSize        time.Duration `json:"window_size"`
	KeyField          string        `json:"key_field"` // "ip", "user", "tunnel"
}

// AuditPolicy defines audit logging policies
type AuditPolicy struct {
	LogAllRequests      bool     `json:"log_all_requests"`
	LogFailedAuth       bool     `json:"log_failed_auth"`
	LogPolicyViolations bool     `json:"log_policy_violations"`
	RetainDays          int      `json:"retain_days"`
	ExcludeFields       []string `json:"exclude_fields,omitempty"`
}

// SecurityEvent represents a security event
type SecurityEvent struct {
	ID          string                 `json:"id"`
	Type        string                 `json:"type"`
	Severity    string                 `json:"severity"` // "low", "medium", "high", "critical"
	Source      string                 `json:"source"`
	Destination string                 `json:"destination"`
	UserID      string                 `json:"user_id,omitempty"`
	TunnelID    string                 `json:"tunnel_id,omitempty"`
	PolicyID    string                 `json:"policy_id,omitempty"`
	Action      string                 `json:"action"`
	Result      string                 `json:"result"` // "allowed", "denied", "logged"
	Message     string                 `json:"message"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
	Timestamp   time.Time              `json:"timestamp"`
}

// Enforcer manages security policy enforcement
type Enforcer struct {
	mu            sync.RWMutex
	policies      map[string]*Policy
	networkPolicy NetworkPolicy
	tlsPolicy     TLSPolicy
	rateLimiters  map[string]*RateLimiter
	auditLog      []SecurityEvent
	maxAuditLog   int
	stopCh        chan struct{}
	wg            sync.WaitGroup
	onEvent       func(event SecurityEvent)
}

// EnforcerConfig holds enforcer configuration
type EnforcerConfig struct {
	NetworkPolicy NetworkPolicy
	TLSPolicy     TLSPolicy
	MaxAuditLog   int
	OnEvent       func(event SecurityEvent)
}

// NewEnforcer creates a new security enforcer
func NewEnforcer(cfg EnforcerConfig) *Enforcer {
	if cfg.MaxAuditLog == 0 {
		cfg.MaxAuditLog = 10000
	}

	e := &Enforcer{
		policies:      make(map[string]*Policy),
		networkPolicy: cfg.NetworkPolicy,
		tlsPolicy:     cfg.TLSPolicy,
		rateLimiters:  make(map[string]*RateLimiter),
		auditLog:      make([]SecurityEvent, 0, cfg.MaxAuditLog),
		maxAuditLog:   cfg.MaxAuditLog,
		stopCh:        make(chan struct{}),
		onEvent:       cfg.OnEvent,
	}

	// Set default TLS policy
	if e.tlsPolicy.MinVersion == 0 {
		e.tlsPolicy.MinVersion = tls.VersionTLS12
	}

	return e
}

// Start starts the enforcer
func (e *Enforcer) Start(ctx context.Context) error {
	// Start cleanup worker
	e.wg.Add(1)
	go e.cleanupWorker(ctx)

	return nil
}

// Stop stops the enforcer
func (e *Enforcer) Stop() {
	close(e.stopCh)
	e.wg.Wait()
}

// AddPolicy adds a security policy
func (e *Enforcer) AddPolicy(policy *Policy) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.policies[policy.ID] = policy
}

// RemovePolicy removes a security policy
func (e *Enforcer) RemovePolicy(policyID string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	delete(e.policies, policyID)
}

// GetPolicy retrieves a security policy
func (e *Enforcer) GetPolicy(policyID string) (*Policy, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	policy, ok := e.policies[policyID]
	if !ok {
		return nil, fmt.Errorf("policy %s not found", policyID)
	}
	return policy, nil
}

// ListPolicies lists all security policies
func (e *Enforcer) ListPolicies() []*Policy {
	e.mu.RLock()
	defer e.mu.RUnlock()
	var policies []*Policy
	for _, policy := range e.policies {
		policies = append(policies, policy)
	}
	return policies
}

// EnforceNetworkPolicy enforces network-level security policies
func (e *Enforcer) EnforceNetworkPolicy(ctx context.Context, remoteAddr string) error {
	e.mu.RLock()
	networkPolicy := e.networkPolicy
	e.mu.RUnlock()

	ip, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		return fmt.Errorf("invalid address: %w", err)
	}

	parsedIP := net.ParseIP(ip)
	if parsedIP == nil {
		return fmt.Errorf("invalid IP: %s", ip)
	}

	// Check blocked CIDRs
	for _, cidr := range networkPolicy.BlockedCIDRs {
		_, network, err := net.ParseCIDR(cidr)
		if err != nil {
			continue
		}
		if network.Contains(parsedIP) {
			e.logEvent(SecurityEvent{
				Type:      "network_blocked",
				Severity:  "medium",
				Source:    remoteAddr,
				Action:    "connect",
				Result:    "denied",
				Message:   fmt.Sprintf("IP %s is in blocked CIDR %s", ip, cidr),
				Timestamp: time.Now(),
			})
			return fmt.Errorf("access denied: IP in blocked range")
		}
	}

	// Check allowed CIDRs (if configured)
	if len(networkPolicy.AllowedCIDRs) > 0 {
		allowed := false
		for _, cidr := range networkPolicy.AllowedCIDRs {
			_, network, err := net.ParseCIDR(cidr)
			if err != nil {
				continue
			}
			if network.Contains(parsedIP) {
				allowed = true
				break
			}
		}
		if !allowed {
			e.logEvent(SecurityEvent{
				Type:      "network_blocked",
				Severity:  "medium",
				Source:    remoteAddr,
				Action:    "connect",
				Result:    "denied",
				Message:   fmt.Sprintf("IP %s not in allowed CIDRs", ip),
				Timestamp: time.Now(),
			})
			return fmt.Errorf("access denied: IP not in allowed range")
		}
	}

	return nil
}

// EnforceTLSPolicy enforces TLS security policies
func (e *Enforcer) EnforceTLSPolicy(state *tls.ConnectionState) error {
	e.mu.RLock()
	tlsPolicy := e.tlsPolicy
	e.mu.RUnlock()

	if state == nil {
		return fmt.Errorf("TLS required")
	}

	// Check TLS version
	if state.Version < tlsPolicy.MinVersion {
		e.logEvent(SecurityEvent{
			Type:      "tls_violation",
			Severity:  "high",
			Action:    "tls_handshake",
			Result:    "denied",
			Message:   fmt.Sprintf("TLS version %x below minimum %x", state.Version, tlsPolicy.MinVersion),
			Timestamp: time.Now(),
		})
		return fmt.Errorf("TLS version too low: minimum %x required", tlsPolicy.MinVersion)
	}

	// Check cipher suite
	if len(tlsPolicy.CipherSuites) > 0 {
		allowed := false
		for _, suite := range tlsPolicy.CipherSuites {
			if state.CipherSuite == suite {
				allowed = true
				break
			}
		}
		if !allowed {
			e.logEvent(SecurityEvent{
				Type:      "tls_violation",
				Severity:  "medium",
				Action:    "tls_handshake",
				Result:    "denied",
				Message:   fmt.Sprintf("Cipher suite %x not in allowed list", state.CipherSuite),
				Timestamp: time.Now(),
			})
			return fmt.Errorf("cipher suite not allowed")
		}
	}

	// Check client certificate
	if tlsPolicy.RequireClientCert {
		if len(state.PeerCertificates) == 0 {
			e.logEvent(SecurityEvent{
				Type:      "tls_violation",
				Severity:  "high",
				Action:    "tls_handshake",
				Result:    "denied",
				Message:   "Client certificate required but not provided",
				Timestamp: time.Now(),
			})
			return fmt.Errorf("client certificate required")
		}

		// Check CN
		if len(tlsPolicy.AllowedCNs) > 0 {
			cn := state.PeerCertificates[0].Subject.CommonName
			allowed := false
			for _, allowedCN := range tlsPolicy.AllowedCNs {
				if cn == allowedCN {
					allowed = true
					break
				}
			}
			if !allowed {
				e.logEvent(SecurityEvent{
					Type:      "tls_violation",
					Severity:  "medium",
					Action:    "tls_handshake",
					Result:    "denied",
					Message:   fmt.Sprintf("CN %s not in allowed list", cn),
					Timestamp: time.Now(),
				})
				return fmt.Errorf("client CN not allowed")
			}
		}

		// Check SANs
		if len(tlsPolicy.AllowedSANs) > 0 {
			cert := state.PeerCertificates[0]
			allowed := false
			for _, san := range tlsPolicy.AllowedSANs {
				for _, dnsName := range cert.DNSNames {
					if dnsName == san {
						allowed = true
						break
					}
				}
				if allowed {
					break
				}
				for _, ip := range cert.IPAddresses {
					if ip.String() == san {
						allowed = true
						break
					}
				}
				if allowed {
					break
				}
			}
			if !allowed {
				e.logEvent(SecurityEvent{
					Type:      "tls_violation",
					Severity:  "medium",
					Action:    "tls_handshake",
					Result:    "denied",
					Message:   "No matching SAN in allowed list",
					Timestamp: time.Now(),
				})
				return fmt.Errorf("client SAN not allowed")
			}
		}
	}

	return nil
}

// EnforceRateLimit enforces rate limiting policies
func (e *Enforcer) EnforceRateLimit(ctx context.Context, policy *RateLimitPolicy, key string) error {
	limiterKey := fmt.Sprintf("%s:%s", policy.KeyField, key)
	e.mu.Lock()
	limiter, ok := e.rateLimiters[limiterKey]
	if !ok {
		limiter = NewRateLimiter(policy.RequestsPerSecond, policy.BurstSize)
		e.rateLimiters[limiterKey] = limiter
	}
	e.mu.Unlock()

	if !limiter.Allow() {
		e.logEvent(SecurityEvent{
			Type:      "rate_limit_exceeded",
			Severity:  "medium",
			Source:    key,
			Action:    "request",
			Result:    "denied",
			Message:   fmt.Sprintf("Rate limit exceeded for %s", key),
			Timestamp: time.Now(),
		})
		return fmt.Errorf("rate limit exceeded")
	}

	return nil
}

// EnforcePolicy enforces a specific policy
func (e *Enforcer) EnforcePolicy(ctx context.Context, policyID string, data map[string]interface{}) error {
	e.mu.RLock()
	policy, ok := e.policies[policyID]
	e.mu.RUnlock()

	if !ok {
		return fmt.Errorf("policy %s not found", policyID)
	}

	if !policy.Enabled {
		return nil
	}

	// Evaluate rules
	matches := e.evaluateRules(policy.Rules, data)

	event := SecurityEvent{
		Type:      "policy_enforcement",
		Severity:  "low",
		PolicyID:  policyID,
		Timestamp: time.Now(),
	}

	if matches {
		event.Action = "matched"
		if policy.Action == PolicyActionDeny {
			event.Result = "denied"
			event.Severity = "medium"
			event.Message = fmt.Sprintf("Policy %s denied action", policy.Name)
			e.logEvent(event)
			return fmt.Errorf("access denied by policy: %s", policy.Name)
		} else if policy.Action == PolicyActionLog {
			event.Result = "logged"
			event.Message = fmt.Sprintf("Policy %s logged action", policy.Name)
			e.logEvent(event)
		}
	} else {
		event.Action = "not_matched"
		event.Result = "allowed"
	}

	return nil
}

func (e *Enforcer) evaluateRules(rules []Rule, data map[string]interface{}) bool {
	if len(rules) == 0 {
		return true
	}

	for _, rule := range rules {
		value, ok := data[rule.Field]
		if !ok {
			return false
		}

		match := false
		switch rule.Operator {
		case "eq":
			match = value == rule.Value
		case "ne":
			match = value != rule.Value
		case "in":
			if values, ok := rule.Value.([]interface{}); ok {
				for _, v := range values {
					if value == v {
						match = true
						break
					}
				}
			}
		case "contains":
			if str, ok := value.(string); ok {
				if substr, ok := rule.Value.(string); ok {
					match = strings.Contains(str, substr)
				}
			}
		case "prefix":
			if str, ok := value.(string); ok {
				if prefix, ok := rule.Value.(string); ok {
					match = strings.HasPrefix(str, prefix)
				}
			}
		case "gt":
			if num, ok := value.(float64); ok {
				if threshold, ok := rule.Value.(float64); ok {
					match = num > threshold
				}
			}
		case "lt":
			if num, ok := value.(float64); ok {
				if threshold, ok := rule.Value.(float64); ok {
					match = num < threshold
				}
			}
		}

		if rule.Negate {
			match = !match
		}

		if !match {
			return false
		}
	}

	return true
}

func (e *Enforcer) logEvent(event SecurityEvent) {
	e.mu.Lock()
	defer e.mu.Unlock()

	// Add to audit log
	if len(e.auditLog) >= e.maxAuditLog {
		e.auditLog = e.auditLog[1:]
	}
	e.auditLog = append(e.auditLog, event)

	// Notify listener
	if e.onEvent != nil {
		e.onEvent(event)
	}
}

// GetAuditLog returns the audit log
func (e *Enforcer) GetAuditLog() []SecurityEvent {
	e.mu.RLock()
	defer e.mu.RUnlock()
	result := make([]SecurityEvent, len(e.auditLog))
	copy(result, e.auditLog)
	return result
}

// GetTLSConfig returns a TLS config based on policy
func (e *Enforcer) GetTLSConfig() *tls.Config {
	e.mu.RLock()
	defer e.mu.RUnlock()

	cfg := &tls.Config{
		MinVersion: e.tlsPolicy.MinVersion,
		MaxVersion: e.tlsPolicy.MaxVersion,
	}

	if len(e.tlsPolicy.CipherSuites) > 0 {
		cfg.CipherSuites = e.tlsPolicy.CipherSuites
	}

	if e.tlsPolicy.RequireClientCert {
		cfg.ClientAuth = tls.RequireAndVerifyClientCert
	}

	return cfg
}

func (e *Enforcer) cleanupWorker(ctx context.Context) {
	defer e.wg.Done()

	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-e.stopCh:
			return
		case <-ticker.C:
			e.cleanupRateLimiters()
		}
	}
}

func (e *Enforcer) cleanupRateLimiters() {
	e.mu.Lock()
	defer e.mu.Unlock()

	for key, limiter := range e.rateLimiters {
		if limiter.Stale() {
			delete(e.rateLimiters, key)
		}
	}
}

// RateLimiter implements token bucket rate limiting
type RateLimiter struct {
	rate       int
	burst      int
	tokens     int
	lastRefill time.Time
	mu         sync.Mutex
}

// NewRateLimiter creates a new rate limiter
func NewRateLimiter(rate, burst int) *RateLimiter {
	return &RateLimiter{
		rate:       rate,
		burst:      burst,
		tokens:     burst,
		lastRefill: time.Now(),
	}
}

// Allow checks if a request is allowed
func (rl *RateLimiter) Allow() bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(rl.lastRefill)
	refill := int(elapsed.Seconds() * float64(rl.rate))

	if refill > 0 {
		rl.tokens = min(rl.burst, rl.tokens+refill)
		rl.lastRefill = now
	}

	if rl.tokens > 0 {
		rl.tokens--
		return true
	}

	return false
}

// Stale returns true if the limiter hasn't been used recently
func (rl *RateLimiter) Stale() bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	return time.Since(rl.lastRefill) > 10*time.Minute
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
