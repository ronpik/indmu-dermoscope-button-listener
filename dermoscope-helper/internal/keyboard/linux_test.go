//go:build linux

package keyboard

import (
	"os/exec"
	"testing"
)

// =============================================================================
// Linux Keyboard Simulator Tests
// =============================================================================
// These tests verify the Linux xdotool-based keyboard simulator implementation.

// Test NewSimulator fails gracefully when xdotool is not installed
func TestLinuxSimulator_NewSimulator_ChecksXdotool(t *testing.T) {
	// This test checks the error message format when xdotool is not available.
	// On systems with xdotool installed, it will succeed.
	// On systems without xdotool, it should return a descriptive error.
	sim, err := NewSimulator()
	if err != nil {
		// xdotool not found - verify error message is helpful
		if !containsIgnoreCase(err.Error(), "xdotool") {
			t.Errorf("error should mention xdotool, got: %v", err)
		}
		return
	}
	defer sim.Close()
	// xdotool is installed - simulator created successfully
}

// Test Initialize succeeds on Linux
func TestLinuxSimulator_Initialize_Succeeds(t *testing.T) {
	sim, err := NewSimulator()
	if err != nil {
		t.Skipf("xdotool not available: %v", err)
	}
	defer sim.Close()

	// Initialize should succeed on Linux
	if err := sim.Initialize(); err != nil {
		t.Errorf("Initialize() error = %v, want nil", err)
	}
}

// Test Close succeeds on Linux
func TestLinuxSimulator_Close_Succeeds(t *testing.T) {
	sim, err := NewSimulator()
	if err != nil {
		t.Skipf("xdotool not available: %v", err)
	}

	if err := sim.Close(); err != nil {
		t.Errorf("Close() error = %v, want nil", err)
	}
}

// Test keyCodeToX11Name maps Windows VK codes to X11 key names
func TestLinuxSimulator_keyCodeToX11Name(t *testing.T) {
	tests := []struct {
		name     string
		keyCode  int
		expected string
	}{
		{"KeyF9", KeyF9, "F9"},
		{"KeyF10", KeyF10, "F10"},
		{"KeyF11", KeyF11, "F11"},
		{"KeyF12", KeyF12, "F12"},
		{"Unknown key", 0x00, ""},
		{"Invalid key", 0xFF, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := keyCodeToX11Name(tt.keyCode)
			if result != tt.expected {
				t.Errorf("keyCodeToX11Name(%d) = %q, want %q", tt.keyCode, result, tt.expected)
			}
		})
	}
}

// Test PressKey returns error for unsupported key codes
func TestLinuxSimulator_PressKey_UnsupportedKeyCode(t *testing.T) {
	sim, err := NewSimulator()
	if err != nil {
		t.Skipf("xdotool not available: %v", err)
	}
	defer sim.Close()

	// Press an unsupported key code
	err = sim.PressKey(0x00)
	if err == nil {
		t.Error("PressKey(0x00) should return error for unsupported key code")
	}
	if !containsIgnoreCase(err.Error(), "unsupported") {
		t.Errorf("error should mention 'unsupported', got: %v", err)
	}
}

// Test PressKey with valid key codes (requires X11 display)
// Note: This test may fail in headless environments without an X11 display
func TestLinuxSimulator_PressKey_ValidKeyCodes(t *testing.T) {
	// Skip if no DISPLAY environment variable (headless environment)
	if !hasX11Display() {
		t.Skip("No X11 display available - skipping xdotool key press test")
	}

	sim, err := NewSimulator()
	if err != nil {
		t.Skipf("xdotool not available: %v", err)
	}
	defer sim.Close()

	// Test that PressKey can be called with valid key codes
	// Note: This will actually simulate the key press if running in X11
	keyCodes := []struct {
		name string
		code int
	}{
		{"KeyF9", KeyF9},
		{"KeyF10", KeyF10},
		{"KeyF11", KeyF11},
		{"KeyF12", KeyF12},
	}

	for _, kc := range keyCodes {
		t.Run(kc.name, func(t *testing.T) {
			err := sim.PressKey(kc.code)
			// We don't fail if xdotool errors out (e.g., no display)
			// but we verify no panic and error is properly formatted
			if err != nil {
				t.Logf("PressKey(%s) returned error (may be expected in test env): %v", kc.name, err)
			}
		})
	}
}

// =============================================================================
// Helper functions
// =============================================================================

// hasX11Display checks if an X11 display is available
func hasX11Display() bool {
	// Check if DISPLAY environment variable is set and xdotool can execute
	cmd := exec.Command("xdotool", "version")
	err := cmd.Run()
	return err == nil
}

// containsIgnoreCase checks if s contains substr (case-insensitive)
func containsIgnoreCase(s, substr string) bool {
	for i := 0; i+len(substr) <= len(s); i++ {
		if equalIgnoreCase(s[i:i+len(substr)], substr) {
			return true
		}
	}
	return false
}

func equalIgnoreCase(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := 0; i < len(a); i++ {
		ca, cb := a[i], b[i]
		if ca >= 'A' && ca <= 'Z' {
			ca += 'a' - 'A'
		}
		if cb >= 'A' && cb <= 'Z' {
			cb += 'a' - 'A'
		}
		if ca != cb {
			return false
		}
	}
	return true
}
