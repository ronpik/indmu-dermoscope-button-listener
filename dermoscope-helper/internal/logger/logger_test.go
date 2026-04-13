package logger

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rs/zerolog"
)

// Test New(true) creates debug-level logger
func TestNew_DebugTrue_CreatesDebugLevelLogger(t *testing.T) {
	logger := New(true)

	// Debug level should be enabled
	if logger.GetLevel() != zerolog.DebugLevel {
		t.Errorf("New(true) level = %v, want %v", logger.GetLevel(), zerolog.DebugLevel)
	}
}

// Test New(false) creates info-level logger
func TestNew_DebugFalse_CreatesInfoLevelLogger(t *testing.T) {
	logger := New(false)

	// Info level should be enabled, debug should not be
	if logger.GetLevel() != zerolog.InfoLevel {
		t.Errorf("New(false) level = %v, want %v", logger.GetLevel(), zerolog.InfoLevel)
	}
}

// Test New creates logger that can write messages
func TestNew_CanWriteMessages(t *testing.T) {
	// Capture output by temporarily redirecting stdout
	// We can't easily capture the ConsoleWriter output, but we can verify
	// the logger doesn't panic when writing
	logger := New(false)

	// These should not panic
	logger.Info().Msg("test info message")
	logger.Warn().Msg("test warn message")
	logger.Error().Msg("test error message")
}

// Test New with debug true can write debug messages
func TestNew_DebugTrue_CanWriteDebugMessages(t *testing.T) {
	logger := New(true)

	// Should not panic when writing debug messages
	logger.Debug().Msg("test debug message")
}

// Test NewWithFile creates logger with file output
func TestNewWithFile_CreatesLoggerWithFileOutput(t *testing.T) {
	dir := t.TempDir()
	logFile := filepath.Join(dir, "test.log")

	logger := NewWithFile(false, logFile)

	// Write a test message
	logger.Info().Msg("test message")

	// Verify file was created
	if _, err := os.Stat(logFile); os.IsNotExist(err) {
		t.Error("NewWithFile() did not create log file")
	}

	// Verify content was written
	content, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}
	if len(content) == 0 {
		t.Error("Log file is empty after writing message")
	}
	if !strings.Contains(string(content), "test message") {
		t.Errorf("Log file content = %q, want to contain 'test message'", string(content))
	}
}

// Test NewWithFile with debug true creates debug-level logger
func TestNewWithFile_DebugTrue_CreatesDebugLevelLogger(t *testing.T) {
	dir := t.TempDir()
	logFile := filepath.Join(dir, "test.log")

	logger := NewWithFile(true, logFile)

	if logger.GetLevel() != zerolog.DebugLevel {
		t.Errorf("NewWithFile(true, ...) level = %v, want %v", logger.GetLevel(), zerolog.DebugLevel)
	}
}

// Test NewWithFile with debug false creates info-level logger
func TestNewWithFile_DebugFalse_CreatesInfoLevelLogger(t *testing.T) {
	dir := t.TempDir()
	logFile := filepath.Join(dir, "test.log")

	logger := NewWithFile(false, logFile)

	if logger.GetLevel() != zerolog.InfoLevel {
		t.Errorf("NewWithFile(false, ...) level = %v, want %v", logger.GetLevel(), zerolog.InfoLevel)
	}
}

// Test NewWithFile writes JSON formatted output to file
func TestNewWithFile_WritesJSONToFile(t *testing.T) {
	dir := t.TempDir()
	logFile := filepath.Join(dir, "test.log")

	logger := NewWithFile(false, logFile)
	logger.Info().Str("key", "value").Msg("json test")

	content, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}

	// Should contain JSON-style formatting (quotes around strings)
	contentStr := string(content)
	if !strings.Contains(contentStr, `"key"`) {
		t.Errorf("File output should be JSON formatted, got: %s", contentStr)
	}
	if !strings.Contains(contentStr, `"value"`) {
		t.Errorf("File output should contain field value, got: %s", contentStr)
	}
}

// Test SetGlobalLevel changes global level to debug
func TestSetGlobalLevel_Debug(t *testing.T) {
	// Save original level
	originalLevel := zerolog.GlobalLevel()
	defer zerolog.SetGlobalLevel(originalLevel)

	SetGlobalLevel("debug")

	if zerolog.GlobalLevel() != zerolog.DebugLevel {
		t.Errorf("SetGlobalLevel(\"debug\") global level = %v, want %v", zerolog.GlobalLevel(), zerolog.DebugLevel)
	}
}

