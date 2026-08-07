package logging

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
)

// Level represents the logging level.
type Level int

const (
	LevelDebug Level = iota
	LevelInfo
	LevelWarn
	LevelError
)

// String returns the string representation of the level.
func (l Level) String() string {
	switch l {
	case LevelDebug:
		return "DEBUG"
	case LevelInfo:
		return "INFO"
	case LevelWarn:
		return "WARN"
	case LevelError:
		return "ERROR"
	default:
		return "UNKNOWN"
	}
}

// ParseLevel parses a string into a Level.
func ParseLevel(s string) (Level, error) {
	switch s {
	case "debug":
		return LevelDebug, nil
	case "info":
		return LevelInfo, nil
	case "warn":
		return LevelWarn, nil
	case "error":
		return LevelError, nil
	default:
		return LevelInfo, fmt.Errorf("unknown level: %s", s)
	}
}

// Logger wraps the standard logger with level support.
type Logger struct {
	level  Level
	logger *log.Logger
}

// New creates a new logger.
func New(level Level, output io.Writer) *Logger {
	return &Logger{
		level:  level,
		logger: log.New(output, "", log.LstdFlags),
	}
}

// NewFromFile creates a new logger that writes to a file.
func NewFromFile(level Level, path string) (*Logger, error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create log directory: %w", err)
	}

	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to open log file: %w", err)
	}

	return New(level, file), nil
}

// Debug logs a debug message.
func (l *Logger) Debug(format string, args ...interface{}) {
	if l.level <= LevelDebug {
		l.logger.Printf("[DEBUG] "+format, args...)
	}
}

// Info logs an info message.
func (l *Logger) Info(format string, args ...interface{}) {
	if l.level <= LevelInfo {
		l.logger.Printf("[INFO] "+format, args...)
	}
}

// Warn logs a warning message.
func (l *Logger) Warn(format string, args ...interface{}) {
	if l.level <= LevelWarn {
		l.logger.Printf("[WARN] "+format, args...)
	}
}

// Error logs an error message.
func (l *Logger) Error(format string, args ...interface{}) {
	if l.level <= LevelError {
		l.logger.Printf("[ERROR] "+format, args...)
	}
}

// SetLevel sets the logging level.
func (l *Logger) SetLevel(level Level) {
	l.level = level
}

// Default returns the default logger.
func Default() *Logger {
	return New(LevelInfo, os.Stderr)
}
