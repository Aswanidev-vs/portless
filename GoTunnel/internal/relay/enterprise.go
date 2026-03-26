package relay

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"net/http"
	"strings"

	"gotunnel/internal/auth"
	"gotunnel/internal/nfr"
	"gotunnel/internal/protocol"
)

func tlsVersion13() uint16 {
	return tls.VersionTLS13
}

func bootstrapUser(a *auth.Authenticator, username, password string) {
	if strings.TrimSpace(username) == "" {
		return
	}
	_, _ = a.CreateUser(context.Background(), auth.CreateUserRequest{
		Username:    username,
		Email:       username + "@gotunnel.local",
		DisplayName: "GoTunnel Admin",
		Password:    password,
		Roles:       []auth.Role{auth.RoleAdmin},
		WorkspaceID: "default",
	})
}

func addDefaultNFRs(v *nfr.Validator) {
	v.AddRequirement(&nfr.NFRRequirement{ID: "relay_latency_p99", Type: nfr.NFRLatency, Name: "Relay latency p99", Threshold: 100, Unit: "ms", Status: nfr.NFRStatusUnknown})
	v.AddRequirement(&nfr.NFRRequirement{ID: "relay_uptime", Type: nfr.NFRUptime, Name: "Relay uptime", Threshold: 99.9, Unit: "percent", Status: nfr.NFRStatusUnknown})
	v.AddRequirement(&nfr.NFRRequirement{ID: "relay_concurrent_tunnels", Type: nfr.NFRScalability, Name: "Concurrent tunnels", Threshold: 10000, Unit: "tunnels", Status: nfr.NFRStatusUnknown})
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	session, err := s.authenticator.AuthenticateWithPassword(r.Context(), req.Username, req.Password)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}
	user, _, _ := s.authenticator.VerifySession(r.Context(), session.Token)
	http.SetCookie(w, &http.Cookie{Name: "gotunnel_session", Value: session.Token, Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode})
	writeJSON(w, http.StatusOK, protocol.AuthSessionResponse{Token: session.Token, ExpiresAt: session.ExpiresAt, MFAVerified: session.MFAVerified, User: user})
}

func (s *Server) handleSSOLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Provider string `json:"provider"`
		Code     string `json:"code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	session, err := s.authenticator.AuthenticateWithSSO(r.Context(), localFirstNonEmpty(req.Provider, "dev-sso"), req.Code)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}
	user, _, _ := s.authenticator.VerifySession(r.Context(), session.Token)
	http.SetCookie(w, &http.Cookie{Name: "gotunnel_session", Value: session.Token, Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode})
	writeJSON(w, http.StatusOK, protocol.AuthSessionResponse{Token: session.Token, ExpiresAt: session.ExpiresAt, MFAVerified: session.MFAVerified, User: user})
}

func (s *Server) handleMFASetup(w http.ResponseWriter, r *http.Request) {
	user, ok := s.authorizePermission(w, r, auth.PermDashboardAccess)
	if !ok {
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	setup, err := s.authenticator.EnableMFA(r.Context(), user.ID, "totp")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, http.StatusCreated, setup)
}

func (s *Server) handleMFAVerify(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Code string `json:"code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	token := sessionTokenFromRequest(r)
	if token == "" {
		http.Error(w, "missing session", http.StatusUnauthorized)
		return
	}
	if err := s.authenticator.VerifyMFA(r.Context(), token, req.Code); err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "verified"})
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	token := sessionTokenFromRequest(r)
	if token == "" {
		http.Error(w, "missing session", http.StatusUnauthorized)
		return
	}
	user, session, err := s.authenticator.VerifySession(r.Context(), token)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}
	writeJSON(w, http.StatusOK, protocol.AuthSessionResponse{Token: session.Token, ExpiresAt: session.ExpiresAt, MFAVerified: session.MFAVerified, User: user})
}

func (s *Server) handleSecurityAudit(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.authorizePermission(w, r, auth.PermAuditView); !ok {
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"events": s.enforcer.GetAuditLog(),
	})
}

func (s *Server) handleNFRStatus(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.authorizePermission(w, r, auth.PermDashboardAccess); !ok {
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"requirements": s.nfrValidator.ListRequirements(),
		"summary":      s.nfrValidator.GetStatusSummary(),
		"failing":      s.nfrValidator.GetFailingRequirements(),
	})
}
