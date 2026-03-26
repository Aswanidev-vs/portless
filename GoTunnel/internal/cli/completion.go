package cli

import (
	"fmt"
	"os"
)

// ShellType represents supported shell types
type ShellType string

const (
	ShellBash ShellType = "bash"
	ShellZsh  ShellType = "zsh"
	ShellFish ShellType = "fish"
)

// GenerateCompletion generates shell completion scripts
func GenerateCompletion(shell ShellType) error {
	switch shell {
	case ShellBash:
		return generateBashCompletion()
	case ShellZsh:
		return generateZshCompletion()
	case ShellFish:
		return generateFishCompletion()
	default:
		return fmt.Errorf("unsupported shell: %s (supported: bash, zsh, fish)", shell)
	}
}

func generateBashCompletion() error {
	script := `#!/bin/bash
# Bash completion for gotunnel

_gotunnel_completion() {
    local cur prev commands
    COMPREPLY=()
    cur="${COMP_WORDS[COMP_CWORD]}"
    prev="${COMP_WORDS[COMP_CWORD-1]}"

    commands="login broker relay config tunnel cert dns session security help version"

    # Global options
    local global_opts="--config --log-level --log-format --json --dry-run --verbose --quiet --no-color --token --profile --timeout --help"

    # Complete commands
    if [[ ${COMP_CWORD} -eq 1 ]]; then
        COMPREPLY=( $(compgen -W "${commands}" -- ${cur}) )
        return 0
    fi

    # Complete subcommands
    case "${COMP_WORDS[1]}" in
        tunnel)
            local subcmds="start stop list inspect share"
            if [[ ${COMP_CWORD} -eq 2 ]]; then
                COMPREPLY=( $(compgen -W "${subcmds}" -- ${cur}) )
                return 0
            fi
            ;;
        config)
            local subcmds="validate init show"
            if [[ ${COMP_CWORD} -eq 2 ]]; then
                COMPREPLY=( $(compgen -W "${subcmds}" -- ${cur}) )
                return 0
            fi
            ;;
        cert)
            local subcmds="list renew revoke info"
            if [[ ${COMP_CWORD} -eq 2 ]]; then
                COMPREPLY=( $(compgen -W "${subcmds}" -- ${cur}) )
                return 0
            fi
            ;;
        dns)
            local subcmds="providers records test"
            if [[ ${COMP_CWORD} -eq 2 ]]; then
                COMPREPLY=( $(compgen -W "${subcmds}" -- ${cur}) )
                return 0
            fi
            ;;
        session)
            local subcmds="list join annotate replay"
            if [[ ${COMP_CWORD} -eq 2 ]]; then
                COMPREPLY=( $(compgen -W "${subcmds}" -- ${cur}) )
                return 0
            fi
            ;;
        security)
            local subcmds="policies network tls rate-limit audit"
            if [[ ${COMP_CWORD} -eq 2 ]]; then
                COMPREPLY=( $(compgen -W "${subcmds}" -- ${cur}) )
                return 0
            fi
            ;;
        relay)
            local subcmds="status health"
            if [[ ${COMP_CWORD} -eq 2 ]]; then
                COMPREPLY=( $(compgen -W "${subcmds}" -- ${cur}) )
                return 0
            fi
            ;;
    esac

    # Complete flags
    if [[ ${cur} == -* ]]; then
        COMPREPLY=( $(compgen -W "${global_opts}" -- ${cur}) )
        return 0
    fi
}

complete -F _gotunnel_completion gotunnel
`
	fmt.Print(script)
	return nil
}

func generateZshCompletion() error {
	script := `#compdef gotunnel

# Zsh completion for gotunnel

_gotunnel() {
    local -a commands
    commands=(
        'login:Authenticate with GoTunnel'
        'broker:Start broker server'
        'relay:Start relay server'
        'config:Manage configuration'
        'tunnel:Manage tunnels'
        'cert:Manage certificates'
        'dns:Manage DNS providers'
        'session:Manage collaborative sessions'
        'security:Manage security policies'
        'help:Show help'
        'version:Show version'
    )

    _arguments -C \
        '--config[Path to configuration file]:file:_files' \
        '--log-level[Log level]:level:(trace debug info warn error fatal)' \
        '--log-format[Log format]:format:(text json)' \
        '--json[Output in JSON format]' \
        '--dry-run[Show what would be done without executing]' \
        '--verbose[Enable verbose output]' \
        '--quiet[Suppress non-error output]' \
        '--no-color[Disable colored output]' \
        '--token[Authentication token]:token:' \
        '--profile[Configuration profile]:profile:(dev staging prod)' \
        '--timeout[Command timeout]:timeout:' \
        '--help[Show help]' \
        '1:command:->command' \
        '*::arg:->args'

    case $state in
        command)
            _describe -t commands 'gotunnel commands' commands
            ;;
        args)
            case ${words[1]} in
                tunnel)
                    _arguments \
                        '1:subcommand:(start stop list inspect share)'
                    ;;
                config)
                    _arguments \
                        '1:subcommand:(validate init show)'
                    ;;
                cert)
                    _arguments \
                        '1:subcommand:(list renew revoke info)'
                    ;;
                dns)
                    _arguments \
                        '1:subcommand:(providers records test)'
                    ;;
                session)
                    _arguments \
                        '1:subcommand:(list join annotate replay)'
                    ;;
                security)
                    _arguments \
                        '1:subcommand:(policies network tls rate-limit audit)'
                    ;;
                relay)
                    _arguments \
                        '1:subcommand:(status health)'
                    ;;
            esac
            ;;
    esac
}

_gotunnel "$@"
`
	fmt.Print(script)
	return nil
}

