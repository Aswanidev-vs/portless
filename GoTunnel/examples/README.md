# GoTunnel Examples

This directory contains practical examples and usage guides for GoTunnel.

## Table of Contents

1. [Basic HTTP Tunnel](#1-basic-http-tunnel)
2. [HTTPS with Auto Certificates](#2-https-with-auto-certificates)
3. [TCP Tunnel for Databases](#3-tcp-tunnel-for-databases)
4. [Multi-Provider DNS Setup](#4-multi-provider-dns-setup)
5. [Enterprise Authentication](#5-enterprise-authentication)
6. [Multi-Node Cluster](#6-multi-node-cluster)
7. [Security Policies](#7-security-policies)
8. [Collaborative Debugging](#8-collaborative-debugging)

---

## 1. Basic HTTP Tunnel

### Simple Local Development

Expose your local web server to the internet.

**Configuration File** (`gotunnel.yaml`):

```yaml
version: 1

auth:
  token: ${GOTUNNEL_TOKEN}

relay:
  broker_url: https://relay.example.com
  region: us-east-1

tunnels:
  - name: web
    protocol: http
    local_url: http://localhost:3000
    subdomain: myapp
    inspect: true
```

**CLI Usage**:

```bash
# Start the tunnel
gotunnel tunnel start --name web

# Check status
gotunnel tunnel list

# View live requests
gotunnel tunnel inspect --name web

# Stop the tunnel
gotunnel tunnel stop --name web
```

**Expected Output**:

```
Tunnel "web" started successfully!
  Public URL: https://myapp.example.com
  Local URL:  http://localhost:3000
  Status:     active
  Inspect:    enabled
```

---

## 2. HTTPS with Auto Certificates

### Automated Let's Encrypt Certificates

Configure automatic HTTPS certificate issuance.

**Configuration File** (`gotunnel-https.yaml`):

```yaml
version: 1

auth:
  token: ${GOTUNNEL_TOKEN}

relay:
  broker_url: https://relay.example.com
  region: us-east-1

# ACME Configuration
acme:
  email: admin@example.com
  directory_url: https://acme-v02.api.letsencrypt.org/directory
  renewal_window: 720h  # Renew 30 days before expiry
  max_retries: 5
  preferred_challenge: dns-01

# DNS Provider for DNS-01 Challenges
dns:
  providers:
    - name: cloudflare
      type: cloudflare
      credentials:
        api_token: ${CLOUDFLARE_API_TOKEN}
      zones:
        - example.com

tunnels:
  - name: web
    protocol: http
    local_url: http://localhost:8080
    subdomain: api
    https: auto  # Enable automatic HTTPS
    inspect: true
```

**CLI Usage**:

```bash
# Start tunnel with automatic HTTPS
gotunnel tunnel start --name web

# Check certificate status
gotunnel cert list

# Force certificate renewal
gotunnel cert renew --domain api.example.com

# Check certificate expiry
gotunnel cert info --domain api.example.com
```

**Certificate Status Output**:

```json
{
  "domain": "api.example.com",
  "state": "issued",
  "not_before": "2026-01-15T00:00:00Z",
  "not_after": "2026-04-15T00:00:00Z",
  "next_renewal": "2026-03-16T00:00:00Z",
  "serial_number": "03:a1:b2:c3:d4:e5:f6...",
  "issuer": "R3",
  "fingerprint": "sha256:a1b2c3d4e5f6..."
}
```

---

## 3. TCP Tunnel for Databases

### Secure Database Access

Expose local database services securely.

**Configuration File** (`gotunnel-db.yaml`):

```yaml
version: 1

auth:
  token: ${GOTUNNEL_TOKEN}

relay:
  broker_url: https://relay.example.com
  region: us-east-1

security:
  network:
    allowed_cidrs:
      - 10.0.0.0/8      # Corporate network
      - 192.168.1.0/24   # VPN
    blocked_cidrs:
      - 0.0.0.0/0        # Block all by default
  tls:
    min_version: "1.3"
    require_client_cert: true

tunnels:
  - name: postgres
    protocol: tcp
    local_url: localhost:5432
    inspect: false  # Don't log database queries

  - name: redis
    protocol: tcp
    local_url: localhost:6379
    inspect: false

  - name: mongodb
    protocol: tcp
    local_url: localhost:27017
    inspect: false
```

**CLI Usage**:

```bash
# Start all database tunnels
gotunnel tunnel start --name postgres
gotunnel tunnel start --name redis
gotunnel tunnel start --name mongodb

# Get connection strings
gotunnel tunnel list --format json | jq '.[] | {name, public_url}'

# Connect to PostgreSQL
psql -h postgres.example.com -p 5432 -U myuser -d mydb

# Connect to Redis
redis-cli -h redis.example.com -p 6379

# Connect to MongoDB
mongosh "mongodb://mongodb.example.com:27017/mydb"
```

**Connection String Output**:

```json
{
  "postgres": {
    "public_url": "tcp://postgres.example.com:5432",
    "local_url": "localhost:5432",
    "status": "active"
  },
  "redis": {
    "public_url": "tcp://redis.example.com:6379",
    "local_url": "localhost:6379",
    "status": "active"
  }
}
```

---

## 4. Multi-Provider DNS Setup

### Cloudflare + Route53 Fallback

Configure multiple DNS providers for redundancy.

**Configuration File** (`gotunnel-dns.yaml`):

```yaml
version: 1

auth:
  token: ${GOTUNNEL_TOKEN}

relay:
  broker_url: https://relay.example.com
  region: us-east-1

acme:
  email: admin@example.com
  preferred_challenge: dns-01

dns:
  providers:
    # Primary: Cloudflare
    - name: cloudflare
      type: cloudflare
      credentials:
        api_token: ${CLOUDFLARE_API_TOKEN}
      zones:
        - example.com
        - example.org
      priority: 1

    # Secondary: Route53
    - name: route53
      type: route53
      credentials:
        access_key_id: ${AWS_ACCESS_KEY_ID}
        secret_access_key: ${AWS_SECRET_ACCESS_KEY}
        region: us-east-1
      zones:
        - example.com
      priority: 2

    # Tertiary: Google Cloud DNS
    - name: gcp-dns
      type: gcp
      credentials:
        project_id: ${GCP_PROJECT_ID}
        service_account_key: ${GCP_SERVICE_ACCOUNT_KEY}
      zones:
        - example-net
      priority: 3

tunnels:
  - name: web
    protocol: http
    local_url: http://localhost:3000
    subdomain: app
    https: auto
```

**CLI Usage**:

```bash
# List DNS providers
gotunnel dns providers

# Check DNS records
gotunnel dns records --zone example.com

# Test DNS propagation
gotunnel dns test --domain app.example.com

# Force DNS update
gotunnel dns update --domain app.example.com
```

**Provider Status Output**:

```json
{
  "providers": [
    {
      "name": "cloudflare",
      "type": "cloudflare",
      "status": "active",
      "zones": ["example.com", "example.org"],
      "priority": 1
    },
    {
      "name": "route53",
      "type": "route53",
      "status": "active",
      "zones": ["example.com"],
      "priority": 2
    },
    {
      "name": "gcp-dns",
      "type": "gcp",
      "status": "active",
      "zones": ["example-net"],
      "priority": 3
    }
  ]
}
```

---

## 5. Enterprise Authentication

### RBAC + MFA + SSO

Configure enterprise-grade authentication.

**Configuration File** (`gotunnel-auth.yaml`):

```yaml
version: 1

auth:
  token: ${GOTUNNEL_TOKEN}
  session_timeout: 24h
  max_concurrent_sessions: 5
  require_mfa: true  # Enforce MFA for all users
  password_policy:
    min_length: 12
    require_uppercase: true
    require_lowercase: true
    require_numbers: true
    require_special: true

relay:
  broker_url: https://relay.example.com
  region: us-east-1

tunnels:
  - name: web
    protocol: http
    local_url: http://localhost:3000
    subdomain: app
    https: auto
```

**CLI Usage**:

```bash
# Create users with different roles
gotunnel user create \
  --username admin1 \
  --email admin@example.com \
  --role admin \
  --workspace engineering

gotunnel user create \
  --username dev1 \
  --email dev@example.com \
  --role developer \
  --workspace engineering

gotunnel user create \
  --username viewer1 \
  --email viewer@example.com \
  --role viewer \
  --workspace engineering

# Enable MFA for a user
gotunnel user enable-mfa --username dev1

# List users
gotunnel user list --workspace engineering

# Check user permissions
gotunnel user permissions --username dev1

# Create SSO configuration
gotunnel sso configure \
  --provider okta \
  --client-id ${OKTA_CLIENT_ID} \
  --client-secret ${OKTA_CLIENT_SECRET} \
  --issuer https://your-org.okta.com

# Test SSO login
gotunnel sso test --provider okta
```

**User List Output**:

```json
{
  "users": [
    {
      "username": "admin1",
      "email": "admin@example.com",
      "roles": ["admin"],
      "permissions": ["tunnel:create", "tunnel:delete", "tunnel:inspect", "..."],
      "mfa_enabled": true,
      "sso_provider": null,
      "last_login": "2026-03-26T10:30:00Z"
    },
    {
      "username": "dev1",
      "email": "dev@example.com",
      "roles": ["developer"],
      "permissions": ["tunnel:create", "tunnel:delete", "tunnel:inspect", "tunnel:share"],
      "mfa_enabled": true,
      "sso_provider": "okta",
      "last_login": "2026-03-26T09:15:00Z"
    }
  ]
}
```

---

## 6. Multi-Node Cluster

### High Availability Setup

Deploy a multi-node cluster with automatic failover.

**Configuration File** (`gotunnel-cluster.yaml`):

```yaml
version: 1

auth:
  token: ${GOTUNNEL_TOKEN}

relay:
  broker_url: https://relay.example.com
  region: us-east-1

state:
  replication:
    enabled: true
    node_id: ${NODE_ID}
    nodes:
      - node1.example.com:8080
      - node2.example.com:8080
      - node3.example.com:8080
    heartbeat_interval: 1s
    election_timeout: 5s
    sync_timeout: 30s
    max_log_entries: 10000
    snapshot_interval: 1000

tunnels:
  - name: web
    protocol: http
    local_url: http://localhost:3000
    subdomain: app
    https: auto
```

**Docker Compose for Cluster** (`docker-compose-cluster.yaml`):

```yaml
version: '3.8'

services:
  node1:
    image: gotunnel:latest
    container_name: gotunnel-node1
    environment:
      - NODE_ID=node1
      - GOTUNNEL_TOKEN=${GOTUNNEL_TOKEN}
    ports:
      - "8080:8080"
      - "8443:8443"
    volumes:
      - ./config/node1:/etc/gotunnel
      - ./data/node1:/var/lib/gotunnel/data

  node2:
    image: gotunnel:latest
    container_name: gotunnel-node2
    environment:
      - NODE_ID=node2
      - GOTUNNEL_TOKEN=${GOTUNNEL_TOKEN}
    ports:
      - "8081:8080"
      - "8444:8443"
    volumes:
      - ./config/node2:/etc/gotunnel
      - ./data/node2:/var/lib/gotunnel/data

  node3:
    image: gotunnel:latest
    container_name: gotunnel-node3
    environment:
      - NODE_ID=node3
      - GOTUNNEL_TOKEN=${GOTUNNEL_TOKEN}
    ports:
      - "8082:8080"
      - "8445:8443"
    volumes:
      - ./config/node3:/etc/gotunnel
      - ./data/node3:/var/lib/gotunnel/data

  loadbalancer:
    image: nginx:alpine
    container_name: gotunnel-lb
    ports:
      - "80:80"
      - "443:443"
    volumes:
      - ./nginx.conf:/etc/nginx/nginx.conf
    depends_on:
      - node1
      - node2
      - node3
```

**CLI Usage**:

```bash
# Check cluster status
gotunnel relay status

# View leader election
gotunnel relay leader

# Check node health
gotunnel relay health

# View replication status
gotunnel relay replication

# Force leader election
gotunnel relay elect-leader

# View cluster logs
gotunnel relay logs --follow
```

**Cluster Status Output**:

```json
{
  "cluster": {
    "leader": "node1",
    "nodes": [
      {
        "id": "node1",
        "status": "active",
        "is_leader": true,
        "last_heartbeat": "2026-03-26T23:59:59Z",
        "sequence_id": 12345,
        "uptime": "72h30m15s"
      },
      {
        "id": "node2",
        "status": "active",
        "is_leader": false,
        "last_heartbeat": "2026-03-26T23:59:58Z",
        "sequence_id": 12345,
        "uptime": "72h30m10s"
      },
      {
        "id": "node3",
        "status": "active",
        "is_leader": false,
        "last_heartbeat": "2026-03-26T23:59:57Z",
        "sequence_id": 12345,
        "uptime": "72h30m05s"
      }
    ],
    "state_replication": "healthy",
    "last_failover": null
  }
}
```

---

## 7. Security Policies

### Network and TLS Policies

Configure comprehensive security policies.

**Configuration File** (`gotunnel-security.yaml`):

```yaml
version: 1

auth:
  token: ${GOTUNNEL_TOKEN}

relay:
  broker_url: https://relay.example.com
  region: us-east-1

security:
  # Network Security
  network:
    allowed_cidrs:
      - 10.0.0.0/8       # Corporate network
      - 172.16.0.0/12    # VPN
      - 192.168.0.0/16   # Home office
    blocked_cidrs:
      - 1.2.3.0/24       # Known malicious IPs
    allowed_ports:
      - 80
      - 443
      - 8080
    blocked_ports:
      - 22               # SSH
      - 3389             # RDP
    max_connections: 10000
    timeout: 30s

  # TLS Security
  tls:
    min_version: "1.3"
    max_version: "1.3"
    cipher_suites:
      - TLS_AES_256_GCM_SHA384
      - TLS_CHACHA20_POLY1305_SHA256
      - TLS_AES_128_GCM_SHA256
    require_client_cert: false
    allowed_cns:
      - "*.example.com"
      - "api.example.com"
    allowed_sans:
      - "example.com"
      - "*.example.com"

  # Rate Limiting
  rate_limit:
    enabled: true
    requests_per_second: 100
    burst_size: 200
    window_size: 1m
    key_field: ip  # or "user", "tunnel"

  # Audit Logging
  audit:
    log_all_requests: true
    log_failed_auth: true
    log_policy_violations: true
    retain_days: 90
    exclude_fields:
      - password
      - token
      - secret
      - authorization

tunnels:
  - name: web
    protocol: http
    local_url: http://localhost:3000
    subdomain: app
    https: auto
    production: true  # Enable stricter security
```

**CLI Usage**:

```bash
# List security policies
gotunnel security policies

# Check network rules
gotunnel security network

# View TLS configuration
gotunnel security tls

# Check rate limit status
gotunnel security rate-limit

# View audit log
gotunnel security audit --tail 100

# Check policy violations
gotunnel security violations --since 24h

# Test network access
gotunnel security test-network --ip 10.0.1.100

# Test TLS connection
gotunnel security test-tls --domain app.example.com
```

**Security Status Output**:

```json
{
  "security": {
    "network": {
      "allowed_cidrs": ["10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16"],
      "blocked_cidrs": ["1.2.3.0/24"],
      "max_connections": 10000,
      "current_connections": 1234
    },
    "tls": {
      "min_version": "1.3",
      "cipher_suites": ["TLS_AES_256_GCM_SHA384", "..."],
      "require_client_cert": false
    },
    "rate_limit": {
      "enabled": true,
      "requests_per_second": 100,
      "current_rate": 45.2
    },
    "audit": {
      "total_events": 1234567,
      "violations": 12,
      "retention_days": 90
    }
  }
}
```

---

## 8. Collaborative Debugging

### Multi-User Debugging Sessions

Share tunnel sessions with team members for collaborative debugging.

**Configuration File** (`gotunnel-collab.yaml`):

```yaml
version: 1

auth:
  token: ${GOTUNNEL_TOKEN}

relay:
  broker_url: https://relay.example.com
  region: us-east-1

tunnels:
  - name: web
    protocol: http
    local_url: http://localhost:3000
    subdomain: app
    https: auto
    inspect: true
    allow_sharing: true  # Enable collaborative sessions
    production: false    # Allow mutations in dev
```

**CLI Usage**:

```bash
# Create a collaborative session
gotunnel tunnel share --name web

# Share the session link
gotunnel tunnel share --name web --invite dev2@example.com

# Join a collaborative session
gotunnel session join --token <session-token>

# List active sessions
gotunnel session list

# Add annotations to requests
gotunnel session annotate --request-id <req-id> "This request is failing"

# Set breakpoints
gotunnel session breakpoint add \
  --path "/api/users" \
  --method POST \
  --condition "status >= 400"

# View breakpoints
gotunnel session breakpoint list

# Replay a request
gotunnel session replay --request-id <req-id>

# Modify and replay
gotunnel session replay --request-id <req-id> \
  --modify-header "X-Debug: true"

# End session
gotunnel session end --session-id <session-id>
```

**Session Output**:

```json
{
  "session": {
    "id": "sess_abc123",
    "tunnel_id": "tunnel_web",
    "owner": "dev1@example.com",
    "participants": [
      {
        "email": "dev1@example.com",
        "role": "owner",
        "joined_at": "2026-03-26T23:00:00Z"
      },
      {
        "email": "dev2@example.com",
        "role": "debugger",
        "joined_at": "2026-03-26T23:05:00Z"
      }
    ],
    "invite_link": "https://dashboard.example.com/join/sess_abc123",
    "created_at": "2026-03-26T23:00:00Z",
    "expires_at": "2026-03-27T23:00:00Z"
  }
}
```

**Live Request Inspection**:

```json
{
  "request": {
    "id": "req_xyz789",
    "method": "POST",
    "path": "/api/users",
    "headers": {
      "Content-Type": "application/json",
      "Authorization": "[REDACTED]"
    },
    "body": "{\"name\": \"John Doe\", \"email\": \"john@example.com\"}",
    "timestamp": "2026-03-26T23:10:00Z",
    "source_ip": "203.0.113.1"
  },
  "response": {
    "status": 500,
    "headers": {
      "Content-Type": "application/json"
    },
    "body": "{\"error\": \"Internal Server Error\"}",
    "duration": "150ms"
  },
  "annotations": [
    {
      "author": "dev1@example.com",
      "message": "This is the failing request we need to debug",
      "timestamp": "2026-03-26T23:10:05Z"
    }
  ]
}
```

---

## Additional Resources

- [Main README](../README.md)
- [Configuration Reference](../gotunnel.example.yaml)
- [Enterprise Architecture](../docs/enterprise-architecture.md)
- [PRD](../PRD.md)
- [Timeline](../timeline.md)