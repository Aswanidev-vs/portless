package cli

import (
	"fmt"
	"strings"
)

// Command represents a CLI command
type Command struct {
	Name        string
	Aliases     []string
	Short       string
	Long        string
	Usage       string
	Subcommands map[string]*Command
	Run         func(args []string) error
}

// RootCommand is the root command for the CLI
var RootCommand *Command

func init() {
	RootCommand = &Command{
		Name:        "gotunnel",
		Short:       "Enterprise-grade tunneling platform",
		Long:        "GoTunnel is an enterprise-grade, self-hostable tunneling platform that provides secure, scalable tunneling with automated HTTPS, multi-provider DNS, RBAC, MFA, and production-grade multiplexing.",
		Usage:       "gotunnel [global-options] <command> [command-options] [arguments]",
		Subcommands: map[string]*Command{},
	}

	// Register all commands
	RootCommand.Subcommands["login"] = LoginCommand
	RootCommand.Subcommands["broker"] = BrokerCommand
	RootCommand.Subcommands["relay"] = RelayCommand
	RootCommand.Subcommands["config"] = ConfigCommand
	RootCommand.Subcommands["tunnel"] = TunnelCommand
	RootCommand.Subcommands["cert"] = CertCommand
	RootCommand.Subcommands["dns"] = DNSCommand
	RootCommand.Subcommands["session"] = SessionCommand
	RootCommand.Subcommands["security"] = SecurityCommand
	RootCommand.Subcommands["completion"] = CompletionCommand
	RootCommand.Subcommands["version"] = VersionCommand
	RootCommand.Subcommands["help"] = HelpCommand
	RootCommand.Subcommands["plugin"] = PluginCommand
	RootCommand.Subcommands["profile"] = ProfileCommand
	RootCommand.Subcommands["cicd"] = CICDCommand
	RootCommand.Subcommands["container"] = ContainerCommand
}

// Execute runs the root command
func Execute() error {
	globalOpts, args := ParseGlobalOptions()
	ApplyGlobalOptions(globalOpts)

	if len(args) == 0 {
		printUsage()
		return nil
	}

	cmdName := args[0]
	cmdArgs := args[1:]

	// Handle global help
	if cmdName == "--help" || cmdName == "-h" {
		printUsage()
		return nil
	}

	// Handle version
	if cmdName == "version" || cmdName == "--version" {
		return runVersion(cmdArgs)
	}

	// Handle completion
	if cmdName == "completion" {
		return runCompletion(cmdArgs)
	}

	// Handle man page
	if cmdName == "man" {
		return runMan(cmdArgs)
	}

	// Find and execute command
	cmd, ok := RootCommand.Subcommands[cmdName]
	if !ok {
		return fmt.Errorf("unknown command %q\n\nRun 'gotunnel help' for usage", cmdName)
	}

	if cmd.Run == nil {
		if len(cmdArgs) > 0 && (cmdArgs[0] == "--help" || cmdArgs[0] == "-h") {
			printCommandHelp(cmd)
			return nil
		}
		printCommandHelp(cmd)
		return nil
	}

	return cmd.Run(cmdArgs)
}

func printUsage() {
	fmt.Println(`GoTunnel - Enterprise-grade tunneling platform

USAGE:
  gotunnel [global-options] <command> [command-options] [arguments]

COMMANDS:
  login      Authenticate with GoTunnel
  broker     Start broker server
  relay      Start relay server
  config     Manage configuration
  tunnel     Manage tunnels
  cert       Manage certificates
  dns        Manage DNS providers
  session    Manage collaborative sessions
  security   Manage security policies
  completion Generate shell completion scripts
  version    Show version information
  help       Show help for a command

GLOBAL OPTIONS:`)
	PrintGlobalHelp()
	fmt.Println(`
Use "gotunnel help <command>" for more information about a command.`)
}

func printCommandHelp(cmd *Command) {
	fmt.Printf("%s - %s\n\n", cmd.Name, cmd.Short)
	if cmd.Long != "" {
		fmt.Printf("DESCRIPTION\n  %s\n\n", cmd.Long)
	}
	if cmd.Usage != "" {
		fmt.Printf("USAGE\n  %s\n\n", cmd.Usage)
	}
	if len(cmd.Subcommands) > 0 {
		fmt.Println("SUBCOMMANDS:")
		for name, sub := range cmd.Subcommands {
			fmt.Printf("  %-15s %s\n", name, sub.Short)
		}
		fmt.Println()
	}
}

