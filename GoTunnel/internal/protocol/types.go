package protocol

import "time"

type OpenSessionRequest struct {
	Token              string `json:"token"`
	Name               string `json:"name"`
	Protocol           string `json:"protocol"`
	RequestedSubdomain string `json:"requested_subdomain,omitempty"`
	RequestedRegion    string `json:"requested_region,omitempty"`
	LocalURL           string `json:"local_url"`
	InspectionEnabled  bool   `json:"inspection_enabled"`
	HTTPSMode          string `json:"https_mode"`
	WebhookURL         string `json:"webhook_url,omitempty"`
	CustomDomain       string `json:"custom_domain,omitempty"`
	Production         bool   `json:"production"`
	AllowSharing       bool   `json:"allow_sharing"`
	Labels             []string `json:"labels,omitempty"`
}

type Lease struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	Protocol   string    `json:"protocol"`
	Subdomain  string    `json:"subdomain"`
	PublicHost string    `json:"public_host"`
	PublicURL  string    `json:"public_url"`
	DebugURL   string    `json:"debug_url"`
	RelayURL   string    `json:"relay_url"`
	Inspection bool      `json:"inspection"`
	HTTPSMode  string    `json:"https_mode"`
	WebhookURL string    `json:"webhook_url,omitempty"`
	IssuedTLS  bool      `json:"issued_tls"`
	Region     string    `json:"region,omitempty"`
	CustomDomain string  `json:"custom_domain,omitempty"`
	Production bool      `json:"production"`
	AllowSharing bool    `json:"allow_sharing"`
	Labels    []string   `json:"labels,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
}

type RegisterTunnelRequest struct {
	LeaseID string `json:"lease_id"`
}

type RegisterTunnelResponse struct {
	ClientID string `json:"client_id"`
	Lease    Lease  `json:"lease"`
	Message  string `json:"message"`
	TCPPort  int    `json:"tcp_port,omitempty"`
	QUICAddr string `json:"quic_addr,omitempty"`
	ReconnectToken string `json:"reconnect_token,omitempty"`
}

type PendingRequest struct {
	ID          string              `json:"id"`
	TunnelID    string              `json:"tunnel_id"`
	Subdomain   string              `json:"subdomain,omitempty"`
	Method      string              `json:"method"`
	Path        string              `json:"path"`
	Query       string              `json:"query,omitempty"`
	Headers     map[string][]string `json:"headers"`
	Body        []byte              `json:"body,omitempty"`
	ReceivedAt  time.Time           `json:"received_at"`
	ContentType string              `json:"content_type,omitempty"`
}

type SubmitResponseRequest struct {
	ClientID   string              `json:"client_id"`
	RequestID  string              `json:"request_id"`
	StatusCode int                 `json:"status_code"`
	Headers    map[string][]string `json:"headers"`
	Body       []byte              `json:"body,omitempty"`
	Error      string              `json:"error,omitempty"`
}

type TunnelSnapshot struct {
	ID            string    `json:"id"`
	Name          string    `json:"name"`
	Subdomain     string    `json:"subdomain"`
	PublicHost    string    `json:"public_host"`
	DebugURL      string    `json:"debug_url"`
	Connected     bool      `json:"connected"`
	Protocol      string    `json:"protocol"`
	Inspection    bool      `json:"inspection"`
	IssuedTLS     bool      `json:"issued_tls"`
	StartedAt     time.Time `json:"started_at"`
	LastRequestAt time.Time `json:"last_request_at,omitempty"`
}

type RequestLog struct {
	RequestID     string    `json:"request_id"`
	TunnelID      string    `json:"tunnel_id"`
	Subdomain     string    `json:"subdomain,omitempty"`
	Method        string    `json:"method"`
	Path          string    `json:"path"`
	StatusCode    int       `json:"status_code"`
	Duration      string    `json:"duration"`
	ReceivedAt    time.Time `json:"received_at"`
	CompletedAt   time.Time `json:"completed_at"`
	ResponseError string    `json:"response_error,omitempty"`
}

type DashboardSnapshot struct {
	Tunnels        []TunnelSnapshot        `json:"tunnels"`
	Requests       []RequestLog            `json:"requests"`
	Sessions       []CollaborationSession  `json:"sessions"`
	PausedRequests []PausedRequestSnapshot `json:"paused_requests"`
	Breakpoints    []Breakpoint            `json:"breakpoints"`
	Events         []StreamEvent           `json:"events"`
}

type WebhookEvent struct {
	Type      string      `json:"type"`
	Timestamp time.Time   `json:"timestamp"`
	Payload   interface{} `json:"payload"`
}

type RelayRegistrationRequest struct {
	ID         string   `json:"id,omitempty"`
	Name       string   `json:"name"`
	Region     string   `json:"region"`
	RelayURL   string   `json:"relay_url"`
	PublicBase string   `json:"public_base"`
	Capacity   int      `json:"capacity"`
	Features   []string `json:"features,omitempty"`
}

type DomainRequest struct {
	Domain   string            `json:"domain"`
	Provider string            `json:"provider"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

type DomainRecord struct {
	Domain     string            `json:"domain"`
	Provider   string            `json:"provider"`
	Status     string            `json:"status"`
	Challenge  string            `json:"challenge,omitempty"`
	Metadata   map[string]string `json:"metadata,omitempty"`
	UpdatedAt  time.Time         `json:"updated_at"`
}

