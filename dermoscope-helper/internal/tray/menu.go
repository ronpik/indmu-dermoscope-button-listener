package tray

import "github.com/getlantern/systray"

// MenuHandler handles tray menu interactions
type MenuHandler struct {
	tray *TrayApp
}

// NewMenuHandler creates a new menu handler
func NewMenuHandler(tray *TrayApp) *MenuHandler {
	return &MenuHandler{
		tray: tray,
	}
}

// SetupMenu initializes the tray menu items
// Note: This is called internally by TrayApp.onReady
// The menu structure is:
// - Status (disabled, shows current state)
// - Separator
// - Start/Stop Monitoring
// - Separator
// - Exit
func (m *MenuHandler) SetupMenu() {
	// Menu setup is handled in TrayApp.onReady
	// This method is provided for compatibility and future extensions
}

// HandleToggle handles the Start/Stop menu item click
func (m *MenuHandler) HandleToggle() {
	if m.tray.onToggle != nil {
		m.tray.onToggle()
	}
}

// HandleExit handles the Exit menu item click
func (m *MenuHandler) HandleExit() {
	if m.tray.onExit != nil {
		m.tray.onExit()
	}
	systray.Quit()
}

// UpdateStatus updates the status menu item text
func (m *MenuHandler) UpdateStatus(text string) {
	m.tray.SetStatus(text)
}

// UpdateToggleText updates the toggle menu item text based on monitoring state
func (m *MenuHandler) UpdateToggleText(isMonitoring bool) {
	if m.tray.toggleItem == nil {
		return
	}

	if isMonitoring {
		m.tray.toggleItem.SetTitle("Stop Monitoring")
	} else {
		m.tray.toggleItem.SetTitle("Start Monitoring")
	}
}
