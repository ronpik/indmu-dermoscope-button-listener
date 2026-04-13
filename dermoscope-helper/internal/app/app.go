// Package app provides the main application logic for the dermoscope helper.
package app

import (
	"sync"

	"github.com/rs/zerolog"
	"github.com/trichoai/dermoscope-helper/internal/keyboard"
	"github.com/trichoai/dermoscope-helper/internal/tray"
	"github.com/trichoai/dermoscope-helper/internal/usb"
)

// App is the main application coordinator
type App struct {
	registry  *usb.ProfileRegistry  // Device profile registry
	deviceMgr *usb.DeviceManager
	monitor   *usb.Monitor
	keyboard  keyboard.Simulator
	tray      *tray.TrayApp
	logger    zerolog.Logger
	state     *StateMachine
	config    *Config

	stopChan chan struct{}
	wg       sync.WaitGroup
}

// New creates a new application instance with profile support
func New(config *Config, logger zerolog.Logger) (*App, error) {
	// Initialize profile registry with built-in profiles
	registry := usb.NewProfileRegistry()

	// Validate all profiles
	if errs := registry.Validate(); len(errs) > 0 {
		logger.Warn().Errs("validation_errors", errs).Msg("Profile validation warnings")
	}

	// Create device manager with registry
	deviceMgr := usb.NewDeviceManager(registry)

	// Create keyboard simulator
	kb, err := keyboard.NewSimulator()
	if err != nil {
		return nil, err
	}

	app := &App{
		registry:  registry,
		deviceMgr: deviceMgr,
		keyboard:  kb,
		logger:    logger,
		config:    config,
		stopChan:  make(chan struct{}),
	}

	// Create state machine with change callback
	app.state = NewStateMachine(app.onStateChange)

	// Create tray app
	app.tray = tray.NewTrayApp(app.Stop, app.toggleMonitoring)

	return app, nil
}

// Run starts the application (blocking)
func (a *App) Run() error {
	a.logger.Info().Msg("Application starting")

	// Initialize keyboard simulator
	if err := a.keyboard.Initialize(); err != nil {
		a.logger.Error().Err(err).Msg("Failed to initialize keyboard simulator")
		return err
	}

	// Transition to searching state
	a.state.Transition(StateSearching)

	// Start device search and monitoring loop
	a.wg.Add(1)
	go a.mainLoop()

	// Run tray (blocks until exit)
	// systray.Run blocks the calling goroutine and handles the event loop
	a.tray.Run()

	// Wait for goroutines to finish
	a.wg.Wait()

	// Cleanup
	a.cleanup()

	a.logger.Info().Msg("Application stopped")
	return nil
}

// Stop gracefully shuts down the application
func (a *App) Stop() {
	a.logger.Info().Msg("Stopping application")
	a.state.Transition(StateStopping)
	close(a.stopChan)
}

// GetRegistry returns the profile registry
func (a *App) GetRegistry() *usb.ProfileRegistry {
	return a.registry
}

// mainLoop runs the main application loop
func (a *App) mainLoop() {
	defer a.wg.Done()

	for {
		select {
		case <-a.stopChan:
			return
		default:
			switch a.state.Current() {
			case StateSearching:
				a.searchForDevice()
			case StateMonitoring:
				a.monitorDevice()
			case StateDisconnected:
				a.waitForReconnect()
			}
		}
	}
}

// searchForDevice searches for a supported dermoscope device
func (a *App) searchForDevice() {
	a.logger.Debug().Msg("Searching for device...")

	info, err := a.deviceMgr.FindDevice()
	if err != nil {
		a.logger.Debug().Err(err).Msg("No device found")
		a.tray.SetState(tray.StateDisconnected)
		// Wait before retrying
		select {
		case <-a.stopChan:
			return
		default:
			// Small delay before retry
		}
		return
	}

	a.logger.Info().
		Str("profile", info.Profile.ID).
		Str("device", info.Profile.Name).
		Str("vidpid", formatVIDPID(info.VendorID, info.ProductID)).
		Msg("Device found")

	// Claim interface
	a.state.Transition(StateClaiming)
	if err := a.deviceMgr.ClaimInterface(); err != nil {
		a.logger.Error().Err(err).Msg("Failed to claim interface")
		a.state.Transition(StateSearching)
		return
	}

	// Create monitor
	a.monitor = usb.NewMonitor(a.deviceMgr, a.config.DebounceMs)
	if err := a.monitor.Start(); err != nil {
		a.logger.Error().Err(err).Msg("Failed to start monitor")
		a.state.Transition(StateSearching)
		return
	}

	a.state.Transition(StateMonitoring)
	a.tray.SetState(tray.StateMonitoring)
}

// monitorDevice monitors the connected device for button events
func (a *App) monitorDevice() {
	select {
	case <-a.stopChan:
		return
	case event := <-a.monitor.Events():
		if event.Pressed {
			a.handleButtonPress(event)
		}
	case err := <-a.monitor.Errors():
		a.handleDeviceDisconnect(err)
	}
}

// waitForReconnect waits for device reconnection
func (a *App) waitForReconnect() {
	a.logger.Debug().Msg("Waiting for device reconnection...")
	a.tray.SetState(tray.StateDisconnected)

	// Clean up current state
	if a.monitor != nil {
		a.monitor.Stop()
		a.monitor = nil
	}
	a.deviceMgr.ReleaseInterface()
	a.deviceMgr.Close()

	// Wait before searching again
	select {
	case <-a.stopChan:
		return
	default:
		a.state.Transition(StateSearching)
	}
}

// handleButtonPress is called when a button press is detected
func (a *App) handleButtonPress(event usb.ButtonEvent) {
	a.logger.Info().
		Str("device", event.DeviceID).
		Msg("Button pressed - sending F9")

	if err := a.keyboard.PressKey(keyboard.KeyF9); err != nil {
		a.logger.Error().Err(err).Msg("Failed to simulate F9 keypress")
	}
}

// handleDeviceDisconnect is called when the device disconnects
func (a *App) handleDeviceDisconnect(err error) {
	a.logger.Warn().Err(err).Msg("Device disconnected or error")
	a.state.Transition(StateDisconnected)
}

// toggleMonitoring toggles the monitoring state
func (a *App) toggleMonitoring() {
	// TODO: Implement toggle functionality
}

// onStateChange is called when the state changes
func (a *App) onStateChange(old, new AppState) {
	a.logger.Debug().
		Str("from", old.String()).
		Str("to", new.String()).
		Msg("State transition")
}

// cleanup releases all resources
func (a *App) cleanup() {
	if a.monitor != nil {
		a.monitor.Stop()
	}
	if a.keyboard != nil {
		a.keyboard.Close()
	}
	if a.deviceMgr != nil {
		a.deviceMgr.Close()
	}
}

// formatVIDPID formats vendor and product IDs as hex string
func formatVIDPID(vid, pid uint16) string {
	return formatHex(vid) + ":" + formatHex(pid)
}

// formatHex formats a uint16 as 4-digit hex string
func formatHex(v uint16) string {
	const hexChars = "0123456789ABCDEF"
	return string([]byte{
		hexChars[(v>>12)&0xF],
		hexChars[(v>>8)&0xF],
		hexChars[(v>>4)&0xF],
		hexChars[v&0xF],
	})
}
