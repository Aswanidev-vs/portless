package cli

import (
	"fmt"
	"os"
	"strings"
)

// ContainerRuntime represents a container runtime
type ContainerRuntime string

const (
	RuntimeDocker ContainerRuntime = "docker"
	RuntimePodman ContainerRuntime = "podman"
	RuntimeCRI    ContainerRuntime = "cri"
)

// DetectContainerRuntime detects available container runtime
func DetectContainerRuntime() ContainerRuntime {
	// Check for Docker
	if _, err := os.Stat("/var/run/docker.sock"); err == nil {
		return RuntimeDocker
	}

	// Check for Podman
	if _, err := os.Stat("/run/podman/podman.sock"); err == nil {
		return RuntimePodman
	}

	// Check environment
	if os.Getenv("DOCKER_HOST") != "" {
		return RuntimeDocker
	}

	return ""
}

// GenerateDockerfile generates a Dockerfile for GoTunnel
func GenerateDockerfile() string {
	return `# Build stage
FROM golang:1.21-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o gotunnel ./cmd/gotunnel

# Runtime stage
FROM alpine:latest

RUN apk --no-cache add ca-certificates tzdata
RUN adduser -D -u 1000 gotunnel

WORKDIR /app
COPY --from=builder /app/gotunnel .
COPY gotunnel.yaml .

USER gotunnel

EXPOSE 8080 8443 9090

ENTRYPOINT ["./gotunnel"]
CMD ["relay", "--listen", ":8080"]
`
}

// GenerateDockerCompose generates a docker-compose.yaml
func GenerateDockerCompose() string {
	return `version: '3.8'

services:
  gotunnel:
    build: .
    image: gotunnel:latest
    container_name: gotunnel
    ports:
      - "8080:8080"
      - "8443:8443"
      - "9090:9090"
    environment:
      - GOTUNNEL_TOKEN=\${GOTUNNEL_TOKEN}
      - ACME_EMAIL=\${ACME_EMAIL}
      - CLOUDFLARE_API_TOKEN=\${CLOUDFLARE_API_TOKEN}
      - DATABASE_URL=postgres://gotunnel:\${DB_PASSWORD}@postgres:5432/gotunnel?sslmode=disable
    volumes:
      - ./config:/etc/gotunnel
      - ./certs:/var/lib/gotunnel/certs
      - ./data:/var/lib/gotunnel/data
    depends_on:
      - postgres
      - redis
    restart: unless-stopped
    healthcheck:
      test: ["CMD", "curl", "-f", "http://localhost:8080/health"]
      interval: 30s
      timeout: 10s
      retries: 3

  postgres:
    image: postgres:15-alpine
    container_name: gotunnel-postgres
    environment:
      - POSTGRES_DB=gotunnel
      - POSTGRES_USER=gotunnel
      - POSTGRES_PASSWORD=\${DB_PASSWORD}
    volumes:
      - postgres_data:/var/lib/postgresql/data
    ports:
      - "5432:5432"
    restart: unless-stopped
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U gotunnel"]
      interval: 10s
      timeout: 5s
      retries: 5

  redis:
    image: redis:7-alpine
    container_name: gotunnel-redis
    command: redis-server --appendonly yes --requirepass \${REDIS_PASSWORD}
    volumes:
      - redis_data:/data
    ports:
      - "6379:6379"
    restart: unless-stopped
    healthcheck:
      test: ["CMD", "redis-cli", "--raw", "incr", "ping"]
      interval: 10s
      timeout: 5s
      retries: 5

volumes:
  postgres_data:
  redis_data:
`
}

