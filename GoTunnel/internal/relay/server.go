package relay

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"gotunnel/internal/acme"
	"gotunnel/internal/auth"
	"gotunnel/internal/dns"
	"gotunnel/internal/metrics"
	"gotunnel/internal/nfr"
	"gotunnel/internal/ops"
	"gotunnel/internal/protocol"
	"gotunnel/internal/security"
	"gotunnel/internal/telemetry"
	"gotunnel/internal/transport"
)

type Server struct {
	addr             string
	brokerURL        string
	relayName        string
	relayRegion      string
	capacity         int
	dashboardUser    string
	dashboardPass    string
	webhookURL       string
	publicBase       string
	client           *http.Client
	metrics          *metrics.Registry
	coordinator      Coordinator
	certFile         string
	keyFile          string
	quicAddr         string
	acmeManager      *acme.Manager
	lifecycleManager *acme.LifecycleManager
	reconnectTokens  map[string]string
	authenticator    *auth.Authenticator
	enforcer         *security.Enforcer
	nfrValidator     *nfr.Validator
	hardener         *ops.Hardener

	mu         sync.RWMutex
	tunnels    map[string]*tunnelState
	clients    map[string]*clientState
	requests   map[string]*requestState
	requestLog []protocol.RequestLog
}

type tunnelState struct {
	Lease protocol.Lease
}

type clientState struct {
	ID       string
	TunnelID string
	Queue    chan protocol.PendingRequest
	QUICSend chan protocol.ControlMessage
	QUICUp   bool
}

type requestState struct {
	Request   protocol.PendingRequest
	ResponseC chan protocol.SubmitResponseRequest
}

func New(addr, brokerURL, relayName, relayRegion string, capacity int, dashboardUser, dashboardPass, webhookURL, webhookSecret, pluginConfig, redisAddr, redisPassword string, redisDB int, certFile, keyFile, quicAddr, acmeCache, acmeHosts, acmeEmail, acmeStorage, dnsProvider, publicBase string, allowMutation bool, insecureSkipVerify bool) *Server {
	coordinator := Coordinator(noopCoordinator{})
	if strings.TrimSpace(redisAddr) != "" {
		if redisCoord, err := NewRedisCoordinator(redisAddr, redisPassword, redisDB); err == nil {
			coordinator = redisCoord
		}
	}
	var acmeManager *acme.Manager
	if certFile == "" && keyFile == "" && strings.TrimSpace(acmeCache) != "" {
		acmeManager = acme.New(acmeCache, splitCSV(acmeHosts), acmeEmail)
	}
	var lifecycleManager *acme.LifecycleManager
	if certFile == "" && keyFile == "" && strings.TrimSpace(acmeStorage) != "" && strings.TrimSpace(acmeEmail) != "" {
		if certStore, err := acme.NewFileStore(acmeStorage); err == nil {
			if dnsRegistry, defaultProvider, err := dns.NewRegistryFromEnv(); err == nil {
				provider := localFirstNonEmpty(dnsProvider, defaultProvider)
				if manager, err := acme.NewLifecycleManager(acme.LifecycleConfig{
					Email: acmeEmail,
					Store: certStore,
				}); err == nil {
					manager.RegisterChallengeHandler(acme.NewDNSChallengeHandler(dnsRegistry, provider))
					lifecycleManager = manager
					acmeManager = nil
				}
			}
		}
	}
	authStore := auth.NewMemoryStore()
	authenticator := auth.NewAuthenticator(auth.Config{
		Store:    authStore,
		Issuer:   "gotunnel-relay",
		Audience: "gotunnel-dashboard",
	})
	authenticator.RegisterSSOProvider(auth.NewDevSSOProvider("dev-sso"))
	if secret := strings.TrimSpace(os.Getenv("GOTUNNEL_SSO_HS256_SECRET")); secret != "" {
		authenticator.RegisterSSOProvider(auth.NewStaticJWTSSOProvider(
			localFirstNonEmpty(os.Getenv("GOTUNNEL_SSO_PROVIDER"), "oidc-hs256"),
			strings.TrimSpace(os.Getenv("GOTUNNEL_SSO_ISSUER")),
			strings.TrimSpace(os.Getenv("GOTUNNEL_SSO_AUDIENCE")),
			secret,
		))
	}
	bootstrapUser(authenticator, dashboardUser, dashboardPass)
	enforcer := security.NewEnforcer(security.EnforcerConfig{
		NetworkPolicy: security.NetworkPolicy{
			AllowedProtocols: []string{"http", "https", "tcp", "udp"},
			MaxConnections:   10000,
			Timeout:          30 * time.Second,
		},
		TLSPolicy: security.TLSPolicy{
			MinVersion: tlsVersion13(),
		},
		MaxAuditLog: 2000,
	})
	validator := nfr.NewValidator(nfr.ValidatorConfig{})
	addDefaultNFRs(validator)
	s := &Server{
		addr:             addr,
		brokerURL:        strings.TrimRight(brokerURL, "/"),
		relayName:        relayName,
		relayRegion:      relayRegion,
		capacity:         capacity,
		dashboardUser:    dashboardUser,
		dashboardPass:    dashboardPass,
		webhookURL:       strings.TrimSpace(webhookURL),
		publicBase:       strings.TrimRight(publicBase, "/"),
		client:           transport.NewHTTPClient(insecureSkipVerify),
		metrics:          metrics.New(),
		coordinator:      coordinator,
		certFile:         certFile,
		keyFile:          keyFile,
		quicAddr:         quicAddr,
		acmeManager:      acmeManager,
		lifecycleManager: lifecycleManager,
		reconnectTokens:  make(map[string]string),
		authenticator:    authenticator,
		enforcer:         enforcer,
		nfrValidator:     validator,
		hardener:         ops.NewHardener(ops.HardenerConfig{Version: "dev"}),
		tunnels:          make(map[string]*tunnelState),
		clients:          make(map[string]*clientState),
		requests:         make(map[string]*requestState),
	}
	initPhase2State(s, webhookSecret, allowMutation)
	initPhase3State(s, pluginConfig)
	return s
}

