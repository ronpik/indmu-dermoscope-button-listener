package app

import (
	"encoding/json"
	"errors"
	"os"

	"github.com/trichoai/dermoscope-helper/internal/keyboard"
	"github.com/trichoai/dermoscope-helper/internal/usb"
)

// Config holds application configuration
type Config struct {
	// Timing settings
	DebounceMs       int `json:"debounce_ms"`
	ReconnectDelayMs int `json:"reconnect_delay_ms"`
	ReadTimeoutMs    int `json:"read_timeout_ms"`

	// Keyboard settings
	TriggerKey int `json:"trigger_key"` // Default: KeyF9 (0x78)

	// Logging settings
	LogFile  string `json:"log_file"`
	LogLevel string `json:"log_level"`

	// Behavior settings
	StartMinimized bool `json:"start_minimized"`
	AutoStart      bool `json:"auto_start"`
}

// DefaultConfig returns the default configuration
func DefaultConfig() *Config {
	return &Config{
		DebounceMs:       usb.DebounceMs,
		ReconnectDelayMs: usb.ReconnectDelayMs,
		ReadTimeoutMs:    usb.ReadTimeoutMs,
		TriggerKey:       keyboard.KeyF9,
		LogFile:          "",
		LogLevel:         "info",
		StartMinimized:   false,
		AutoStart:        false,
	}
}

// LoadConfig loads configuration from file (if exists).
// Returns default configuration if file does not exist.
// Returns an error only for permission issues or invalid JSON.
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		// If file doesn't exist, return defaults (graceful handling)
		if errors.Is(err, os.ErrNotExist) {
			return DefaultConfig(), nil
		}
		// Other errors (permission denied, etc.) are returned
		return nil, err
	}

	// Start with defaults so partial configs work
	config := DefaultConfig()
	if err := json.Unmarshal(data, config); err != nil {
		return nil, err
	}

	return config, nil
}

// SaveConfig saves configuration to file
func (c *Config) SaveConfig(path string) error {
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0644)
}