// LoginCommand handles authentication
var LoginCommand = &Command{
	Name:    "login",
	Aliases: []string{"auth"},
	Short:   "Authenticate with GoTunnel",
	Long:    "Store an access token for authenticating with GoTunnel services.",
	Usage:   "gotunnel login --token <token>",
	Run:     runLoginCmd,
}

func runLoginCmd(args []string) error {
	return runLogin(args)
}

// BrokerCommand handles broker server
var BrokerCommand = &Command{
	Name:  "broker",
	Short: "Start broker server",
	Long:  "Start the broker server for tunnel coordination and session management.",
	Usage: "gotunnel broker [--listen :8090]",
	Run:   runBrokerCmd,
}

func runBrokerCmd(args []string) error {
	return runBroker(args)
}

// RelayCommand handles relay server
var RelayCommand = &Command{
	Name:  "relay",
	Short: "Start relay server",
	Long:  "Start the relay server for tunnel traffic forwarding.",
	Usage: "gotunnel relay [--listen :8091]",
	Run:   runRelayCmd,
}

func runRelayCmd(args []string) error {
	return runRelay(args)
}

// ConfigCommand handles configuration management
var ConfigCommand *Command

func init() {
	ConfigCommand = &Command{
		Name:    "config",
		Aliases: []string{"cfg"},
		Short:   "Manage configuration",
		Long:    "Manage GoTunnel configuration files including validation, initialization, and display.",
		Usage:   "gotunnel config <validate|init|show> [options]",
		Subcommands: map[string]*Command{
			"validate": {
				Name:  "validate",
				Short: "Validate configuration file",
				Run:   runConfigValidate,
			},
			"init": {
				Name:  "init",
				Short: "Initialize new configuration file",
				Run:   runConfigInit,
			},
			"show": {
				Name:  "show",
				Short: "Show current configuration",
				Run:   runConfigShow,
			},
		},
		Run: runConfigCmd,
	}
}

func runConfigCmd(args []string) error {
	return runConfig(args)
}

func runConfigValidate(args []string) error {
	configPath := "gotunnel.yaml"
	for i, arg := range args {
		if (arg == "--file" || arg == "-f") && i+1 < len(args) {
			configPath = args[i+1]
			break
		}
	}

	warnings, err := ValidateConfig(configPath)
	if err != nil {
		return err
	}

	if len(warnings) > 0 {
		fmt.Println("Configuration warnings:")
		for _, w := range warnings {
			fmt.Printf("  - %s\n", w)
		}
	} else {
		fmt.Println("Configuration is valid")
	}
	return nil
}

func runConfigInit(args []string) error {
	fmt.Println("Creating default configuration file...")
	// Implementation would create gotunnel.yaml with defaults
	return nil
}

func runConfigShow(args []string) error {
	return runConfig(append([]string{"show"}, args...))
}

// TunnelCommand handles tunnel management
var TunnelCommand *Command

func init() {
	TunnelCommand = &Command{
		Name:    "tunnel",
		Aliases: []string{"tun"},
		Short:   "Manage tunnels",
		Long:    "Manage tunnel lifecycle including starting, stopping, listing, inspecting, and sharing tunnels.",
		Usage:   "gotunnel tunnel <start|stop|list|inspect|share> [options]",
		Subcommands: map[string]*Command{
			"start": {
				Name:  "start",
				Short: "Start a new tunnel",
				Run:   runTunnelStartCmd,
			},
			"stop": {
				Name:  "stop",
				Short: "Stop a running tunnel",
				Run:   runTunnelStop,
			},
			"list": {
				Name:    "list",
				Aliases: []string{"ls"},
				Short:   "List all tunnels",
				Run:     runTunnelList,
			},
			"inspect": {
				Name:  "inspect",
				Short: "Inspect tunnel traffic",
				Run:   runTunnelInspect,
			},
			"share": {
				Name:  "share",
				Short: "Create collaborative debugging session",
				Run:   runTunnelShareCmd,
			},
		},
		Run: runTunnelCmd,
	}
}

func runTunnelCmd(args []string) error {
	return runTunnel(args)
}

func runTunnelStartCmd(args []string) error {
	return runTunnelStart(args)
}

func runTunnelShareCmd(args []string) error {
	return runTunnelShare(args)
}

func runTunnelStop(args []string) error {
	fmt.Println("Stopping tunnel...")
	return nil
}

func runTunnelList(args []string) error {
	fmt.Println("Listing tunnels...")
	return nil
}

