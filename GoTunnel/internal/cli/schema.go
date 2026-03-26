package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// ConfigSchema defines the JSON schema for configuration validation
var ConfigSchema = map[string]interface{}{
	"$schema":              "http://json-schema.org/draft-07/schema#",
	"title":                "GoTunnel Configuration",
	"description":          "Configuration schema for GoTunnel enterprise tunneling platform",
	"type":                 "object",
	"additionalProperties": false,
	"properties": map[string]interface{}{
		"version": map[string]interface{}{
			"type":        "integer",
			"description": "Configuration version",
			"enum":        []int{1},
		},
		"auth": map[string]interface{}{
			"type":                 "object",
			"description":          "Authentication configuration",
			"additionalProperties": false,
			"properties": map[string]interface{}{
				"token": map[string]interface{}{
					"type":        "string",
					"description": "Authentication token (supports env: prefix for environment variables)",
				},
				"session_timeout": map[string]interface{}{
					"type":        "string",
					"description": "Session timeout duration (e.g., 24h, 30m)",
					"pattern":     "^[0-9]+(h|m|s)$",
				},
				"max_concurrent_sessions": map[string]interface{}{
					"type":        "integer",
					"description": "Maximum concurrent sessions per user",
					"minimum":     1,
					"maximum":     100,
				},
				"require_mfa": map[string]interface{}{
					"type":        "boolean",
					"description": "Require multi-factor authentication",
				},
			},
		},
		"relay": map[string]interface{}{
			"type":                 "object",
			"description":          "Relay server configuration",
			"additionalProperties": false,
			"required":             []string{"broker_url"},
			"properties": map[string]interface{}{
				"broker_url": map[string]interface{}{
					"type":        "string",
					"description": "Broker server URL",
					"format":      "uri",
				},
				"region": map[string]interface{}{
					"type":        "string",
					"description": "Preferred relay region",
				},
			},
		},
		"tunnels": map[string]interface{}{
			"type":        "array",
			"description": "Tunnel definitions",
			"items": map[string]interface{}{
				"type":                 "object",
				"additionalProperties": false,
				"required":             []string{"name", "protocol", "local_url"},
				"properties": map[string]interface{}{
					"name": map[string]interface{}{
						"type":        "string",
						"description": "Tunnel name",
						"minLength":   1,
						"maxLength":   63,
						"pattern":     "^[a-z0-9]([a-z0-9-]*[a-z0-9])?$",
					},
					"protocol": map[string]interface{}{
						"type":        "string",
						"description": "Tunnel protocol",
						"enum":        []string{"http", "tcp", "udp"},
					},
					"local_url": map[string]interface{}{
						"type":        "string",
						"description": "Local service URL",
					},
					"subdomain": map[string]interface{}{
						"type":        "string",
						"description": "Requested subdomain",
						"pattern":     "^[a-z0-9]([a-z0-9-]*[a-z0-9])?$",
					},
					"https": map[string]interface{}{
						"type":        "string",
						"description": "HTTPS mode",
						"enum":        []string{"auto", "manual", "disabled"},
						"default":     "auto",
					},
					"inspect": map[string]interface{}{
						"type":        "boolean",
						"description": "Enable request inspection",
						"default":     true,
					},
					"production": map[string]interface{}{
						"type":        "boolean",
						"description": "Mark as production tunnel",
						"default":     false,
					},
					"allow_sharing": map[string]interface{}{
						"type":        "boolean",
						"description": "Allow collaborative sharing",
						"default":     true,
					},
				},
			},
		},
		"acme": map[string]interface{}{
			"type":                 "object",
			"description":          "ACME certificate configuration",
			"additionalProperties": false,
			"properties": map[string]interface{}{
				"email": map[string]interface{}{
					"type":        "string",
					"description": "ACME account email",
					"format":      "email",
				},
				"directory_url": map[string]interface{}{
					"type":        "string",
					"description": "ACME directory URL",
					"format":      "uri",
				},
				"renewal_window": map[string]interface{}{
					"type":        "string",
					"description": "Time before expiry to renew (e.g., 720h)",
					"pattern":     "^[0-9]+(h|m|s)$",
				},
			},
		},
		"dns": map[string]interface{}{
			"type":                 "object",
			"description":          "DNS provider configuration",
			"additionalProperties": false,
			"properties": map[string]interface{}{
				"providers": map[string]interface{}{
					"type":        "array",
					"description": "DNS provider list",
					"items": map[string]interface{}{
						"type":                 "object",
						"additionalProperties": false,
						"required":             []string{"name", "type"},
						"properties": map[string]interface{}{
							"name": map[string]interface{}{
								"type":        "string",
								"description": "Provider name",
							},
							"type": map[string]interface{}{
								"type":        "string",
								"description": "Provider type",
								"enum":        []string{"cloudflare", "route53", "gcp", "azure", "manual", "memory"},
							},
							"enabled": map[string]interface{}{
								"type":        "boolean",
								"description": "Enable provider",
								"default":     true,
							},
						},
					},
				},
			},
		},
		"security": map[string]interface{}{
			"type":                 "object",
			"description":          "Security policies",
			"additionalProperties": false,
			"properties": map[string]interface{}{
				"network": map[string]interface{}{
					"type":                 "object",
					"description":          "Network security policies",
					"additionalProperties": false,
					"properties": map[string]interface{}{
						"allowed_cidrs": map[string]interface{}{
							"type":        "array",
							"description": "Allowed CIDR ranges",
							"items": map[string]interface{}{
								"type": "string",
							},
						},
						"blocked_cidrs": map[string]interface{}{
							"type":        "array",
							"description": "Blocked CIDR ranges",
							"items": map[string]interface{}{
								"type": "string",
							},
						},
					},
				},
				"tls": map[string]interface{}{
					"type":                 "object",
					"description":          "TLS security policies",
					"additionalProperties": false,
					"properties": map[string]interface{}{
						"min_version": map[string]interface{}{
							"type":        "string",
							"description": "Minimum TLS version",
							"enum":        []string{"1.0", "1.1", "1.2", "1.3"},
						},
					},
				},
			},
		},
	},
}

