package cli

import (
	"flag"
	"fmt"
	"os"
	"strings"
)

// GlobalOptions holds options available to all commands
type GlobalOptions struct {
	ConfigPath string
	LogLevel   string
	LogFormat  string
	JSONOutput bool
	DryRun     bool
	Verbose    bool
	Quiet      bool
	NoColor    bool
	Token      string
	Profile    string
	Timeout    string
}

// ParseGlobalOptions extracts global options from command-line arguments
func ParseGlobalOptions() (*GlobalOptions, []string) {
	opts := &GlobalOptions{}
	var remainingArgs []string

	args := os.Args[1:]
	for i := 0; i < len(args); i++ {
		arg := args[i]

		switch {
		case arg == "--config" || arg == "-c":
			if i+1 < len(args) {
				opts.ConfigPath = args[i+1]
				i++
			}
		case strings.HasPrefix(arg, "--config="):
			opts.ConfigPath = strings.TrimPrefix(arg, "--config=")
		case arg == "--log-level" || arg == "-l":
			if i+1 < len(args) {
				opts.LogLevel = args[i+1]
				i++
			}
		case strings.HasPrefix(arg, "--log-level="):
			opts.LogLevel = strings.TrimPrefix(arg, "--log-level=")
		case arg == "--log-format":
			if i+1 < len(args) {
				opts.LogFormat = args[i+1]
				i++
			}
		case strings.HasPrefix(arg, "--log-format="):
			opts.LogFormat = strings.TrimPrefix(arg, "--log-format=")
		case arg == "--json":
			opts.JSONOutput = true
		case arg == "--dry-run":
			opts.DryRun = true
		case arg == "--verbose" || arg == "-v":
			opts.Verbose = true
		case arg == "--quiet" || arg == "-q":
			opts.Quiet = true
		case arg == "--no-color":
			opts.NoColor = true
		case arg == "--token" || arg == "-t":
			if i+1 < len(args) {
				opts.Token = args[i+1]
				i++
			}
		case strings.HasPrefix(arg, "--token="):
			opts.Token = strings.TrimPrefix(arg, "--token=")
		case arg == "--profile" || arg == "-p":
			if i+1 < len(args) {
				opts.Profile = args[i+1]
				i++
			}
		case strings.HasPrefix(arg, "--profile="):
			opts.Profile = strings.TrimPrefix(arg, "--profile=")
		case arg == "--timeout":
			if i+1 < len(args) {
				opts.Timeout = args[i+1]
				i++
			}
		case strings.HasPrefix(arg, "--timeout="):
			opts.Timeout = strings.TrimPrefix(arg, "--timeout=")
		case arg == "--help" || arg == "-h":
			remainingArgs = append(remainingArgs, arg)
		default:
			remainingArgs = append(remainingArgs, arg)
		}
	}

	return opts, remainingArgs
}

// ApplyGlobalOptions applies global options to the runtime
func ApplyGlobalOptions(opts *GlobalOptions) {
	// Configure logging
	if opts.LogLevel != "" {
		level, err := ParseLogLevel(opts.LogLevel)
		if err == nil {
			SetGlobalLogLevel(level)
		}
	}

	// Configure verbose mode
	if opts.Verbose {
		SetGlobalLogLevel(LevelDebug)
	}

	// Configure quiet mode
	if opts.Quiet {
		SetGlobalLogLevel(LevelError)
	}

	// Configure JSON output
	if opts.JSONOutput {
		globalLogger.jsonMode = true
	}

	// Set token from flag
	if opts.Token != "" {
		os.Setenv("GOTUNNEL_TOKEN", opts.Token)
	}
}

// RegisterGlobalFlags adds global flags to a flag set
func RegisterGlobalFlags(fs *flag.FlagSet, opts *GlobalOptions) {
	fs.StringVar(&opts.ConfigPath, "config", "", "Path to configuration file")
	fs.StringVar(&opts.ConfigPath, "c", "", "Path to configuration file (shorthand)")
	fs.StringVar(&opts.LogLevel, "log-level", "", "Log level (trace, debug, info, warn, error, fatal)")
	fs.StringVar(&opts.LogLevel, "l", "", "Log level (shorthand)")
	fs.StringVar(&opts.LogFormat, "log-format", "", "Log format (text, json)")
	fs.BoolVar(&opts.JSONOutput, "json", false, "Output in JSON format")
	fs.BoolVar(&opts.DryRun, "dry-run", false, "Show what would be done without executing")
	fs.BoolVar(&opts.Verbose, "verbose", false, "Enable verbose output")
	fs.BoolVar(&opts.Verbose, "v", false, "Enable verbose output (shorthand)")
	fs.BoolVar(&opts.Quiet, "quiet", false, "Suppress non-error output")
	fs.BoolVar(&opts.Quiet, "q", false, "Suppress non-error output (shorthand)")
	fs.BoolVar(&opts.NoColor, "no-color", false, "Disable colored output")
	fs.StringVar(&opts.Token, "token", "", "Authentication token")
	fs.StringVar(&opts.Token, "t", "", "Authentication token (shorthand)")
	fs.StringVar(&opts.Profile, "profile", "", "Configuration profile (dev, staging, prod)")
	fs.StringVar(&opts.Profile, "p", "", "Configuration profile (shorthand)")
	fs.StringVar(&opts.Timeout, "timeout", "", "Command timeout (e.g., 30s, 5m)")
}

// PrintGlobalHelp prints help for global options
func PrintGlobalHelp() {
	fmt.Println(`GLOBAL OPTIONS:
  -c, --config PATH      Path to configuration file (default: gotunnel.yaml)
  -l, --log-level LEVEL  Log level: trace, debug, info, warn, error, fatal (default: info)
      --log-format FMT   Log format: text, json (default: text)
      --json             Output in JSON format
      --dry-run          Show what would be done without executing
  -v, --verbose          Enable verbose output
  -q, --quiet            Suppress non-error output
      --no-color         Disable colored output
  -t, --token TOKEN      Authentication token
  -p, --profile PROFILE  Configuration profile: dev, staging, prod
      --timeout DURATION Command timeout (e.g., 30s, 5m)
  -h, --help             Show help`)
}