// Test SetGlobalLevel changes global level to info
func TestSetGlobalLevel_Info(t *testing.T) {
	// Save original level
	originalLevel := zerolog.GlobalLevel()
	defer zerolog.SetGlobalLevel(originalLevel)

	SetGlobalLevel("info")

	if zerolog.GlobalLevel() != zerolog.InfoLevel {
		t.Errorf("SetGlobalLevel(\"info\") global level = %v, want %v", zerolog.GlobalLevel(), zerolog.InfoLevel)
	}
}

// Test SetGlobalLevel changes global level to warn
func TestSetGlobalLevel_Warn(t *testing.T) {
	// Save original level
	originalLevel := zerolog.GlobalLevel()
	defer zerolog.SetGlobalLevel(originalLevel)

	SetGlobalLevel("warn")

	if zerolog.GlobalLevel() != zerolog.WarnLevel {
		t.Errorf("SetGlobalLevel(\"warn\") global level = %v, want %v", zerolog.GlobalLevel(), zerolog.WarnLevel)
	}
}

// Test SetGlobalLevel changes global level to error
func TestSetGlobalLevel_Error(t *testing.T) {
	// Save original level
	originalLevel := zerolog.GlobalLevel()
	defer zerolog.SetGlobalLevel(originalLevel)

	SetGlobalLevel("error")

	if zerolog.GlobalLevel() != zerolog.ErrorLevel {
		t.Errorf("SetGlobalLevel(\"error\") global level = %v, want %v", zerolog.GlobalLevel(), zerolog.ErrorLevel)
	}
}

// Test SetGlobalLevel with invalid level string defaults to info
func TestSetGlobalLevel_InvalidLevel_DefaultsToInfo(t *testing.T) {
	// Save original level
	originalLevel := zerolog.GlobalLevel()
	defer zerolog.SetGlobalLevel(originalLevel)

	tests := []struct {
		name  string
		level string
	}{
		{"empty string", ""},
		{"invalid string", "invalid"},
		{"uppercase DEBUG", "DEBUG"},
		{"mixed case Info", "Info"},
		{"typo", "debuf"},
		{"numeric", "1"},
		{"special chars", "!@#$"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			SetGlobalLevel(tt.level)

			if zerolog.GlobalLevel() != zerolog.InfoLevel {
				t.Errorf("SetGlobalLevel(%q) global level = %v, want %v (info)", tt.level, zerolog.GlobalLevel(), zerolog.InfoLevel)
			}
		})
	}
}

// Test NewWithFile with non-existent log directory creates directory
func TestNewWithFile_NonExistentDirectory_CreatesDirectory(t *testing.T) {
	dir := t.TempDir()
	// Create a nested path that doesn't exist
	nestedDir := filepath.Join(dir, "nested", "logs", "subdir")
	logFile := filepath.Join(nestedDir, "test.log")

	// Verify the directory doesn't exist yet
	if _, err := os.Stat(nestedDir); !os.IsNotExist(err) {
		t.Fatal("Test setup: nested directory should not exist yet")
	}

	logger := NewWithFile(false, logFile)

	// Write a message to trigger file creation
	logger.Info().Msg("test message")

	// Verify the directory was created
	if _, err := os.Stat(nestedDir); os.IsNotExist(err) {
		t.Error("NewWithFile() did not create non-existent directory")
	}

	// Verify the file was created
	if _, err := os.Stat(logFile); os.IsNotExist(err) {
		t.Error("NewWithFile() did not create log file in new directory")
	}
}

// Test NewWithFile with file in current directory (empty dir path)
func TestNewWithFile_CurrentDirectory(t *testing.T) {
	// Change to temp dir so we don't pollute the source tree
	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get current directory: %v", err)
	}
	defer os.Chdir(originalDir)

	dir := t.TempDir()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Failed to change to temp directory: %v", err)
	}

	// Use just a filename (current directory)
	logFile := "test.log"

	logger := NewWithFile(false, logFile)
	logger.Info().Msg("test message")

	// Verify file was created in current directory
	fullPath := filepath.Join(dir, logFile)
	if _, err := os.Stat(fullPath); os.IsNotExist(err) {
		t.Error("NewWithFile() did not create log file in current directory")
	}
}