// GenerateKubernetesManifest generates Kubernetes manifests
func GenerateKubernetesManifest() string {
	return `apiVersion: v1
kind: Namespace
metadata:
  name: gotunnel

---
apiVersion: v1
kind: Secret
metadata:
  name: gotunnel-secrets
  namespace: gotunnel
type: Opaque
data:
  token: \${GOTUNNEL_TOKEN_BASE64}
  db-password: \${DB_PASSWORD_BASE64}
  redis-password: \${REDIS_PASSWORD_BASE64}

---
apiVersion: v1
kind: ConfigMap
metadata:
  name: gotunnel-config
  namespace: gotunnel
data:
  gotunnel.yaml: |
    version: 1
    auth:
      token: \${GOTUNNEL_TOKEN}
    relay:
      broker_url: https://gotunnel-broker.default.svc.cluster.local
    tunnels:
      - name: web
        protocol: http
        local_url: http://localhost:3000
        subdomain: app
        https: auto

---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: gotunnel
  namespace: gotunnel
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
        - containerPort: 9090
          name: metrics
        env:
        - name: GOTUNNEL_TOKEN
          valueFrom:
            secretKeyRef:
              name: gotunnel-secrets
              key: token
        volumeMounts:
        - name: config
          mountPath: /etc/gotunnel
        - name: certs
          mountPath: /var/lib/gotunnel/certs
        - name: data
          mountPath: /var/lib/gotunnel/data
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
      volumes:
      - name: config
        configMap:
          name: gotunnel-config
      - name: certs
        emptyDir: {}
      - name: data
        emptyDir: {}

---
apiVersion: v1
kind: Service
metadata:
  name: gotunnel
  namespace: gotunnel
spec:
  selector:
    app: gotunnel
  ports:
  - name: relay
    port: 8080
    targetPort: 8080
  - name: https
    port: 8443
    targetPort: 8443
  - name: metrics
    port: 9090
    targetPort: 9090
  type: ClusterIP

---
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: gotunnel-ingress
  namespace: gotunnel
  annotations:
    kubernetes.io/ingress.class: nginx
    cert-manager.io/cluster-issuer: letsencrypt
spec:
  tls:
  - hosts:
    - gotunnel.example.com
    secretName: gotunnel-tls
  rules:
  - host: gotunnel.example.com
    http:
      paths:
      - path: /
        pathType: Prefix
        backend:
          service:
            name: gotunnel
            port:
              number: 8080
`
}

// GenerateHelmChart generates a basic Helm chart structure
func GenerateHelmChart() string {
	return `# Helm Chart for GoTunnel
# Create the following structure:

gotunnel/
├── Chart.yaml
├── values.yaml
├── templates/
│   ├── deployment.yaml
│   ├── service.yaml
│   ├── configmap.yaml
│   ├── secret.yaml
│   └── ingress.yaml
└── charts/
    └── postgresql/
        └── values.yaml

# Chart.yaml
apiVersion: v2
name: gotunnel
description: Enterprise-grade tunneling platform
version: 1.0.0
appVersion: "1.0.0"

# values.yaml
replicaCount: 3
image:
  repository: gotunnel
  tag: latest
  pullPolicy: IfNotPresent

service:
  type: ClusterIP
  port: 8080

ingress:
  enabled: true
  className: nginx
  annotations:
    cert-manager.io/cluster-issuer: letsencrypt
  hosts:
    - host: gotunnel.example.com
      paths:
        - path: /
          pathType: Prefix
  tls:
    - secretName: gotunnel-tls
      hosts:
        - gotunnel.example.com

config:
  version: 1
  auth:
    token: ""
  relay:
    broker_url: "https://gotunnel-broker.default.svc.cluster.local"
  tunnels:
    - name: web
      protocol: http
      local_url: "http://localhost:3000"
      subdomain: app
      https: auto
`
}

// ContainerCommand handles containerization
var ContainerCommand *Command

func init() {
	ContainerCommand = &Command{
		Name:    "container",
		Aliases: []string{"docker", "k8s"},
		Short:   "Containerization support",
		Long:    "Generate container configurations including Dockerfile, docker-compose, Kubernetes manifests, and Helm charts.",
		Usage:   "gotunnel container <generate> [options]",
		Subcommands: map[string]*Command{
			"generate": {
				Name:  "generate",
				Short: "Generate container configs",
				Run:   runContainerGenerate,
			},
		},
		Run: runContainerCmd,
	}
}

func runContainerCmd(args []string) error {
	if len(args) == 0 {
		printCommandHelp(ContainerCommand)
		return nil
	}
	return fmt.Errorf("unknown container subcommand: %s", args[0])
}

func runContainerGenerate(args []string) error {
	target := ""
	for i := 0; i < len(args); i++ {
		if (args[i] == "--type" || args[i] == "-t") && i+1 < len(args) {
			target = args[i+1]
			break
		}
	}

	if target == "" {
		fmt.Println("Available types: dockerfile, docker-compose, kubernetes, helm")
		return fmt.Errorf("usage: gotunnel container generate --type <type>")
	}

	var output string
	switch strings.ToLower(target) {
	case "dockerfile":
		output = GenerateDockerfile()
	case "docker-compose", "compose":
		output = GenerateDockerCompose()
	case "kubernetes", "k8s":
		output = GenerateKubernetesManifest()
	case "helm":
		output = GenerateHelmChart()
	default:
		return fmt.Errorf("unsupported type: %s", target)
	}

	fmt.Println(output)
	return nil
}

// Container registration is handled in root.go init()
