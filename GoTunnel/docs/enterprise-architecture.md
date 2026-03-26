# GoTunnel Enterprise Architecture

## Overview

GoTunnel is an enterprise-grade, self-hostable tunneling platform with comprehensive security, observability, and operational maturity features.

## Core Components

### 1. Certificate Lifecycle Management (`internal/acme/`)

The ACME module provides full Let's Encrypt certificate lifecycle management:

- **Automated Certificate Issuance**: Supports HTTP-01, DNS-01, and TLS-ALPN-01 challenges
- **Automatic Renewal**: Proactive renewal 30 days before expiration
- **Multi-Provider DNS**: Integration with Route53, Cloudflare, Google Cloud DNS, Azure DNS
- **Certificate Storage**: Encrypted persistent storage with metadata tracking
- **Revocation Support**: Manual and automated certificate revocation
- **Self-Signed Fallback**: Testing and development support

**Key Features**:
- Certificate state tracking (pending, validating, issued, renewing, expired, revoked, failed)
- Exponential backoff retry on failures
- Maximum retry limits with clear failure states
- Webhook notifications for certificate events

### 2. Multi-Provider DNS Automation (`internal/dns/`)

DNS provider abstraction for automated challenge management:

**Supported Providers**:
- **Route53**: AWS DNS management
- **Cloudflare**: Cloudflare DNS API
- **Google Cloud DNS**: GCP DNS management
- **Azure DNS**: Azure DNS management
- **Manual**: Manual DNS record creation
- **Memory**: In-memory provider for testing

**Features**:
- Automatic zone discovery
- Record creation, update, deletion
- Challenge record management
- Credential validation
- Provider health monitoring

### 3. Enterprise Authentication (`internal/auth/`)

Full enterprise authentication stack:

**RBAC (Role-Based Access Control)**:
- Predefined roles: Admin, Developer, Viewer, Debugger, Operator
- Granular permissions: tunnel:create, tunnel:delete, domain:manage, etc.
- Role-based permission inheritance
- Custom permission grants

**MFA (Multi-Factor Authentication)**:
- TOTP support (Google Authenticator, Authy)
- SMS verification
- Email verification
- Backup codes for recovery

**SSO (Single Sign-On)**:
- OAuth 2.0 / OpenID Connect support
- SAML 2.0 integration
- LDAP/Active Directory integration
- Just-in-time user provisioning

**Session Management**:
- Secure session tokens
- Configurable session timeouts
- Concurrent session limits
- Session revocation

### 4. Durable Replicated State (`internal/state/`)

Distributed state management with failover:

**Features**:
- Leader election (Raft-based consensus)
- State replication across nodes
- Snapshot and recovery
- Replication logs
- Automatic failover
- Split-brain prevention

**Consistency Guarantees**:
- Strong consistency for critical operations
- Eventual consistency for metrics
- Write-ahead logging
- Checksummed state entries

### 5. Production Multiplexing (`internal/multiplexer/`)

High-performance stream multiplexing:

**Features**:
- Multiple concurrent streams per connection
- Backpressure management with window-based flow control
- Stream recovery and retransmission
- Delivery semantics (at-most-once, at-least-once, exactly-once)
- Frame-level sequencing
- ACK/NACK mechanisms

**Performance**:
- Ring buffer for efficient I/O
- Zero-copy operations where possible
- Configurable window sizes
- Stream prioritization

### 6. Security Architecture (`internal/security/`)

Comprehensive security policy enforcement:

**Network Policies**:
- CIDR-based allow/block lists
- Port-based filtering
- Protocol restrictions
- Rate limiting per IP/user/tunnel

**TLS Policies**:
- Minimum TLS version enforcement (TLS 1.2+)
- Cipher suite restrictions
- Client certificate requirements
- SAN/CN validation

**Audit Logging**:
- All security events logged
- Policy violation tracking
- Authentication failures
- Rate limit exceedances

### 7. NFR Validation (`internal/nfr/`)

Non-functional requirement monitoring:

**Monitored Metrics**:
- Latency (p50, p95, p99)
- Uptime percentage
- Throughput (requests/second)
- Recovery time
- Concurrent tunnel count

**Validation**:
- Real-time threshold checking
- Historical trend analysis
- Alert generation on violations
- SLA compliance reporting

### 8. Operational Hardening (`internal/ops/`)

Production operational maturity:

