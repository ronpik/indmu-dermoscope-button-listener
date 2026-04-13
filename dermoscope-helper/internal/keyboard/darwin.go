//go:build darwin

// Package keyboard provides cross-platform keyboard simulation.
package keyboard

import (
	"fmt"
	"os/exec"
)

// appleScriptKeyCodes maps Windows virtual key codes to AppleScript key codes.
// AppleScript key codes differ from Windows VK codes.
var appleScriptKeyCodes = map[int]int{
	KeyF9:  101, // F9 in AppleScript
	KeyF10: 109, // F10 in AppleScript
	KeyF11: 103, // F11 in AppleScript
	KeyF12: 111, // F12 in AppleScript
}

// darwinSimulator implements Simulator using AppleScript via osascript.
// This approach requires Accessibility permissions to be granted in
// System Preferences > Security & Privacy > Accessibility.
type darwinSimulator struct{}

// newPlatformSimulator creates a macOS-specific keyboard simulator.
func newPlatformSimulator() (Simulator, error) {
	return &darwinSimulator{}, nil
}

// Initialize sets up the keyboard simulator.
// On macOS, Accessibility permissions are checked at runtime when the first
// key simulation occurs. The system will prompt the user to grant permissions
// if they haven't been granted yet.
func (d *darwinSimulator) Initialize() error {
	// Accessibility permissions are checked at runtime by the system.
	// We don't need to explicitly check here - the osascript call will
	// fail with an appropriate error if permissions are not granted.
	return nil
}

// PressKey simulates a key press and release for the given key code.
// Uses AppleScript via osascript to simulate the keystroke.
// Requires Accessibility permissions in System Preferences.
func (d *darwinSimulator) PressKey(keyCode int) error {
	appleKeyCode, ok := appleScriptKeyCodes[keyCode]
	if !ok {
		return fmt.Errorf("unsupported key code: 0x%02X", keyCode)
	}

	// Use AppleScript to simulate the key press via System Events
	script := fmt.Sprintf(`tell application "System Events" to key code %d`, appleKeyCode)
	cmd := exec.Command("osascript", "-e", script)

	output, err := cmd.CombinedOutput()
	if err != nil {
		// Provide a helpful error message that includes the osascript output
		if len(output) > 0 {
			return fmt.Errorf("osascript failed: %w (output: %s)", err, string(output))
		}
		return fmt.Errorf("osascript failed: %w", err)
	}

	return nil
}

// Close cleans up resources used by the keyboard simulator.
// For the macOS implementation, no cleanup is required.
func (d *darwinSimulator) Close() error {
	return nil
}
