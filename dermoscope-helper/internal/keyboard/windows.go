//go:build windows

// Package keyboard provides cross-platform keyboard simulation.
package keyboard

import (
	"time"

	"github.com/micmonay/keybd_event"
)

// windowsSimulator implements Simulator using the Windows SendInput API
// via the keybd_event library.
type windowsSimulator struct {
	kb keybd_event.KeyBonding
}

// newPlatformSimulator creates a Windows-specific keyboard simulator.
func newPlatformSimulator() (Simulator, error) {
	kb, err := keybd_event.NewKeyBonding()
	if err != nil {
		return nil, err
	}
	return &windowsSimulator{kb: kb}, nil
}

// Initialize sets up the keyboard simulator.
// On Windows, keybd_event requires a small delay on some systems
// before it can reliably send keystrokes.
func (w *windowsSimulator) Initialize() error {
	// keybd_event requires a 2 second delay on some Windows versions
	// to ensure the keyboard simulation is ready
	time.Sleep(2 * time.Second)
	return nil
}

// PressKey simulates a key press and release for the given key code.
// The keybd_event library handles both the press and release as a single operation.
func (w *windowsSimulator) PressKey(keyCode int) error {
	w.kb.SetKeys(keyCode)
	err := w.kb.Launching()
	if err != nil {
		return err
	}
	return nil
}

// Close cleans up resources used by the keyboard simulator.
// For the Windows implementation, no cleanup is required.
func (w *windowsSimulator) Close() error {
	return nil
}
