# GoTunnel

**Enterprise-grade, self-hostable tunneling platform** - A secure, scalable alternative to ngrok with deep RBAC, automated HTTPS, multi-provider DNS, and production-grade multiplexing.

[![Go Version](https://img.shields.io/badge/Go-1.21+-00ADD8?style=flat-square&logo=go)](https://golang.org)
[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg?style=flat-square)](LICENSE)
[![PRs Welcome](https://img.shields.io/badge/PRs-welcome-brightgreen.svg?style=flat-square)](CONTRIBUTING.md)

---

## Why GoTunnel?

GoTunnel is built for teams that need more than a simple tunnel. It provides enterprise-grade security, observability, and operational maturity while remaining self-hostable and open-source.

### Key Differentiators from ngrok

| Feature | GoTunnel | ngrok |
|---------|----------|-------|
| **Self-hosted** | ✅ Full control, no vendor lock-in | ❌ SaaS-only (limited self-hosted for enterprise) |
| **Open Source** | ✅ Apache 2.0 license | ❌ Proprietary |
| **Multi-provider DNS** | ✅ Cloudflare, Route53, GCP, Azure | ❌ Limited to ngrok managed DNS |
| **Certificate Lifecycle** | ✅ Full ACME automation with state tracking | ✅ Automated but opaque |
| **Enterprise Auth** | ✅ RBAC, MFA, SSO (OAuth, SAML, LDAP) | ✅ SSO (paid plans only) |
| **State Replication** | ✅ Raft-based consensus with failover | ❌ Managed by ngrok infrastructure |
| **Multiplexing** | ✅ Custom protocol with delivery semantics | ✅ HTTP/2 multiplexing |
| **Security Policies** | ✅ Network, TLS, rate limiting, audit logs | ⚠️ Basic IP restrictions |
| **NFR Validation** | ✅ Latency, uptime, throughput monitoring | ⚠️ Limited to dashboard metrics |
| **Operational Maturity** | ✅ Health probes, circuit breakers, graceful shutdown | ✅ Production-grade infrastructure |
| **Data Sovereignty** | ✅ Full control over data location | ❌ Data stored in ngrok infrastructure |
| **Cost** | ✅ Free, self-hosted | 💰 Paid plans for production features |

---

## Quick Start

### Prerequisites

- Go 1.21 or higher
- PostgreSQL (for state persistence)
- Redis (optional, for caching)
- DNS provider account (Cloudflare, Route53, etc.)

### Installation

```bash
# Clone the repository
git clone https://github.com/portless/gotunnel.git
cd GoTunnel

# Build
go build -o gotunnel ./cmd/gotunnel

# Or install directly
go install ./cmd/gotunnel
```

### Basic Usage

```bash
# Start the relay server
gotunnel relay --config gotunnel.example.yaml

# In another terminal, start a tunnel
gotunnel tunnel start --name web --protocol http --local-port 8080

# View tunnel status
gotunnel tunnel list
```

### Docker Deployment

```bash
# Using Docker Compose
docker-compose up -d

# Or build directly
docker build -t gotunnel .
docker run -p 8080:8080 -p 8443:8443 gotunnel
```

---

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                    GoTunnel Platform                        │
├─────────────────────────────────────────────────────────────┤
│                                                             │
│  ┌─────────────┐    ┌─────────────┐    ┌─────────────┐     │
│  │   Relay      │    │   Broker    │    │  Dashboard  │     │
│  │   Server     │    │   Server    │    │   Server    │     │
│  └──────┬──────┘    └──────┬──────┘    └──────┬──────┘     │
│         │                  │                  │             │
│         │                  │                  │             │
│  ┌──────┴──────────────────┴──────────────────┴──────┐     │
│  │              Core Services                         │     │
│  │  ┌──────────┐ ┌──────────┐ ┌──────────┐          │     │
│  │  │   ACME   │ │   DNS    │ │   Auth   │          │     │
│  │  │ Manager  │ │ Provider │ │  System  │          │     │
│  │  └──────────┘ └──────────┘ └──────────┘          │     │
│  │  ┌──────────┐ ┌──────────┐ ┌──────────┐          │     │
│  │  │  State   │ │ Security │ │   NFR    │          │     │
│  │  │ Replicat.│ │ Enforcer │ │Validator │          │     │
│  │  └──────────┘ └──────────┘ └──────────┘          │     │
│  │  ┌──────────┐ ┌──────────┐ ┌──────────┐          │     │
│  │  │Multiplexer│ │  Ops    │ │ Metrics  │          │     │
│  │  │          │ │Hardener │ │          │          │     │
│  │  └──────────┘ └──────────┘ └──────────┘          │     │
│  └──────────────────────────────────────────────────┘     │
│                                                             │
└─────────────────────────────────────────────────────────────┘
```

---

## Enterprise Features

### 1. Let's Encrypt Certificate Lifecycle Management

Full operator-grade certificate automation:

- **Automated Issuance**: HTTP-01, DNS-01, TLS-ALPN-01 challenges
- **Proactive Renewal**: 30 days before expiration
- **State Tracking**: pending → validating → issued → renewing → expired/revoked/failed
- **Retry Logic**: Exponential backoff with configurable max retries
- **Encrypted Storage**: Certificates stored with AES-256 encryption
- **Revocation Support**: Manual and automated certificate revocation
- **Self-signed Fallback**: For development and testing

```yaml
acme:
  email: admin@example.com
  directory_url: https://acme-v02.api.letsencrypt.org/directory
  renewal_window: 720h  # 30 days
  max_retries: 5
  preferred_challenge: dns-01
```

### 2. Multi-Provider DNS Automation

Real multi-provider support for DNS-01 challenges:

- **Cloudflare**: Full API integration
- **AWS Route53**: IAM-based authentication
- **Google Cloud DNS**: Service account support
- **Azure DNS**: Service principal authentication
- **Manual**: For unsupported providers
- **Memory**: For testing

```yaml
dns:
  providers:
    - name: cloudflare
      type: cloudflare
      credentials:
        api_token: ${CLOUDFLARE_API_TOKEN}
      zones:
        - example.com
```

### 3. Enterprise Authentication Stack

#### RBAC (Role-Based Access Control)

5 predefined roles with 15 granular permissions:

| Role | Permissions |
|------|-------------|
| **Admin** | All permissions |
| **Developer** | Tunnel CRUD, inspection, sharing, replay |
| **Viewer** | Read-only access, dashboard |
| **Debugger** | Create tunnels, inspect, replay |
| **Operator** | Tunnel CRUD, domain/certificate management |

#### MFA (Multi-Factor Authentication)

- **TOTP**: Google Authenticator, Authy compatible
- **Backup Codes**: 10 single-use recovery codes
- **QR Code Generation**: For easy setup

#### SSO (Single Sign-On)

- **OAuth 2.0 / OpenID Connect**: Generic provider support
- **SAML 2.0**: Enterprise federation
- **LDAP/Active Directory**: On-premises integration
- **Just-in-time Provisioning**: Auto-create users on first SSO login

```go
// Example: Creating a user with roles
auth.CreateUser(ctx, auth.CreateUserRequest{
    Username: "developer1",
    Email: "dev@example.com",
    Roles: []auth.Role{auth.RoleDeveloper},
    WorkspaceID: "team-alpha",
})
```

### 4. Durable Replicated State

Production-grade state management with true failover:

- **Leader Election**: Raft-based consensus algorithm
- **State Replication**: Across multiple nodes
- **Snapshots**: Periodic state snapshots for fast recovery
- **Write-ahead Logging**: Durable operation logs
- **Automatic Failover**: < 30 second recovery time
- **Split-brain Prevention**: Quorum-based decisions

```yaml
state:
  replication:
    enabled: true
    nodes:
      - node1.example.com:8080
      - node2.example.com:8080
      - node3.example.com:8080
    heartbeat_interval: 1s
    election_timeout: 5s
```

### 5. Production-Grade Multiplexing

High-performance stream multiplexing with strong guarantees:

- **Multiple Streams**: Up to 1000 concurrent streams per connection
- **Backpressure Control**: Window-based flow control
- **Stream Recovery**: Automatic retransmission on packet loss
- **Delivery Semantics**: At-most-once, at-least-once, exactly-once
- **Frame-level Sequencing**: ACK/NACK mechanisms
- **Ring Buffers**: Efficient memory management

```yaml
relay:
  multiplexing:
    max_streams: 1000
    window_size: 65535
    delivery_mode: at_least_once
```

### 6. Security Architecture & Policy Enforcement

Comprehensive security policies:

#### Network Policies
- CIDR-based allow/block lists
- Port restrictions
- Protocol filtering
- Connection limits

#### TLS Policies
- TLS 1.3 enforcement (configurable minimum)
- Cipher suite restrictions
- Client certificate validation
- SAN/CN validation

#### Rate Limiting
- Per-IP, per-user, per-tunnel limits
- Token bucket algorithm
- Configurable burst sizes

#### Audit Logging
- All security events logged
- Policy violation tracking
- Authentication failures
- Exportable to SIEM systems

```yaml
security:
  network:
    allowed_cidrs:
      - 10.0.0.0/8
      - 172.16.0.0/12
  tls:
    min_version: "1.3"
    require_client_cert: false
  rate_limit:
    requests_per_second: 100
    burst_size: 200
```

### 7. NFR Validation & Metrics

Measurable non-functional requirements:

| Metric | Target | Measurement |
|--------|--------|-------------|
| Tunnel establishment | < 3s (p95) | End-to-end connection time |
| End-to-end latency | < 100ms (p99) | Same-region HTTP traffic |
| Concurrent tunnels | 10,000+ | Per cluster |
| Monthly uptime | 99.9% | Control plane + data plane |
| Recovery time | < 30s | Single node failure |
| Certificate issuance | 98%+ | Success rate |

### 8. Operational Maturity

Production-ready operational features:

- **Health Probes**: Kubernetes-compatible readiness/liveness checks
- **Circuit Breakers**: Automatic failure detection and recovery
- **Retry with Backoff**: Exponential backoff for transient failures
- **Graceful Shutdown**: Connection draining with configurable timeout
- **Runtime Metrics**: Goroutines, memory, GC statistics
- **Resource Limits**: Configurable memory, connection, and file descriptor limits

---

## Configuration

See [`gotunnel.example.yaml`](gotunnel.example.yaml) for a complete configuration example.

### Key Configuration Sections

```yaml
# Authentication
auth:
  token: ${GOTUNNEL_TOKEN}
  session_timeout: 24h
  require_mfa: false

# Relay
relay:
  region: us-east-1
  transport:
    protocol: auto  # quic, http2, auto
  multiplexing:
    max_streams: 1000

# Tunnels
tunnels:
  - name: web
    protocol: http
    local_url: http://localhost:3000
    subdomain: myapp
    https: auto
    inspect: true

# Security
security:
  network:
    allowed_cidrs: ["10.0.0.0/8"]
  tls:
    min_version: "1.3"

# ACME
acme:
  email: admin@example.com
  directory_url: https://acme-v02.api.letsencrypt.org/directory

# DNS
dns:
  providers:
    - name: cloudflare
      type: cloudflare
      credentials:
        api_token: ${CLOUDFLARE_API_TOKEN}

# State
state:
  replication:
    enabled: true
    nodes:
      - node1:8080
      - node2:8080

# Observability
observability:
  metrics:
    enabled: true
    endpoint: /metrics
  logging:
    level: info
    format: json
```

---

## CLI Commands

```bash
# Authentication
gotunnel login                          # Interactive login
gotunnel login --token <token>          # Token-based login

# Tunnel Management
gotunnel tunnel create --name web --protocol http --local-port 8080
gotunnel tunnel start --name web
gotunnel tunnel stop --name web
gotunnel tunnel list
gotunnel tunnel inspect --name web
gotunnel tunnel share --name web        # Create collaborative session

# Configuration
gotunnel config validate               # Validate configuration
gotunnel config init                   # Initialize configuration

# Relay Status
gotunnel relay status                  # Show relay cluster status
gotunnel relay health                  # Health check

# Certificate Management
gotunnel cert list                     # List managed certificates
gotunnel cert renew --domain example.com
gotunnel cert revoke --domain example.com

# DNS Management
gotunnel dns providers                 # List DNS providers
gotunnel dns records --zone example.com

# Users & Auth (Admin)
gotunnel user list
gotunnel user create --username dev1 --email dev@example.com --role developer
gotunnel user enable-mfa --username dev1
gotunnel session list --username dev1

# Backup & Recovery
gotunnel backup create                 # Create state backup
gotunnel backup restore --file backup.tar.gz
```

---

## Deployment

### Docker Compose

```yaml
version: '3.8'
services:
  gotunnel:
    image: gotunnel:latest
    ports:
      - "8080:8080"   # Relay
      - "8443:8443"   # HTTPS
      - "9090:9090"   # Metrics
    environment:
      - GOTUNNEL_TOKEN=${GOTUNNEL_TOKEN}
      - ACME_EMAIL=${ACME_EMAIL}
      - CLOUDFLARE_API_TOKEN=${CLOUDFLARE_API_TOKEN}
    volumes:
      - ./config:/etc/gotunnel
      - ./certs:/var/lib/gotunnel/certs
      - ./data:/var/lib/gotunnel/data
    depends_on:
      - postgres
      - redis

  postgres:
    image: postgres:15
    environment:
      - POSTGRES_DB=gotunnel
      - POSTGRES_USER=gotunnel
      - POSTGRES_PASSWORD=${DB_PASSWORD}
    volumes:
      - postgres_data:/var/lib/postgresql/data

  redis:
    image: redis:7-alpine
    volumes:
      - redis_data:/data

volumes:
  postgres_data:
  redis_data:
```

### Kubernetes

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: gotunnel
spec:
  replicas: 3
  selector:
    matchLabels:
      app: gotunnel
  template:
    metadata:
      labels:
        app: gotunnel
    spec:
      containers:
      - name: gotunnel
        image: gotunnel:latest
        ports:
        - containerPort: 8080
          name: relay
        - containerPort: 8443
          name: https
        env:
        - name: GOTUNNEL_TOKEN
          valueFrom:
            secretKeyRef:
              name: gotunnel-secrets
              key: token
        livenessProbe:
          httpGet:
            path: /health
            port: 8080
          initialDelaySeconds: 30
          periodSeconds: 10
        readinessProbe:
          httpGet:
            path: /ready
            port: 8080
          initialDelaySeconds: 5
          periodSeconds: 5
        resources:
          limits:
            memory: "512Mi"
            cpu: "1000m"
          requests:
            memory: "256Mi"
            cpu: "500m"
```

### Terraform

```hcl
module "gotunnel" {
  source = "./modules/gotunnel"
  
  environment = "production"
  region      = "us-east-1"
  
  # Database
  db_instance_class = "db.t3.medium"
  db_name           = "gotunnel"
  db_username       = "gotunnel"
  db_password       = var.db_password
  
  # DNS
  dns_provider      = "cloudflare"
  dns_zone          = "example.com"
  cloudflare_token  = var.cloudflare_token
  
  # Scaling
  min_capacity      = 2
  max_capacity      = 10
  desired_capacity  = 3
}
```

---

## Monitoring

### Prometheus Metrics

```yaml
# prometheus.yml
scrape_configs:
  - job_name: 'gotunnel'
    static_configs:
      - targets: ['gotunnel:9090']
    metrics_path: /metrics
```

Key metrics exposed:

- `gotunnel_tunnels_active` - Number of active tunnels
- `gotunnel_requests_total` - Total requests processed
- `gotunnel_request_duration_seconds` - Request latency histogram
- `gotunnel_connections_active` - Active connections
- `gotunnel_bytes_transferred_total` - Data transfer volume
- `gotunnel_certificate_expiry_seconds` - Certificate expiration time
- `gotunnel_auth_failures_total` - Authentication failures
- `gotunnel_rate_limit_hits_total` - Rate limit violations

### Health Checks

```bash
# Liveness probe
curl http://localhost:8080/health

# Readiness probe
curl http://localhost:8080/ready

# Detailed health
curl http://localhost:8080/health/detailed
```

---

## Security

### Best Practices

1. **Token Security**: Use strong, randomly generated tokens
2. **MFA**: Enable MFA for all admin accounts
3. **Network Segmentation**: Use CIDR allow lists
4. **TLS 1.3**: Enforce TLS 1.3 for all connections
5. **Audit Logging**: Enable comprehensive audit logging
6. **Regular Updates**: Keep GoTunnel updated
7. **Backup Strategy**: Regular state backups
8. **Access Control**: Use RBAC with least privilege

### Reporting Security Issues

Please report security vulnerabilities to security@portless.dev

---

## Contributing

We welcome contributions! Please see [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines.

### Development Setup

```bash
# Clone
git clone https://github.com/portless/gotunnel.git
cd GoTunnel

# Install dependencies
go mod download

# Run tests
go test ./...

# Run linter
golangci-lint run

# Build
go build ./cmd/gotunnel
```

---

## License

Apache License 2.0 - see [LICENSE](LICENSE) for details.

---

## Support

- **Documentation**: [docs/](docs/)
- **Examples**: [examples/](examples/)
- **Issues**: [GitHub Issues](https://github.com/portless/gotunnel/issues)
- **Discussions**: [GitHub Discussions](https://github.com/portless/gotunnel/discussions)

---

## Roadmap

See [timeline.md](timeline.md) for detailed implementation roadmap and future plans.

---

## Acknowledgments

- [quic-go](https://github.com/quic-go/quic-go) - QUIC protocol implementation
- [golang.org/x/crypto/acme](https://pkg.go.dev/golang.org/x/crypto/acme) - ACME client
- [cobra](https://github.com/spf13/cobra) - CLI framework
- [viper](https://github.com/spf13/viper) - Configuration management