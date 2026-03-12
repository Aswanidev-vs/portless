# PRD — Portless Dev Router

*A lightweight local service router with automatic port management and clean internal domains.*

## 1. Overview

**Portless Dev Router** is a lightweight developer tool that removes the need to remember or manage ports when running local services.

Instead of accessing services via ports:

```text
localhost:8000
localhost:3000
localhost:5173
```

Developers can use clean internal domains:

```text
yuki.backend.internal
horiz.internal
api.internal
grafana.internal
```

The tool automatically:

* assigns free ports
* routes traffic
* resolves domains locally
* prevents port conflicts

All services become accessible via **clean URLs without ports**.

The tool runs as a **single lightweight Go binary**.

---

# 2. Goals

### Primary Goals

1. Eliminate port conflicts during local development.
2. Replace `localhost:PORT` with clean internal domains.
3. Provide automatic port allocation.
4. Require minimal configuration.
5. Maintain extremely low resource usage.

### Non-Goals

The tool is NOT intended to:

* replace Kubernetes
* manage production infrastructure
* run heavy container orchestration
* replace full reverse proxies in production

This tool focuses on **local development environments**.

---

# 3. Target Users

Primary users:

* Backend developers
* Full-stack developers
* DevOps engineers
* Open-source contributors
* Students building multi-service apps

Typical users running services like:

* FastAPI
* Node.js
* Next.js
* Django
* Go APIs
* Docker containers

---

# 4. Core Problem

Local development environments frequently suffer from:

* port conflicts
* difficulty remembering ports
* messy URLs
* manual configuration

Example problem:

```text
frontend → localhost:3000
backend → localhost:8000
admin → localhost:8080
grafana → localhost:3001
```

Developers must constantly remember these mappings.

The goal is to transform this into:

```text
frontend.dev.local
backend.dev.local
admin.dev.local
grafana.dev.local
```

supports internal domains like
```
*.internal
*.local
*.pc
*.{suggest more ones; people can even configure own ones}
```

---

# 5. Key Features

## 5.1 Automatic Port Allocation

When a service starts, the system assigns an available port.

Example range:

```
40000–50000
```

Algorithm:

1. Scan port range
2. Identify unused port
3. Assign port
4. Register mapping

Example:

```
service: yuki-backend
assigned_port: 40123
```

Port conflicts are automatically prevented.

---

## 5.2 Domain-Based Routing

The router listens on:

```
80
443 (optional)
```

Incoming requests are routed based on **hostname**.

Example routing table:

```
yuki.backend.internal → localhost:40123
horiz.internal → localhost:40124
grafana.internal → localhost:40125
```

Routing logic:

```
request.host → lookup target → proxy request
```

---

## 5.3 Local DNS Resolution

The system resolves wildcard domains locally.

Example rule:

```
*.internal → 127.0.0.1
```

Supported domains:

```
*.internal
*.dev.local
*.local.dev
```

This allows any service name to resolve automatically.

---

## 5.4 YAML Configuration

Services can be declared using YAML.

Example:

```yaml
services:
  yuki-backend:
    domain: yuki.backend.internal
    command: uvicorn main:app

  horiz-frontend:
    domain: horiz.internal
    command: npm run dev
```

Running:

```
devrouter start
```

Will:

1. start services
2. assign ports
3. register routing
4. enable DNS

---

## 5.5 CLI Interface

Example commands:

Start router

```
devrouter start
```

Add service

```
devrouter add yuki.backend.internal 8000
```

List services

```
devrouter list
```

Remove service

```
devrouter remove yuki.backend.internal
```

---

## 5.6 Automatic Service Discovery (Future Feature)

Detect common dev servers automatically.

Examples:

* FastAPI
* Vite
* Next.js
* Node.js
* Docker containers

Example detection:

```
Detected service:
uvicorn running on port 8000
```

Auto-map:

```
api.dev.local → localhost:8000
```

---

## 5.7 Local Network Access

Allow services to be accessible from devices in the same network.

Example:

Instead of:

```
192.168.1.10:3000
```

Users access:

```
horiz.internal
```

LAN DNS resolution supported.

---

# 6. Architecture

High-level architecture:

```
           Browser
              │
              ▼
        Local DNS Resolver
              │
              ▼
        Reverse Proxy Router
              │
              ▼
          Service Ports
```

Components:

```
CLI
Config Loader
Port Manager
Routing Engine
DNS Resolver
Reverse Proxy
```

---

# 7. System Components

## 7.1 Port Manager

Responsible for:

* detecting used ports
* assigning free ports
* preventing conflicts

Example internal map:

```
service → port
```

---

## 7.2 Router

Handles HTTP requests.

Responsibilities:

* inspect Host header
* map domain → port
* proxy request

Uses Go HTTP reverse proxy.

---

## 7.3 DNS Resolver

Resolves wildcard domain to localhost.

Example:

```
*.internal → 127.0.0.1
```

Possible implementation:

* lightweight DNS server
* OS hosts file management

---

## 7.4 Config Manager

Loads YAML configuration.

Stores service mappings.

Example structure:

```
domain → service
service → port
```

---

# 8. Technology Stack

Primary language:

**Go**

Reasons:

* low memory usage
* fast networking
* single binary deployment
* easy concurrency

Suggested Go packages:

```
net/http
net/http/httputil
gopkg.in/yaml.v3
miekg/dns
cobra (CLI)
```

---

# 9. Performance Targets

Resource usage target:

| Metric      | Target    |
| ----------- | --------- |
| Memory      | <10 MB    |
| CPU         | near idle |
| Binary size | <15 MB    |

The router should handle thousands of requests per second locally.

---

# 10. Security Considerations

Security scope is limited since the tool runs locally.

However:

* restrict exposed interfaces
* avoid open DNS abuse
* prevent external traffic injection

Default DNS binding:

```
127.0.0.1
```

---

# 11. Future Enhancements

Possible improvements:

### HTTPS support

Automatic TLS using local certificates.

Example:

```
https://horiz.internal
```

### Docker integration

Auto-detect containers.

### Service dashboard

Web UI showing running services.

### Hot reload

Reload config without restarting.

### Plugin system

Allow custom routing rules.

---

# 12. Success Metrics

Success of the project measured by:

* developer adoption
* GitHub stars
* ease of setup
* reliability
* performance

---

# 13. Example Developer Workflow

Typical usage:

```
git clone project
devrouter start
```

Services become accessible via:

```
api.internal
frontend.internal
grafana.internal
```

No ports required.

---

# 14. Possible Project Names

Potential repository names:

```
portless ( Agreed )
devrouter
localmesh
devfabric
portless-dev
devdns
```

Example:

```
github.com/ivintitus/portless
```

---

✅ This project is **excellent for your GitHub portfolio** because it demonstrates:

* networking knowledge
* Go backend engineering
* DevOps tooling
* CLI development
* DNS + reverse proxy systems

It’s the kind of project that **stands out compared to typical student repos**.
