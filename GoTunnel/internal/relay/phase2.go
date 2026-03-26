package relay

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"gotunnel/internal/auth"
	"gotunnel/internal/protocol"
)

type phase2State struct {
	mu            sync.RWMutex
	webhookSecret string
	allowMutation bool
	paused        map[string]*pausedState
	sessions      map[string]*sessionState
	invites       map[string]string
	annotations   map[string][]protocol.Annotation
	breakpoints   map[string][]protocol.Breakpoint
	events        []protocol.StreamEvent
	subscribers   map[chan protocol.StreamEvent]struct{}
}

type pausedState struct {
	Request      protocol.PendingRequest
	ActionC      chan string
	PausedAt     time.Time
	BreakpointID string
}

type sessionState struct {
	Session protocol.CollaborationSession
}

var phase2Registry sync.Map

func initPhase2State(s *Server, webhookSecret string, allowMutation bool) {
	phase2Registry.Store(s, &phase2State{
		webhookSecret: strings.TrimSpace(webhookSecret),
		allowMutation: allowMutation,
		paused:        make(map[string]*pausedState),
		sessions:      make(map[string]*sessionState),
		invites:       make(map[string]string),
		annotations:   make(map[string][]protocol.Annotation),
		breakpoints:   make(map[string][]protocol.Breakpoint),
		subscribers:   make(map[chan protocol.StreamEvent]struct{}),
	})
}

func p2(s *Server) *phase2State {
	state, _ := phase2Registry.Load(s)
	return state.(*phase2State)
}

func newPausedState(req protocol.PendingRequest, breakpointID string) *pausedState {
	return &pausedState{
		Request:      req,
		ActionC:      make(chan string, 1),
		PausedAt:     time.Now().UTC(),
		BreakpointID: breakpointID,
	}
}

func storePausedRequest(s *Server, paused *pausedState) {
	state := p2(s)
	state.mu.Lock()
	defer state.mu.Unlock()
	state.paused[paused.Request.ID] = paused
}

func deletePausedRequest(s *Server, requestID string) {
	state := p2(s)
	state.mu.Lock()
	defer state.mu.Unlock()
	delete(state.paused, requestID)
}

func listPausedRequests(s *Server) []protocol.PausedRequestSnapshot {
	state := p2(s)
	state.mu.RLock()
	defer state.mu.RUnlock()
	out := make([]protocol.PausedRequestSnapshot, 0, len(state.paused))
	for _, paused := range state.paused {
		out = append(out, pausedSnapshot(paused, findSessionsForTunnel(s, paused.Request.TunnelID)))
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].PausedAt.After(out[j].PausedAt)
	})
	return out
}

func pausedSnapshot(state *pausedState, sessionIDs []string) protocol.PausedRequestSnapshot {
	return protocol.PausedRequestSnapshot{
		Request:      state.Request,
		SessionIDs:   sessionIDs,
		BreakpointID: state.BreakpointID,
		PausedAt:     state.PausedAt,
	}
}

