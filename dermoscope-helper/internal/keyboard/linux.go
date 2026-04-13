//go:build linux

// Package keyboard provides cross-platform keyboard simulation.
package keyboard

import (
	"fmt"
	"os/exec"
)

type linuxSimulator struct{}

func newPlatformSimulator() (Simulator, error) {
	// Check if xdotool is installed
	_, err := exec.LookPath("xdotool")
	if err != nil {
		return nil, fmt.Errorf("xdotool not found: %w", err)
	}
	return &linuxSimulator{}, nil
}

func (l *linuxSimulator) Initialize() error {
	return nil
}

func (l *linuxSimulator) PressKey(keyCode int) error {
	// Map keyCode to X11 key name
	keyName := keyCodeToX11Name(keyCode)
	if keyName == "" {
		return fmt.Errorf("unsupported key code: %d", keyCode)
	}

	cmd := exec.Command("xdotool", "key", keyName)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("xdotool failed: %w (output: %s)", err, string(output))
	}
	return nil
}

func (l *linuxSimulator) Close() error {
	return nil
}

func keyCodeToX11Name(keyCode int) string {
	switch keyCode {
	case KeyF9:
		return "F9"
	case KeyF10:
		return "F10"
	case KeyF11:
		return "F11"
	case KeyF12:
		return "F12"
	default:
		return ""
	}
}
