package nfr

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// NFRType represents the type of non-functional requirement
type NFRType string

const (
	NFRLatency      NFRType = "latency"
	NFRUptime       NFRType = "uptime"
	NFRThroughput   NFRType = "throughput"
	NFRRecovery     NFRType = "recovery"
	NFRScalability  NFRType = "scalability"
	NFRAvailability NFRType = "availability"
)

// NFRStatus represents the status of an NFR
type NFRStatus string

const (
	NFRStatusPassing NFRStatus = "passing"
	NFRStatusWarning NFRStatus = "warning"
	NFRStatusFailing NFRStatus = "failing"
	NFRStatusUnknown NFRStatus = "unknown"
)

// NFRMetric represents a metric for an NFR
type NFRMetric struct {
	Name      string    `json:"name"`
	Value     float64   `json:"value"`
	Unit      string    `json:"unit"`
	Threshold float64   `json:"threshold"`
	Timestamp time.Time `json:"timestamp"`
}

// NFRRequirement represents a non-functional requirement
type NFRRequirement struct {
	ID          string      `json:"id"`
	Type        NFRType     `json:"type"`
	Name        string      `json:"name"`
	Description string      `json:"description"`
	Threshold   float64     `json:"threshold"`
	Unit        string      `json:"unit"`
	Status      NFRStatus   `json:"status"`
	LastChecked time.Time   `json:"last_checked"`
	History     []NFRMetric `json:"history,omitempty"`
}

// Validator validates non-functional requirements
type Validator struct {
	mu           sync.RWMutex
	requirements map[string]*NFRRequirement
	metrics      map[string][]NFRMetric
	stopCh       chan struct{}
	wg           sync.WaitGroup
	onViolation  func(req *NFRRequirement, metric NFRMetric)
}

// ValidatorConfig holds validator configuration
type ValidatorConfig struct {
	OnViolation func(req *NFRRequirement, metric NFRMetric)
}

// NewValidator creates a new NFR validator
func NewValidator(cfg ValidatorConfig) *Validator {
	return &Validator{
		requirements: make(map[string]*NFRRequirement),
		metrics:      make(map[string][]NFRMetric),
		stopCh:       make(chan struct{}),
		onViolation:  cfg.OnViolation,
	}
}

// Start starts the validator
func (v *Validator) Start(ctx context.Context) error {
	// Start validation worker
	v.wg.Add(1)
	go v.validationWorker(ctx)

	return nil
}

// Stop stops the validator
func (v *Validator) Stop() {
	close(v.stopCh)
	v.wg.Wait()
}

// AddRequirement adds an NFR requirement
func (v *Validator) AddRequirement(req *NFRRequirement) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.requirements[req.ID] = req
}

// RemoveRequirement removes an NFR requirement
func (v *Validator) RemoveRequirement(reqID string) {
	v.mu.Lock()
	defer v.mu.Unlock()
	delete(v.requirements, reqID)
}

// GetRequirement retrieves an NFR requirement
func (v *Validator) GetRequirement(reqID string) (*NFRRequirement, error) {
	v.mu.RLock()
	defer v.mu.RUnlock()
	req, ok := v.requirements[reqID]
	if !ok {
		return nil, fmt.Errorf("requirement %s not found", reqID)
	}
	return req, nil
}

// ListRequirements lists all NFR requirements
func (v *Validator) ListRequirements() []*NFRRequirement {
	v.mu.RLock()
	defer v.mu.RUnlock()
	var reqs []*NFRRequirement
	for _, req := range v.requirements {
		reqs = append(reqs, req)
	}
	return reqs
}

// RecordMetric records a metric for validation
func (v *Validator) RecordMetric(reqID string, metric NFRMetric) error {
	v.mu.Lock()
	defer v.mu.Unlock()

	req, ok := v.requirements[reqID]
	if !ok {
		return fmt.Errorf("requirement %s not found", reqID)
	}

	// Add to metrics history
	if len(v.metrics[reqID]) >= 100 {
		v.metrics[reqID] = v.metrics[reqID][1:]
	}
	v.metrics[reqID] = append(v.metrics[reqID], metric)

	// Validate metric
	v.validateMetric(req, metric)

	return nil
}

// GetMetrics returns metrics for a requirement
func (v *Validator) GetMetrics(reqID string) []NFRMetric {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return v.metrics[reqID]
}

