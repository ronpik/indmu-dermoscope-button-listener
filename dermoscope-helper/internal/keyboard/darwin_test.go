//go:build darwin

package keyboard

import (
	"strings"
	"testing"
)

// =============================================================================
// macOS Darwin Keyboard Simulator Tests
// =============================================================================
// These tests verify the macOS AppleScript-based keyboard simulation
// implementation. Full keyboard simulation requires Accessibility permissions.

// Test that the darwin simulator can be created
func TestDarwinSimulator_Creation(t *testing.T) {
	sim, err := NewSimulator()
	if err != nil {
		t.Fatalf("NewSimulator() error = %v, want nil", err)
	}
	if sim == nil {
		t.Fatal("NewSimulator() returned nil simulator")
	}
	defer sim.Close()
}

// Test that Initialize succeeds on macOS
func TestDarwinSimulator_Initialize_Succeeds(t *testing.T) {
	sim, err := NewSimulator()
	if err != nil {
		t.Fatalf("NewSimulator() error = %v", err)
	}
	defer sim.Close()

	// Initialize should succeed (it doesn't actively check permissions)
	if err := sim.Initialize(); err != nil {
		t.Errorf("Initialize() error = %v, want nil", err)
	}
}

// Test that Close succeeds on macOS
func TestDarwinSimulator_Close_Succeeds(t *testing.T) {
	sim, err := NewSimulator()
	if err != nil {
		t.Fatalf("NewSimulator() error = %v", err)
	}

	if err := sim.Close(); err != nil {
		t.Errorf("Close() error = %v, want nil", err)
	}
}

// Test appleScriptKeyCodes mapping has all required keys
func TestDarwinSimulator_AppleScriptKeyCodes_HasRequiredKeys(t *testing.T) {
	requiredKeys := []struct {
		name     string
		keyCode  int
		expected int
	}{
		{"KeyF9", KeyF9, 101},
		{"KeyF10", KeyF10, 109},
		{"KeyF11", KeyF11, 103},
		{"KeyF12", KeyF12, 111},
	}

	for _, tc := range requiredKeys {
		t.Run(tc.name, func(t *testing.T) {
			appleCode, ok := appleScriptKeyCodes[tc.keyCode]
			if !ok {
				t.Errorf("appleScriptKeyCodes missing key for %s (0x%02X)", tc.name, tc.keyCode)
				return
			}
			if appleCode != tc.expected {
				t.Errorf("appleScriptKeyCodes[%s] = %d, want %d", tc.name, appleCode, tc.expected)
			}
		})
	}
}

// Test PressKey returns error for unsupported key code
func TestDarwinSimulator_PressKey_UnsupportedKeyCode_ReturnsError(t *testing.T) {
	sim, err := NewSimulator()
	if err != nil {
		t.Fatalf("NewSimulator() error = %v", err)
	}
	defer sim.Close()

	unsupportedCodes := []int{0, 1, 127, 0xFF, 0x1000}
	for _, code := range unsupportedCodes {
		t.Run("code_0x"+hexString(code), func(t *testing.T) {
			err := sim.PressKey(code)
			if err == nil {
				t.Errorf("PressKey(0x%02X) should return error for unsupported key code", code)
				return
			}
			// Error should mention "unsupported key code"
			if !strings.Contains(err.Error(), "unsupported key code") {
				t.Errorf("PressKey(0x%02X) error = %q, want to contain 'unsupported key code'", code, err.Error())
			}
		})
	}
}

// Test PressKey does not panic for invalid key codes
func TestDarwinSimulator_PressKey_InvalidKeyCode_DoesNotPanic(t *testing.T) {
	sim, err := NewSimulator()
	if err != nil {
		t.Fatalf("NewSimulator() error = %v", err)
	}
	defer sim.Close()

	invalidCodes := []int{-1, -100, 0, 0xFFFF, 0xFFFFFF}
	for _, code := range invalidCodes {
		t.Run("code_"+intToString(code), func(t *testing.T) {
			// Should not panic, error is acceptable
			_ = sim.PressKey(code)
		})
	}
}

// Test that PressKey with valid key codes (F9-F12) does not panic
// Note: Actual key simulation requires Accessibility permissions
// Without permissions, osascript will fail but should not panic
func TestDarwinSimulator_PressKey_ValidKeyCodes_DoesNotPanic(t *testing.T) {
	sim, err := NewSimulator()
	if err != nil {
		t.Fatalf("NewSimulator() error = %v", err)
	}
	defer sim.Close()

	if err := sim.Initialize(); err != nil {
		t.Logf("Initialize() returned: %v", err)
	}

	validKeys := []struct {
		name string
		code int
	}{
		{"F9", KeyF9},
		{"F10", KeyF10},
		{"F11", KeyF11},
		{"F12", KeyF12},
	}

	for _, key := range validKeys {
		t.Run(key.name, func(t *testing.T) {
			// Call PressKey - it may fail if Accessibility permissions are not granted
			// but it should not panic
			err := sim.PressKey(key.code)
			// Log the result but don't fail the test since permissions may not be granted
			if err != nil {
				t.Logf("PressKey(%s) returned: %v (this may be expected without Accessibility permissions)", key.name, err)
			} else {
				t.Logf("PressKey(%s) succeeded", key.name)
			}
		})
	}
}

// Test that multiple simulators can be created concurrently
func TestDarwinSimulator_MultipleInstances(t *testing.T) {
	const numInstances = 3
	simulators := make([]Simulator, numInstances)

	// Create multiple simulators
	for i := 0; i < numInstances; i++ {
		sim, err := NewSimulator()
		if err != nil {
			t.Fatalf("NewSimulator() instance %d error = %v", i, err)
		}
		simulators[i] = sim
	}

	// Close all
	for i, sim := range simulators {
		if err := sim.Close(); err != nil {
			t.Errorf("Close() instance %d error = %v", i, err)
		}
	}
}

// Test Initialize and Close can be called multiple times
func TestDarwinSimulator_InitializeAndClose_MultipleTimes(t *testing.T) {
	sim, err := NewSimulator()
	if err != nil {
		t.Fatalf("NewSimulator() error = %v", err)
	}

	// Initialize multiple times
	for i := 0; i < 3; i++ {
		if err := sim.Initialize(); err != nil {
			t.Errorf("Initialize() call %d error = %v, want nil", i+1, err)
		}
	}

	// Close multiple times
	for i := 0; i < 3; i++ {
		if err := sim.Close(); err != nil {
			t.Errorf("Close() call %d error = %v, want nil", i+1, err)
		}
	}
}

// Test that darwinSimulator struct is correctly typed
func TestDarwinSimulator_ImplementsInterface(t *testing.T) {
	sim, err := NewSimulator()
	if err != nil {
		t.Fatalf("NewSimulator() error = %v", err)
	}
	defer sim.Close()

	// Verify the simulator implements the Simulator interface
	var _ Simulator = sim
}

// =============================================================================
// Helper functions
// =============================================================================

func hexString(n int) string {
	const hexDigits = "0123456789ABCDEF"
	if n == 0 {
		return "0"
	}
	result := ""
	for n > 0 {
		result = string(hexDigits[n%16]) + result
		n /= 16
	}
	return result
}

func intToString(n int) string {
	if n == 0 {
		return "0"
	}
	negative := n < 0
	if negative {
		n = -n
	}
	result := ""
	for n > 0 {
		result = string(rune('0'+n%10)) + result
		n /= 10
	}
	if negative {
		result = "-" + result
	}
	return result
}