func (s *Server) ListenAndServe(ctx context.Context) error {
	telemetry.Init("gotunnel-relay")
	_ = s.enforcer.Start(ctx)
	_ = s.nfrValidator.Start(ctx)
	if s.lifecycleManager != nil {
		_ = s.lifecycleManager.Start(ctx)
	}
	s.registerOperationalChecks()
	_ = s.hardener.Start(ctx)
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, s.hardener.GetSystemHealth(r.Context()))
	})
	mux.HandleFunc("/ready", func(w http.ResponseWriter, r *http.Request) {
		if !s.hardener.IsReady(r.Context()) {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "not_ready"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
	})
	mux.HandleFunc("/alive", func(w http.ResponseWriter, r *http.Request) {
		if !s.hardener.IsAlive(r.Context()) {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "not_alive"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "alive"})
	})
	mux.Handle("/metrics", s.metrics.Handler())
	mux.HandleFunc("/v1/auth/login", s.handleLogin)
	mux.HandleFunc("/v1/auth/sso", s.handleSSOLogin)
	mux.HandleFunc("/v1/auth/mfa/setup", s.handleMFASetup)
	mux.HandleFunc("/v1/auth/mfa/verify", s.handleMFAVerify)
	mux.HandleFunc("/v1/auth/me", s.handleMe)
	mux.HandleFunc("/v1/client/register", s.handleRegisterClient)
	mux.HandleFunc("/v1/client/next", s.handleNextRequest)
	mux.HandleFunc("/v1/client/respond", s.handleSubmitResponse)
	mux.HandleFunc("/v1/client/tcp/next", s.handleNextTCPEvent)
	mux.HandleFunc("/v1/client/tcp/chunk", s.handleTCPChunk)
	mux.HandleFunc("/v1/client/udp/next", s.handleNextUDPEvent)
	mux.HandleFunc("/v1/client/udp/chunk", s.handleUDPChunk)
	mux.HandleFunc("/v1/tunnels", s.handleListTunnels)
	mux.HandleFunc("/v1/sessions", s.handleSessions)
	mux.HandleFunc("/v1/sessions/join", s.handleJoinSession)
	mux.HandleFunc("/v1/annotations", s.handleAnnotation)
	mux.HandleFunc("/v1/breakpoints", s.handleBreakpoints)
	mux.HandleFunc("/v1/paused/action", s.handlePausedAction)
	mux.HandleFunc("/v1/replay", s.handleReplayAPI)
	mux.HandleFunc("/v1/audit/export", s.handleRelayAuditExport)
	mux.HandleFunc("/v1/security/audit", s.handleSecurityAudit)
	mux.HandleFunc("/v1/nfr/status", s.handleNFRStatus)
	mux.HandleFunc("/dashboard/events", s.handleDashboardEvents)
	mux.HandleFunc("/dashboard/replay", s.handleReplay)
	mux.HandleFunc("/dashboard", s.handleDashboard)
	mux.HandleFunc("/t/", s.handleDebugIngress)
	mux.HandleFunc("/", s.handleHostIngress)

	var handler http.Handler = mux
	if s.acmeManager != nil {
		handler = s.acmeManager.HTTPHandler(mux)
	}
	handler = s.wrapSecurity(handler)
	server := transport.NewServer(transport.ServerOptions{Addr: s.addr, Handler: handler, CertFile: s.certFile, KeyFile: s.keyFile})
	if s.lifecycleManager != nil {
		server.TLSConfig = s.lifecycleManager.TLSConfig()
	} else if s.acmeManager != nil {
		server.TLSConfig = s.acmeManager.TLSConfig()
	}

	go s.registrationLoop(ctx)
	go s.runQUICListener(ctx)

	go func() {
		<-ctx.Done()
		s.hardener.Stop()
		s.nfrValidator.Stop()
		s.enforcer.Stop()
		if s.lifecycleManager != nil {
			s.lifecycleManager.Stop()
		}
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()

	if s.certFile != "" && s.keyFile != "" {
		if err := server.ListenAndServeTLS(s.certFile, s.keyFile); err != nil && err != http.ErrServerClosed {
			return err
		}
		return nil
	}
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

func (s *Server) registerOperationalChecks() {
	s.hardener.RegisterHealthCheck("relay_runtime", func(context.Context) ops.ComponentHealth {
		s.mu.RLock()
		defer s.mu.RUnlock()
		return ops.ComponentHealth{
			Name:    "relay_runtime",
			Status:  ops.HealthStatusHealthy,
			Message: "relay runtime healthy",
			Metrics: map[string]float64{
				"tunnels":  float64(len(s.tunnels)),
				"clients":  float64(len(s.clients)),
				"requests": float64(len(s.requests)),
			},
		}
	})
	s.hardener.RegisterReadinessProbe(ops.ReadinessProbe{
		Name:    "relay_broker_url",
		Timeout: 2 * time.Second,
		Check: func(context.Context) error {
			if s.brokerURL == "" {
				return fmt.Errorf("broker URL not configured")
			}
			return nil
		},
	})
	s.hardener.RegisterLivenessProbe(ops.LivenessProbe{
		Name:    "relay_process",
		Timeout: 2 * time.Second,
		Check: func(context.Context) error {
			return nil
		},
	})
}

func (s *Server) wrapSecurity(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := s.enforcer.EnforceNetworkPolicy(r.Context(), r.RemoteAddr); err != nil {
			http.Error(w, err.Error(), http.StatusForbidden)
			return
		}
		if r.TLS != nil || s.certFile != "" || s.keyFile != "" || s.acmeManager != nil {
			state := r.TLS
			if state == nil {
				http.Error(w, "TLS required", http.StatusUpgradeRequired)
				return
			}
			if err := s.enforcer.EnforceTLSPolicy(state); err != nil {
				http.Error(w, err.Error(), http.StatusForbidden)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) handleRegisterClient(w http.ResponseWriter, r *http.Request) {
	_, span := telemetry.Start(r.Context(), "gotunnel/relay", "register_client")
	defer span.End()
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req protocol.RegisterTunnelRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("decode request: %v", err), http.StatusBadRequest)
		return
	}

	lease, err := s.fetchLease(r.Context(), req.LeaseID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}

	clientID := fmt.Sprintf("client-%d", time.Now().UnixNano())
	clientState := &clientState{
		ID:       clientID,
		TunnelID: lease.ID,
		Queue:    make(chan protocol.PendingRequest, 64),
		QUICSend: make(chan protocol.ControlMessage, 256),
	}

	s.mu.Lock()
	s.clients[clientID] = clientState
	s.tunnels[lease.Subdomain] = &tunnelState{Lease: lease}
	if lease.CustomDomain != "" {
		s.tunnels[lease.CustomDomain] = &tunnelState{Lease: lease}
	}
	reconnectToken := fmt.Sprintf("reconnect-%d", time.Now().UnixNano())
	s.reconnectTokens[reconnectToken] = clientID
	s.mu.Unlock()
	tcpPort := 0
	if lease.Protocol == "tcp" {
		if port, err := ensureTCPTunnelListener(s, lease, clientID); err == nil {
			tcpPort = port
		}
	}
	if lease.Protocol == "udp" {
		if port, err := ensureUDPTunnelListener(s, lease, clientID); err == nil {
			tcpPort = port
		}
	}

	s.metrics.IncCounter("gotunnel_relay_tunnel_registrations_total")
	s.emitEvent("tunnel_registered", "Tunnel registered with relay", lease.ID, "", "", lease)
	s.dispatchWebhook("tunnel_registered", lease)
	if lease.CustomDomain != "" && lease.HTTPSMode == "auto" {
		s.ensureManagedCertificate(r.Context(), lease)
	}

	quicAddr := s.quicAddr
	if strings.HasPrefix(quicAddr, ":") {
		quicAddr = "127.0.0.1" + quicAddr
	}
	writeJSON(w, http.StatusCreated, protocol.RegisterTunnelResponse{
		ClientID:       clientID,
		Lease:          lease,
		Message:        "tunnel registered with relay",
		TCPPort:        tcpPort,
		QUICAddr:       quicAddr,
		ReconnectToken: reconnectToken,
	})
}

func (s *Server) handleNextRequest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	clientID := r.URL.Query().Get("client_id")
	s.mu.RLock()
	clientState, ok := s.clients[clientID]
	s.mu.RUnlock()
	if !ok {
		http.Error(w, "client not found", http.StatusNotFound)
		return
	}

	select {
	case pending := <-clientState.Queue:
		s.metrics.IncCounter("gotunnel_relay_client_polls_delivered_total")
		writeJSON(w, http.StatusOK, pending)
	case <-time.After(25 * time.Second):
		w.WriteHeader(http.StatusNoContent)
	case <-r.Context().Done():
		w.WriteHeader(http.StatusRequestTimeout)
	}
}

