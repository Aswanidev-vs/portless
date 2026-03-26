package acme

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"sync"
	"time"

	"golang.org/x/crypto/acme"
)

// CertificateState represents the lifecycle state of a certificate
type CertificateState string

const (
	CertStatePending    CertificateState = "pending"
	CertStateValidating CertificateState = "validating"
	CertStateIssued     CertificateState = "issued"
	CertStateRenewing   CertificateState = "renewing"
	CertStateExpired    CertificateState = "expired"
	CertStateRevoked    CertificateState = "revoked"
	CertStateFailed     CertificateState = "failed"
)

// CertificateMetadata holds certificate lifecycle information
type CertificateMetadata struct {
	Domain        string            `json:"domain"`
	State         CertificateState  `json:"state"`
	SerialNumber  string            `json:"serial_number,omitempty"`
	Issuer        string            `json:"issuer,omitempty"`
	NotBefore     time.Time         `json:"not_before,omitempty"`
	NotAfter      time.Time         `json:"not_after,omitempty"`
	Fingerprint   string            `json:"fingerprint,omitempty"`
	ACMEAccountID string            `json:"acme_account_id,omitempty"`
	OrderURL      string            `json:"order_url,omitempty"`
	ChallengeType string            `json:"challenge_type,omitempty"`
	LastAttempt   time.Time         `json:"last_attempt"`
	NextRenewal   time.Time         `json:"next_renewal"`
	RetryCount    int               `json:"retry_count"`
	LastError     string            `json:"last_error,omitempty"`
	Metadata      map[string]string `json:"metadata,omitempty"`
}

// ChallengeHandler defines the interface for ACME challenge handlers
type ChallengeHandler interface {
	Type() string
	HandleChallenge(ctx context.Context, domain, challenge string) error
	CleanupChallenge(ctx context.Context, domain, challenge string) error
}

// CertificateStore defines the interface for certificate persistence
type CertificateStore interface {
	SaveCertificate(ctx context.Context, domain string, cert []byte, key []byte) error
	LoadCertificate(ctx context.Context, domain string) ([]byte, []byte, error)
	DeleteCertificate(ctx context.Context, domain string) error
	SaveMetadata(ctx context.Context, meta CertificateMetadata) error
	LoadMetadata(ctx context.Context, domain string) (CertificateMetadata, error)
	ListCertificates(ctx context.Context) ([]CertificateMetadata, error)
}

// LifecycleManager manages the full certificate lifecycle with automated renewal
type LifecycleManager struct {
	mu            sync.RWMutex
	client        *acme.Client
	accountKey    crypto.Signer
	accountURL    string
	email         string
	directoryURL  string
	store         CertificateStore
	challenges    map[string]ChallengeHandler
	certificates  map[string]*tls.Certificate
	metadata      map[string]*CertificateMetadata
	renewalWindow time.Duration
	maxRetries    int
	retryBackoff  time.Duration
	stopCh        chan struct{}
	wg            sync.WaitGroup
	onCertIssued  func(domain string, cert *tls.Certificate)
	onCertRenewed func(domain string, cert *tls.Certificate)
	onCertFailed  func(domain string, err error)
}

// LifecycleConfig holds configuration for the certificate lifecycle manager
type LifecycleConfig struct {
	Email         string
	DirectoryURL  string
	Store         CertificateStore
	RenewalWindow time.Duration
	MaxRetries    int
	RetryBackoff  time.Duration
	OnCertIssued  func(domain string, cert *tls.Certificate)
	OnCertRenewed func(domain string, cert *tls.Certificate)
	OnCertFailed  func(domain string, err error)
}

// NewLifecycleManager creates a new certificate lifecycle manager
func NewLifecycleManager(cfg LifecycleConfig) (*LifecycleManager, error) {
	if cfg.DirectoryURL == "" {
		cfg.DirectoryURL = acme.LetsEncryptURL
	}
	if cfg.RenewalWindow == 0 {
		cfg.RenewalWindow = 30 * 24 * time.Hour // 30 days
	}
	if cfg.MaxRetries == 0 {
		cfg.MaxRetries = 5
	}
	if cfg.RetryBackoff == 0 {
		cfg.RetryBackoff = 5 * time.Minute
	}

	// Generate or load account key
	accountKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generate account key: %w", err)
	}

	client := &acme.Client{
		Key:          accountKey,
		DirectoryURL: cfg.DirectoryURL,
	}

	mgr := &LifecycleManager{
		client:        client,
		accountKey:    accountKey,
		email:         cfg.Email,
		directoryURL:  cfg.DirectoryURL,
		store:         cfg.Store,
		challenges:    make(map[string]ChallengeHandler),
		certificates:  make(map[string]*tls.Certificate),
		metadata:      make(map[string]*CertificateMetadata),
		renewalWindow: cfg.RenewalWindow,
		maxRetries:    cfg.MaxRetries,
		retryBackoff:  cfg.RetryBackoff,
		stopCh:        make(chan struct{}),
		onCertIssued:  cfg.OnCertIssued,
		onCertRenewed: cfg.OnCertRenewed,
		onCertFailed:  cfg.OnCertFailed,
	}

	return mgr, nil
}

