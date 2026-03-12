package router

import (
	"fmt"
	"net/url"
	"sync"
)

// Target represents where a specific domain should be routed
type Target struct {
	ServiceName string
	Port        int
	URL         *url.URL
}

// Engine holds the mapping of incoming host domains to local Target configurations
type Engine struct {
	routes map[string]*Target
	mu     sync.RWMutex
}

// NewEngine initializes an empty thread-safe routing table
func NewEngine() *Engine {
	return &Engine{
		routes: make(map[string]*Target),
	}
}

// AddRoute maps an incoming domain to a local port
func (e *Engine) AddRoute(domain, serviceName string, port int) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	targetURL, err := url.Parse(fmt.Sprintf("http://127.0.0.1:%d", port))
	if err != nil {
		return fmt.Errorf("failed to parse local proxy URL: %w", err)
	}

	e.routes[domain] = &Target{
		ServiceName: serviceName,
		Port:        port,
		URL:         targetURL,
	}
	return nil
}

// RemoveRoute deletes a mapping
func (e *Engine) RemoveRoute(domain string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	delete(e.routes, domain)
}

// GetTarget returns the URL target for a specific domain. Returns nil if not found.
func (e *Engine) GetTarget(domain string) *Target {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.routes[domain]
}

// GetAllRoutes returns a copy of the current routing map
func (e *Engine) GetAllRoutes() map[string]*Target {
	e.mu.RLock()
	defer e.mu.RUnlock()

	copyOfRoutes := make(map[string]*Target)
	for k, v := range e.routes {
		copyOfRoutes[k] = v
	}
	return copyOfRoutes
}