func (s *Server) handleSubmitResponse(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var resp protocol.SubmitResponseRequest
	if err := json.NewDecoder(r.Body).Decode(&resp); err != nil {
		http.Error(w, fmt.Sprintf("decode response: %v", err), http.StatusBadRequest)
		return
	}

	s.mu.RLock()
	state, ok := s.requests[resp.RequestID]
	s.mu.RUnlock()
	if !ok {
		http.Error(w, "request not found", http.StatusNotFound)
		return
	}

	select {
	case state.ResponseC <- resp:
		writeJSON(w, http.StatusAccepted, map[string]string{"status": "accepted"})
	default:
		http.Error(w, "response channel unavailable", http.StatusConflict)
	}
}

func (s *Server) handleListTunnels(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.authorizePermission(w, r, auth.PermDashboardAccess); !ok {
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, s.snapshot())
}

func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.authorizeDashboard(w, r); !ok {
		return
	}
	tpl := template.Must(template.New("dashboard").Parse(dashboardTemplate))
	if err := tpl.Execute(w, s.snapshot()); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *Server) handleReplay(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if _, ok := s.authorizePermission(w, r, auth.PermTunnelReplay); !ok {
		return
	}

	if _, err := s.replayRequest(protocol.ReplayRequest{
		RequestID: r.URL.Query().Get("id"),
		Actor:     "dashboard",
	}); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
}

