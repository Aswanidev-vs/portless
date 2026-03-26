package protocol

import "time"

type SessionMode string

const (
	SessionModeHTTP2 SessionMode = "http2"
	SessionModeQUIC  SessionMode = "quic"
)

type StreamKind string

const (
	StreamKindHTTP StreamKind = "http"
	StreamKindTCP  StreamKind = "tcp"
	StreamKindUDP  StreamKind = "udp"
)

type TransportSession struct {
	ID         string      `json:"id"`
	ClientID   string      `json:"client_id"`
	TunnelID   string      `json:"tunnel_id"`
	Mode       SessionMode `json:"mode"`
	Kind       StreamKind  `json:"kind"`
	CreatedAt  time.Time   `json:"created_at"`
	LastSeenAt time.Time   `json:"last_seen_at"`
}

type ControlMessage struct {
	Type     string                 `json:"type"`
	ClientID string                 `json:"client_id,omitempty"`
	ReconnectToken string           `json:"reconnect_token,omitempty"`
	Session  *TransportSession      `json:"session,omitempty"`
	Request  *PendingRequest        `json:"request,omitempty"`
	Response *SubmitResponseRequest `json:"response,omitempty"`
	TCPEvent *TCPEvent              `json:"tcp_event,omitempty"`
	TCPChunk *TCPChunkRequest       `json:"tcp_chunk,omitempty"`
	UDPEvent *UDPEvent              `json:"udp_event,omitempty"`
	UDPChunk *UDPChunkRequest       `json:"udp_chunk,omitempty"`
}

type UDPEvent struct {
	Type         string `json:"type"`
	ConnectionID string `json:"connection_id"`
	TunnelID     string `json:"tunnel_id"`
	PeerAddr     string `json:"peer_addr,omitempty"`
	Payload      []byte `json:"payload,omitempty"`
}

type UDPChunkRequest struct {
	ClientID     string `json:"client_id"`
	ConnectionID string `json:"connection_id"`
	PeerAddr     string `json:"peer_addr,omitempty"`
	Payload      []byte `json:"payload,omitempty"`
	Close        bool   `json:"close,omitempty"`
}
