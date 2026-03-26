package acme

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"

	"golang.org/x/crypto/acme/autocert"
)

type Manager struct {
	Auto  *autocert.Manager
	Hosts map[string]struct{}
}

func New(cacheDir string, hosts []string, email string) *Manager {
	hostMap := make(map[string]struct{}, len(hosts))
	for _, host := range hosts {
		host = strings.TrimSpace(host)
		if host != "" {
			hostMap[host] = struct{}{}
		}
	}
	m := &autocert.Manager{
		Prompt: autocert.AcceptTOS,
		Email:  email,
	}
	if cacheDir != "" {
		m.Cache = autocert.DirCache(filepath.Clean(cacheDir))
	}
	if len(hostMap) > 0 {
		m.HostPolicy = func(_ context.Context, host string) error {
			if _, ok := hostMap[host]; ok {
				return nil
			}
			return fmt.Errorf("host %s is not configured for autocert", host)
		}
	}
	return &Manager{Auto: m, Hosts: hostMap}
}

func (m *Manager) TLSConfig() *tls.Config {
	return m.Auto.TLSConfig()
}

func (m *Manager) HTTPHandler(next http.Handler) http.Handler {
	return m.Auto.HTTPHandler(next)
}
