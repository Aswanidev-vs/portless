package ops

import (
	"context"
	"fmt"
	"runtime"
	"sync"
	"time"
)

// HealthStatus represents the health status of a component
type HealthStatus string

const (
	HealthStatusHealthy   HealthStatus = "healthy"
	HealthStatusDegraded  HealthStatus = "degraded"
	HealthStatusUnhealthy HealthStatus = "unhealthy"
	HealthStatusUnknown   HealthStatus = "unknown"
)

// ComponentHealth represents the health of a system component
type ComponentHealth struct {
	Name        string             `json:"name"`
	Status      HealthStatus       `json:"status"`
	Message     string             `json:"message,omitempty"`
	Metrics     map[string]float64 `json:"metrics,omitempty"`
	LastChecked time.Time          `json:"last_checked"`
	Latency     time.Duration      `json:"latency"`
}

// SystemHealth represents the overall system health
type SystemHealth struct {
	Status     HealthStatus               `json:"status"`
	Components map[string]ComponentHealth `json:"components"`
	Uptime     time.Duration              `json:"uptime"`
	Version    string                     `json:"version"`
	Timestamp  time.Time                  `json:"timestamp"`
}

// ReadinessProbe defines a readiness check
type ReadinessProbe struct {
	Name     string
	Check    func(ctx context.Context) error
	Timeout  time.Duration
	Interval time.Duration
}

// LivenessProbe defines a liveness check
type LivenessProbe struct {
	Name     string
	Check    func(ctx context.Context) error
	Timeout  time.Duration
	Interval time.Duration
}

// Hardener manages production hardening and operational maturity
type Hardener struct {
	mu              sync.RWMutex
	healthChecks    map[string]func(ctx context.Context) ComponentHealth
	readinessProbes map[string]ReadinessProbe
	livenessProbes  map[string]LivenessProbe
	startTime       time.Time
	version         string
	stopCh          chan struct{}
	wg              sync.WaitGroup
}

// HardenerConfig holds hardener configuration
type HardenerConfig struct {
	Version string
}

// NewHardener creates a new hardener
func NewHardener(cfg HardenerConfig) *Hardener {
	return &Hardener{
		healthChecks:    make(map[string]func(ctx context.Context) ComponentHealth),
		readinessProbes: make(map[string]ReadinessProbe),
		livenessProbes:  make(map[string]LivenessProbe),
		startTime:       time.Now(),
		version:         cfg.Version,
		stopCh:          make(chan struct{}),
	}
}

// Start starts the hardener
func (h *Hardener) Start(ctx context.Context) error {
	// Start health check worker
	h.wg.Add(1)
	go h.healthCheckWorker(ctx)

	return nil
}

// Stop stops the hardener
func (h *Hardener) Stop() {
	close(h.stopCh)
	h.wg.Wait()
}

// RegisterHealthCheck registers a health check
func (h *Hardener) RegisterHealthCheck(name string, check func(ctx context.Context) ComponentHealth) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.healthChecks[name] = check
}

// RegisterReadinessProbe registers a readiness probe
func (h *Hardener) RegisterReadinessProbe(probe ReadinessProbe) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.readinessProbes[probe.Name] = probe
}

// RegisterLivenessProbe registers a liveness probe
func (h *Hardener) RegisterLivenessProbe(probe LivenessProbe) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.livenessProbes[probe.Name] = probe
}

// GetSystemHealth returns the overall system health
func (h *Hardener) GetSystemHealth(ctx context.Context) SystemHealth {
	h.mu.RLock()
	defer h.mu.RUnlock()

	components := make(map[string]ComponentHealth)
	overallStatus := HealthStatusHealthy

	for name, check := range h.healthChecks {
		start := time.Now()
		health := check(ctx)
		health.Latency = time.Since(start)
		health.LastChecked = time.Now()
		components[name] = health

		if health.Status == HealthStatusUnhealthy {
			overallStatus = HealthStatusUnhealthy
		} else if health.Status == HealthStatusDegraded && overallStatus == HealthStatusHealthy {
			overallStatus = HealthStatusDegraded
		}
	}

	return SystemHealth{
		Status:     overallStatus,
		Components: components,
		Uptime:     time.Since(h.startTime),
		Version:    h.version,
		Timestamp:  time.Now(),
	}
}

// CheckReadiness checks all readiness probes
func (h *Hardener) CheckReadiness(ctx context.Context) map[string]error {
	h.mu.RLock()
	defer h.mu.RUnlock()

	results := make(map[string]error)
	for name, probe := range h.readinessProbes {
		probeCtx, cancel := context.WithTimeout(ctx, probe.Timeout)
		err := probe.Check(probeCtx)
		cancel()
		results[name] = err
	}
	return results
}

// CheckLiveness checks all liveness probes
func (h *Hardener) CheckLiveness(ctx context.Context) map[string]error {
	h.mu.RLock()
	defer h.mu.RUnlock()

	results := make(map[string]error)
	for name, probe := range h.livenessProbes {
		probeCtx, cancel := context.WithTimeout(ctx, probe.Timeout)
		err := probe.Check(probeCtx)
		cancel()
		results[name] = err
	}
	return results
}

// IsReady returns true if all readiness probes pass
func (h *Hardener) IsReady(ctx context.Context) bool {
	results := h.CheckReadiness(ctx)
	for _, err := range results {
		if err != nil {
			return false
		}
	}
	return true
}

