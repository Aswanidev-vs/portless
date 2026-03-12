# Portless

Portless is a local service router and DNS orchestrator designed for Linux environments. It eliminates port conflicts by dynamically allocating ephemeral ports to child processes and routing HTTP traffic over clean wildcard domains (e.g., `*.localhost`, `*.internal`).

## Architecture Overview

Portless operates simultaneously as a Process Supervisor, a DNS Resolver, and an HTTP Reverse Proxy:
- **Port Manager**: Binds securely to `:0` to allocate collision-free TCP ports from the host network.
- **Process Supervisor**: Injects `$PORT` into process execution environments, monitors stdout/stderr with custom prefixes, and uses OS Process Groups (`Setpgid`) to safely SIGTERM entire process trees upon exit.
- **Local DNS Resolver**: Intercepts UDP/53 queries for defined pseudo-TLDs and resolves them to the host's Local Area Network IP or `127.0.0.1`.
- **IPC Daemon**: Exposes a Unix Domain Socket (`/tmp/portless.sock`) providing a RESTful API to hot-reload routes and mutate process state without restarting the core router.

*For a detailed sequence diagram, see the [Architecture Docs](docs/architecture.md).*

## Requirements
- Go 1.21+
- Linux (for capability support)

## Installation & Setup

Build the project and make it executable globally:

```bash
git clone https://github.com/ivin-titus/portless.git
cd portless
go build -o portless ./cmd/portless
sudo mv portless /usr/local/bin/
```

### Network Capabilities (`setcap`)

Portless requires elevated permissions to bind to Port 80 and Port 53. To avoid running the daemon as root, explicitly grant the binary `cap_net_bind_service`:

```bash
sudo setcap cap_net_bind_service=+ep /usr/local/bin/portless
```
> If capabilities are not assigned, Portless will gracefully fall back to binding on Port `8080`.

### DNS Configuration

To unconditionally route `*.localhost` or `*.internal` to Portless, configure your system's resolver (e.g., `systemd-resolved`):

```bash
sudo mkdir -p /etc/systemd/resolved.conf.d/
echo -e "[Resolve]\nDNS=127.0.0.1:53\nDomains=~internal ~localhost" | sudo tee /etc/systemd/resolved.conf.d/portless.conf
sudo systemctl restart systemd-resolved
```

## Usage

### 1. Configuration (`portless.yaml`)

Define routing rules and executing commands in the root of your project:

```yaml
services:
  python-api:
    domain: api.localhost
    command: uvicorn main:app
  
  frontend:
    domain: web.localhost
    command: npm run dev
```

*Note: Ensure your framework respects the `$PORT` environment variable. Portless assigns ports strictly through `$PORT` injection.*

### 2. Execution

Start the core routing daemon:

```bash
portless start
```

### 3. IPC Operations

Add, view, or remove services dynamically over the Unix socket via secondary terminal windows:

```bash
portless list
portless add grafana.localhost "npm run start:ui"
portless remove grafana.localhost
```

## Contributing
See [CONTRIBUTING.md](CONTRIBUTING.md) for branch naming conventions, workflow architecture, and testing guidelines. Code behavior policies are found in [CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md).

## License
Portless is licensed under the AGPL-3.0 License. See [LICENSE](LICENSE) for the full text.
