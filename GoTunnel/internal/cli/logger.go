package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"
)

// LogLevel represents the logging verbosity level
type LogLevel int

const (
	LevelTrace LogLevel = iota
	LevelDebug
	LevelInfo
	LevelWarn
	LevelError
	LevelFatal
)

var levelNames = map[LogLevel]string{
	LevelTrace: "trace",
	LevelDebug: "debug",
	LevelInfo:  "info",
	LevelWarn:  "warn",
	LevelError: "error",
	LevelFatal: "fatal",
}

var levelLabels = map[LogLevel]string{
	LevelTrace: "TRC",
	LevelDebug: "DBG",
	LevelInfo:  "INF",
	LevelWarn:  "WRN",
	LevelError: "ERR",
	LevelFatal: "FTL",
}

// ParseLogLevel converts a string to a LogLevel
func ParseLogLevel(s string) (LogLevel, error) {
	s = strings.ToLower(strings.TrimSpace(s))
	for level, name := range levelNames {
		if name == s {
			return level, nil
		}
	}
	return LevelInfo, fmt.Errorf("unknown log level: %s (valid: trace, debug, info, warn, error, fatal)", s)
}

// LogEntry represents a structured log entry
type LogEntry struct {
	Timestamp string      `json:"timestamp"`
	Level     string      `json:"level"`
	Message   string      `json:"message"`
	Component string      `json:"component,omitempty"`
	Fields    interface{} `json:"fields,omitempty"`
}

// Logger provides structured logging with configurable verbosity
type Logger struct {
	mu        sync.RWMutex
	level     LogLevel
	output    io.Writer
	component string
	jsonMode  bool
}

// NewLogger creates a new logger instance
func NewLogger(level LogLevel, component string) *Logger {
	return &Logger{
		level:     level,
		output:    os.Stderr,
		component: component,
		jsonMode:  false,
	}
}

// NewJSONLogger creates a logger that outputs JSON
func NewJSONLogger(level LogLevel, component string) *Logger {
	return &Logger{
		level:     level,
		output:    os.Stderr,
		component: component,
		jsonMode:  true,
	}
}

// SetLevel changes the log level dynamically
func (l *Logger) SetLevel(level LogLevel) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.level = level
}

// SetOutput changes the output writer
func (l *Logger) SetOutput(w io.Writer) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.output = w
}

// IsLevelEnabled checks if a given level would be logged
func (l *Logger) IsLevelEnabled(level LogLevel) bool {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return level >= l.level
}

func (l *Logger) log(level LogLevel, msg string, fields interface{}) {
	l.mu.RLock()
	defer l.mu.RUnlock()

	if level < l.level {
		return
	}

	if l.jsonMode {
		entry := LogEntry{
			Timestamp: time.Now().UTC().Format(time.RFC3339),
			Level:     levelNames[level],
			Message:   msg,
			Component: l.component,
			Fields:    fields,
		}
		data, _ := json.Marshal(entry)
		fmt.Fprintln(l.output, string(data))
	} else {
		timestamp := time.Now().Format("15:04:05")
		label := levelLabels[level]
		prefix := fmt.Sprintf("[%s] %s %-5s", timestamp, label, l.component)
		if fields != nil {
			fmt.Fprintf(l.output, "%s %s %v\n", prefix, msg, fields)
		} else {
			fmt.Fprintf(l.output, "%s %s\n", prefix, msg)
		}
	}
}

// Trace logs a trace-level message
func (l *Logger) Trace(msg string, fields ...interface{}) {
	var f interface{}
	if len(fields) > 0 {
		f = fields[0]
	}
	l.log(LevelTrace, msg, f)
}

// Debug logs a debug-level message
func (l *Logger) Debug(msg string, fields ...interface{}) {
	var f interface{}
	if len(fields) > 0 {
		f = fields[0]
	}
	l.log(LevelDebug, msg, f)
}

// Info logs an info-level message
func (l *Logger) Info(msg string, fields ...interface{}) {
	var f interface{}
	if len(fields) > 0 {
		f = fields[0]
	}
	l.log(LevelInfo, msg, f)
}

// Warn logs a warning-level message
func (l *Logger) Warn(msg string, fields ...interface{}) {
	var f interface{}
	if len(fields) > 0 {
		f = fields[0]
	}
	l.log(LevelWarn, msg, f)
}

// Error logs an error-level message
func (l *Logger) Error(msg string, fields ...interface{}) {
	var f interface{}
	if len(fields) > 0 {
		f = fields[0]
	}
	l.log(LevelError, msg, f)
}

// Fatal logs a fatal-level message and exits
func (l *Logger) Fatal(msg string, fields ...interface{}) {
	var f interface{}
	if len(fields) > 0 {
		f = fields[0]
	}
	l.log(LevelFatal, msg, f)
	os.Exit(1)
}

// WithComponent creates a new logger with a different component name
func (l *Logger) WithComponent(component string) *Logger {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return &Logger{
		level:     l.level,
		output:    l.output,
		component: component,
		jsonMode:  l.jsonMode,
	}
}

// Global logger instance
var globalLogger = NewLogger(LevelInfo, "gotunnel")

// SetGlobalLogLevel sets the log level for the global logger
func SetGlobalLogLevel(level LogLevel) {
	globalLogger.SetLevel(level)
}

// GetGlobalLogger returns the global logger instance
func GetGlobalLogger() *Logger {
	return globalLogger
}
