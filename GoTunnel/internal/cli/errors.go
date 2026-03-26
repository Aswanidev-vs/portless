package cli

import (
	"errors"
	"fmt"
	"strings"
)

// Error categories for structured error handling
const (
	ErrCategoryConfig     = "CONFIG"
	ErrCategoryAuth       = "AUTH"
	ErrCategoryNetwork    = "NETWORK"
	ErrCategoryCert       = "CERTIFICATE"
	ErrCategoryDNS        = "DNS"
	ErrCategoryTunnel     = "TUNNEL"
	ErrCategoryInternal   = "INTERNAL"
	ErrCategoryValidation = "VALIDATION"
)

// CLIError represents a structured CLI error with actionable information
type CLIError struct {
	Category   string
	Code       string
	Message    string
	Suggestion string
	Details    map[string]interface{}
	WrappedErr error
}

// Error implements the error interface
func (e *CLIError) Error() string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("[%s] %s", e.Category, e.Message))
	if e.Suggestion != "" {
		sb.WriteString(fmt.Sprintf("\n\nSuggestion: %s", e.Suggestion))
	}
	if e.WrappedErr != nil {
		sb.WriteString(fmt.Sprintf("\n\nUnderlying error: %v", e.WrappedErr))
	}
	return sb.String()
}

// Unwrap returns the wrapped error
func (e *CLIError) Unwrap() error {
	return e.WrappedErr
}

// NewConfigError creates a configuration-related error
func NewConfigError(message, suggestion string, err error) *CLIError {
	return &CLIError{
		Category:   ErrCategoryConfig,
		Code:       "CONFIG_ERROR",
		Message:    message,
		Suggestion: suggestion,
		WrappedErr: err,
	}
}

// NewAuthError creates an authentication-related error
func NewAuthError(message, suggestion string, err error) *CLIError {
	return &CLIError{
		Category:   ErrCategoryAuth,
		Code:       "AUTH_ERROR",
		Message:    message,
		Suggestion: suggestion,
		WrappedErr: err,
	}
}

// NewNetworkError creates a network-related error
func NewNetworkError(message, suggestion string, err error) *CLIError {
	return &CLIError{
		Category:   ErrCategoryNetwork,
		Code:       "NETWORK_ERROR",
		Message:    message,
		Suggestion: suggestion,
		WrappedErr: err,
	}
}

// NewCertError creates a certificate-related error
func NewCertError(message, suggestion string, err error) *CLIError {
	return &CLIError{
		Category:   ErrCategoryCert,
		Code:       "CERT_ERROR",
		Message:    message,
		Suggestion: suggestion,
		WrappedErr: err,
	}
}

// NewDNSError creates a DNS-related error
func NewDNSError(message, suggestion string, err error) *CLIError {
	return &CLIError{
		Category:   ErrCategoryDNS,
		Code:       "DNS_ERROR",
		Message:    message,
		Suggestion: suggestion,
		WrappedErr: err,
	}
}

// NewTunnelError creates a tunnel-related error
func NewTunnelError(message, suggestion string, err error) *CLIError {
	return &CLIError{
		Category:   ErrCategoryTunnel,
		Code:       "TUNNEL_ERROR",
		Message:    message,
		Suggestion: suggestion,
		WrappedErr: err,
	}
}

// NewValidationError creates a validation-related error
func NewValidationError(message, suggestion string, err error) *CLIError {
	return &CLIError{
		Category:   ErrCategoryValidation,
		Code:       "VALIDATION_ERROR",
		Message:    message,
		Suggestion: suggestion,
		WrappedErr: err,
	}
}

// Common error constructors with predefined suggestions

// ErrTokenNotFound creates an error for missing authentication token
func ErrTokenNotFound() *CLIError {
	return NewAuthError(
		"No authentication token found",
		"Set the GOTUNNEL_TOKEN environment variable, use 'gotunnel login --token <token>', or pass --token flag",
		nil,
	)
}

// ErrConfigNotFound creates an error for missing configuration file
func ErrConfigNotFound(path string) *CLIError {
	return NewConfigError(
		fmt.Sprintf("Configuration file not found: %s", path),
		"Run 'gotunnel config init' to create a new configuration file, or specify a different path with --config",
		nil,
	)
}

// ErrInvalidConfig creates an error for invalid configuration
func ErrInvalidConfig(reason string, err error) *CLIError {
	return NewConfigError(
		fmt.Sprintf("Invalid configuration: %s", reason),
		"Run 'gotunnel config validate' to check your configuration file for errors",
		err,
	)
}

// ErrConnectionFailed creates an error for connection failures
func ErrConnectionFailed(host string, err error) *CLIError {
	return NewNetworkError(
		fmt.Sprintf("Failed to connect to %s", host),
		"Check your network connection, verify the host is reachable, and ensure firewall rules allow the connection",
		err,
	)
}

// ErrCertExpired creates an error for expired certificates
func ErrCertExpired(domain string) *CLIError {
	return NewCertError(
		fmt.Sprintf("Certificate for %s has expired", domain),
		"Run 'gotunnel cert renew --domain "+domain+"' to renew the certificate, or check ACME configuration",
		nil,
	)
}

// ErrDNSResolution creates an error for DNS resolution failures
func ErrDNSResolution(domain string, err error) *CLIError {
	return NewDNSError(
		fmt.Sprintf("Failed to resolve DNS for %s", domain),
		"Verify the domain exists, check DNS provider credentials, and ensure DNS propagation is complete",
		err,
	)
}

// ErrTunnelStart creates an error for tunnel startup failures
func ErrTunnelStart(reason string, err error) *CLIError {
	return NewTunnelError(
		fmt.Sprintf("Failed to start tunnel: %s", reason),
		"Check local service is running, verify configuration, and ensure network connectivity to relay",
		err,
	)
}

// ErrValidation creates an error for validation failures
func ErrValidation(field, reason string) *CLIError {
	return NewValidationError(
		fmt.Sprintf("Validation failed for %s: %s", field, reason),
		"Check the value meets the required format and constraints",
		nil,
	)
}

// IsCLIError checks if an error is a CLIError
func IsCLIError(err error) bool {
	var cliErr *CLIError
	return errors.As(err, &cliErr)
}

// GetCLIError extracts a CLIError from an error chain
func GetCLIError(err error) *CLIError {
	var cliErr *CLIError
	if errors.As(err, &cliErr) {
		return cliErr
	}
	return nil
}
