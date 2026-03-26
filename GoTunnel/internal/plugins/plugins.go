package plugins

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"gotunnel/internal/protocol"
)

type Manifest struct {
	Plugins []PluginSpec `json:"plugins" yaml:"plugins"`
}

type PluginSpec struct {
	Name    string            `json:"name" yaml:"name"`
	Type    string            `json:"type" yaml:"type"`
	Enabled bool              `json:"enabled" yaml:"enabled"`
	Config  map[string]string `json:"config" yaml:"config"`
}

type Runtime struct {
	specs []PluginSpec
}

func Load(path string) (*Runtime, error) {
	if strings.TrimSpace(path) == "" {
		return &Runtime{}, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read plugin config: %w", err)
	}
	var manifest Manifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil, fmt.Errorf("parse plugin config: %w", err)
	}
	return &Runtime{specs: manifest.Plugins}, nil
}

func (r *Runtime) FeatureNames() []string {
	out := []string{}
	for _, spec := range r.specs {
		if spec.Enabled {
			out = append(out, spec.Name)
		}
	}
	return out
}

func (r *Runtime) AllowSharing(lease protocol.Lease) bool {
	allowed := lease.AllowSharing
	for _, spec := range r.specs {
		if !spec.Enabled {
			continue
		}
		switch spec.Name {
		case "production_guard":
			if lease.Production && spec.Config["allow_production_sharing"] != "true" {
				allowed = false
			}
		}
	}
	return allowed
}

func (r *Runtime) EnrichLabels(labels []string) []string {
	out := append([]string(nil), labels...)
	for _, spec := range r.specs {
		if !spec.Enabled {
			continue
		}
		switch spec.Name {
		case "audit_labels":
			if label := strings.TrimSpace(spec.Config["label"]); label != "" {
				out = append(out, label)
			}
		}
	}
	return out
}

func (r *Runtime) EnrichEventPayload(payload map[string]interface{}) map[string]interface{} {
	if payload == nil {
		payload = map[string]interface{}{}
	}
	for _, spec := range r.specs {
		if !spec.Enabled {
			continue
		}
		switch spec.Name {
		case "audit_labels":
			if label := strings.TrimSpace(spec.Config["label"]); label != "" {
				payload["plugin_label"] = label
			}
		}
	}
	return payload
}