// IsAlive returns true if all liveness probes pass
func (h *Hardener) IsAlive(ctx context.Context) bool {
	results := h.CheckLiveness(ctx)
	for _, err := range results {
		if err != nil {
			return false
		}
	}
	return true
}

// GetRuntimeStats returns runtime statistics
func (h *Hardener) GetRuntimeStats() RuntimeStats {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	return RuntimeStats{
		Goroutines:      runtime.NumGoroutine(),
		CPUCount:        runtime.NumCPU(),
		MemoryAlloc:     m.Alloc,
		MemoryTotal:     m.TotalAlloc,
		MemorySys:       m.Sys,
		MemoryHeapAlloc: m.HeapAlloc,
		MemoryHeapSys:   m.HeapSys,
		GCCycles:        m.NumGC,
		GCPauseTotal:    time.Duration(m.PauseTotalNs),
		Uptime:          time.Since(h.startTime),
	}
}

// RuntimeStats contains runtime statistics
type RuntimeStats struct {
	Goroutines      int           `json:"goroutines"`
	CPUCount        int           `json:"cpu_count"`
	MemoryAlloc     uint64        `json:"memory_alloc"`
	MemoryTotal     uint64        `json:"memory_total"`
	MemorySys       uint64        `json:"memory_sys"`
	MemoryHeapAlloc uint64        `json:"memory_heap_alloc"`
	MemoryHeapSys   uint64        `json:"memory_heap_sys"`
	GCCycles        uint32        `json:"gc_cycles"`
	GCPauseTotal    time.Duration `json:"gc_pause_total"`
	Uptime          time.Duration `json:"uptime"`
}

func (h *Hardener) healthCheckWorker(ctx context.Context) {
	defer h.wg.Done()

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-h.stopCh:
			return
		case <-ticker.C:
			// Run health checks
			h.GetSystemHealth(ctx)
		}
	}
}

// GracefulShutdown performs a graceful shutdown
func (h *Hardener) GracefulShutdown(ctx context.Context, timeout time.Duration) error {
	shutdownCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Wait for in-flight requests to complete
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-shutdownCtx.Done():
			return fmt.Errorf("shutdown timeout exceeded")
		case <-ticker.C:
			// Check if there are any in-flight requests
			stats := h.GetRuntimeStats()
			if stats.Goroutines <= 10 { // Allow some background goroutines
				return nil
			}
		}
	}
}

// CircuitBreaker implements circuit breaker pattern
type CircuitBreaker struct {
	mu               sync.RWMutex
	name             string
	state            string // "closed", "open", "half-open"
	failureCount     int
	successCount     int
	failureThreshold int
	successThreshold int
	timeout          time.Duration
	lastFailure      time.Time
}

// NewCircuitBreaker creates a new circuit breaker
func NewCircuitBreaker(name string, failureThreshold, successThreshold int, timeout time.Duration) *CircuitBreaker {
	return &CircuitBreaker{
		name:             name,
		state:            "closed",
		failureThreshold: failureThreshold,
		successThreshold: successThreshold,
		timeout:          timeout,
	}
}

// Execute executes a function with circuit breaker protection
func (cb *CircuitBreaker) Execute(fn func() error) error {
	cb.mu.Lock()

	if cb.state == "open" {
		if time.Since(cb.lastFailure) > cb.timeout {
			cb.state = "half-open"
			cb.successCount = 0
		} else {
			cb.mu.Unlock()
			return fmt.Errorf("circuit breaker %s is open", cb.name)
		}
	}

	cb.mu.Unlock()

	err := fn()

	cb.mu.Lock()
	defer cb.mu.Unlock()

	if err != nil {
		cb.failureCount++
		cb.lastFailure = time.Now()
		if cb.failureCount >= cb.failureThreshold {
			cb.state = "open"
		}
		return err
	}

	if cb.state == "half-open" {
		cb.successCount++
		if cb.successCount >= cb.successThreshold {
			cb.state = "closed"
			cb.failureCount = 0
		}
	} else {
		cb.failureCount = 0
	}

	return nil
}

// GetState returns the current state of the circuit breaker
func (cb *CircuitBreaker) GetState() string {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.state
}

// RetryWithBackoff retries a function with exponential backoff
func RetryWithBackoff(ctx context.Context, maxRetries int, initialBackoff time.Duration, fn func() error) error {
	backoff := initialBackoff
	var lastErr error

	for i := 0; i < maxRetries; i++ {
		if err := fn(); err != nil {
			lastErr = err
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(backoff):
				backoff *= 2
			}
			continue
		}
		return nil
	}

	return fmt.Errorf("max retries exceeded: %w", lastErr)
}

// RateLimiter implements token bucket rate limiting
type RateLimiter struct {
	mu       sync.Mutex
	rate     int
	burst    int
	tokens   int
	lastTime time.Time
}

// NewRateLimiter creates a new rate limiter
func NewRateLimiter(rate, burst int) *RateLimiter {
	return &RateLimiter{
		rate:     rate,
		burst:    burst,
		tokens:   burst,
		lastTime: time.Now(),
	}
}

// Allow checks if a request is allowed
func (rl *RateLimiter) Allow() bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(rl.lastTime)
	refill := int(elapsed.Seconds() * float64(rl.rate))

	if refill > 0 {
		rl.tokens = min(rl.burst, rl.tokens+refill)
		rl.lastTime = now
	}

	if rl.tokens > 0 {
		rl.tokens--
		return true
	}

	return false
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