func (s *Server) handleSessions(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		if _, ok := s.authorizePermission(w, r, auth.PermCollabJoin); !ok {
			return
		}
		writeJSON(w, http.StatusOK, listSessions(s))
	case http.MethodPost:
		if _, ok := s.authorizePermission(w, r, auth.PermCollabManage); !ok {
			return
		}
		var req protocol.CreateSessionRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		session, err := createSession(s, req)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusCreated, session)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleJoinSession(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.authorizePermission(w, r, auth.PermCollabJoin); !ok {
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req protocol.JoinSessionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	session, err := joinSession(s, req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, http.StatusOK, session)
}

func (s *Server) handleAnnotation(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.authorizePermission(w, r, auth.PermTunnelInspect); !ok {
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req protocol.AnnotateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	annotation, err := addAnnotation(s, req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, http.StatusCreated, annotation)
}

func (s *Server) handleBreakpoints(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		if _, ok := s.authorizePermission(w, r, auth.PermTunnelInspect); !ok {
			return
		}
		writeJSON(w, http.StatusOK, listBreakpoints(s))
	case http.MethodPost:
		if _, ok := s.authorizePermission(w, r, auth.PermCollabManage); !ok {
			return
		}
		var req protocol.CreateBreakpointRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		breakpoint, err := addBreakpoint(s, req)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusCreated, breakpoint)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handlePausedAction(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.authorizePermission(w, r, auth.PermCollabManage); !ok {
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req protocol.PauseActionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if req.Action != "release" && req.Action != "discard" {
		http.Error(w, "action must be release or discard", http.StatusBadRequest)
		return
	}
	if err := resolvePausedRequest(s, req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": req.Action})
}

func (s *Server) handleReplayAPI(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.authorizePermission(w, r, auth.PermTunnelReplay); !ok {
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req protocol.ReplayRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	pending, err := s.replayRequest(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, http.StatusAccepted, pending)
}

func (s *Server) handleDashboardEvents(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.authorizeDashboard(w, r); !ok {
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	state := p2(s)
	ch := make(chan protocol.StreamEvent, 16)
	state.mu.Lock()
	state.subscribers[ch] = struct{}{}
	history := append([]protocol.StreamEvent(nil), state.events...)
	state.mu.Unlock()
	defer func() {
		state.mu.Lock()
		delete(state.subscribers, ch)
		state.mu.Unlock()
		close(ch)
	}()

	for i := len(history) - 1; i >= 0; i-- {
		writeSSE(w, history[i])
	}
	flusher.Flush()

	ticker := time.NewTicker(20 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case event := <-ch:
			writeSSE(w, event)
			flusher.Flush()
		case <-ticker.C:
			_, _ = fmt.Fprint(w, ": keepalive\n\n")
			flusher.Flush()
		case <-r.Context().Done():
			return
		}
	}
}

func createSession(s *Server, req protocol.CreateSessionRequest) (protocol.CollaborationSession, error) {
	if strings.TrimSpace(req.Owner) == "" {
		return protocol.CollaborationSession{}, fmt.Errorf("owner is required")
	}
	tunnelID, subdomain, ok := resolveTunnelIdentity(s, req.TunnelID, req.Subdomain)
	if !ok {
		return protocol.CollaborationSession{}, fmt.Errorf("tunnel not found")
	}
	lease := findLeaseByTunnelID(s, tunnelID)
	if !lease.AllowSharing || !p3(s).plugins.AllowSharing(lease) {
		return protocol.CollaborationSession{}, fmt.Errorf("sharing is disabled for this tunnel by policy")
	}

	session := protocol.CollaborationSession{
		ID:            fmt.Sprintf("sess-%d", time.Now().UnixNano()),
		TunnelID:       tunnelID,
		Subdomain:      subdomain,
		InviteToken:    fmt.Sprintf("invite-%d", time.Now().UnixNano()),
		InviteURL:      strings.TrimRight(s.publicBase, "/") + "/dashboard?invite=" + fmt.Sprintf("invite-%d", time.Now().UnixNano()),
		Owner:          req.Owner,
		Status:         "active",
		Participants:   []protocol.Participant{{Name: req.Owner, Role: "owner", JoinedAt: time.Now().UTC()}},
		CreatedAt:      time.Now().UTC(),
		LastActivityAt: time.Now().UTC(),
	}
	session.InviteURL = strings.TrimRight(s.publicBase, "/") + "/dashboard?invite=" + session.InviteToken

	state := p2(s)
	state.mu.Lock()
	state.sessions[session.ID] = &sessionState{Session: session}
	state.invites[session.InviteToken] = session.ID
	state.mu.Unlock()

	s.emitEvent("session_created", "Collaborative session created", session.TunnelID, "", session.ID, session)
	s.dispatchWebhook("collaboration_session_started", session)
	return session, nil
}

func joinSession(s *Server, req protocol.JoinSessionRequest) (protocol.CollaborationSession, error) {
	if strings.TrimSpace(req.Token) == "" || strings.TrimSpace(req.Name) == "" {
		return protocol.CollaborationSession{}, fmt.Errorf("token and name are required")
	}
	role := req.Role
	if role == "" {
		role = "debugger"
	}

	state := p2(s)
	state.mu.Lock()
	defer state.mu.Unlock()

	sessionID, ok := state.invites[req.Token]
	if !ok {
		return protocol.CollaborationSession{}, fmt.Errorf("invite not found")
	}
	session := state.sessions[sessionID]
	for _, participant := range session.Session.Participants {
		if participant.Name == req.Name {
			return session.Session, nil
		}
	}
	session.Session.Participants = append(session.Session.Participants, protocol.Participant{
		Name:     req.Name,
		Role:     role,
		JoinedAt: time.Now().UTC(),
	})
	session.Session.LastActivityAt = time.Now().UTC()
	out := session.Session
	go s.emitEvent("session_joined", "Participant joined collaborative session", out.TunnelID, "", out.ID, out)
	return out, nil
}

func addAnnotation(s *Server, req protocol.AnnotateRequest) (protocol.Annotation, error) {
	if strings.TrimSpace(req.RequestID) == "" || strings.TrimSpace(req.Author) == "" || strings.TrimSpace(req.Message) == "" {
		return protocol.Annotation{}, fmt.Errorf("request_id, author, and message are required")
	}
	annotation := protocol.Annotation{
		ID:        fmt.Sprintf("ann-%d", time.Now().UnixNano()),
		RequestID: req.RequestID,
		SessionID: req.SessionID,
		Author:    req.Author,
		Message:   req.Message,
		CreatedAt: time.Now().UTC(),
	}
	state := p2(s)
	state.mu.Lock()
	state.annotations[req.RequestID] = append(state.annotations[req.RequestID], annotation)
	state.mu.Unlock()
	s.emitEvent("annotation_created", "Request annotation added", "", req.RequestID, req.SessionID, annotation)
	return annotation, nil
}

func addBreakpoint(s *Server, req protocol.CreateBreakpointRequest) (protocol.Breakpoint, error) {
	if strings.TrimSpace(req.TunnelID) == "" || strings.TrimSpace(req.CreatedBy) == "" {
		return protocol.Breakpoint{}, fmt.Errorf("tunnel_id and created_by are required")
	}
	breakpoint := protocol.Breakpoint{
		ID:          fmt.Sprintf("bp-%d", time.Now().UnixNano()),
		TunnelID:    req.TunnelID,
		Method:      strings.ToUpper(req.Method),
		PathPrefix:  req.PathPrefix,
		HeaderName:  http.CanonicalHeaderKey(req.HeaderName),
		HeaderValue: req.HeaderValue,
		CreatedBy:   req.CreatedBy,
		CreatedAt:   time.Now().UTC(),
		Enabled:     true,
	}
	state := p2(s)
	state.mu.Lock()
	state.breakpoints[req.TunnelID] = append(state.breakpoints[req.TunnelID], breakpoint)
	state.mu.Unlock()
	s.emitEvent("breakpoint_created", "Traffic breakpoint created", req.TunnelID, "", "", breakpoint)
	return breakpoint, nil
}

func resolvePausedRequest(s *Server, req protocol.PauseActionRequest) error {
	state := p2(s)
	state.mu.RLock()
	paused, ok := state.paused[req.RequestID]
	state.mu.RUnlock()
	if !ok {
		return fmt.Errorf("paused request not found")
	}
	select {
	case paused.ActionC <- req.Action:
		s.emitEvent("paused_request_"+req.Action, "Paused request action applied", paused.Request.TunnelID, paused.Request.ID, "", req)
		return nil
	default:
		return fmt.Errorf("paused request already resolved")
	}
}

func (s *Server) replayRequest(req protocol.ReplayRequest) (protocol.PendingRequest, error) {
	if strings.TrimSpace(req.RequestID) == "" {
		return protocol.PendingRequest{}, fmt.Errorf("request_id is required")
	}
	s.mu.RLock()
	state, ok := s.requests[req.RequestID]
	s.mu.RUnlock()
	if !ok {
		return protocol.PendingRequest{}, fmt.Errorf("request not found")
	}

	replayed := state.Request
	replayed.ID = fmt.Sprintf("req-%d", time.Now().UnixNano())
	replayed.ReceivedAt = time.Now().UTC()
	phase := p2(s)
	if phase.allowMutation {
		if req.Path != "" {
			replayed.Path = req.Path
		}
		if len(req.Body) > 0 {
			replayed.Body = req.Body
		}
		if len(req.Headers) > 0 {
			replayed.Headers = req.Headers
		}
	}
	if !s.enqueueRequest(replayed) {
		return protocol.PendingRequest{}, fmt.Errorf("tunnel is offline")
	}
	s.emitEvent("request_replayed", "Request replay queued", replayed.TunnelID, replayed.ID, "", replayed)
	return replayed, nil
}

func (s *Server) matchBreakpoint(req protocol.PendingRequest) (protocol.Breakpoint, bool) {
	state := p2(s)
	state.mu.RLock()
	breakpoints := append([]protocol.Breakpoint(nil), state.breakpoints[req.TunnelID]...)
	state.mu.RUnlock()

	for _, bp := range breakpoints {
		if !bp.Enabled {
			continue
		}
		if bp.Method != "" && bp.Method != strings.ToUpper(req.Method) {
			continue
		}
		if bp.PathPrefix != "" && !strings.HasPrefix(req.Path, bp.PathPrefix) {
			continue
		}
		if bp.HeaderName != "" {
			values := req.Headers[bp.HeaderName]
			if len(values) == 0 {
				continue
			}
			if bp.HeaderValue != "" {
				match := false
				for _, value := range values {
					if value == bp.HeaderValue {
						match = true
						break
					}
				}
				if !match {
					continue
				}
			}
		}
		return bp, true
	}
	return protocol.Breakpoint{}, false
}

func resolveTunnelIdentity(s *Server, tunnelID, subdomain string) (string, string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if tunnelID != "" {
		for sub, tunnel := range s.tunnels {
			if tunnel.Lease.ID == tunnelID {
				return tunnel.Lease.ID, sub, true
			}
		}
	}
	if subdomain != "" {
		tunnel, ok := s.tunnels[subdomain]
		if !ok {
			return "", "", false
		}
		return tunnel.Lease.ID, subdomain, true
	}
	return "", "", false
}

func findLeaseByTunnelID(s *Server, tunnelID string) protocol.Lease {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, tunnel := range s.tunnels {
		if tunnel.Lease.ID == tunnelID {
			return tunnel.Lease
		}
	}
	return protocol.Lease{}
}

func listSessions(s *Server) []protocol.CollaborationSession {
	state := p2(s)
	state.mu.RLock()
	defer state.mu.RUnlock()
	out := make([]protocol.CollaborationSession, 0, len(state.sessions))
	for _, session := range state.sessions {
		out = append(out, session.Session)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].CreatedAt.After(out[j].CreatedAt)
	})
	return out
}

func findSessionsForTunnel(s *Server, tunnelID string) []string {
	state := p2(s)
	state.mu.RLock()
	defer state.mu.RUnlock()
	out := []string{}
	for _, session := range state.sessions {
		if session.Session.TunnelID == tunnelID && session.Session.Status == "active" {
			out = append(out, session.Session.ID)
		}
	}
	return out
}

func listBreakpoints(s *Server) []protocol.Breakpoint {
	state := p2(s)
	state.mu.RLock()
	defer state.mu.RUnlock()
	out := []protocol.Breakpoint{}
	for _, items := range state.breakpoints {
		out = append(out, items...)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].CreatedAt.After(out[j].CreatedAt)
	})
	return out
}

func listEvents(s *Server) []protocol.StreamEvent {
	state := p2(s)
	state.mu.RLock()
	defer state.mu.RUnlock()
	return append([]protocol.StreamEvent(nil), state.events...)
}

func (s *Server) emitEvent(kind, message, tunnelID, requestID, sessionID string, payload interface{}) {
	event := protocol.StreamEvent{
		ID:        fmt.Sprintf("evt-%d", time.Now().UnixNano()),
		Type:      kind,
		Message:   message,
		TunnelID:  tunnelID,
		RequestID: requestID,
		SessionID: sessionID,
		CreatedAt: time.Now().UTC(),
		Payload:   payload,
	}
	state := p2(s)
	state.mu.Lock()
	state.events = append([]protocol.StreamEvent{event}, state.events...)
	if len(state.events) > 100 {
		state.events = state.events[:100]
	}
	subscribers := make([]chan protocol.StreamEvent, 0, len(state.subscribers))
	for sub := range state.subscribers {
		subscribers = append(subscribers, sub)
	}
	state.mu.Unlock()

	for _, subscriber := range subscribers {
		select {
		case subscriber <- event:
		default:
		}
	}
}

func writeSSE(w http.ResponseWriter, event protocol.StreamEvent) {
	data, _ := json.Marshal(event)
	_, _ = fmt.Fprintf(w, "event: %s\n", event.Type)
	_, _ = fmt.Fprintf(w, "data: %s\n\n", data)
}

func dispatchWebhook(s *Server, eventType string, payload interface{}) {
	if s.webhookURL == "" {
		return
	}
	event := protocol.WebhookEvent{
		Type:      eventType,
		Timestamp: time.Now().UTC(),
		Payload:   payload,
	}
	data, err := json.Marshal(event)
	if err != nil {
		return
	}

	signature := ""
	state := p2(s)
	if state.webhookSecret != "" {
		mac := hmac.New(sha256.New, []byte(state.webhookSecret))
		_, _ = mac.Write(data)
		signature = "sha256=" + hex.EncodeToString(mac.Sum(nil))
	}

	go func() {
		for attempt := 0; attempt < 3; attempt++ {
			req, err := http.NewRequest(http.MethodPost, s.webhookURL, bytes.NewReader(data))
			if err != nil {
				return
			}
			req.Header.Set("Content-Type", "application/json")
			if signature != "" {
				req.Header.Set("X-GoTunnel-Signature", signature)
			}
			resp, err := s.client.Do(req)
			if err == nil && resp != nil && resp.StatusCode >= 200 && resp.StatusCode < 300 {
				if resp.Body != nil {
					resp.Body.Close()
				}
				return
			}
			if resp != nil && resp.Body != nil {
				resp.Body.Close()
			}
			time.Sleep(time.Duration(attempt+1) * 500 * time.Millisecond)
		}
	}()
}
