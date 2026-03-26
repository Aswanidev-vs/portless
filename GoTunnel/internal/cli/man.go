package cli

import (
	"fmt"
	"strings"
	"time"
)

// ManPage represents a man page entry
type ManPage struct {
	Name        string
	Synopsis    string
	Description string
	Options     []ManOption
	Commands    []ManCommand
	Examples    []ManExample
	SeeAlso     []string
	Author      string
	Version     string
	Date        time.Time
}

// ManOption represents a command-line option
type ManOption struct {
	Short       string
	Long        string
	Description string
	Default     string
	Required    bool
}

// ManCommand represents a subcommand
type ManCommand struct {
	Name        string
	Synopsis    string
	Description string
}

// ManExample represents a usage example
type ManExample struct {
	Description string
	Command     string
}

// GenerateManPage generates a man page for the CLI
func GenerateManPage() string {
	page := ManPage{
		Name:        "gotunnel",
		Synopsis:    "gotunnel [global-options] <command> [command-options] [arguments]",
		Description: "GoTunnel is an enterprise-grade, self-hostable tunneling platform that provides secure, scalable tunneling with automated HTTPS, multi-provider DNS, RBAC, MFA, and production-grade multiplexing.",
		Options: []ManOption{
			{Short: "-c", Long: "--config", Description: "Path to configuration file", Default: "gotunnel.yaml"},
			{Short: "-l", Long: "--log-level", Description: "Log level (trace, debug, info, warn, error, fatal)", Default: "info"},
			{Long: "--log-format", Description: "Log format (text, json)", Default: "text"},
			{Long: "--json", Description: "Output in JSON format"},
			{Long: "--dry-run", Description: "Show what would be done without executing"},
			{Short: "-v", Long: "--verbose", Description: "Enable verbose output"},
			{Short: "-q", Long: "--quiet", Description: "Suppress non-error output"},
			{Long: "--no-color", Description: "Disable colored output"},
			{Short: "-t", Long: "--token", Description: "Authentication token"},
			{Short: "-p", Long: "--profile", Description: "Configuration profile (dev, staging, prod)"},
			{Long: "--timeout", Description: "Command timeout (e.g., 30s, 5m)"},
			{Short: "-h", Long: "--help", Description: "Show help"},
		},
		Commands: []ManCommand{
			{Name: "login", Synopsis: "gotunnel login --token <token>", Description: "Authenticate with GoTunnel by storing an access token"},
			{Name: "broker", Synopsis: "gotunnel broker [--listen :8090]", Description: "Start the broker server for tunnel coordination"},
			{Name: "relay", Synopsis: "gotunnel relay [--listen :8091]", Description: "Start the relay server for tunnel traffic"},
			{Name: "config", Synopsis: "gotunnel config <validate|init|show>", Description: "Manage configuration files"},
			{Name: "tunnel", Synopsis: "gotunnel tunnel <start|stop|list|inspect|share>", Description: "Manage tunnel lifecycle"},
			{Name: "cert", Synopsis: "gotunnel cert <list|renew|revoke|info>", Description: "Manage TLS certificates"},
			{Name: "dns", Synopsis: "gotunnel dns <providers|records|test>", Description: "Manage DNS providers and records"},
			{Name: "session", Synopsis: "gotunnel session <list|join|annotate|replay>", Description: "Manage collaborative debugging sessions"},
			{Name: "security", Synopsis: "gotunnel security <policies|network|tls|rate-limit|audit>", Description: "Manage security policies"},
			{Name: "completion", Synopsis: "gotunnel completion <bash|zsh|fish>", Description: "Generate shell completion scripts"},
			{Name: "version", Synopsis: "gotunnel version", Description: "Show version information"},
			{Name: "help", Synopsis: "gotunnel help [command]", Description: "Show help for a command"},
		},
		Examples: []ManExample{
			{Description: "Start a simple HTTP tunnel", Command: "gotunnel tunnel start --name web --local-url http://localhost:3000"},
			{Description: "Start a tunnel with custom subdomain", Command: "gotunnel tunnel start --name api --subdomain myapi --local-url http://localhost:8080"},
			{Description: "Validate configuration", Command: "gotunnel config validate --file gotunnel.yaml"},
			{Description: "List active tunnels", Command: "gotunnel tunnel list"},
			{Description: "Renew a certificate", Command: "gotunnel cert renew --domain example.com"},
			{Description: "Generate bash completion", Command: "gotunnel completion bash"},
		},
		SeeAlso: []string{"gotunnel.yaml(5)"},
		Author:  "GoTunnel Contributors",
		Version: "1.0.0",
		Date:    time.Now(),
	}

	return formatManPage(page)
}