func (s *Server) handleDebugIngress(w http.ResponseWriter, r *http.Request) {
	trimmed := strings.TrimPrefix(r.URL.Path, "/t/")
	parts := strings.SplitN(trimmed, "/", 2)
	if len(parts) == 0 || parts[0] == "" {
		http.Error(w, "missing subdomain", http.StatusBadRequest)
		return
	}
	path := "/"
	if len(parts) == 2 {
		path += parts[1]
	}
	s.handleTunnelRequest(w, r, parts[0], path)
}

func (s *Server) handleHostIngress(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/v1/") || strings.HasPrefix(r.URL.Path, "/dashboard") || strings.HasPrefix(r.URL.Path, "/healthz") {
		http.NotFound(w, r)
		return
	}
	host := r.Host
	if idx := strings.Index(host, ":"); idx > 0 {
		host = host[:idx]
	}
	if !strings.HasSuffix(host, ".gotunnel.local") {
		s.mu.RLock()
		_, ok := s.tunnels[host]
		s.mu.RUnlock()
		if !ok {
			http.NotFound(w, r)
			return
		}
		s.handleTunnelRequest(w, r, host, r.URL.Path)
		return
	}
	subdomain := strings.TrimSuffix(host, ".gotunnel.local")
	s.handleTunnelRequest(w, r, subdomain, r.URL.Path)
}

