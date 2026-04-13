// Package tray provides system tray integration for the dermoscope helper.
package tray

import (
	"sync"

	"github.com/getlantern/systray"
)

// TrayApp manages the system tray icon and menu
type TrayApp struct {
	onExit       func()
	onToggle     func()
	statusItem   *systray.MenuItem
	toggleItem   *systray.MenuItem
	currentState State
	statusText   string
	mu           sync.Mutex

	// ready signals when the tray is ready for updates
	ready chan struct{}
}

// State represents the tray icon state
type State int

const (
	StateDisconnected State = iota
	StateConnected
	StateMonitoring
	StateError
)

// String returns the string representation of the state
func (s State) String() string {
	switch s {
	case StateDisconnected:
		return "disconnected"
	case StateConnected:
		return "connected"
	case StateMonitoring:
		return "monitoring"
	case StateError:
		return "error"
	default:
		return "unknown"
	}
}

// stateToStatusText returns the default status text for a state
func stateToStatusText(state State) string {
	switch state {
	case StateDisconnected:
		return "No device connected"
	case StateConnected:
		return "Device connected"
	case StateMonitoring:
		return "Monitoring for button press"
	case StateError:
		return "Error - check logs"
	default:
		return "Unknown state"
	}
}

// NewTrayApp creates a new system tray application
func NewTrayApp(onExit func(), onToggle func()) *TrayApp {
	return &TrayApp{
		onExit:       onExit,
		onToggle:     onToggle,
		currentState: StateDisconnected,
		statusText:   stateToStatusText(StateDisconnected),
		ready:        make(chan struct{}),
	}
}

// Run starts the system tray (blocks until quit)
// This must be called from the main goroutine on some platforms
func (t *TrayApp) Run() {
	systray.Run(t.onReady, t.onQuit)
}

// onReady is called when the systray is ready
func (t *TrayApp) onReady() {
	// Set initial icon and tooltip
	systray.SetIcon(GetIcon(StateDisconnected))
	systray.SetTitle("Dermoscope Helper")
	systray.SetTooltip("Dermoscope Button Helper")

	// Create menu items
	t.statusItem = systray.AddMenuItem(t.statusText, "Current status")
	t.statusItem.Disable() // Status is informational only

	systray.AddSeparator()

	t.toggleItem = systray.AddMenuItem("Start Monitoring", "Start/Stop monitoring")

	systray.AddSeparator()

	exitItem := systray.AddMenuItem("Exit", "Exit the application")

	// Signal that tray is ready
	close(t.ready)

	// Handle menu item clicks in a goroutine
	go t.handleMenuClicks(exitItem)
}

// handleMenuClicks handles menu item click events
func (t *TrayApp) handleMenuClicks(exitItem *systray.MenuItem) {
	for {
		select {
		case <-t.toggleItem.ClickedCh:
			if t.onToggle != nil {
				t.onToggle()
			}
		case <-exitItem.ClickedCh:
			t.Quit()
			return
		}
	}
}

// onQuit is called when the systray is about to quit
func (t *TrayApp) onQuit() {
	if t.onExit != nil {
		t.onExit()
	}
}

// SetState updates the tray icon and status text
func (t *TrayApp) SetState(state State) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.currentState = state
	t.statusText = stateToStatusText(state)

	// Wait for tray to be ready before updating
	select {
	case <-t.ready:
		// Tray is ready, update it
		systray.SetIcon(GetIcon(state))
		systray.SetTooltip("Dermoscope Helper - " + t.statusText)

		if t.statusItem != nil {
			t.statusItem.SetTitle(t.statusText)
		}

		// Update toggle item text based on state
		if t.toggleItem != nil {
			if state == StateMonitoring {
				t.toggleItem.SetTitle("Stop Monitoring")
			} else {
				t.toggleItem.SetTitle("Start Monitoring")
			}
		}
	default:
		// Tray not ready yet, state will be applied when ready
	}
}

// GetState returns the current state
func (t *TrayApp) GetState() State {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.currentState
}

// SetStatus sets the status menu item text
func (t *TrayApp) SetStatus(text string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.statusText = text

	// Wait for tray to be ready before updating
	select {
	case <-t.ready:
		if t.statusItem != nil {
			t.statusItem.SetTitle(text)
		}
	default:
		// Tray not ready yet
	}
}

// ShowNotification displays a system notification
// Note: This is a no-op for now as systray doesn't support notifications directly
// Platform-specific notification code would be needed
func (t *TrayApp) ShowNotification(title, message string) {
	// TODO: Implement platform-specific notifications if needed
	// For now, this is a no-op
}

// Quit triggers the tray to quit
func (t *TrayApp) Quit() {
	systray.Quit()
}