func formatManPage(page ManPage) string {
	var sb strings.Builder

	// Header
	sb.WriteString(fmt.Sprintf("GOTUNNEL(1)                   User Commands                   GOTUNNEL(1)\n\n"))

	// Name
	sb.WriteString("NAME\n")
	sb.WriteString(fmt.Sprintf("       %s - %s\n\n", page.Name, page.Description))

	// Synopsis
	sb.WriteString("SYNOPSIS\n")
	sb.WriteString(fmt.Sprintf("       %s\n\n", page.Synopsis))

	// Description
	sb.WriteString("DESCRIPTION\n")
	sb.WriteString(fmt.Sprintf("       %s\n\n", page.Description))

	// Options
	sb.WriteString("OPTIONS\n")
	for _, opt := range page.Options {
		var flag string
		if opt.Short != "" && opt.Long != "" {
			flag = fmt.Sprintf("%s, %s", opt.Short, opt.Long)
		} else if opt.Long != "" {
			flag = opt.Long
		} else {
			flag = opt.Short
		}

		desc := opt.Description
		if opt.Default != "" {
			desc += fmt.Sprintf(" (default: %s)", opt.Default)
		}
		if opt.Required {
			desc += " (required)"
		}
		sb.WriteString(fmt.Sprintf("       %s\n              %s\n\n", flag, desc))
	}

	// Commands
	sb.WriteString("COMMANDS\n")
	for _, cmd := range page.Commands {
		sb.WriteString(fmt.Sprintf("       %s\n              %s\n\n", cmd.Name, cmd.Description))
		sb.WriteString(fmt.Sprintf("              Usage: %s\n\n", cmd.Synopsis))
	}

	// Examples
	sb.WriteString("EXAMPLES\n")
	for i, ex := range page.Examples {
		sb.WriteString(fmt.Sprintf("       %d. %s\n", i+1, ex.Description))
		sb.WriteString(fmt.Sprintf("              $ %s\n\n", ex.Command))
	}

	// See Also
	if len(page.SeeAlso) > 0 {
		sb.WriteString("SEE ALSO\n")
		sb.WriteString(fmt.Sprintf("       %s\n\n", strings.Join(page.SeeAlso, ", ")))
	}

	// Author
	sb.WriteString("AUTHOR\n")
	sb.WriteString(fmt.Sprintf("       %s\n\n", page.Author))

	// Version
	sb.WriteString("VERSION\n")
	sb.WriteString(fmt.Sprintf("       %s\n", page.Version))

	return sb.String()
}

// GenerateCommandManPage generates a man page for a specific command
func GenerateCommandManPage(command string) string {
	pages := map[string]string{
		"tunnel": generateTunnelManPage(),
		"config": generateConfigManPage(),
		"cert":   generateCertManPage(),
		"dns":    generateDNSManPage(),
	}

	if page, ok := pages[command]; ok {
		return page
	}
	return fmt.Sprintf("No manual entry for %s\n", command)
}