func runTunnelInspect(args []string) error {
	fmt.Println("Inspecting tunnel traffic...")
	return nil
}

// CertCommand handles certificate management
var CertCommand *Command

func init() {
	CertCommand = &Command{
		Name:    "certificate",
		Aliases: []string{"cert"},
		Short:   "Manage certificates",
		Long:    "Manage TLS certificates including listing, renewing, revoking, and viewing certificate information.",
		Usage:   "gotunnel cert <list|renew|revoke|info> [options]",
		Subcommands: map[string]*Command{
			"list": {
				Name:  "list",
				Short: "List all certificates",
				Run:   runCertList,
			},
			"renew": {
				Name:  "renew",
				Short: "Renew a certificate",
				Run:   runCertRenew,
			},
			"revoke": {
				Name:  "revoke",
				Short: "Revoke a certificate",
				Run:   runCertRevoke,
			},
			"info": {
				Name:  "info",
				Short: "Show certificate information",
				Run:   runCertInfo,
			},
		},
		Run: runCertCmd,
	}
}

func runCertCmd(args []string) error {
	if len(args) == 0 {
		printCommandHelp(CertCommand)
		return nil
	}
	return fmt.Errorf("unknown cert subcommand: %s", args[0])
}

func runCertList(args []string) error {
	fmt.Println("Listing certificates...")
	return nil
}

func runCertRenew(args []string) error {
	fmt.Println("Renewing certificate...")
	return nil
}

func runCertRevoke(args []string) error {
	fmt.Println("Revoking certificate...")
	return nil
}

func runCertInfo(args []string) error {
	fmt.Println("Showing certificate info...")
	return nil
}

// DNSCommand handles DNS management
var DNSCommand *Command

func init() {
	DNSCommand = &Command{
		Name:  "dns",
		Short: "Manage DNS providers",
		Long:  "Manage DNS providers and records for ACME challenge validation and domain management.",
		Usage: "gotunnel dns <providers|records|test> [options]",
		Subcommands: map[string]*Command{
			"providers": {
				Name:  "providers",
				Short: "List DNS providers",
				Run:   runDNSProviders,
			},
			"records": {
				Name:  "records",
				Short: "List DNS records",
				Run:   runDNSRecords,
			},
			"test": {
				Name:  "test",
				Short: "Test DNS propagation",
				Run:   runDNSTest,
			},
		},
		Run: runDNSCmd,
	}
}

func runDNSCmd(args []string) error {
	if len(args) == 0 {
		printCommandHelp(DNSCommand)
		return nil
	}
	return fmt.Errorf("unknown dns subcommand: %s", args[0])
}

func runDNSProviders(args []string) error {
	fmt.Println("Listing DNS providers...")
	return nil
}

func runDNSRecords(args []string) error {
	fmt.Println("Listing DNS records...")
	return nil
}

func runDNSTest(args []string) error {
	fmt.Println("Testing DNS propagation...")
	return nil
}

// SessionCommand handles collaborative sessions
var SessionCommand *Command

func init() {
	SessionCommand = &Command{
		Name:    "session",
		Aliases: []string{"sess"},
		Short:   "Manage collaborative sessions",
		Long:    "Manage collaborative debugging sessions including listing, joining, annotating, and replaying requests.",
		Usage:   "gotunnel session <list|join|annotate|replay> [options]",
		Subcommands: map[string]*Command{
			"list": {
				Name:  "list",
				Short: "List active sessions",
				Run:   runSessionList,
			},
			"join": {
				Name:  "join",
				Short: "Join a collaborative session",
				Run:   runSessionJoin,
			},
			"annotate": {
				Name:  "annotate",
				Short: "Add annotation to a request",
				Run:   runSessionAnnotate,
			},
			"replay": {
				Name:  "replay",
				Short: "Replay a request",
				Run:   runSessionReplay,
			},
		},
		Run: runSessionCmd,
	}
}

func runSessionCmd(args []string) error {
	if len(args) == 0 {
		printCommandHelp(SessionCommand)
		return nil
	}
	return fmt.Errorf("unknown session subcommand: %s", args[0])
}

func runSessionList(args []string) error {
	fmt.Println("Listing sessions...")
	return nil
}

func runSessionJoin(args []string) error {
	fmt.Println("Joining session...")
	return nil
}

func runSessionAnnotate(args []string) error {
	fmt.Println("Adding annotation...")
	return nil
}

func runSessionReplay(args []string) error {
	fmt.Println("Replaying request...")
	return nil
}