func (s *Server) ensureManagedCertificate(ctx context.Context, lease protocol.Lease) {
	if s.lifecycleManager == nil || lease.CustomDomain == "" {
		return
	}
	if _, err := s.lifecycleManager.GetCertificate(lease.CustomDomain); err == nil {
		return
	}
	if _, err := s.lifecycleManager.IssueCertificate(ctx, lease.CustomDomain, "dns-01"); err == nil {
		s.mu.Lock()
		if tunnel, ok := s.tunnels[lease.Subdomain]; ok {
			tunnel.Lease.IssuedTLS = true
		}
		if tunnel, ok := s.tunnels[lease.CustomDomain]; ok {
			tunnel.Lease.IssuedTLS = true
		}
		s.mu.Unlock()
	}
}

func (s *Server) handleTunnelRequest(w http.ResponseWriter, r *http.Request, subdomain, path string) {
	if err := s.enforcer.EnforceNetworkPolicy(r.Context(), r.RemoteAddr); err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}
	if r.TLS != nil {
		state := *r.TLS
		if err := s.enforcer.EnforceTLSPolicy(&state); err != nil {
			http.Error(w, err.Error(), http.StatusUpgradeRequired)
			return
		}
	}
	if err := s.enforcer.EnforceRateLimit(r.Context(), &security.RateLimitPolicy{
		RequestsPerSecond: 100,
		BurstSize:         200,
		WindowSize:        time.Second,
		KeyField:          "ip",
	}, r.RemoteAddr); err != nil {
		http.Error(w, err.Error(), http.StatusTooManyRequests)
		return
	}
	s.mu.RLock()
	tunnel, ok := s.tunnels[subdomain]
	s.mu.RUnlock()
	if !ok {
		http.Error(w, "tunnel not found", http.StatusNotFound)
		return
	}

	clientState, ok := s.findClientByTunnelID(tunnel.Lease.ID)
	if !ok {
		http.Error(w, "tunnel client is offline", http.StatusBadGateway)
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, 2<<20))
	if err != nil {
		http.Error(w, fmt.Sprintf("read body: %v", err), http.StatusBadRequest)
		return
	}

	pending := protocol.PendingRequest{
		ID:          fmt.Sprintf("req-%d", time.Now().UnixNano()),
		TunnelID:    tunnel.Lease.ID,
		Subdomain:   subdomain,
		Method:      r.Method,
		Path:        path,
		Query:       r.URL.RawQuery,
		Headers:     cloneHeaders(r.Header),
		Body:        body,
		ReceivedAt:  time.Now().UTC(),
		ContentType: r.Header.Get("Content-Type"),
	}

	start := time.Now()
	respChan := make(chan protocol.SubmitResponseRequest, 1)

	s.mu.Lock()
	s.requests[pending.ID] = &requestState{
		Request:   pending,
		ResponseC: respChan,
	}
	s.mu.Unlock()

	if breakpoint, matched := s.matchBreakpoint(pending); matched {
		paused := newPausedState(pending, breakpoint.ID)
		storePausedRequest(s, paused)
		s.emitEvent("request_paused", "Request paused by breakpoint", pending.TunnelID, pending.ID, "", pausedSnapshot(paused, findSessionsForTunnel(s, pending.TunnelID)))

		select {
		case action := <-paused.ActionC:
			deletePausedRequest(s, pending.ID)
			if action == "discard" {
				s.recordRequest(protocol.RequestLog{
					RequestID:     pending.ID,
					TunnelID:      pending.TunnelID,
					Subdomain:     pending.Subdomain,
					Method:        pending.Method,
					Path:          pending.Path,
					StatusCode:    http.StatusNoContent,
					Duration:      "discarded",
					ReceivedAt:    pending.ReceivedAt,
					CompletedAt:   time.Now().UTC(),
					ResponseError: "discarded by debugger",
				})
				w.WriteHeader(http.StatusNoContent)
				return
			}
		case <-time.After(60 * time.Second):
			deletePausedRequest(s, pending.ID)
			http.Error(w, "paused request expired", http.StatusRequestTimeout)
			return
		case <-r.Context().Done():
			deletePausedRequest(s, pending.ID)
			return
		}
	}

	if !s.sendHTTPToClient(clientState, pending) {
		s.metrics.IncCounter("gotunnel_relay_queue_full_total")
		http.Error(w, "tunnel queue is full", http.StatusServiceUnavailable)
		return
	}
	s.metrics.IncCounter("gotunnel_relay_requests_forwarded_total")
	s.emitEvent("request_forwarded", "Request forwarded to local client", pending.TunnelID, pending.ID, "", pending)

	select {
	case result := <-respChan:
		for key, values := range result.Headers {
			for _, value := range values {
				w.Header().Add(key, value)
			}
		}
		if result.StatusCode == 0 {
			result.StatusCode = http.StatusBadGateway
		}
		w.WriteHeader(result.StatusCode)
		_, _ = w.Write(result.Body)

		s.recordRequest(protocol.RequestLog{
			RequestID:     pending.ID,
			TunnelID:      pending.TunnelID,
			Subdomain:     pending.Subdomain,
			Method:        pending.Method,
			Path:          pending.Path,
			StatusCode:    result.StatusCode,
			Duration:      time.Since(start).String(),
			ReceivedAt:    pending.ReceivedAt,
			CompletedAt:   time.Now().UTC(),
			ResponseError: result.Error,
		})
		_ = s.nfrValidator.ValidateLatency("relay_latency_p99", time.Since(start))
		_ = s.nfrValidator.ValidateScalability("relay_concurrent_tunnels", len(s.snapshot().Tunnels))
	case <-time.After(30 * time.Second):
		http.Error(w, "client did not respond in time", http.StatusGatewayTimeout)
		s.recordRequest(protocol.RequestLog{
			RequestID:   pending.ID,
			TunnelID:    pending.TunnelID,
			Subdomain:   pending.Subdomain,
			Method:      pending.Method,
			Path:        pending.Path,
			StatusCode:  http.StatusGatewayTimeout,
			Duration:    time.Since(start).String(),
			ReceivedAt:  pending.ReceivedAt,
			CompletedAt: time.Now().UTC(),
		})
		_ = s.nfrValidator.ValidateLatency("relay_latency_p99", 30*time.Second)
	}
}