// RegisterChallengeHandler registers a challenge handler for a specific type
func (m *LifecycleManager) RegisterChallengeHandler(handler ChallengeHandler) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.challenges[handler.Type()] = handler
}

// Start starts the lifecycle manager and renewal background worker
func (m *LifecycleManager) Start(ctx context.Context) error {
	// Register ACME account
	account := &acme.Account{
		Contact: []string{"mailto:" + m.email},
	}
	reg, err := m.client.Register(ctx, account, acme.AcceptTOS)
	if err != nil {
		// Account might already exist, try to look it up
		if err != acme.ErrAccountAlreadyExists {
			return fmt.Errorf("register ACME account: %w", err)
		}
	}
	if reg != nil {
		m.accountURL = reg.URI
	}

	// Load existing certificates from store
	if err := m.loadExistingCertificates(ctx); err != nil {
		return fmt.Errorf("load existing certificates: %w", err)
	}

	// Start renewal worker
	m.wg.Add(1)
	go m.renewalWorker(ctx)

	return nil
}

// Stop stops the lifecycle manager
func (m *LifecycleManager) Stop() {
	close(m.stopCh)
	m.wg.Wait()
}

// IssueCertificate issues a new certificate for the given domain
func (m *LifecycleManager) IssueCertificate(ctx context.Context, domain string, challengeType string) (*tls.Certificate, error) {
	m.mu.Lock()
	meta := &CertificateMetadata{
		Domain:        domain,
		State:         CertStatePending,
		ChallengeType: challengeType,
		LastAttempt:   time.Now(),
		Metadata:      make(map[string]string),
	}
	m.metadata[domain] = meta
	m.mu.Unlock()

	authz, err := m.client.Authorize(ctx, domain)
	if err != nil {
		m.updateMetadataState(domain, CertStateFailed, err.Error())
		return nil, fmt.Errorf("create authorization: %w", err)
	}

	meta.OrderURL = authz.URI
	meta.State = CertStateValidating
	if m.store != nil {
		_ = m.store.SaveMetadata(ctx, *meta)
	}

	// Find appropriate challenge
	var challenge *acme.Challenge
	for _, ch := range authz.Challenges {
		if ch.Type == challengeType {
			challenge = ch
			break
		}
	}
	if challenge == nil {
		// Fallback to any supported challenge
		for _, ch := range authz.Challenges {
			if handler, ok := m.challenges[ch.Type]; ok {
				challenge = ch
				challengeType = handler.Type()
				break
			}
		}
	}
	if challenge == nil {
		err := fmt.Errorf("no supported challenge found")
		m.updateMetadataState(domain, CertStateFailed, err.Error())
		return nil, err
	}

	// Handle challenge
	handler, ok := m.challenges[challengeType]
	if !ok {
		err := fmt.Errorf("no handler for challenge type %s", challengeType)
		m.updateMetadataState(domain, CertStateFailed, err.Error())
		return nil, err
	}

	challengeValue := challenge.Token
	if challengeType == "dns-01" {
		challengeValue, err = m.client.DNS01ChallengeRecord(challenge.Token)
		if err != nil {
			m.updateMetadataState(domain, CertStateFailed, err.Error())
			return nil, fmt.Errorf("dns01 challenge record: %w", err)
		}
	}

	if err := handler.HandleChallenge(ctx, domain, challengeValue); err != nil {
		m.updateMetadataState(domain, CertStateFailed, err.Error())
		return nil, fmt.Errorf("handle challenge: %w", err)
	}

	// Accept challenge
	if _, err := m.client.Accept(ctx, challenge); err != nil {
		handler.CleanupChallenge(ctx, domain, challengeValue)
		m.updateMetadataState(domain, CertStateFailed, err.Error())
		return nil, fmt.Errorf("accept challenge: %w", err)
	}

	// Wait for validation
	if _, err := m.client.WaitAuthorization(ctx, authz.URI); err != nil {
		handler.CleanupChallenge(ctx, domain, challengeValue)
		m.updateMetadataState(domain, CertStateFailed, err.Error())
		return nil, fmt.Errorf("wait authorization: %w", err)
	}

	// Cleanup challenge
	handler.CleanupChallenge(ctx, domain, challengeValue)

	// Generate certificate key
	certKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		m.updateMetadataState(domain, CertStateFailed, err.Error())
		return nil, fmt.Errorf("generate certificate key: %w", err)
	}

	// Create CSR
	template := &x509.CertificateRequest{
		Subject: pkix.Name{
			CommonName: domain,
		},
		DNSNames: []string{domain},
	}
	csrDER, err := x509.CreateCertificateRequest(rand.Reader, template, certKey)
	if err != nil {
		m.updateMetadataState(domain, CertStateFailed, err.Error())
		return nil, fmt.Errorf("create CSR: %w", err)
	}

	// Create certificate from CSR
	certs, _, err := m.client.CreateCert(ctx, csrDER, 90*24*time.Hour, true)
	if err != nil {
		m.updateMetadataState(domain, CertStateFailed, err.Error())
		return nil, fmt.Errorf("create certificate: %w", err)
	}

	// Parse certificate
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certs[0]})
	keyDER, err := x509.MarshalECPrivateKey(certKey)
	if err != nil {
		m.updateMetadataState(domain, CertStateFailed, err.Error())
		return nil, fmt.Errorf("marshal key: %w", err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})

	// Save to store
	if m.store != nil {
		if err := m.store.SaveCertificate(ctx, domain, certPEM, keyPEM); err != nil {
			m.updateMetadataState(domain, CertStateFailed, err.Error())
			return nil, fmt.Errorf("save certificate: %w", err)
		}
	}

	// Parse certificate for metadata
	parsedCert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		m.updateMetadataState(domain, CertStateFailed, err.Error())
		return nil, fmt.Errorf("parse certificate: %w", err)
	}

	// Update metadata
	m.mu.Lock()
	meta.State = CertStateIssued
	meta.RetryCount = 0
	meta.LastError = ""
	if len(parsedCert.Certificate) > 0 {
		parsed, _ := x509.ParseCertificate(parsedCert.Certificate[0])
		if parsed != nil {
			meta.SerialNumber = parsed.SerialNumber.String()
			meta.Issuer = parsed.Issuer.CommonName
			meta.NotBefore = parsed.NotBefore
			meta.NotAfter = parsed.NotAfter
			meta.Fingerprint = fmt.Sprintf("%x", parsed.AuthorityKeyId)
			meta.NextRenewal = parsed.NotAfter.Add(-m.renewalWindow)
		}
	}
	m.certificates[domain] = &parsedCert
	m.mu.Unlock()

	if m.store != nil {
		_ = m.store.SaveMetadata(ctx, *meta)
	}

	if m.onCertIssued != nil {
		m.onCertIssued(domain, &parsedCert)
	}

	return &parsedCert, nil
}

