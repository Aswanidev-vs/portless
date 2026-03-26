package config

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Version int         `yaml:"version"`
	Auth    AuthConfig  `yaml:"auth"`
	Relay   RelayConfig `yaml:"relay"`
	Tunnels []Tunnel    `yaml:"tunnels"`
}

type AuthConfig struct {
	Token string `yaml:"token"`
}

type RelayConfig struct {
	BrokerURL string `yaml:"broker_url"`
	Region    string `yaml:"region"`
	PluginConfig string `yaml:"plugin_config"`
}

type Tunnel struct {
	Name       string `yaml:"name"`
	Protocol   string `yaml:"protocol"`
	LocalURL   string `yaml:"local_url"`
	Region     string `yaml:"region"`
	Subdomain  string `yaml:"subdomain"`
	Inspect    bool   `yaml:"inspect"`
	HTTPS      string `yaml:"https"`
	WebhookURL string `yaml:"webhook_url"`
	CustomDomain string `yaml:"custom_domain"`
	Production bool   `yaml:"production"`
	AllowSharing *bool `yaml:"allow_sharing"`
	Labels     []string `yaml:"labels"`
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	expandEnv(&cfg)
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func expandEnv(cfg *Config) {
	cfg.Auth.Token = expandValue(cfg.Auth.Token)
	cfg.Relay.BrokerURL = expandValue(cfg.Relay.BrokerURL)
	cfg.Relay.PluginConfig = expandValue(cfg.Relay.PluginConfig)
	for i := range cfg.Tunnels {
		cfg.Tunnels[i].LocalURL = expandValue(cfg.Tunnels[i].LocalURL)
		cfg.Tunnels[i].WebhookURL = expandValue(cfg.Tunnels[i].WebhookURL)
		cfg.Tunnels[i].CustomDomain = expandValue(cfg.Tunnels[i].CustomDomain)
	}
}

func expandValue(v string) string {
	if strings.HasPrefix(v, "env:") {
		return os.Getenv(strings.TrimPrefix(v, "env:"))
	}
	return v
}

func (c *Config) Validate() error {
	if c.Version == 0 {
		c.Version = 1
	}
	if c.Version != 1 {
		return fmt.Errorf("unsupported config version %d", c.Version)
	}
	if strings.TrimSpace(c.Relay.BrokerURL) == "" {
		return errors.New("relay.broker_url is required")
	}
	if len(c.Tunnels) == 0 {
		return errors.New("at least one tunnel is required")
	}
	for i := range c.Tunnels {
		tunnel := c.Tunnels[i]
		if strings.TrimSpace(tunnel.Name) == "" {
			return fmt.Errorf("tunnels[%d].name is required", i)
		}
		if strings.TrimSpace(tunnel.Protocol) == "" {
			return fmt.Errorf("tunnels[%d].protocol is required", i)
		}
		if tunnel.Protocol != "http" && tunnel.Protocol != "tcp" && tunnel.Protocol != "udp" {
			return fmt.Errorf("tunnels[%d].protocol=%q is not supported; use http, tcp, or udp", i, tunnel.Protocol)
		}
		if strings.TrimSpace(tunnel.LocalURL) == "" {
			return fmt.Errorf("tunnels[%d].local_url is required", i)
		}
		if tunnel.HTTPS == "" {
			c.Tunnels[i].HTTPS = "auto"
		}
		if c.Tunnels[i].Region == "" {
			c.Tunnels[i].Region = c.Relay.Region
		}
		if c.Tunnels[i].AllowSharing == nil {
			defaultAllow := !tunnel.Production
			c.Tunnels[i].AllowSharing = &defaultAllow
		}
	}
	return nil
}
