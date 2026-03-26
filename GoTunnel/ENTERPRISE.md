# GoTunnel Enterprise Features

## Overview

GoTunnel has been enhanced with comprehensive enterprise-grade features including:

1. **Full Let's Encrypt Certificate Lifecycle Management**
2. **Multi-Provider DNS Automation**
3. **Enterprise Authentication Stack (RBAC, MFA, SSO)**
4. **Durable Replicated State with Failover**
5. **Production-Grade Multiplexing**
6. **Security Architecture and Policy Enforcement**
7. **NFR Validation and Metrics**
8. **Production Hardening and Operational Maturity**

## Quick Start

### 1. Basic Setup

```bash
# Copy the enterprise configuration
cp gotunnel.enterprise.yaml gotunnel.yaml

# Set required environment variables
export GOTUNNEL_TOKEN="your-secure-token"
export BROKER_URL="https://your-broker.example.com"
export ACME_EMAIL="admin@example.com"
export CLOUDFLARE_API_TOKEN="your-cloudflare-token"

# Start GoTunnel
gotunnel --config gotunnel.yaml
```

### 2. Certificate Management

GoTunnel automatically manages TLS certificates:

```yaml
acme:
  email: admin@example.com
  directory_url: https://acme-v02.api.letsencrypt.org/directory
  renewal_window: 720h  # Renew 30 days before expiry
  preferred_challenge: dns-01
```

**Features**:
- Automatic certificate issuance
- Automatic renewal before expiry
- Multi-provider DNS challenge support
- Encrypted certificate storage
- Revocation support

### 3. DNS Automation

Configure DNS providers for automatic challenge validation:

```yaml
dns:
  providers:
    - name: cloudflare
      type: cloudflare
      credentials:
        api_token: env:CLOUDFLARE_API_TOKEN
```

**Supported Providers**:
- Cloudflare
- AWS Route53
- Google Cloud DNS
- Azure DNS
- Manual (for any provider)

### 4. Enterprise Authentication

#### RBAC (Role-Based Access Control)

Predefined roles:
- `admin`: Full system access
- `developer`: Create/manage tunnels
- `viewer`: Read-only access
- `debugger`: Inspect and replay traffic
- `operator`: Infrastructure management

```go
// Example: Creating a user with specific role
authenticator.CreateUser(ctx, auth.CreateUserRequest{
    Username: "developer1",
    Email: "dev@example.com",
    Roles: []auth.Role{auth.RoleDeveloper},
    WorkspaceID: "team-alpha",
})
```

#### MFA (Multi-Factor Authentication)

```go
// Enable MFA for a user
setup, err := authenticator.EnableMFA(ctx, userID, "totp")
// Returns: QR code URL, backup codes

// Verify MFA code
err := authenticator.VerifyMFA(ctx, sessionToken, "123456")
```

#### SSO (Single Sign-On)

```go
// Register SSO provider
authenticator.RegisterSSOProvider(&OIDCProvider{
    Name: "okta",
    ClientID: "...",
    ClientSecret: "...",
    IssuerURL: "https://your-org.okta.com",
})

// Authenticate via SSO
session, err := authenticator.AuthenticateWithSSO(ctx, "okta", authCode)
```

### 5. State Replication

Configure multi-node deployment with automatic failover:

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

**Features**:
- Leader election (Raft consensus)
- Automatic failover on leader failure
- State snapshots and recovery
- Write-ahead logging
- Strong consistency guarantees

### 6. Security Policies

#### Network Policies

```yaml
security:
  network:
    allowed_cidrs:
      - 10.0.0.0/8
    blocked_cidrs:
      - 192.168.1.0/24
    max_connections: 10000
```

#### TLS Policies

```yaml
security:
  tls:
    min_version: "1.2"
    cipher_suites:
      - TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384
    require_client_cert: false
```

#### Rate Limiting

```yaml
security:
  rate_limit:
    requests_per_second: 100
    burst_size: 200
    key_field: ip  # or "user", "tunnel"
```

### 7. NFR Validation

Monitor and enforce non-functional requirements:

```yaml
nfr:
  requirements:
    - id: latency-p99
      type: latency
      threshold: 100  # ms
    - id: uptime-monthly
      type: uptime
      threshold: 99.9  # percent
```

**Metrics Tracked**:
- Latency (p50, p95, p99)
- Uptime percentage
- Throughput (requests/second)
- Recovery time
- Concurrent tunnel count

### 8. Operational Features

#### Health Checks

```bash
# Check system health
curl https://your-server:8081/health

# Readiness probe (for Kubernetes)
curl https://your-server:8081/ready

# Liveness probe
curl https://your-server:8081/alive
```

#### Metrics

Prometheus-compatible metrics available at `/metrics`:

```prometheus
gotunnel_tunnel_establishment_duration_seconds
gotunnel_tunnel_active_count
gotunnel_certificate_issuance_total
gotunnel_certificate_renewal_failures_total
gotunnel_auth_failures_total
```

#### Graceful Shutdown

```bash
# Send SIGTERM for graceful shutdown
kill -TERM <pid>

# GoTunnel will:
# 1. Stop accepting new connections
# 2. Drain existing connections
# 3. Save state
# 4. Exit cleanly
```

## Architecture

### Component Overview