**Health Checks**:
- Readiness probes (Kubernetes-compatible)
- Liveness probes
- Component health monitoring
- Dependency health tracking

**Resilience Patterns**:
- Circuit breaker for external calls
- Retry with exponential backoff
- Graceful degradation
- Bulkhead isolation

**Observability**:
- Runtime statistics (goroutines, memory, GC)
- Request tracing
- Performance metrics
- Resource utilization

## Configuration

### YAML Configuration

```yaml
version: 1

auth:
  token: env:GOTUNNEL_TOKEN
  
relay:
  broker_url: https://broker.example.com
  region: us-east-1
  
tunnels:
  - name: web-app
    protocol: http
    local_url: http://localhost:3000
    subdomain: myapp
    https: auto
    inspect: true
    
  - name: database
    protocol: tcp
    local_url: localhost:5432

# Security policies
security:
  network:
    allowed_cidrs:
      - 10.0.0.0/8
      - 172.16.0.0/12
    blocked_cidrs: []
    
  tls:
    min_version: "1.2"
    require_client_cert: false
    
  rate_limit:
    requests_per_second: 100
    burst_size: 200

# Certificate management
acme:
  email: admin@example.com
  directory_url: https://acme-v02.api.letsencrypt.org/directory
  renewal_window: 720h  # 30 days
  max_retries: 5
  
# DNS providers
dns:
  providers:
    - name: cloudflare
      type: cloudflare
      credentials:
        api_token: env:CLOUDFLARE_API_TOKEN
        
# State replication
state:
  replication:
    enabled: true
    nodes:
      - node1.example.com:8080
      - node2.example.com:8080
    heartbeat_interval: 1s
    election_timeout: 5s

# Observability
observability:
  metrics:
    enabled: true
    endpoint: /metrics
  tracing:
    enabled: true
    sampler: always_on
  logging:
    level: info
    format: json
```

## Deployment Architecture

### Single-Node Deployment

```
┌─────────────────────────────────────┐
│           GoTunnel Server           │
├─────────────────────────────────────┤
│  • Relay Server                     │
│  • Broker Server                    │
│  • Dashboard                        │
│  • ACME Manager                     │
│  • DNS Providers                    │
│  • State Store                      │
└─────────────────────────────────────┘
```

### High-Availability Deployment

```
                    ┌─────────────┐
                    │   Load      │
                    │  Balancer   │
                    └──────┬──────┘
                           │
            ┌──────────────┼──────────────┐
            │              │              │
    ┌───────▼──────┐ ┌─────▼──────┐ ┌─────▼──────┐
    │   Relay 1    │ │  Relay 2   │ │  Relay 3   │
    │  (Active)    │ │ (Standby)  │ │ (Standby)  │
    └───────┬──────┘ └─────┬──────┘ └─────┬──────┘
            │              │              │
            └──────────────┼──────────────┘
                           │
                    ┌──────▼──────┐
                    │   Shared    │
                    │   State     │
                    │  (Raft)     │
                    └─────────────┘
```

## Security Considerations

### Network Security
- All internal communication over TLS 1.3
- Network segmentation between components
- Firewall rules for east-west traffic
- DDoS protection at load balancer

### Authentication Security
- Password hashing with bcrypt/argon2
- Token-based authentication
- Session fixation prevention
- CSRF protection

### Data Security
- Encryption at rest for sensitive data
- Encrypted backups
- Audit logging for compliance
- PII redaction in logs

## Monitoring and Alerting

### Key Metrics
- `gotunnel_tunnel_establishment_duration_seconds`
- `gotunnel_tunnel_active_count`
- `gotunnel_certificate_issuance_total`
- `gotunnel_certificate_renewal_failures_total`
- `gotunnel_auth_failures_total`
- `gotunnel_request_latency_seconds`

### Alert Rules
- Certificate expiry within 7 days
- Authentication failure rate > 10%
- Tunnel establishment latency > 3s
- Recovery time > 30s
- Concurrent tunnels > 8000

## Disaster Recovery

### Backup Strategy
- State snapshots every 5 minutes
- Transaction logs for point-in-time recovery
- Offsite backup replication
- Automated backup verification

### Recovery Procedures
1. Restore from latest snapshot
2. Replay transaction logs
3. Verify state consistency
4. Rejoin cluster

### RTO/RPO Targets
- RTO: < 5 minutes
- RPO: < 1 minute