// ValidateConfig validates a configuration against the schema
func ValidateConfig(configPath string) ([]string, error) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var config map[string]interface{}
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse YAML: %w", err)
	}

	var warnings []string

	// Basic validation
	if version, ok := config["version"]; ok {
		if v, ok := version.(int); !ok || v != 1 {
			warnings = append(warnings, "version should be 1")
		}
	}

	if auth, ok := config["auth"].(map[string]interface{}); ok {
		if token, ok := auth["token"].(string); ok {
			if strings.HasPrefix(token, "env:") {
				envVar := strings.TrimPrefix(token, "env:")
				if os.Getenv(envVar) == "" {
					warnings = append(warnings, fmt.Sprintf("environment variable %s is not set", envVar))
				}
			}
		}
	}

	if relay, ok := config["relay"].(map[string]interface{}); ok {
		if brokerURL, ok := relay["broker_url"].(string); ok {
			if brokerURL == "" {
				warnings = append(warnings, "relay.broker_url is empty")
			}
		}
	}

	if tunnels, ok := config["tunnels"].([]interface{}); ok {
		if len(tunnels) == 0 {
			warnings = append(warnings, "no tunnels defined")
		}
		for i, t := range tunnels {
			if tunnel, ok := t.(map[string]interface{}); ok {
				if name, ok := tunnel["name"].(string); ok {
					if name == "" {
						warnings = append(warnings, fmt.Sprintf("tunnels[%d].name is empty", i))
					}
				}
				if protocol, ok := tunnel["protocol"].(string); ok {
					if protocol != "http" && protocol != "tcp" && protocol != "udp" {
						warnings = append(warnings, fmt.Sprintf("tunnels[%d].protocol is invalid: %s", i, protocol))
					}
				}
			}
		}
	}

	return warnings, nil
}

// GetSchemaJSON returns the schema as JSON
func GetSchemaJSON() (string, error) {
	data, err := json.MarshalIndent(ConfigSchema, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// PrintSchema prints the configuration schema
func PrintSchema() error {
	schema, err := GetSchemaJSON()
	if err != nil {
		return err
	}
	fmt.Println(schema)
	return nil
}
