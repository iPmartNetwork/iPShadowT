package logger

import (
	"fmt"
	"os"
	"time"
)

// Level represents log severity
type Level int

const (
	DEBUG Level = iota
	INFO
	WARN
	ERROR
	FATAL
)

// Logger provides structured logging
type Logger struct {
	level Level
}

// New creates a new logger with the specified level
func New(level string) *Logger {
	l := &Logger{}
	switch level {
	case "debug":
		l.level = DEBUG
	case "info":
		l.level = INFO
	case "warn":
		l.level = WARN
	case "error":
		l.level = ERROR
	default:
		l.level = INFO
	}
	return l
}

func (l *Logger) log(level Level, prefix, format string, args ...interface{}) {
	if level < l.level {
		return
	}
	timestamp := time.Now().Format("2006-01-02 15:04:05")
	msg := fmt.Sprintf(format, args...)
	fmt.Printf("%s %s %s\n", timestamp, prefix, msg)
}

// Debug logs a debug message
func (l *Logger) Debug(format string, args ...interface{}) {
	l.log(DEBUG, "\033[36m[DBG]\033[0m", format, args...)
}

// Info logs an info message
func (l *Logger) Info(format string, args ...interface{}) {
	l.log(INFO, "\033[32m[INF]\033[0m", format, args...)
}

// Warn logs a warning message
func (l *Logger) Warn(format string, args ...interface{}) {
	l.log(WARN, "\033[33m[WRN]\033[0m", format, args...)
}

// Error logs an error message
func (l *Logger) Error(format string, args ...interface{}) {
	l.log(ERROR, "\033[31m[ERR]\033[0m", format, args...)
}

// Fatal logs a fatal message and exits
func (l *Logger) Fatal(format string, args ...interface{}) {
	l.log(FATAL, "\033[35m[FTL]\033[0m", format, args...)
	os.Exit(1)
}