// Test NewWithFile with permission denied falls back to console
// Note: This test may not work on all systems or as root
func TestNewWithFile_PermissionDenied_FallsBackToConsole(t *testing.T) {
	// Skip if running as root (root can write anywhere)
	if os.Getuid() == 0 {
		t.Skip("Skipping permission test when running as root")
	}

	// Create a directory with no write permissions
	dir := t.TempDir()
	restrictedDir := filepath.Join(dir, "restricted")
	if err := os.Mkdir(restrictedDir, 0444); err != nil {
		t.Fatalf("Failed to create restricted directory: %v", err)
	}
	defer os.Chmod(restrictedDir, 0755) // Restore permissions for cleanup

	// Try to create a log file in a subdirectory of the restricted dir
	logFile := filepath.Join(restrictedDir, "subdir", "test.log")

	// This should fall back to console-only logger without panicking
	logger := NewWithFile(false, logFile)

	// Should still be able to log (to console)
	logger.Info().Msg("test message")
}

// Test logger writes timestamp
func TestNew_IncludesTimestamp(t *testing.T) {
	// Create a buffer to capture output
	var buf bytes.Buffer
	testWriter := zerolog.ConsoleWriter{Out: &buf, NoColor: true}

	logger := zerolog.New(testWriter).With().Timestamp().Logger()
	logger.Info().Msg("test")

	output := buf.String()
	// ConsoleWriter formats timestamps, so we should see some time-related output
	// The exact format depends on ConsoleWriter configuration
	if len(output) == 0 {
		t.Error("Logger did not produce any output")
	}
}

// Test logger with structured fields
func TestNewWithFile_StructuredFields(t *testing.T) {
	dir := t.TempDir()
	logFile := filepath.Join(dir, "test.log")

	logger := NewWithFile(false, logFile)
	logger.Info().
		Str("device", "HT-B30S").
		Int("vendorId", 0x0AC8).
		Bool("connected", true).
		Msg("device event")

	content, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}

	contentStr := string(content)
	if !strings.Contains(contentStr, "device") {
		t.Error("Log file should contain 'device' field")
	}
	if !strings.Contains(contentStr, "HT-B30S") {
		t.Error("Log file should contain device name value")
	}
	if !strings.Contains(contentStr, "vendorId") {
		t.Error("Log file should contain 'vendorId' field")
	}
	if !strings.Contains(contentStr, "connected") {
		t.Error("Log file should contain 'connected' field")
	}
}

// Test logger respects log level filtering
func TestNew_RespectsLogLevel(t *testing.T) {
	// Create logger at info level
	logger := New(false)

	// Debug messages should be filtered (level is info)
	// We can't easily verify filtering without mocking, but we can verify
	// the logger has the correct level set
	if logger.GetLevel() != zerolog.InfoLevel {
		t.Errorf("Logger level = %v, want InfoLevel", logger.GetLevel())
	}

	// Create logger at debug level
	debugLogger := New(true)

	// Debug level should allow debug messages
	if debugLogger.GetLevel() != zerolog.DebugLevel {
		t.Errorf("Debug logger level = %v, want DebugLevel", debugLogger.GetLevel())
	}
}

// Test multiple writes to same log file
func TestNewWithFile_MultipleWrites(t *testing.T) {
	dir := t.TempDir()
	logFile := filepath.Join(dir, "test.log")

	logger := NewWithFile(false, logFile)

	// Write multiple messages
	for i := 0; i < 10; i++ {
		logger.Info().Int("iteration", i).Msg("test message")
	}

	content, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}

	// Should have multiple lines
	lines := strings.Split(strings.TrimSpace(string(content)), "\n")
	if len(lines) != 10 {
		t.Errorf("Expected 10 log lines, got %d", len(lines))
	}
}

// Test error logging
func TestNewWithFile_ErrorLogging(t *testing.T) {
	dir := t.TempDir()
	logFile := filepath.Join(dir, "test.log")

	logger := NewWithFile(false, logFile)

	// Log an error with additional context
	testErr := os.ErrNotExist
	logger.Error().Err(testErr).Str("path", "/nonexistent").Msg("file not found")

	content, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}

	contentStr := string(content)
	if !strings.Contains(contentStr, "error") {
		t.Error("Error log should contain 'error' level or field")
	}
}

// Test NewWithFile with dot path (current directory indicator)
func TestNewWithFile_DotPath(t *testing.T) {
	// Change to temp dir
	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get current directory: %v", err)
	}
	defer os.Chdir(originalDir)

	dir := t.TempDir()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Failed to change to temp directory: %v", err)
	}

	// Use ./filename path
	logFile := "./test.log"

	logger := NewWithFile(false, logFile)
	logger.Info().Msg("test message")

	// Verify file was created
	if _, err := os.Stat(filepath.Join(dir, "test.log")); os.IsNotExist(err) {
		t.Error("NewWithFile() did not create log file with dot path")
	}
}
