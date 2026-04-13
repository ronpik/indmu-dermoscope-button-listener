package keyboard

import (
	"runtime"
	"testing"
)

// Test NewSimulator creates simulator on current platform
func TestNewSimulator_CreatesSimulatorOnCurrentPlatform(t *testing.T) {
	sim, err := NewSimulator()
	if err != nil {
		t.Fatalf("NewSimulator() error = %v, want nil", err)
	}
	if sim == nil {
		t.Fatal("NewSimulator() returned nil simulator")
	}
}

// Test NewSimulator returns non-nil Simulator that implements interface
func TestNewSimulator_ReturnsValidSimulatorInterface(t *testing.T) {
	sim, err := NewSimulator()
	if err != nil {
		t.Fatalf("NewSimulator() error = %v", err)
	}

	// Verify it implements the Simulator interface by calling methods
	// Initialize should not error on any platform
	if err := sim.Initialize(); err != nil {
		t.Logf("Initialize() returned: %v", err)
	}

	// Close should not error
	if err := sim.Close(); err != nil {
		t.Errorf("Close() error = %v, want nil", err)
	}
}

// Test key code constants have correct values
func TestKeyCodeConstants_HaveCorrectValues(t *testing.T) {
	tests := []struct {
		name     string
		constant int
		expected int
	}{
		{"KeyF9", KeyF9, 0x78},
		{"KeyF10", KeyF10, 0x79},
		{"KeyF11", KeyF11, 0x7A},
		{"KeyF12", KeyF12, 0x7B},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.constant != tt.expected {
				t.Errorf("%s = 0x%02X, want 0x%02X", tt.name, tt.constant, tt.expected)
			}
		})
	}
}

// Test key code constants decimal values (for verification)
func TestKeyCodeConstants_DecimalValues(t *testing.T) {
	tests := []struct {
		name     string
		constant int
		expected int
	}{
		{"KeyF9", KeyF9, 120},
		{"KeyF10", KeyF10, 121},
		{"KeyF11", KeyF11, 122},
		{"KeyF12", KeyF12, 123},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.constant != tt.expected {
				t.Errorf("%s = %d, want %d", tt.name, tt.constant, tt.expected)
			}
		})
	}
}

// Test key codes are sequential (F9, F10, F11, F12)
func TestKeyCodeConstants_AreSequential(t *testing.T) {
	if KeyF10 != KeyF9+1 {
		t.Errorf("KeyF10 (%d) should be KeyF9 (%d) + 1", KeyF10, KeyF9)
	}
	if KeyF11 != KeyF10+1 {
		t.Errorf("KeyF11 (%d) should be KeyF10 (%d) + 1", KeyF11, KeyF10)
	}
	if KeyF12 != KeyF11+1 {
		t.Errorf("KeyF12 (%d) should be KeyF11 (%d) + 1", KeyF12, KeyF11)
	}
}

// Test Initialize succeeds on non-Windows platforms
func TestInitialize_NonWindows_Succeeds(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping non-Windows test on Windows")
	}

	sim, err := NewSimulator()
	if err != nil {
		t.Fatalf("NewSimulator() error = %v", err)
	}
	defer sim.Close()

	// Initialize should succeed on all platforms
	if err := sim.Initialize(); err != nil {
		t.Errorf("Initialize() error = %v, want nil", err)
	}
}

// Test Close succeeds on all platforms
func TestClose_Succeeds(t *testing.T) {
	sim, err := NewSimulator()
	if err != nil {
		t.Fatalf("NewSimulator() error = %v", err)
	}

	// Close should succeed
	if err := sim.Close(); err != nil {
		t.Errorf("Close() error = %v, want nil", err)
	}
}

// Test Close can be called multiple times without error
func TestClose_MultipleCallsSucceed(t *testing.T) {
	sim, err := NewSimulator()
	if err != nil {
		t.Fatalf("NewSimulator() error = %v", err)
	}

	// Multiple close calls should succeed
	for i := 0; i < 3; i++ {
		if err := sim.Close(); err != nil {
			t.Errorf("Close() call %d error = %v, want nil", i+1, err)
		}
	}
}

// Test PressKey with invalid key code does not panic
func TestPressKey_InvalidKeyCode_DoesNotPanic(t *testing.T) {
	sim, err := NewSimulator()
	if err != nil {
		t.Fatalf("NewSimulator() error = %v", err)
	}
	defer sim.Close()

	// Should not panic even with invalid key codes
	invalidCodes := []int{0, -1, 255, 1000, 0xFFFF}
	for _, code := range invalidCodes {
		t.Run("code_"+string(rune(code)), func(t *testing.T) {
			// Simply calling PressKey should not panic
			// Error is acceptable
			_ = sim.PressKey(code)
		})
	}
}

// Test that multiple simulators can be created
func TestNewSimulator_MultipleSimulators(t *testing.T) {
	sim1, err := NewSimulator()
	if err != nil {
		t.Fatalf("NewSimulator() first call error = %v", err)
	}
	defer sim1.Close()

	sim2, err := NewSimulator()
	if err != nil {
		t.Fatalf("NewSimulator() second call error = %v", err)
	}
	defer sim2.Close()

	// Both should be valid
	if sim1 == nil || sim2 == nil {
		t.Error("Both simulators should be non-nil")
	}
}

// Test Initialize can be called after Close
func TestSimulator_InitializeAfterClose(t *testing.T) {
	sim, err := NewSimulator()
	if err != nil {
		t.Fatalf("NewSimulator() error = %v", err)
	}

	// Initialize, close, then initialize again should work
	if err := sim.Initialize(); err != nil {
		t.Logf("First Initialize() returned: %v", err)
	}

	if err := sim.Close(); err != nil {
		t.Errorf("Close() error = %v", err)
	}

	// Re-initializing after close may or may not work depending on platform
	// but it should not panic
	_ = sim.Initialize()
}