func generateFishCompletion() error {
	script := `# Fish completion for gotunnel

# Disable file completion by default
complete -c gotunnel -f

# Global options
complete -c gotunnel -l config -d 'Path to configuration file' -r -F
complete -c gotunnel -l log-level -d 'Log level' -a 'trace debug info warn error fatal'
complete -c gotunnel -l log-format -d 'Log format' -a 'text json'
complete -c gotunnel -l json -d 'Output in JSON format'
complete -c gotunnel -l dry-run -d 'Show what would be done without executing'
complete -c gotunnel -l verbose -d 'Enable verbose output'
complete -c gotunnel -l quiet -d 'Suppress non-error output'
complete -c gotunnel -l no-color -d 'Disable colored output'
complete -c gotunnel -l token -d 'Authentication token' -r
complete -c gotunnel -l profile -d 'Configuration profile' -a 'dev staging prod'
complete -c gotunnel -l timeout -d 'Command timeout' -r
complete -c gotunnel -l help -d 'Show help'

# Commands
complete -c gotunnel -n '__fish_use_subcommand' -a login -d 'Authenticate with GoTunnel'
complete -c gotunnel -n '__fish_use_subcommand' -a broker -d 'Start broker server'
complete -c gotunnel -n '__fish_use_subcommand' -a relay -d 'Start relay server'
complete -c gotunnel -n '__fish_use_subcommand' -a config -d 'Manage configuration'
complete -c gotunnel -n '__fish_use_subcommand' -a tunnel -d 'Manage tunnels'
complete -c gotunnel -n '__fish_use_subcommand' -a cert -d 'Manage certificates'
complete -c gotunnel -n '__fish_use_subcommand' -a dns -d 'Manage DNS providers'
complete -c gotunnel -n '__fish_use_subcommand' -a session -d 'Manage collaborative sessions'
complete -c gotunnel -n '__fish_use_subcommand' -a security -d 'Manage security policies'
complete -c gotunnel -n '__fish_use_subcommand' -a help -d 'Show help'
complete -c gotunnel -n '__fish_use_subcommand' -a version -d 'Show version'

# Subcommands
complete -c gotunnel -n '__fish_seen_subcommand_from tunnel' -a 'start stop list inspect share'
complete -c gotunnel -n '__fish_seen_subcommand_from config' -a 'validate init show'
complete -c gotunnel -n '__fish_seen_subcommand_from cert' -a 'list renew revoke info'
complete -c gotunnel -n '__fish_seen_subcommand_from dns' -a 'providers records test'
complete -c gotunnel -n '__fish_seen_subcommand_from session' -a 'list join annotate replay'
complete -c gotunnel -n '__fish_seen_subcommand_from security' -a 'policies network tls rate-limit audit'
complete -c gotunnel -n '__fish_seen_subcommand_from relay' -a 'status health'
`
	fmt.Print(script)
	return nil
}

// InstallCompletion prints instructions for installing completion
func InstallCompletion(shell ShellType) {
	instructions := map[ShellType]string{
		ShellBash: `To install bash completion:

  # Add to ~/.bashrc:
  eval "$(gotunnel completion bash)"

  # Or save to file:
  gotunnel completion bash > /etc/bash_completion.d/gotunnel`,
		ShellZsh: `To install zsh completion:

  # Add to ~/.zshrc:
  eval "$(gotunnel completion zsh)"

  # Or save to file:
  gotunnel completion zsh > ~/.zsh/completions/_gotunnel`,
		ShellFish: `To install fish completion:

  # Save to completions directory:
  gotunnel completion fish > ~/.config/fish/completions/gotunnel.fish`,
	}

	if instr, ok := instructions[shell]; ok {
		fmt.Fprintln(os.Stderr, instr)
	}
}