func (s *Server) fetchLease(ctx context.Context, leaseID string) (protocol.Lease, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.brokerURL+"/v1/leases/"+leaseID, nil)
	if err != nil {
		return protocol.Lease{}, fmt.Errorf("build lease lookup request: %w", err)
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return protocol.Lease{}, fmt.Errorf("fetch lease: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return protocol.Lease{}, fmt.Errorf("lease lookup failed: %s", strings.TrimSpace(string(body)))
	}

	var lease protocol.Lease
	if err := json.NewDecoder(resp.Body).Decode(&lease); err != nil {
		return protocol.Lease{}, fmt.Errorf("decode lease: %w", err)
	}
	if lease.DebugURL == "" && s.publicBase != "" {
		lease.DebugURL = s.publicBase + "/t/" + lease.Subdomain
	}
	return lease, nil
}

func (s *Server) enqueueRequestToClient(clientState *clientState, pending protocol.PendingRequest) bool {
	select {
	case clientState.Queue <- pending:
		return true
	default:
		return false
	}
}

func (s *Server) sendHTTPToClient(clientState *clientState, pending protocol.PendingRequest) bool {
	if clientState.QUICUp {
		select {
		case clientState.QUICSend <- protocol.ControlMessage{
			Type:     "http_request",
			ClientID: clientState.ID,
			Request:  &pending,
		}:
			return true
		default:
		}
	}
	return s.enqueueRequestToClient(clientState, pending)
}

