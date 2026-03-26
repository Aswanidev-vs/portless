package cli

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"
)

// ProgressBar represents a progress indicator
type ProgressBar struct {
	mu         sync.Mutex
	total      int64
	current    int64
	width      int
	prefix     string
	suffix     string
	output     io.Writer
	startTime  time.Time
	lastUpdate time.Time
	showRate   bool
	showETA    bool
	completed  bool
	barChar    string
	emptyChar  string
}

// ProgressBarConfig holds configuration for the progress bar
type ProgressBarConfig struct {
	Total    int64
	Prefix   string
	Suffix   string
	Width    int
	ShowRate bool
	ShowETA  bool
	Output   io.Writer
}

// NewProgressBar creates a new progress bar
func NewProgressBar(cfg ProgressBarConfig) *ProgressBar {
	if cfg.Width == 0 {
		cfg.Width = 40
	}
	if cfg.Output == nil {
		cfg.Output = os.Stderr
	}
	return &ProgressBar{
		total:      cfg.Total,
		width:      cfg.Width,
		prefix:     cfg.Prefix,
		suffix:     cfg.Suffix,
		output:     cfg.Output,
		startTime:  time.Now(),
		lastUpdate: time.Now(),
		showRate:   cfg.ShowRate,
		showETA:    cfg.ShowETA,
		barChar:    "█",
		emptyChar:  "░",
	}
}

// Update updates the progress bar with the current value
func (p *ProgressBar) Update(current int64) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.current = current
	now := time.Now()

	// Throttle updates to avoid excessive output
	if now.Sub(p.lastUpdate) < 100*time.Millisecond && current < p.total {
		return
	}
	p.lastUpdate = now

	p.render()
}

// Increment increments the progress by 1
func (p *ProgressBar) Increment() {
	p.mu.Lock()
	p.current++
	current := p.current
	p.mu.Unlock()
	p.Update(current)
}

// Add adds a value to the current progress
func (p *ProgressBar) Add(n int64) {
	p.mu.Lock()
	p.current += n
	current := p.current
	p.mu.Unlock()
	p.Update(current)
}

// SetTotal sets the total value
func (p *ProgressBar) SetTotal(total int64) {
	p.mu.Lock()
	p.total = total
	p.mu.Unlock()
}

// Finish marks the progress bar as complete
func (p *ProgressBar) Finish() {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.current = p.total
	p.completed = true
	p.render()
	fmt.Fprintln(p.output)
}

// render draws the progress bar
func (p *ProgressBar) render() {
	if p.total <= 0 {
		return
	}

	percent := float64(p.current) / float64(p.total)
	if percent > 1 {
		percent = 1
	}

	filled := int(percent * float64(p.width))
	empty := p.width - filled

	bar := strings.Repeat(p.barChar, filled) + strings.Repeat(p.emptyChar, empty)

	line := fmt.Sprintf("\r%s [%s] %d/%d", p.prefix, bar, p.current, p.total)

	if p.showRate {
		elapsed := time.Since(p.startTime).Seconds()
		if elapsed > 0 {
			rate := float64(p.current) / elapsed
			line += fmt.Sprintf(" (%.1f/s)", rate)
		}
	}

	if p.showETA && percent > 0 {
		elapsed := time.Since(p.startTime)
		remaining := time.Duration(float64(elapsed) / percent * (1 - percent))
		line += fmt.Sprintf(" ETA: %s", remaining.Round(time.Second))
	}

	if p.suffix != "" {
		line += " " + p.suffix
	}

	fmt.Fprint(p.output, line)
}

// Spinner represents an animated spinner for indeterminate progress
type Spinner struct {
	mu      sync.Mutex
	frames  []string
	current int
	output  io.Writer
	stopCh  chan struct{}
	message string
	running bool
}

// NewSpinner creates a new spinner
func NewSpinner(message string) *Spinner {
	return &Spinner{
		frames:  []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"},
		output:  os.Stderr,
		stopCh:  make(chan struct{}),
		message: message,
	}
}

// Start starts the spinner animation
func (s *Spinner) Start() {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return
	}
	s.running = true
	s.mu.Unlock()

	go func() {
		ticker := time.NewTicker(80 * time.Millisecond)
		defer ticker.Stop()

		for {
			select {
			case <-s.stopCh:
				return
			case <-ticker.C:
				s.mu.Lock()
				frame := s.frames[s.current]
				s.current = (s.current + 1) % len(s.frames)
				msg := s.message
				s.mu.Unlock()
				fmt.Fprintf(s.output, "\r%s %s", frame, msg)
			}
		}
	}()
}

// Stop stops the spinner
func (s *Spinner) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.running {
		return
	}

	s.running = false
	close(s.stopCh)
	fmt.Fprintf(s.output, "\r")
}

// SetMessage updates the spinner message
func (s *Spinner) SetMessage(message string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.message = message
}

// StopWithMessage stops the spinner and displays a final message
func (s *Spinner) StopWithMessage(message string) {
	s.Stop()
	fmt.Fprintln(s.output, message)
}