// SecurityCommand handles security policies
var SecurityCommand *Command

func init() {
	SecurityCommand = &Command{
		Name:    "security",
		Aliases: []string{"sec"},
		Short:   "Manage security policies",
		Long:    "Manage security policies including network, TLS, rate limiting, and audit settings.",
		Usage:   "gotunnel security <policies|network|tls|rate-limit|audit> [options]",
		Subcommands: map[string]*Command{
			"policies": {
				Name:  "policies",
				Short: "List security policies",
				Run:   runSecurityPolicies,
			},
			"network": {
				Name:  "network",
				Short: "Show network security",
				Run:   runSecurityNetwork,
			},
			"tls": {
				Name:  "tls",
				Short: "Show TLS configuration",
				Run:   runSecurityTLS,
			},
			"rate-limit": {
				Name:  "rate-limit",
				Short: "Show rate limit status",
				Run:   runSecurityRateLimit,
			},
			"audit": {
				Name:  "audit",
				Short: "Show audit log",
				Run:   runSecurityAudit,
			},
		},
		Run: runSecurityCmd,
	}
}

func runSecurityCmd(args []string) error {
	if len(args) == 0 {
		printCommandHelp(SecurityCommand)
		return nil
	}
	return fmt.Errorf("unknown security subcommand: %s", args[0])
}

func runSecurityPolicies(args []string) error {
	fmt.Println("Listing security policies...")
	return nil
}

func runSecurityNetwork(args []string) error {
	fmt.Println("Showing network security...")
	return nil
}

func runSecurityTLS(args []string) error {
	fmt.Println("Showing TLS configuration...")
	return nil
}

func runSecurityRateLimit(args []string) error {
	fmt.Println("Showing rate limit status...")
	return nil
}

func runSecurityAudit(args []string) error {
	fmt.Println("Showing audit log...")
	return nil
}

// CompletionCommand handles shell completion
var CompletionCommand = &Command{
	Name:  "completion",
	Short: "Generate shell completion scripts",
	Long:  "Generate shell completion scripts for bash, zsh, or fish shells.",
	Usage: "gotunnel completion <bash|zsh|fish>",
	Run:   runCompletion,
}

func runCompletion(args []string) error {
	if len(args) == 0 {
		fmt.Println("Usage: gotunnel completion <bash|zsh|fish>")
		return nil
	}

	shell := ShellType(args[0])
	return GenerateCompletion(shell)
}

// VersionCommand shows version information
var VersionCommand = &Command{
	Name:    "version",
	Aliases: []string{"ver"},
	Short:   "Show version information",
	Usage:   "gotunnel version",
	Run:     runVersionCmd,
}

func runVersionCmd(args []string) error {
	return runVersion(args)
}

func runVersion(args []string) error {
	fmt.Println("GoTunnel v1.0.0")
	fmt.Println("Enterprise-grade tunneling platform")
	return nil
}

// HelpCommand shows help
var HelpCommand = &Command{
	Name:    "help",
	Aliases: []string{"h"},
	Short:   "Show help for a command",
	Usage:   "gotunnel help [command]",
	Run:     runHelpCmd,
}

func runHelpCmd(args []string) error {
	if len(args) == 0 {
		printUsage()
		return nil
	}

	cmdName := args[0]
	cmd, ok := RootCommand.Subcommands[cmdName]
	if !ok {
		return fmt.Errorf("unknown command %q", cmdName)
	}

	printCommandHelp(cmd)
	return nil
}

func runMan(args []string) error {
	if len(args) == 0 {
		fmt.Println(GenerateManPage())
		return nil
	}

	command := args[0]
	fmt.Println(GenerateCommandManPage(command))
	return nil
}

// FindCommand finds a command by name or alias
func FindCommand(name string) *Command {
	for _, cmd := range RootCommand.Subcommands {
		if cmd.Name == name {
			return cmd
		}
		for _, alias := range cmd.Aliases {
			if alias == name {
				return cmd
			}
		}
	}
	return nil
}

// ListCommands returns a list of all command names
func ListCommands() []string {
	var commands []string
	for name := range RootCommand.Subcommands {
		commands = append(commands, name)
	}
	return commands
}

// SuggestCommand suggests a similar command
func SuggestCommand(input string) string {
	commands := ListCommands()
	for _, cmd := range commands {
		if strings.HasPrefix(cmd, input) || strings.Contains(cmd, input) {
			return cmd
		}
	}
	return ""
}