func (s *Server) enqueueRequest(pending protocol.PendingRequest) bool {
	clientState, ok := s.findClientByTunnelID(pending.TunnelID)
	if !ok {
		return false
	}
	return s.enqueueRequestToClient(clientState, pending)
}

func (s *Server) findClientByTunnelID(tunnelID string) (*clientState, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, clientState := range s.clients {
		if clientState.TunnelID == tunnelID {
			return clientState, true
		}
	}
	return nil, false
}

func (s *Server) recordRequest(log protocol.RequestLog) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.requestLog = append([]protocol.RequestLog{log}, s.requestLog...)
	if len(s.requestLog) > 100 {
		s.requestLog = s.requestLog[:100]
	}
	go s.emitEvent("request_completed", "Request completed", log.TunnelID, log.RequestID, "", log)
	go s.dispatchWebhook("request_completed", log)
}

func (s *Server) snapshot() protocol.DashboardSnapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()

	tunnels := make([]protocol.TunnelSnapshot, 0, len(s.tunnels))
	for _, tunnel := range s.tunnels {
		snapshot := protocol.TunnelSnapshot{
			ID:         tunnel.Lease.ID,
			Name:       tunnel.Lease.Name,
			Subdomain:  tunnel.Lease.Subdomain,
			PublicHost: tunnel.Lease.PublicHost,
			DebugURL:   tunnel.Lease.DebugURL,
			Connected:  s.hasClientLocked(tunnel.Lease.ID),
			Protocol:   tunnel.Lease.Protocol,
			Inspection: tunnel.Lease.Inspection,
			IssuedTLS:  tunnel.Lease.IssuedTLS,
			StartedAt:  tunnel.Lease.CreatedAt,
		}
		for _, log := range s.requestLog {
			if log.TunnelID == tunnel.Lease.ID {
				snapshot.LastRequestAt = log.ReceivedAt
				break
			}
		}
		tunnels = append(tunnels, snapshot)
	}
	sort.Slice(tunnels, func(i, j int) bool {
		return tunnels[i].StartedAt.After(tunnels[j].StartedAt)
	})

	requests := append([]protocol.RequestLog(nil), s.requestLog...)
	return protocol.DashboardSnapshot{
		Tunnels:        tunnels,
		Requests:       requests,
		Sessions:       listSessions(s),
		PausedRequests: listPausedRequests(s),
		Breakpoints:    listBreakpoints(s),
		Events:         listEvents(s),
	}
}

func (s *Server) hasClientLocked(tunnelID string) bool {
	for _, clientState := range s.clients {
		if clientState.TunnelID == tunnelID {
			return true
		}
	}
	return false
}

func (s *Server) checkBasicAuth(w http.ResponseWriter, r *http.Request) bool {
	if s.dashboardUser == "" && s.dashboardPass == "" {
		return true
	}
	user, pass, ok := r.BasicAuth()
	if !ok || user != s.dashboardUser || pass != s.dashboardPass {
		w.Header().Set("WWW-Authenticate", `Basic realm="GoTunnel Dashboard"`)
		http.Error(w, "authentication required", http.StatusUnauthorized)
		return false
	}
	return true
}

func (s *Server) authorizeDashboard(w http.ResponseWriter, r *http.Request) (auth.User, bool) {
	if user, ok := s.authorizePermission(w, r, auth.PermDashboardAccess); ok {
		return user, true
	}
	if s.checkBasicAuth(w, r) {
		return auth.User{Username: s.dashboardUser, Permissions: auth.CalculatePermissions([]auth.Role{auth.RoleAdmin}, nil)}, true
	}
	return auth.User{}, false
}

