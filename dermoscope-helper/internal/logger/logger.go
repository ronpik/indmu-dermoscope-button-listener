// Package logger provides logging setup for the dermoscope helper.
package logger

import (
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/rs/zerolog"
	"gopkg.in/natefinch/lumberjack.v2"
)

// New creates a new configured logger (console only)
func New(debug bool) zerolog.Logger {
	// Configure console output with human-readable format and colors
	consoleWriter := zerolog.ConsoleWriter{
		Out:        os.Stdout,
		TimeFormat: time.RFC3339,
	}

	// Set log level based on debug flag
	level := zerolog.InfoLevel
	if debug {
		level = zerolog.DebugLevel
	}

	return zerolog.New(consoleWriter).
		Level(level).
		With().
		Timestamp().
		Logger()
}

// NewWithFile creates a logger that writes to both console and file
func NewWithFile(debug bool, logFile string) zerolog.Logger {
	// Ensure log directory exists
	logDir := filepath.Dir(logFile)
	if logDir != "" && logDir != "." {
		if err := os.MkdirAll(logDir, 0755); err != nil {
			// If we can't create directory, fall back to console-only
			return New(debug)
		}
	}

	// Configure console output with human-readable format and colors
	consoleWriter := zerolog.ConsoleWriter{
		Out:        os.Stdout,
		TimeFormat: time.RFC3339,
	}

	// Configure file output with lumberjack for rotation
	// File output is JSON formatted for easy parsing
	fileWriter := &lumberjack.Logger{
		Filename:   logFile,
		MaxSize:    10, // megabytes
		MaxBackups: 5,  // number of backup files to keep
		MaxAge:     0,  // days to retain (0 = no limit, rely on MaxBackups)
		Compress:   false,
	}

	// Set log level based on debug flag
	level := zerolog.InfoLevel
	if debug {
		level = zerolog.DebugLevel
	}

	// Create multi-writer for both console and file
	// Console gets human-readable output, file gets JSON
	multi := io.MultiWriter(consoleWriter, fileWriter)

	return zerolog.New(multi).
		Level(level).
		With().
		Timestamp().
		Logger()
}

// SetGlobalLevel sets the global log level
func SetGlobalLevel(level string) {
	switch level {
	case "debug":
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	case "info":
		zerolog.SetGlobalLevel(zerolog.InfoLevel)
	case "warn":
		zerolog.SetGlobalLevel(zerolog.WarnLevel)
	case "error":
		zerolog.SetGlobalLevel(zerolog.ErrorLevel)
	default:
		// Default to info for invalid level strings
		zerolog.SetGlobalLevel(zerolog.InfoLevel)
	}
}
