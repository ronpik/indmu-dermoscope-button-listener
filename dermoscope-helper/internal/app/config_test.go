package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/trichoai/dermoscope-helper/internal/keyboard"
	"github.com/trichoai/dermoscope-helper/internal/usb"
)

func TestDefaultConfig_ReturnsValidConfiguration(t *testing.T) {
	cfg := DefaultConfig()
	if cfg == nil {
		t.Fatal("DefaultConfig() returned nil")
	}
}

func TestDefaultConfig_HasCorrectTimingValues(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.DebounceMs != 250 {
		t.Errorf("DebounceMs = %d, want 250", cfg.DebounceMs)
	}
	if cfg.ReconnectDelayMs != 2000 {
		t.Errorf("ReconnectDelayMs = %d, want 2000", cfg.ReconnectDelayMs)
	}
	if cfg.ReadTimeoutMs != 500 {
		t.Errorf("ReadTimeoutMs = %d, want 500", cfg.ReadTimeoutMs)
	}
}

func TestDefaultConfig_HasCorrectKeyboardValues(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.TriggerKey != 0x78 {
		t.Errorf("TriggerKey = 0x%02X, want 0x78", cfg.TriggerKey)
	}
}

func TestDefaultConfig_UsesUSBPackageConstants(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.DebounceMs != usb.DebounceMs {
		t.Errorf("DebounceMs doesn't match usb.DebounceMs")
	}
	if cfg.ReconnectDelayMs != usb.ReconnectDelayMs {
		t.Errorf("ReconnectDelayMs doesn't match usb.ReconnectDelayMs")
	}
	if cfg.ReadTimeoutMs != usb.ReadTimeoutMs {
		t.Errorf("ReadTimeoutMs doesn't match usb.ReadTimeoutMs")
	}
}

func TestDefaultConfig_UsesKeyboardPackageConstants(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.TriggerKey != keyboard.KeyF9 {
		t.Errorf("TriggerKey doesn't match keyboard.KeyF9")
	}
}

func TestDefaultConfig_HasCorrectBehaviorDefaults(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.StartMinimized != false {
		t.Error("StartMinimized should default to false")
	}
	if cfg.AutoStart != false {
		t.Error("AutoStart should default to false")
	}
}

func TestDefaultConfig_HasCorrectLoggingDefaults(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.LogFile != "" {
		t.Errorf("LogFile should default to empty, got %q", cfg.LogFile)
	}
	if cfg.LogLevel != "info" {
		t.Errorf("LogLevel = %q, want \"info\"", cfg.LogLevel)
	}
}

func TestLoadConfig_NonExistentFile_ReturnsDefaults(t *testing.T) {
	cfg, err := LoadConfig("/nonexistent/path/config.json")
	if err != nil {
		t.Fatalf("LoadConfig() returned error for non-existent file: %v", err)
	}
	if cfg == nil {
		t.Fatal("LoadConfig() returned nil for non-existent file")
	}

	// Verify it returns defaults
	expected := DefaultConfig()
	if cfg.DebounceMs != expected.DebounceMs {
		t.Errorf("DebounceMs = %d, want %d", cfg.DebounceMs, expected.DebounceMs)
	}
	if cfg.TriggerKey != expected.TriggerKey {
		t.Errorf("TriggerKey = 0x%02X, want 0x%02X", cfg.TriggerKey, expected.TriggerKey)
	}
}

func TestLoadConfig_ValidJSONFile(t *testing.T) {
	// Create temp file with valid JSON
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	content := `{
		"debounce_ms": 300,
		"reconnect_delay_ms": 3000,
		"read_timeout_ms": 600,
		"trigger_key": 121,
		"log_file": "/tmp/test.log",
		"log_level": "debug",
		"start_minimized": true,
		"auto_start": true
	}`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}

	if cfg.DebounceMs != 300 {
		t.Errorf("DebounceMs = %d, want 300", cfg.DebounceMs)
	}
	if cfg.ReconnectDelayMs != 3000 {
		t.Errorf("ReconnectDelayMs = %d, want 3000", cfg.ReconnectDelayMs)
	}
	if cfg.ReadTimeoutMs != 600 {
		t.Errorf("ReadTimeoutMs = %d, want 600", cfg.ReadTimeoutMs)
	}
	if cfg.TriggerKey != 121 {
		t.Errorf("TriggerKey = %d, want 121", cfg.TriggerKey)
	}
	if cfg.LogFile != "/tmp/test.log" {
		t.Errorf("LogFile = %q, want \"/tmp/test.log\"", cfg.LogFile)
	}
	if cfg.LogLevel != "debug" {
		t.Errorf("LogLevel = %q, want \"debug\"", cfg.LogLevel)
	}
	if cfg.StartMinimized != true {
		t.Error("StartMinimized should be true")
	}
	if cfg.AutoStart != true {
		t.Error("AutoStart should be true")
	}
}