```
┌─────────────────────────────────────────────────────────┐
│                    GoTunnel Server                       │
├─────────────────────────────────────────────────────────┤
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐  │
│  │    Relay      │  │    Broker    │  │   Dashboard  │  │
│  │   Server      │  │    Server    │  │              │  │
│  └──────────────┘  └──────────────┘  └──────────────┘  │
│                                                         │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐  │
│  │    ACME       │  │     DNS      │  │    Auth      │  │
│  │   Manager     │  │  Providers   │  │   System     │  │
│  └──────────────┘  └──────────────┘  └──────────────┘  │
│                                                         │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐  │
│  │    State      │  │  Multiplexer │  │  Security    │  │
│  │  Replication  │  │              │  │   Enforcer   │  │
│  └──────────────┘  └──────────────┘  └──────────────┘  │
│                                                         │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐  │
│  │     NFR      │  │  Hardener    │  │   Metrics    │  │
│  │  Validator   │  │              │  │   Collector  │  │
│  └──────────────┘  └──────────────┘  └──────────────┘  │
└─────────────────────────────────────────────────────────┘
```

### Data Flow

1. **Client Connection**: Client connects via HTTP/2 or QUIC
2. **Authentication**: Token validated, MFA checked if required
3. **Authorization**: RBAC permissions verified
4. **Rate Limiting**: Request rate checked
5. **TLS Handshake**: Certificate served (ACME-managed)
6. **Tunnel Establishment**: Multiplexed streams created
7. **Traffic Forwarding**: Data proxied with backpressure control
8. **State Replication**: Changes replicated across nodes
9. **Audit Logging**: All actions logged

## Deployment

### Docker Deployment

```dockerfile
FROM golang:1.21-alpine AS builder
WORKDIR /app
COPY . .
RUN go build -o gotunnel ./cmd/gotunnel

FROM alpine:latest
RUN apk --no-cache add ca-certificates
WORKDIR /root/
COPY --from=builder /app/gotunnel .
COPY gotunnel.enterprise.yaml .
EXPOSE 8080 8081
CMD ["./gotunnel", "--config", "gotunnel.enterprise.yaml"]
```

### Kubernetes Deployment

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
        - containerPort: 8081
          name: dashboard
        env:
        - name: GOTUNNEL_TOKEN
          valueFrom:
            secretKeyRef:
              name: gotunnel-secrets
              key: token
        - name: BROKER_URL
          value: "https://gotunnel-broker.default.svc.cluster.local"
        livenessProbe:
          httpGet:
            path: /alive
            port: 8081
          initialDelaySeconds: 30
          periodSeconds: 10
        readinessProbe:
          httpGet:
            path: /ready
            port: 8081
          initialDelaySeconds: 5
          periodSeconds: 5
```

### Terraform Deployment

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
  
  # ACME
  acme_email        = "admin@example.com"
  
  # Scaling
  min_capacity      = 2
  max_capacity      = 10
  desired_capacity  = 3
}
```

## Monitoring

### Prometheus Metrics

```yaml
# prometheus.yml
scrape_configs:
  - job_name: 'gotunnel'
    static_configs:
      - targets: ['gotunnel:8081']
    metrics_path: /metrics
```

### Grafana Dashboard

Import the provided Grafana dashboard for visualization:

- Tunnel establishment latency
- Active tunnel count
- Certificate status
- Authentication failures
- Resource utilization

### Alerting Rules

```yaml
groups:
  - name: gotunnel
    rules:
      - alert: CertificateExpiringSoon
        expr: gotunnel_certificate_not_after - time() < 7 * 24 * 3600
        for: 1h
        labels:
          severity: warning
        
      - alert: HighAuthFailureRate
        expr: rate(gotunnel_auth_failures_total[5m]) > 0.1
        for: 5m
        labels:
          severity: critical
```

## Troubleshooting

### Common Issues

#### Certificate Renewal Failures

```bash
# Check certificate status
curl https://localhost:8081/api/certificates

# Force renewal
curl -X POST https://localhost:8081/api/certificates/renew

# Check ACME logs
tail -f logs/gotunnel.log | grep acme
```

#### DNS Challenge Failures

```bash
# Verify DNS provider credentials
curl https://localhost:8081/api/dns/providers

# Test DNS propagation
dig TXT _acme-challenge.example.com

# Check DNS provider logs
tail -f logs/gotunnel.log | grep dns
```

#### State Replication Issues

```bash
# Check cluster status
curl https://localhost:8081/api/cluster/status

# Force leader election
curl -X POST https://localhost:8081/api/cluster/elect

# View replication logs
tail -f logs/gotunnel.log | grep replication
```

## Security Best Practices

1. **Use Strong Tokens**: Generate cryptographically secure tokens
2. **Enable MFA**: Require MFA for admin accounts
3. **Restrict Network Access**: Use CIDR allow lists
4. **Rotate Secrets**: Regularly rotate API keys and tokens
5. **Monitor Audit Logs**: Review logs for suspicious activity
6. **Keep Updated**: Regularly update GoTunnel to latest version
7. **Backup Regularly**: Configure automated backups
8. **Test Recovery**: Regularly test disaster recovery procedures

## Support

For enterprise support, contact:
- Email: support@gotunnel.example.com
- Documentation: https://docs.gotunnel.example.com
- Status Page: https://status.gotunnel.example.com