// RevokeCertificate revokes a certificate
func (m *LifecycleManager) RevokeCertificate(ctx context.Context, domain string) error {
	m.mu.Lock()
	cert, ok := m.certificates[domain]
	if !ok {
		m.mu.Unlock()
		return fmt.Errorf("no certificate for domain %s", domain)
	}
	delete(m.certificates, domain)
	delete(m.metadata, domain)
	m.mu.Unlock()

	if len(cert.Certificate) > 0 {
		if err := m.client.RevokeCert(ctx, m.accountKey, cert.Certificate[0], 0); err != nil {
			return fmt.Errorf("revoke certificate: %w", err)
		}
	}

	if m.store != nil {
		_ = m.store.DeleteCertificate(ctx, domain)
	}
	return nil
}

// GetCertificate retrieves a certificate for a domain
func (m *LifecycleManager) GetCertificate(domain string) (*tls.Certificate, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	cert, ok := m.certificates[domain]
	if !ok {
		return nil, fmt.Errorf("no certificate for domain %s", domain)
	}
	return cert, nil
}

// GetMetadata retrieves certificate metadata
func (m *LifecycleManager) GetMetadata(domain string) (CertificateMetadata, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	meta, ok := m.metadata[domain]
	if !ok {
		return CertificateMetadata{}, fmt.Errorf("no metadata for domain %s", domain)
	}
	return *meta, nil
}