func generateTunnelManPage() string {
	return `GOTUNNEL-TUNNEL(1)            Tunnel Commands            GOTUNNEL-TUNNEL(1)

NAME
       gotunnel-tunnel - Manage tunnel lifecycle

SYNOPSIS
       gotunnel tunnel start [options]
       gotunnel tunnel stop [options]
       gotunnel tunnel list [options]
       gotunnel tunnel inspect [options]
       gotunnel tunnel share [options]

DESCRIPTION
       The tunnel command manages the lifecycle of tunnels including starting,
       stopping, listing, inspecting, and sharing tunnels.

SUBCOMMANDS
       start   Start a new tunnel
       stop    Stop a running tunnel
       list    List all tunnels
       inspect Inspect tunnel traffic
       share   Create a collaborative debugging session

START OPTIONS
       --name NAME           Tunnel name (default: app)
       --protocol PROTO      Protocol: http, tcp, udp (default: http)
       --local-url URL       Local service URL
       --subdomain SUB       Requested subdomain
       --https MODE          HTTPS mode: auto, manual, disabled (default: auto)
       --inspect BOOL        Enable inspection (default: true)
       --production BOOL     Mark as production (default: false)

EXAMPLES
       Start an HTTP tunnel:
              $ gotunnel tunnel start --name web --local-url http://localhost:3000

       Start a TCP tunnel:
              $ gotunnel tunnel start --name db --protocol tcp --local-url localhost:5432

SEE ALSO
       gotunnel(1), gotunnel-config(1)
`
}

func generateConfigManPage() string {
	return `GOTUNNEL-CONFIG(1)           Config Commands           GOTUNNEL-CONFIG(1)

NAME
       gotunnel-config - Manage configuration files

SYNOPSIS
       gotunnel config validate [options]
       gotunnel config init [options]
       gotunnel config show [options]

DESCRIPTION
       The config command manages GoTunnel configuration files including
       validation, initialization, and display.

SUBCOMMANDS
       validate  Validate a configuration file
       init      Initialize a new configuration file
       show      Show current configuration

VALIDATE OPTIONS
       --file PATH   Path to config file (default: gotunnel.yaml)

EXAMPLES
       Validate configuration:
              $ gotunnel config validate --file gotunnel.yaml

       Initialize new config:
              $ gotunnel config init

SEE ALSO
       gotunnel(1), gotunnel-tunnel(1)
`
}

func generateCertManPage() string {
	return `GOTUNNEL-CERT(1)             Certificate Commands      GOTUNNEL-CERT(1)

NAME
       gotunnel-cert - Manage TLS certificates

SYNOPSIS
       gotunnel cert list [options]
       gotunnel cert renew --domain DOMAIN
       gotunnel cert revoke --domain DOMAIN
       gotunnel cert info --domain DOMAIN

DESCRIPTION
       The cert command manages TLS certificates including listing, renewing,
       revoking, and viewing certificate information.

SUBCOMMANDS
       list    List all managed certificates
       renew   Renew a certificate
       revoke  Revoke a certificate
       info    Show certificate information

OPTIONS
       --domain DOMAIN   Domain name for certificate operations

EXAMPLES
       List certificates:
              $ gotunnel cert list

       Renew certificate:
              $ gotunnel cert renew --domain example.com

SEE ALSO
       gotunnel(1), gotunnel-dns(1)
`
}

func generateDNSManPage() string {
	return `GOTUNNEL-DNS(1)              DNS Commands              GOTUNNEL-DNS(1)

NAME
       gotunnel-dns - Manage DNS providers and records

SYNOPSIS
       gotunnel dns providers
       gotunnel dns records --zone ZONE
       gotunnel dns test --domain DOMAIN

DESCRIPTION
       The dns command manages DNS providers and records for ACME
       challenge validation and domain management.

SUBCOMMANDS
       providers  List configured DNS providers
       records    List DNS records for a zone
       test       Test DNS propagation for a domain

OPTIONS
       --zone ZONE       DNS zone name
       --domain DOMAIN   Domain name to test

EXAMPLES
       List DNS providers:
              $ gotunnel dns providers

       Test DNS propagation:
              $ gotunnel dns test --domain app.example.com

SEE ALSO
       gotunnel(1), gotunnel-cert(1)
`
}