type AuthSessionResponse struct {
	Token       string      `json:"token"`
	ExpiresAt   time.Time   `json:"expires_at"`
	MFAVerified bool        `json:"mfa_verified"`
	User        interface{} `json:"user"`
}

type RelayRegistration struct {
	ID            string    `json:"id"`
	Name          string    `json:"name"`
	Region        string    `json:"region"`
	RelayURL      string    `json:"relay_url"`
	PublicBase    string    `json:"public_base"`
	Capacity      int       `json:"capacity"`
	Features      []string  `json:"features,omitempty"`
	RegisteredAt  time.Time `json:"registered_at"`
	LastHeartbeat time.Time `json:"last_heartbeat"`
	AssignedTunnels int     `json:"assigned_tunnels"`
	Status        string    `json:"status"`
}

type RelayHeartbeatRequest struct {
	ID              string `json:"id"`
	AssignedTunnels int    `json:"assigned_tunnels"`
}

type AuditExport struct {
	GeneratedAt time.Time             `json:"generated_at"`
	Tunnels     []TunnelSnapshot      `json:"tunnels"`
	Requests    []RequestLog          `json:"requests"`
	Sessions    []CollaborationSession `json:"sessions"`
	Breakpoints []Breakpoint          `json:"breakpoints"`
	Events      []StreamEvent         `json:"events"`
}

type CollaborationSession struct {
	ID            string        `json:"id"`
	TunnelID       string        `json:"tunnel_id"`
	Subdomain      string        `json:"subdomain"`
	InviteToken    string        `json:"invite_token,omitempty"`
	InviteURL      string        `json:"invite_url,omitempty"`
	Owner          string        `json:"owner"`
	Status         string        `json:"status"`
	Participants   []Participant `json:"participants"`
	CreatedAt      time.Time     `json:"created_at"`
	LastActivityAt time.Time     `json:"last_activity_at"`
}

type Participant struct {
	Name     string    `json:"name"`
	Role     string    `json:"role"`
	JoinedAt time.Time `json:"joined_at"`
}

type Annotation struct {
	ID        string    `json:"id"`
	RequestID string    `json:"request_id"`
	SessionID string    `json:"session_id,omitempty"`
	Author    string    `json:"author"`
	Message   string    `json:"message"`
	CreatedAt time.Time `json:"created_at"`
}

type Breakpoint struct {
	ID          string    `json:"id"`
	TunnelID    string    `json:"tunnel_id"`
	Method      string    `json:"method,omitempty"`
	PathPrefix  string    `json:"path_prefix,omitempty"`
	HeaderName  string    `json:"header_name,omitempty"`
	HeaderValue string    `json:"header_value,omitempty"`
	CreatedBy   string    `json:"created_by"`
	CreatedAt   time.Time `json:"created_at"`
	Enabled     bool      `json:"enabled"`
}

type PauseActionRequest struct {
	RequestID string `json:"request_id"`
	Actor     string `json:"actor"`
	Action    string `json:"action"`
}

type PausedRequestSnapshot struct {
	Request      PendingRequest `json:"request"`
	SessionIDs   []string       `json:"session_ids,omitempty"`
	BreakpointID string         `json:"breakpoint_id,omitempty"`
	PausedAt     time.Time      `json:"paused_at"`
}

type CreateSessionRequest struct {
	TunnelID   string `json:"tunnel_id"`
	Subdomain  string `json:"subdomain,omitempty"`
	Owner      string `json:"owner"`
	InviteRole string `json:"invite_role,omitempty"`
}

type JoinSessionRequest struct {
	Token string `json:"token"`
	Name  string `json:"name"`
	Role  string `json:"role,omitempty"`
}

type AnnotateRequest struct {
	RequestID string `json:"request_id"`
	SessionID string `json:"session_id,omitempty"`
	Author    string `json:"author"`
	Message   string `json:"message"`
}

type CreateBreakpointRequest struct {
	TunnelID    string `json:"tunnel_id"`
	Method      string `json:"method,omitempty"`
	PathPrefix  string `json:"path_prefix,omitempty"`
	HeaderName  string `json:"header_name,omitempty"`
	HeaderValue string `json:"header_value,omitempty"`
	CreatedBy   string `json:"created_by"`
}

type ReplayRequest struct {
	RequestID string              `json:"request_id"`
	Actor     string              `json:"actor"`
	Path      string              `json:"path,omitempty"`
	Body      []byte              `json:"body,omitempty"`
	Headers   map[string][]string `json:"headers,omitempty"`
}

type StreamEvent struct {
	ID        string      `json:"id"`
	Type      string      `json:"type"`
	Message   string      `json:"message"`
	TunnelID   string     `json:"tunnel_id,omitempty"`
	RequestID string      `json:"request_id,omitempty"`
	SessionID string      `json:"session_id,omitempty"`
	CreatedAt time.Time   `json:"created_at"`
	Payload   interface{} `json:"payload,omitempty"`
}

type TCPEvent struct {
	Type         string `json:"type"`
	ConnectionID string `json:"connection_id"`
	TunnelID      string `json:"tunnel_id"`
	Payload      []byte `json:"payload,omitempty"`
}

type TCPChunkRequest struct {
	ClientID     string `json:"client_id"`
	ConnectionID string `json:"connection_id"`
	Payload      []byte `json:"payload,omitempty"`
	Close        bool   `json:"close,omitempty"`
}