func (s *Server) authorizePermission(w http.ResponseWriter, r *http.Request, perm auth.Permission) (auth.User, bool) {
	token := sessionTokenFromRequest(r)
	if token == "" {
		return auth.User{}, false
	}
	user, session, err := s.authenticator.VerifySession(r.Context(), token)
	if err != nil {
		http.Error(w, "invalid session", http.StatusUnauthorized)
		return auth.User{}, false
	}
	if user.MFA.Enabled && !session.MFAVerified {
		http.Error(w, "mfa verification required", http.StatusForbidden)
		return auth.User{}, false
	}
	if !auth.HasPermission(user, perm) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return auth.User{}, false
	}
	return user, true
}

func sessionTokenFromRequest(r *http.Request) string {
	if header := r.Header.Get("Authorization"); strings.HasPrefix(header, "Bearer ") {
		return strings.TrimSpace(strings.TrimPrefix(header, "Bearer "))
	}
	if cookie, err := r.Cookie("gotunnel_session"); err == nil {
		return strings.TrimSpace(cookie.Value)
	}
	return ""
}

func (s *Server) dispatchWebhook(eventType string, payload interface{}) {
	dispatchWebhook(s, eventType, payload)
}

func cloneHeaders(h http.Header) map[string][]string {
	out := make(map[string][]string, len(h))
	for key, values := range h {
		out[key] = append([]string(nil), values...)
	}
	return out
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

const dashboardTemplate = `
<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <title>GoTunnel Dashboard</title>
  <style>
    body { font-family: Georgia, serif; margin: 2rem; background: #f3efe7; color: #1d1b18; }
    h1, h2 { margin-bottom: 0.5rem; }
    table { border-collapse: collapse; width: 100%; background: #fffaf1; margin-bottom: 2rem; }
    th, td { border: 1px solid #d9cfbb; padding: 0.6rem; text-align: left; vertical-align: top; }
    code { background: #eee4d2; padding: 0.1rem 0.3rem; }
    .online { color: #0f6a37; font-weight: bold; }
    .offline { color: #9d3c1c; font-weight: bold; }
    form { margin: 0; }
    button { background: #0c6b58; color: white; border: none; padding: 0.4rem 0.7rem; cursor: pointer; }
  </style>
</head>
<body>
  <h1>GoTunnel Dashboard</h1>
  <p>Current active tunnels, inspection history, and replay controls for the MVP relay.</p>

  <h2>Tunnels</h2>
  <table>
    <thead>
      <tr>
        <th>Name</th>
        <th>Subdomain</th>
        <th>Debug URL</th>
        <th>Status</th>
        <th>TLS</th>
        <th>Last Request</th>
      </tr>
    </thead>
    <tbody>
      {{range .Tunnels}}
      <tr>
        <td>{{.Name}}</td>
        <td><code>{{.Subdomain}}</code></td>
        <td><code>{{.DebugURL}}</code></td>
        <td>{{if .Connected}}<span class="online">online</span>{{else}}<span class="offline">offline</span>{{end}}</td>
        <td>{{if .IssuedTLS}}managed{{else}}pending{{end}}</td>
        <td>{{if .LastRequestAt.IsZero}}-{{else}}{{.LastRequestAt}}{{end}}</td>
      </tr>
      {{else}}
      <tr><td colspan="6">No tunnels registered yet.</td></tr>
      {{end}}
    </tbody>
  </table>

  <h2>Recent Requests</h2>
  <table>
    <thead>
      <tr>
        <th>Request ID</th>
        <th>Tunnel ID</th>
        <th>Route</th>
        <th>Status</th>
        <th>Duration</th>
        <th>Replay</th>
      </tr>
    </thead>
    <tbody>
      {{range .Requests}}
      <tr>
        <td><code>{{.RequestID}}</code></td>
        <td><code>{{.TunnelID}}</code></td>
        <td>{{.Method}} {{.Path}}</td>
        <td>{{.StatusCode}}</td>
        <td>{{.Duration}}</td>
        <td>
          <form method="post" action="/dashboard/replay?id={{.RequestID}}">
            <button type="submit">Replay</button>
          </form>
        </td>
      </tr>
      {{else}}
      <tr><td colspan="6">No requests captured yet.</td></tr>
      {{end}}
    </tbody>
  </table>
</body>
</html>
`