func TestLoadConfig_InvalidJSON_ReturnsError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	// Write invalid JSON
	if err := os.WriteFile(path, []byte("{ invalid json }"), 0644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	_, err := LoadConfig(path)
	if err == nil {
		t.Error("LoadConfig() should return error for invalid JSON")
	}
}

func TestLoadConfig_PartialConfig_MergesWithDefaults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	// Only specify some values
	content := `{"debounce_ms": 400}`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}

	// Specified value should be used
	if cfg.DebounceMs != 400 {
		t.Errorf("DebounceMs = %d, want 400", cfg.DebounceMs)
	}

	// Unspecified values should be defaults
	expected := DefaultConfig()
	if cfg.ReconnectDelayMs != expected.ReconnectDelayMs {
		t.Errorf("ReconnectDelayMs = %d, want %d", cfg.ReconnectDelayMs, expected.ReconnectDelayMs)
	}
	if cfg.TriggerKey != expected.TriggerKey {
		t.Errorf("TriggerKey = 0x%02X, want 0x%02X", cfg.TriggerKey, expected.TriggerKey)
	}
}

func TestSaveConfig_CreatesValidJSONFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	cfg := &Config{
		DebounceMs:       350,
		ReconnectDelayMs: 2500,
		ReadTimeoutMs:    550,
		TriggerKey:       0x79,
		LogFile:          "/var/log/test.log",
		LogLevel:         "warn",
		StartMinimized:   true,
		AutoStart:        false,
	}

	if err := cfg.SaveConfig(path); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	// Read and verify
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("Failed to read saved file: %v", err)
	}

	var loaded Config
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatalf("Saved file is not valid JSON: %v", err)
	}

	if loaded.DebounceMs != 350 {
		t.Errorf("DebounceMs = %d, want 350", loaded.DebounceMs)
	}
	if loaded.TriggerKey != 0x79 {
		t.Errorf("TriggerKey = 0x%02X, want 0x79", loaded.TriggerKey)
	}
}

func TestSaveConfig_ThenLoadConfig_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	original := &Config{
		DebounceMs:       275,
		ReconnectDelayMs: 1500,
		ReadTimeoutMs:    450,
		TriggerKey:       0x7A,
		LogFile:          "/tmp/roundtrip.log",
		LogLevel:         "error",
		StartMinimized:   false,
		AutoStart:        true,
	}

	if err := original.SaveConfig(path); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	loaded, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}

	if loaded.DebounceMs != original.DebounceMs {
		t.Errorf("DebounceMs = %d, want %d", loaded.DebounceMs, original.DebounceMs)
	}
	if loaded.ReconnectDelayMs != original.ReconnectDelayMs {
		t.Errorf("ReconnectDelayMs = %d, want %d", loaded.ReconnectDelayMs, original.ReconnectDelayMs)
	}
	if loaded.ReadTimeoutMs != original.ReadTimeoutMs {
		t.Errorf("ReadTimeoutMs = %d, want %d", loaded.ReadTimeoutMs, original.ReadTimeoutMs)
	}
	if loaded.TriggerKey != original.TriggerKey {
		t.Errorf("TriggerKey = 0x%02X, want 0x%02X", loaded.TriggerKey, original.TriggerKey)
	}
	if loaded.LogFile != original.LogFile {
		t.Errorf("LogFile = %q, want %q", loaded.LogFile, original.LogFile)
	}
	if loaded.LogLevel != original.LogLevel {
		t.Errorf("LogLevel = %q, want %q", loaded.LogLevel, original.LogLevel)
	}
	if loaded.StartMinimized != original.StartMinimized {
		t.Errorf("StartMinimized = %v, want %v", loaded.StartMinimized, original.StartMinimized)
	}
	if loaded.AutoStart != original.AutoStart {
		t.Errorf("AutoStart = %v, want %v", loaded.AutoStart, original.AutoStart)
	}
}

func TestLoadConfig_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	// Empty file is valid JSON (empty object would be "{}")
	// But truly empty file is invalid JSON
	if err := os.WriteFile(path, []byte(""), 0644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	_, err := LoadConfig(path)
	if err == nil {
		t.Error("LoadConfig() should return error for empty file")
	}
}

func TestLoadConfig_EmptyObject(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	// Empty JSON object should use all defaults
	if err := os.WriteFile(path, []byte("{}"), 0644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}

	expected := DefaultConfig()
	if cfg.DebounceMs != expected.DebounceMs {
		t.Errorf("DebounceMs = %d, want %d", cfg.DebounceMs, expected.DebounceMs)
	}
	if cfg.TriggerKey != expected.TriggerKey {
		t.Errorf("TriggerKey = 0x%02X, want 0x%02X", cfg.TriggerKey, expected.TriggerKey)
	}
}

func TestLoadConfig_InvalidTypes_ReturnsError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	// debounce_ms should be int, not string
	content := `{"debounce_ms": "not_a_number"}`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	_, err := LoadConfig(path)
	if err == nil {
		t.Error("LoadConfig() should return error for invalid types")
	}
}