// ListCertificates lists all managed certificates
func (m *LifecycleManager) ListCertificates() []CertificateMetadata {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]CertificateMetadata, 0, len(m.metadata))
	for _, meta := range m.metadata {
		result = append(result, *meta)
	}
	return result
}

// TLSConfig returns a TLS config that uses managed certificates
func (m *LifecycleManager) TLSConfig() *tls.Config {
	return &tls.Config{
		GetCertificate: func(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
			cert, err := m.GetCertificate(hello.ServerName)
			if err != nil {
				// Try wildcard or base domain
				if idx := findDotIndex(hello.ServerName); idx > 0 {
					wildcard := "*." + hello.ServerName[idx+1:]
					cert, err = m.GetCertificate(wildcard)
				}
			}
			return cert, err
		},
		MinVersion: tls.VersionTLS12,
	}
}

func (m *LifecycleManager) renewalWorker(ctx context.Context) {
	defer m.wg.Done()

	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-m.stopCh:
			return
		case <-ticker.C:
			m.checkAndRenewCertificates(ctx)
		}
	}
}

func (m *LifecycleManager) checkAndRenewCertificates(ctx context.Context) {
	m.mu.RLock()
	var needRenewal []string
	for domain, meta := range m.metadata {
		if meta.State == CertStateIssued && time.Now().After(meta.NextRenewal) {
			needRenewal = append(needRenewal, domain)
		}
	}
	m.mu.RUnlock()

	for _, domain := range needRenewal {
		if err := m.renewCertificate(ctx, domain); err != nil {
			m.mu.Lock()
			if meta, ok := m.metadata[domain]; ok {
				meta.RetryCount++
				meta.LastError = err.Error()
				meta.LastAttempt = time.Now()
				if meta.RetryCount >= m.maxRetries {
					meta.State = CertStateFailed
				}
			}
			m.mu.Unlock()

			if m.onCertFailed != nil {
				m.onCertFailed(domain, err)
			}
		}
	}
}

func (m *LifecycleManager) renewCertificate(ctx context.Context, domain string) error {
	m.mu.Lock()
	meta, ok := m.metadata[domain]
	if !ok {
		m.mu.Unlock()
		return fmt.Errorf("no metadata for domain %s", domain)
	}
	meta.State = CertStateRenewing
	meta.LastAttempt = time.Now()
	challengeType := meta.ChallengeType
	if challengeType == "" {
		challengeType = "dns-01"
	}
	m.mu.Unlock()

	if m.store != nil {
		_ = m.store.SaveMetadata(ctx, *meta)
	}

	// Revoke old certificate
	oldCert, ok := m.certificates[domain]
	if ok && len(oldCert.Certificate) > 0 {
		_ = m.client.RevokeCert(ctx, m.accountKey, oldCert.Certificate[0], 0)
	}

	// Issue new certificate
	newCert, err := m.IssueCertificate(ctx, domain, challengeType)
	if err != nil {
		return err
	}

	if m.onCertRenewed != nil {
		m.onCertRenewed(domain, newCert)
	}

	return nil
}

func (m *LifecycleManager) loadExistingCertificates(ctx context.Context) error {
	if m.store == nil {
		return nil
	}

	certs, err := m.store.ListCertificates(ctx)
	if err != nil {
		return err
	}

	for _, meta := range certs {
		certPEM, keyPEM, err := m.store.LoadCertificate(ctx, meta.Domain)
		if err != nil {
			continue
		}

		cert, err := tls.X509KeyPair(certPEM, keyPEM)
		if err != nil {
			continue
		}

		m.certificates[meta.Domain] = &cert
		metaCopy := meta
		m.metadata[meta.Domain] = &metaCopy
	}

	return nil
}

func (m *LifecycleManager) updateMetadataState(domain string, state CertificateState, lastErr string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if meta, ok := m.metadata[domain]; ok {
		meta.State = state
		meta.LastError = lastErr
		meta.LastAttempt = time.Now()
	}
}

func findDotIndex(s string) int {
	for i := 0; i < len(s); i++ {
		if s[i] == '.' {
			return i
		}
	}
	return -1
}

// GenerateSelfSigned generates a self-signed certificate (for testing/fallback)
func GenerateSelfSigned(domain string) ([]byte, []byte, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, err
	}

	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			CommonName: domain,
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(365 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}

	if ip := net.ParseIP(domain); ip != nil {
		template.IPAddresses = []net.IP{ip}
	} else {
		template.DNSNames = []string{domain}
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		return nil, nil, err
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return nil, nil, err
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})

	return certPEM, keyPEM, nil
}