// Validate validates all requirements
func (v *Validator) Validate(ctx context.Context) map[string]NFRStatus {
	v.mu.RLock()
	defer v.mu.RUnlock()

	results := make(map[string]NFRStatus)
	for id, req := range v.requirements {
		results[id] = req.Status
	}
	return results
}

// ValidateLatency validates latency requirements
func (v *Validator) ValidateLatency(reqID string, latency time.Duration) error {
	metric := NFRMetric{
		Name:      "latency",
		Value:     float64(latency.Milliseconds()),
		Unit:      "ms",
		Timestamp: time.Now(),
	}
	return v.RecordMetric(reqID, metric)
}

// ValidateUptime validates uptime requirements
func (v *Validator) ValidateUptime(reqID string, uptimePercent float64) error {
	metric := NFRMetric{
		Name:      "uptime",
		Value:     uptimePercent,
		Unit:      "percent",
		Timestamp: time.Now(),
	}
	return v.RecordMetric(reqID, metric)
}

// ValidateThroughput validates throughput requirements
func (v *Validator) ValidateThroughput(reqID string, throughput float64) error {
	metric := NFRMetric{
		Name:      "throughput",
		Value:     throughput,
		Unit:      "requests/second",
		Timestamp: time.Now(),
	}
	return v.RecordMetric(reqID, metric)
}

// ValidateRecovery validates recovery time requirements
func (v *Validator) ValidateRecovery(reqID string, recoveryTime time.Duration) error {
	metric := NFRMetric{
		Name:      "recovery_time",
		Value:     float64(recoveryTime.Seconds()),
		Unit:      "seconds",
		Timestamp: time.Now(),
	}
	return v.RecordMetric(reqID, metric)
}

// ValidateScalability validates scalability requirements
func (v *Validator) ValidateScalability(reqID string, concurrentTunnels int) error {
	metric := NFRMetric{
		Name:      "concurrent_tunnels",
		Value:     float64(concurrentTunnels),
		Unit:      "tunnels",
		Timestamp: time.Now(),
	}
	return v.RecordMetric(reqID, metric)
}

func (v *Validator) validateMetric(req *NFRRequirement, metric NFRMetric) {
	// Check if metric exceeds threshold
	passing := true
	switch req.Type {
	case NFRLatency:
		// For latency, value should be below threshold
		passing = metric.Value <= req.Threshold
	case NFRUptime:
		// For uptime, value should be above threshold
		passing = metric.Value >= req.Threshold
	case NFRThroughput:
		// For throughput, value should be above threshold
		passing = metric.Value >= req.Threshold
	case NFRRecovery:
		// For recovery, value should be below threshold
		passing = metric.Value <= req.Threshold
	case NFRScalability:
		// For scalability, value should be above threshold
		passing = metric.Value >= req.Threshold
	}

	if passing {
		req.Status = NFRStatusPassing
	} else {
		req.Status = NFRStatusFailing
		if v.onViolation != nil {
			v.onViolation(req, metric)
		}
	}

	req.LastChecked = time.Now()
}

func (v *Validator) validationWorker(ctx context.Context) {
	defer v.wg.Done()

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-v.stopCh:
			return
		case <-ticker.C:
			v.checkRequirements()
		}
	}
}

func (v *Validator) checkRequirements() {
	v.mu.RLock()
	defer v.mu.RUnlock()

	for _, req := range v.requirements {
		// Check if requirement is stale
		if time.Since(req.LastChecked) > 5*time.Minute {
			req.Status = NFRStatusUnknown
		}
	}
}

// GetStatusSummary returns a summary of all requirement statuses
func (v *Validator) GetStatusSummary() map[NFRStatus]int {
	v.mu.RLock()
	defer v.mu.RUnlock()

	summary := make(map[NFRStatus]int)
	for _, req := range v.requirements {
		summary[req.Status]++
	}
	return summary
}

// GetFailingRequirements returns all failing requirements
func (v *Validator) GetFailingRequirements() []*NFRRequirement {
	v.mu.RLock()
	defer v.mu.RUnlock()

	var failing []*NFRRequirement
	for _, req := range v.requirements {
		if req.Status == NFRStatusFailing {
			failing = append(failing, req)
		}
	}
	return failing
}
