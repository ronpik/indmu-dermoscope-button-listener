# Dermoscope Button Helper - Go Implementation Specification

**Version:** 1.1
**Date:** February 2025
**Related:** [DESIGN.md](./DESIGN.md)

---

## Table of Contents

1. [Overview](#1-overview)
2. [Technical Specifications](#2-technical-specifications)
3. [Project Structure](#3-project-structure)
4. [Module Specifications](#4-module-specifications)
5. [Device Profile System](#5-device-profile-system)
6. [Build & Distribution](#6-build--distribution)
7. [Testing Strategy](#7-testing-strategy)
8. [Implementation Phases](#8-implementation-phases)

---

## 1. Overview

### 1.1 Purpose

This document specifies the Go implementation of the Dermoscope Button Helper application as designed in [DESIGN.md](./DESIGN.md).

### 1.2 Summary

A native Go application that:
- Monitors supported dermoscope devices via USB interrupt endpoint for button press events
- Supports multiple device models through a profile system (one device at a time)
- Simulates F9 keypress when the button is pressed
- Runs as a system tray application with status indicators
- Auto-reconnects when the device is disconnected/reconnected
- Cross-compiles from macOS to Windows

### 1.3 Device Protocol Reference

Device-specific values (VID/PID, button patterns) are defined per-profile in Section 5.
Common USB constants used across all UVC-based dermoscopes:

| Constant | Value | Description |
|----------|-------|-------------|
| INTERFACE_CLASS | 0x0E | Video (UVC) |
| INTERFACE_SUBCLASS | 0x01 | Video Control |
| ENDPOINT_TYPE | 0x03 | Interrupt |
| ENDPOINT_DIRECTION | 0x80 | IN |

**Example (HT-B30S profile):**

| Field | Value | Description |
|-------|-------|-------------|
| VENDOR_ID | 0xAB02 | Sonix Technology |
| PRODUCT_ID | 0xAB01 | HT-B30S Dermoscope |
| BUTTON_PRESS | [0x02, 0x01, 0x00, 0x00] | Press event |
| BUTTON_RELEASE | [0x02, 0x01, 0x00, 0x01] | Release event |

See [DESIGN.md Section 10](./DESIGN.md#10-adding-new-device-profiles) for how to discover these values for new devices.

---

## 2. Technical Specifications

### 2.1 Language & Version

| Specification | Value |
|---------------|-------|
| Language | Go 1.21+ |
| Module Name | `github.com/trichoai/dermoscope-helper` |
| Binary Name | `dermoscope-helper` (.exe on Windows) |

### 2.2 Dependencies

| Package | Purpose | Version |
|---------|---------|---------|
| `github.com/google/gousb` | USB device access | latest |
| `github.com/getlantern/systray` | System tray integration | latest |
| `github.com/micmonay/keybd_event` | Cross-platform keyboard simulation | latest |
| `github.com/rs/zerolog` | Structured logging | latest |
| `gopkg.in/natefinch/lumberjack.v2` | Log rotation | latest |

### 2.3 Build Targets

| Platform | GOOS | GOARCH | Output |
|----------|------|--------|--------|
| Windows 64-bit | windows | amd64 | dermoscope-helper.exe |
| macOS Intel | darwin | amd64 | dermoscope-helper-mac |
| macOS Apple Silicon | darwin | arm64 | dermoscope-helper-mac-arm64 |
| Linux 64-bit | linux | amd64 | dermoscope-helper-linux |

### 2.4 Configuration Constants

```go
const (
    // USB Constants (shared across all devices)
    VideoControlClass    = 0x0E  // UVC Video class
    VideoControlSubclass = 0x01  // Video Control subclass
    InterruptEndpointType = 0x03
    EndpointDirectionIn   = 0x80

    // Timing
    ReadTimeoutMs     = 500
    DebounceMs        = 250
    ReconnectDelayMs  = 2000
    DevicePollMs      = 1000

    // Keyboard
    KeyF9 = 0x78  // F9 virtual key code
)

// Device-specific values (VID/PID, button patterns) are defined
// in Device Profiles - see Section 5: Device Profile System
```

---

## 3. Project Structure

```
dermoscope-helper/
├── cmd/
│   └── dermoscope-helper/
│       └── main.go              # Application entry point
├── internal/
│   ├── usb/
│   │   ├── device.go            # USB device management
│   │   ├── monitor.go           # Interrupt endpoint monitoring
│   │   ├── profiles.go          # Device profile registry
│   │   └── types.go             # USB-related types
│   ├── keyboard/
│   │   ├── simulator.go         # Cross-platform keyboard simulation
│   │   ├── windows.go           # Windows-specific implementation
│   │   ├── darwin.go            # macOS-specific implementation
│   │   └── linux.go             # Linux-specific implementation
│   ├── tray/
│   │   ├── tray.go              # System tray management
│   │   ├── icons.go             # Embedded icon resources
│   │   └── menu.go              # Tray menu handling
│   ├── app/
│   │   ├── app.go               # Main application logic
│   │   ├── state.go             # State machine implementation
│   │   └── config.go            # Configuration management
│   └── logger/
│       └── logger.go            # Logging setup
├── assets/
│   ├── icon-connected.ico       # Tray icon: device connected
│   ├── icon-disconnected.ico    # Tray icon: device disconnected
│   ├── icon-monitoring.ico      # Tray icon: actively monitoring
│   └── icon-error.ico           # Tray icon: error state
├── scripts/
│   ├── build-all.sh             # Cross-platform build script
│   └── build-windows.sh         # Windows-only build
├── go.mod
├── go.sum
├── Makefile
└── README.md
```

---

## 4. Module Specifications

### 4.1 USB Module (`internal/usb/`)

#### 4.1.1 types.go

```go
package usb

import "time"

// DeviceState represents the current state of the USB device
type DeviceState int

const (
    StateDisconnected DeviceState = iota
    StateConnected
    StateMonitoring
    StateError
)

// DeviceProfile defines the USB characteristics of a supported dermoscope device
type DeviceProfile struct {
    // Identification
    ID           string // Unique profile identifier (e.g., "ht-b30s")
    Name         string // Human-readable name
    Manufacturer string // Device manufacturer

    // USB Identification
    VendorID  uint16 // USB Vendor ID
    ProductID uint16 // USB Product ID

    // Interface Configuration
    InterfaceClass    uint8 // USB interface class (0x0E for Video)
    InterfaceSubclass uint8 // USB interface subclass (0x01 for Video Control)

    // Button Event Patterns
    ButtonPressPattern   []byte // Byte pattern indicating button press
    ButtonReleasePattern []byte // Byte pattern indicating button release

    // Metadata
    Notes string // Optional notes about the device
}

// ButtonEvent represents a parsed button event
type ButtonEvent struct {
    Pressed   bool
    Timestamp time.Time
    RawData   []byte
    DeviceID  string // Profile ID of the device that generated the event
}

// DeviceInfo contains information about a connected device
type DeviceInfo struct {
    Profile      *DeviceProfile // Matched profile
    VendorID     uint16
    ProductID    uint16
    Manufacturer string
    Product      string
    SerialNumber string
}

// MatchesProfile checks if a VID/PID matches this profile
func (p *DeviceProfile) MatchesProfile(vid, pid uint16) bool {
    return p.VendorID == vid && p.ProductID == pid
}

// MatchesButtonPress checks if data matches the button press pattern
func (p *DeviceProfile) MatchesButtonPress(data []byte) bool {
    return bytesEqual(data, p.ButtonPressPattern)
}

// MatchesButtonRelease checks if data matches the button release pattern
func (p *DeviceProfile) MatchesButtonRelease(data []byte) bool {
    return bytesEqual(data, p.ButtonReleasePattern)
}

func bytesEqual(a, b []byte) bool {
    if len(a) != len(b) {
        return false
    }
    for i := range a {
        if a[i] != b[i] {
            return false
        }
    }
    return true
}
```

#### 4.1.2 device.go

```go
package usb

import (
    "sync"

    "github.com/google/gousb"
)

// DeviceManager handles USB device connection and lifecycle
type DeviceManager struct {
    ctx       *gousb.Context
    device    *gousb.Device
    intf      *gousb.Interface
    endpoint  *gousb.InEndpoint
    state     DeviceState
    profile   *DeviceProfile  // The matched profile for this device
    registry  *ProfileRegistry
    mu        sync.RWMutex
}

// NewDeviceManager creates a new device manager with profile support
func NewDeviceManager(registry *ProfileRegistry) *DeviceManager

// FindDevice searches for any supported dermoscope device
// Iterates through all connected USB devices and matches against registered profiles
func (dm *DeviceManager) FindDevice() (*DeviceInfo, error)

// FindDeviceByProfile searches for a specific device profile
func (dm *DeviceManager) FindDeviceByProfile(profileID string) (*DeviceInfo, error)

// GetProfile returns the matched profile for the connected device
func (dm *DeviceManager) GetProfile() *DeviceProfile

// ClaimInterface claims the Video Control interface based on profile settings
func (dm *DeviceManager) ClaimInterface() error

// ReleaseInterface releases the claimed interface
func (dm *DeviceManager) ReleaseInterface() error

// GetEndpoint returns the interrupt IN endpoint
func (dm *DeviceManager) GetEndpoint() (*gousb.InEndpoint, error)

// GetState returns the current device state
func (dm *DeviceManager) GetState() DeviceState

// Close releases all resources
func (dm *DeviceManager) Close() error
```

#### 4.1.3 profiles.go

```go
package usb

import "sync"

// ProfileRegistry maintains the list of supported device profiles
type ProfileRegistry struct {
    profiles []DeviceProfile
    mu       sync.RWMutex
}

// NewProfileRegistry creates a registry with built-in profiles
func NewProfileRegistry() *ProfileRegistry {
    return &ProfileRegistry{
        profiles: builtInProfiles,
    }
}

// builtInProfiles contains all known device profiles
// Add new profiles here when supporting new devices
var builtInProfiles = []DeviceProfile{
    {
        ID:                   "ht-b30s",
        Name:                 "HT-B30S Dermoscope",
        Manufacturer:         "Sonix Technology",
        VendorID:             0xAB02,
        ProductID:            0xAB01,
        InterfaceClass:       0x0E, // Video
        InterfaceSubclass:    0x01, // Video Control
        ButtonPressPattern:   []byte{0x02, 0x01, 0x00, 0x00},
        ButtonReleasePattern: []byte{0x02, 0x01, 0x00, 0x01},
        Notes:                "Primary clinic device - verified working on Windows",
    },
    // === ADD NEW DEVICE PROFILES BELOW THIS LINE ===
    //
    // Example:
    // {
    //     ID:                   "device-model",
    //     Name:                 "Device Model Name",
    //     Manufacturer:         "Manufacturer",
    //     VendorID:             0x1234,
    //     ProductID:            0x5678,
    //     InterfaceClass:       0x0E,
    //     InterfaceSubclass:    0x01,
    //     ButtonPressPattern:   []byte{0x02, 0x01, 0x00, 0x00},
    //     ButtonReleasePattern: []byte{0x02, 0x01, 0x00, 0x01},
    //     Notes:                "Added YYYY-MM-DD",
    // },
}

// GetAll returns all registered profiles
func (r *ProfileRegistry) GetAll() []DeviceProfile

// GetByID returns a profile by its ID
func (r *ProfileRegistry) GetByID(id string) (*DeviceProfile, bool)

// FindMatchingProfile finds a profile that matches the given VID/PID
func (r *ProfileRegistry) FindMatchingProfile(vid, pid uint16) (*DeviceProfile, bool)

// Register adds a new profile to the registry (for runtime additions)
func (r *ProfileRegistry) Register(profile DeviceProfile) error

// Count returns the number of registered profiles
func (r *ProfileRegistry) Count() int
```

#### 4.1.4 monitor.go

```go
package usb

import "time"

// Monitor continuously reads from the interrupt endpoint
type Monitor struct {
    dm          *DeviceManager
    eventChan   chan ButtonEvent
    errorChan   chan error
    stopChan    chan struct{}
    debounceMs  int
    lastPressMs int64
}

// NewMonitor creates a new interrupt endpoint monitor
func NewMonitor(dm *DeviceManager, debounceMs int) *Monitor

// Start begins monitoring in a goroutine
func (m *Monitor) Start() error

// Stop stops the monitoring goroutine
func (m *Monitor) Stop()

// Events returns the channel for button events
func (m *Monitor) Events() <-chan ButtonEvent

// Errors returns the channel for errors
func (m *Monitor) Errors() <-chan error

// parseEvent parses raw USB data into a ButtonEvent using the device's profile
func (m *Monitor) parseEvent(data []byte) (*ButtonEvent, error) {
    profile := m.dm.GetProfile()
    if profile == nil {
        return nil, errors.New("no profile set for device")
    }

    event := &ButtonEvent{
        Timestamp: time.Now(),
        RawData:   data,
        DeviceID:  profile.ID,
    }

    // Match against profile patterns
    if profile.MatchesButtonPress(data) {
        event.Pressed = true
        return event, nil
    }

    if profile.MatchesButtonRelease(data) {
        event.Pressed = false
        return event, nil
    }

    // Unknown event pattern
    return nil, fmt.Errorf("unknown event pattern: %v", data)
}
```

### 4.2 Keyboard Module (`internal/keyboard/`)

#### 4.2.1 simulator.go

```go
package keyboard

// Simulator provides cross-platform keyboard simulation
type Simulator interface {
    // PressKey simulates a key press and release
    PressKey(keyCode int) error

    // Initialize sets up the keyboard simulator
    Initialize() error

    // Close cleans up resources
    Close() error
}

// NewSimulator returns a platform-specific keyboard simulator
func NewSimulator() (Simulator, error)

// Key codes
const (
    KeyF9  = 0x78
    KeyF10 = 0x79
    KeyF11 = 0x7A
    KeyF12 = 0x7B
)
```

#### 4.2.2 windows.go (build tag: windows)

```go
//go:build windows

package keyboard

import "github.com/micmonay/keybd_event"

type windowsSimulator struct {
    kb keybd_event.KeyBonding
}

func newPlatformSimulator() (Simulator, error) {
    // Uses Windows SendInput API via keybd_event
}
```

#### 4.2.3 darwin.go (build tag: darwin)

```go
//go:build darwin

package keyboard

import "os/exec"

type darwinSimulator struct{}

func newPlatformSimulator() (Simulator, error) {
    // Uses CGEventCreateKeyboardEvent or AppleScript
    // Note: Requires Accessibility permissions
}
```

### 4.3 System Tray Module (`internal/tray/`)

#### 4.3.1 tray.go

```go
package tray

import "github.com/getlantern/systray"

// TrayApp manages the system tray icon and menu
type TrayApp struct {
    onExit       func()
    onToggle     func()
    statusItem   *systray.MenuItem
    toggleItem   *systray.MenuItem
    currentState State
}

// State represents the tray icon state
type State int

const (
    StateDisconnected State = iota
    StateConnected
    StateMonitoring
    StateError
)

// NewTrayApp creates a new system tray application
func NewTrayApp(onExit func(), onToggle func()) *TrayApp

// Run starts the system tray (blocks until quit)
func (t *TrayApp) Run()

// SetState updates the tray icon and status text
func (t *TrayApp) SetState(state State)

// SetStatus sets the status menu item text
func (t *TrayApp) SetStatus(text string)

// ShowNotification displays a system notification
func (t *TrayApp) ShowNotification(title, message string)
```

#### 4.3.2 icons.go

```go
package tray

import _ "embed"

//go:embed assets/icon-connected.ico
var iconConnected []byte

//go:embed assets/icon-disconnected.ico
var iconDisconnected []byte

//go:embed assets/icon-monitoring.ico
var iconMonitoring []byte

//go:embed assets/icon-error.ico
var iconError []byte

// GetIcon returns the icon bytes for a given state
func GetIcon(state State) []byte
```

### 4.4 Application Module (`internal/app/`)

#### 4.4.1 app.go

```go
package app

import (
    "dermoscope-helper/internal/usb"
    "dermoscope-helper/internal/keyboard"
    "dermoscope-helper/internal/tray"
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
func New(config *Config) (*App, error) {
    // Initialize profile registry with built-in profiles
    registry := usb.NewProfileRegistry()

    // Validate all profiles
    if errs := registry.Validate(); len(errs) > 0 {
        return nil, fmt.Errorf("invalid profiles: %v", errs)
    }

    // Create device manager with registry
    deviceMgr := usb.NewDeviceManager(registry)

    // ... rest of initialization
}

// Run starts the application (blocking)
func (a *App) Run() error

// Stop gracefully shuts down the application
func (a *App) Stop()

// handleButtonPress is called when a button press is detected
func (a *App) handleButtonPress(event usb.ButtonEvent)

// handleDeviceDisconnect is called when the device disconnects
func (a *App) handleDeviceDisconnect()

// handleDeviceReconnect attempts to reconnect to the device
func (a *App) handleDeviceReconnect()
```

#### 4.4.2 state.go

```go
package app

// AppState represents the application state
type AppState int

const (
    StateStartup AppState = iota
    StateSearching
    StateClaiming
    StateMonitoring
    StateDisconnected
    StateStopping
)

// StateMachine manages application state transitions
type StateMachine struct {
    current AppState
    mu      sync.RWMutex
    onChange func(old, new AppState)
}

// NewStateMachine creates a new state machine
func NewStateMachine(onChange func(old, new AppState)) *StateMachine

// Transition attempts to transition to a new state
func (sm *StateMachine) Transition(newState AppState) error

// Current returns the current state
func (sm *StateMachine) Current() AppState
```

#### 4.4.3 config.go

```go
package app

// Config holds application configuration
type Config struct {
    // Timing settings
    DebounceMs       int
    ReconnectDelayMs int
    ReadTimeoutMs    int

    // Keyboard settings
    TriggerKey int  // Default: KeyF9 (0x78)

    // Logging settings
    LogFile  string
    LogLevel string

    // Behavior settings
    StartMinimized bool
    AutoStart      bool
}

// Note: Device identification (VID/PID) is handled by the profile system.
// See Section 5 for device profiles.

// DefaultConfig returns the default configuration
func DefaultConfig() *Config

// LoadConfig loads configuration from file (if exists)
func LoadConfig(path string) (*Config, error)

// SaveConfig saves configuration to file
func (c *Config) SaveConfig(path string) error
```

### 4.5 Main Entry Point

#### cmd/dermoscope-helper/main.go

```go
package main

import (
    "flag"
    "os"
    "os/signal"
    "syscall"

    "dermoscope-helper/internal/app"
    "dermoscope-helper/internal/logger"
)

func main() {
    // Parse command line flags
    configPath := flag.String("config", "", "Path to config file")
    debug := flag.Bool("debug", false, "Enable debug logging")
    listProfiles := flag.Bool("list-profiles", false, "List supported device profiles and exit")
    flag.Parse()

    // Initialize logger
    log := logger.New(*debug)

    // Handle --list-profiles
    if *listProfiles {
        registry := usb.NewProfileRegistry()
        fmt.Println("Supported Device Profiles:")
        fmt.Println("==========================")
        for _, p := range registry.GetAll() {
            fmt.Printf("\n  ID:           %s\n", p.ID)
            fmt.Printf("  Name:         %s\n", p.Name)
            fmt.Printf("  Manufacturer: %s\n", p.Manufacturer)
            fmt.Printf("  VID:PID:      %04X:%04X\n", p.VendorID, p.ProductID)
            if p.Notes != "" {
                fmt.Printf("  Notes:        %s\n", p.Notes)
            }
        }
        fmt.Printf("\nTotal: %d profile(s)\n", registry.Count())
        return
    }

    // Load configuration
    config := app.DefaultConfig()
    if *configPath != "" {
        var err error
        config, err = app.LoadConfig(*configPath)
        if err != nil {
            log.Fatal().Err(err).Msg("Failed to load config")
        }
    }

    // Create application
    application, err := app.New(config)
    if err != nil {
        log.Fatal().Err(err).Msg("Failed to create application")
    }

    // Log profile info in debug mode
    if *debug {
        registry := application.GetRegistry()
        log.Debug().Int("count", registry.Count()).Msg("Loaded device profiles")
        for _, p := range registry.GetAll() {
            log.Debug().
                Str("id", p.ID).
                Str("name", p.Name).
                Str("vidpid", fmt.Sprintf("%04X:%04X", p.VendorID, p.ProductID)).
                Msg("Profile registered")
        }
    }

    // Handle OS signals
    sigChan := make(chan os.Signal, 1)
    signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

    go func() {
        <-sigChan
        log.Info().Msg("Received shutdown signal")
        application.Stop()
    }()

    // Run application
    if err := application.Run(); err != nil {
        log.Fatal().Err(err).Msg("Application error")
    }
}
```

---

## 5. Device Profile System

### 5.1 Overview

The Device Profile System enables support for **different dermoscope models** without code changes for each new device. Profiles are defined in `internal/usb/profiles.go` and compiled into the binary.

**Note:** The application monitors one device at a time. The profile system allows the same binary to work with different dermoscope models (e.g., different clinics may have different equipment).

### 5.2 Profile Definition

Each profile must define:

```go
type DeviceProfile struct {
    // Required - Identification
    ID           string   // Unique identifier (lowercase, hyphenated)
    Name         string   // Display name
    Manufacturer string   // Device manufacturer
    VendorID     uint16   // USB Vendor ID (hex)
    ProductID    uint16   // USB Product ID (hex)

    // Required - Interface Configuration
    InterfaceClass    uint8   // Usually 0x0E (Video)
    InterfaceSubclass uint8   // Usually 0x01 (Video Control)

    // Required - Button Events
    ButtonPressPattern   []byte  // Exact byte sequence for press
    ButtonReleasePattern []byte  // Exact byte sequence for release

    // Optional
    Notes string  // Documentation notes
}
```

### 5.3 Built-in Profile Registry

```go
// File: internal/usb/profiles.go

var builtInProfiles = []DeviceProfile{
    // =========================================================
    // HT-B30S - Primary Device
    // =========================================================
    {
        ID:                   "ht-b30s",
        Name:                 "HT-B30S Dermoscope",
        Manufacturer:         "Sonix Technology",
        VendorID:             0xAB02,
        ProductID:            0xAB01,
        InterfaceClass:       0x0E,
        InterfaceSubclass:    0x01,
        ButtonPressPattern:   []byte{0x02, 0x01, 0x00, 0x00},
        ButtonReleasePattern: []byte{0x02, 0x01, 0x00, 0x01},
        Notes:                "Verified: Windows ✓, macOS (blocks camera)",
    },

    // =========================================================
    // ADD NEW PROFILES HERE
    // =========================================================
    // See DESIGN.md Section 10 for instructions on discovering
    // the required values for new devices.
    //
    // Template:
    // {
    //     ID:                   "device-id",
    //     Name:                 "Device Name",
    //     Manufacturer:         "Manufacturer",
    //     VendorID:             0x0000,
    //     ProductID:            0x0000,
    //     InterfaceClass:       0x0E,
    //     InterfaceSubclass:    0x01,
    //     ButtonPressPattern:   []byte{},
    //     ButtonReleasePattern: []byte{},
    //     Notes:                "Added YYYY-MM-DD by [name]",
    // },
}
```

### 5.4 Device Detection Algorithm

```go
func (dm *DeviceManager) FindDevice() (*DeviceInfo, error) {
    // 1. Open USB context
    ctx := gousb.NewContext()

    // 2. Enumerate all USB devices
    devices, _ := ctx.OpenDevices(func(desc *gousb.DeviceDesc) bool {
        // 3. Check each device against all profiles
        for _, profile := range dm.registry.GetAll() {
            if profile.MatchesProfile(desc.Vendor, desc.Product) {
                return true  // Open this device
            }
        }
        return false  // Skip this device
    })

    // 4. For each matched device, find the matching profile
    for _, dev := range devices {
        profile, _ := dm.registry.FindMatchingProfile(
            uint16(dev.Desc.Vendor),
            uint16(dev.Desc.Product),
        )

        return &DeviceInfo{
            Profile:      profile,
            VendorID:     uint16(dev.Desc.Vendor),
            ProductID:    uint16(dev.Desc.Product),
            // ... other fields
        }, nil
    }

    return nil, ErrNoDeviceFound
}
```

### 5.5 Single Device Operation

The application monitors **one device at a time**. If multiple supported devices are connected, the first one found is used.

```go
// FindDevice returns the first device matching any registered profile
func (dm *DeviceManager) FindDevice() (*DeviceInfo, error) {
    // ... enumeration code ...

    // Return first match, ignore others
    for _, dev := range devices {
        profile, found := dm.registry.FindMatchingProfile(vid, pid)
        if found {
            log.Info().
                Str("profile", profile.ID).
                Str("device", profile.Name).
                Msg("Device matched")
            return &DeviceInfo{Profile: profile, ...}, nil
        }
    }

    return nil, ErrNoDeviceFound
}
```

**Behavior:**
- Scans USB devices in enumeration order
- Returns first device matching any profile
- Other connected devices are ignored
- To switch devices: disconnect current, connect new one

### 5.6 Adding a New Profile

**Quick reference** (see DESIGN.md Section 10 for detailed instructions):

1. **Discover VID/PID:**
   ```bash
   # macOS
   system_profiler SPUSBDataType | grep -A5 "dermoscope"

   # Linux
   lsusb

   # Windows
   # Device Manager → Device → Properties → Details → Hardware IDs
   ```

2. **Capture button events:**
   ```bash
   sudo python3 capture_button_events.py 0xVID 0xPID
   # Press button, record the byte patterns
   ```

3. **Add profile to `profiles.go`:**
   ```go
   {
       ID:                   "new-device",
       Name:                 "New Device Model",
       Manufacturer:         "Manufacturer",
       VendorID:             0xVID,
       ProductID:            0xPID,
       InterfaceClass:       0x0E,
       InterfaceSubclass:    0x01,
       ButtonPressPattern:   []byte{0x02, 0x01, 0x00, 0x00},
       ButtonReleasePattern: []byte{0x02, 0x01, 0x00, 0x01},
       Notes:                "Added YYYY-MM-DD",
   },
   ```

4. **Rebuild:**
   ```bash
   make build-windows
   ```

### 5.7 Profile Validation

On startup, the application validates all profiles:

```go
func (r *ProfileRegistry) Validate() []error {
    var errs []error

    ids := make(map[string]bool)
    vidpids := make(map[string]bool)

    for _, p := range r.profiles {
        // Check for duplicate IDs
        if ids[p.ID] {
            errs = append(errs, fmt.Errorf("duplicate profile ID: %s", p.ID))
        }
        ids[p.ID] = true

        // Check for duplicate VID:PID
        key := fmt.Sprintf("%04x:%04x", p.VendorID, p.ProductID)
        if vidpids[key] {
            errs = append(errs, fmt.Errorf("duplicate VID:PID: %s", key))
        }
        vidpids[key] = true

        // Validate required fields
        if p.ID == "" {
            errs = append(errs, errors.New("profile missing ID"))
        }
        if len(p.ButtonPressPattern) == 0 {
            errs = append(errs, fmt.Errorf("profile %s missing ButtonPressPattern", p.ID))
        }
    }

    return errs
}
```

### 5.8 Debug Output

With `--debug` flag, the application logs profile information:

```
[DEBUG] Loaded 2 device profiles:
[DEBUG]   - ht-b30s: HT-B30S Dermoscope (AB02:AB01)
[DEBUG]   - new-device: New Device Model (1234:5678)
[DEBUG] Scanning for devices...
[DEBUG] Found device: VID=AB02 PID=AB01
[DEBUG] Matched profile: ht-b30s (HT-B30S Dermoscope)
```

---

## 6. Build & Distribution

### 6.1 Makefile

```makefile
.PHONY: all build-windows build-darwin build-linux clean

VERSION := 1.0.0
LDFLAGS := -ldflags "-s -w -X main.Version=$(VERSION)"

all: build-windows build-darwin build-linux

build-windows:
	GOOS=windows GOARCH=amd64 go build $(LDFLAGS) \
		-o dist/dermoscope-helper-$(VERSION)-windows-amd64.exe \
		./cmd/dermoscope-helper

build-darwin:
	GOOS=darwin GOARCH=amd64 go build $(LDFLAGS) \
		-o dist/dermoscope-helper-$(VERSION)-darwin-amd64 \
		./cmd/dermoscope-helper
	GOOS=darwin GOARCH=arm64 go build $(LDFLAGS) \
		-o dist/dermoscope-helper-$(VERSION)-darwin-arm64 \
		./cmd/dermoscope-helper

build-linux:
	GOOS=linux GOARCH=amd64 go build $(LDFLAGS) \
		-o dist/dermoscope-helper-$(VERSION)-linux-amd64 \
		./cmd/dermoscope-helper

clean:
	rm -rf dist/

# Development build (current platform)
dev:
	go build -o dermoscope-helper ./cmd/dermoscope-helper

# Run tests
test:
	go test -v ./...

# Install dependencies
deps:
	go mod download
	go mod tidy
```

### 6.2 Build Script (scripts/build-all.sh)

```bash
#!/bin/bash
set -e

VERSION=${1:-"1.0.0"}
OUTPUT_DIR="dist"

echo "Building Dermoscope Helper v${VERSION}"

# Clean
rm -rf ${OUTPUT_DIR}
mkdir -p ${OUTPUT_DIR}

# Build for Windows
echo "Building for Windows..."
GOOS=windows GOARCH=amd64 CGO_ENABLED=1 \
    CC=x86_64-w64-mingw32-gcc \
    go build -ldflags "-s -w -H windowsgui" \
    -o ${OUTPUT_DIR}/dermoscope-helper.exe \
    ./cmd/dermoscope-helper

# Build for macOS (Intel)
echo "Building for macOS (Intel)..."
GOOS=darwin GOARCH=amd64 go build -ldflags "-s -w" \
    -o ${OUTPUT_DIR}/dermoscope-helper-mac-intel \
    ./cmd/dermoscope-helper

# Build for macOS (Apple Silicon)
echo "Building for macOS (Apple Silicon)..."
GOOS=darwin GOARCH=arm64 go build -ldflags "-s -w" \
    -o ${OUTPUT_DIR}/dermoscope-helper-mac-arm64 \
    ./cmd/dermoscope-helper

echo "Build complete! Outputs in ${OUTPUT_DIR}/"
ls -la ${OUTPUT_DIR}/
```

### 6.3 Windows-Specific Notes

For Windows GUI application (no console window):
```go
// Add to main.go for Windows builds
//go:build windows

// Hide console window
// Use linker flag: -H windowsgui
```

### 6.4 Distribution Checklist

| Platform | File | Size Target | Notes |
|----------|------|-------------|-------|
| Windows | dermoscope-helper.exe | <10MB | No console, single exe |
| macOS Intel | dermoscope-helper-mac-intel | <10MB | Needs code signing for Gatekeeper |
| macOS ARM | dermoscope-helper-mac-arm64 | <10MB | Needs code signing for Gatekeeper |
| Linux | dermoscope-helper-linux | <10MB | May need udev rules |

---

## 7. Testing Strategy

### 7.1 Unit Tests

```go
// internal/usb/profiles_test.go
func TestProfileRegistry(t *testing.T) {
    registry := NewProfileRegistry()

    t.Run("has built-in profiles", func(t *testing.T) {
        if registry.Count() == 0 {
            t.Error("registry should have built-in profiles")
        }
    })

    t.Run("find by ID", func(t *testing.T) {
        profile, found := registry.GetByID("ht-b30s")
        if !found {
            t.Error("should find ht-b30s profile")
        }
        if profile.VendorID != 0xAB02 {
            t.Errorf("wrong VendorID: got %04x, want AB02", profile.VendorID)
        }
    })

    t.Run("find by VID/PID", func(t *testing.T) {
        profile, found := registry.FindMatchingProfile(0xAB02, 0xAB01)
        if !found {
            t.Error("should find profile by VID/PID")
        }
        if profile.ID != "ht-b30s" {
            t.Errorf("wrong profile: got %s, want ht-b30s", profile.ID)
        }
    })

    t.Run("unknown device returns not found", func(t *testing.T) {
        _, found := registry.FindMatchingProfile(0x0000, 0x0000)
        if found {
            t.Error("should not find unknown device")
        }
    })
}

func TestProfileButtonPatterns(t *testing.T) {
    profile := DeviceProfile{
        ID:                   "test",
        ButtonPressPattern:   []byte{0x02, 0x01, 0x00, 0x00},
        ButtonReleasePattern: []byte{0x02, 0x01, 0x00, 0x01},
    }

    tests := []struct {
        name        string
        data        []byte
        wantPress   bool
        wantRelease bool
    }{
        {
            name:      "button press",
            data:      []byte{0x02, 0x01, 0x00, 0x00},
            wantPress: true,
        },
        {
            name:        "button release",
            data:        []byte{0x02, 0x01, 0x00, 0x01},
            wantRelease: true,
        },
        {
            name: "unknown pattern",
            data: []byte{0xFF, 0xFF, 0xFF, 0xFF},
        },
        {
            name: "partial match",
            data: []byte{0x02, 0x01},
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            if got := profile.MatchesButtonPress(tt.data); got != tt.wantPress {
                t.Errorf("MatchesButtonPress() = %v, want %v", got, tt.wantPress)
            }
            if got := profile.MatchesButtonRelease(tt.data); got != tt.wantRelease {
                t.Errorf("MatchesButtonRelease() = %v, want %v", got, tt.wantRelease)
            }
        })
    }
}

// internal/usb/monitor_test.go
func TestParseButtonPress(t *testing.T) {
    profile := &DeviceProfile{
        ID:                   "test-device",
        ButtonPressPattern:   []byte{0x02, 0x01, 0x00, 0x00},
        ButtonReleasePattern: []byte{0x02, 0x01, 0x00, 0x01},
    }

    dm := &DeviceManager{profile: profile}
    m := NewMonitor(dm, 250)

    tests := []struct {
        name    string
        data    []byte
        want    *ButtonEvent
        wantErr bool
    }{
        {
            name: "valid press",
            data: []byte{0x02, 0x01, 0x00, 0x00},
            want: &ButtonEvent{Pressed: true, DeviceID: "test-device"},
        },
        {
            name: "valid release",
            data: []byte{0x02, 0x01, 0x00, 0x01},
            want: &ButtonEvent{Pressed: false, DeviceID: "test-device"},
        },
        {
            name:    "unknown pattern",
            data:    []byte{0xFF, 0xFF, 0xFF, 0xFF},
            wantErr: true,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got, err := m.parseEvent(tt.data)
            if (err != nil) != tt.wantErr {
                t.Errorf("parseEvent() error = %v, wantErr %v", err, tt.wantErr)
            }
            if !tt.wantErr {
                if got.Pressed != tt.want.Pressed {
                    t.Errorf("parseEvent().Pressed = %v, want %v", got.Pressed, tt.want.Pressed)
                }
                if got.DeviceID != tt.want.DeviceID {
                    t.Errorf("parseEvent().DeviceID = %v, want %v", got.DeviceID, tt.want.DeviceID)
                }
            }
        })
    }
}
```

### 7.2 Integration Tests

```go
// internal/usb/device_integration_test.go
//go:build integration

func TestDeviceConnection(t *testing.T) {
    if testing.Short() {
        t.Skip("Skipping integration test")
    }

    registry := NewProfileRegistry()
    dm := NewDeviceManager(registry)
    defer dm.Close()

    info, err := dm.FindDevice()
    if err != nil {
        t.Skipf("No supported device connected: %v", err)
    }

    t.Logf("Found device: %s - %s (profile: %s)",
        info.Manufacturer, info.Product, info.Profile.ID)

    if err := dm.ClaimInterface(); err != nil {
        t.Fatalf("Failed to claim interface: %v", err)
    }

    // Verify profile is set
    profile := dm.GetProfile()
    if profile == nil {
        t.Fatal("Profile should be set after FindDevice")
    }

    // Test endpoint read with short timeout
    // ...
}
```

### 7.3 Manual Testing Checklist

| Test Case | Steps | Expected Result |
|-----------|-------|-----------------|
| TC1: Basic detection | Connect dermoscope, run app | Device detected, icon shows connected |
| TC2: Button press | Press dermoscope button | F9 sent, logged |
| TC3: Disconnect | Unplug dermoscope | Icon shows disconnected |
| TC4: Reconnect | Plug dermoscope back in | Auto-reconnects, icon shows connected |
| TC5: Debounce | Rapidly press button | Only one F9 per press |
| TC6: Exit | Click Exit in tray menu | Clean shutdown |
| TC7: Web app | Press button with TrichoAI open | Image captured in web app |
| TC8: Profile match | Connect device, check `--debug` logs | Correct profile ID logged |
| TC9: Unknown device | Connect unsupported USB device | Device ignored, no errors |
| TC10: Device switch | Disconnect device A, connect device B | Auto-detects new device |

### 7.4 Profile Testing Checklist

When adding a new device profile, verify:

| Test | Pass |
|------|------|
| VID/PID correctly identified from device |  |
| Button press pattern captured correctly |  |
| Button release pattern captured correctly |  |
| Profile added to `profiles.go` |  |
| Application builds without errors |  |
| Device detected on startup |  |
| Correct profile shown in debug logs |  |
| Button press triggers F9 |  |
| No double-triggers on single press |  |
| Debouncing works correctly |  |
| Device reconnection works |  |
| Camera still functions (Windows) |  |

---

## 8. Implementation Phases

### Phase 1: Core Functionality (P0)

**Goal:** Working button detection and F9 simulation

**Tasks:**
1. Set up Go project structure
2. Implement USB device detection
3. Implement interface claiming
4. Implement interrupt endpoint reading
5. Implement button event parsing
6. Implement keyboard simulation (Windows first)
7. Implement debouncing
8. Basic error handling

**Deliverable:** Command-line app that detects button and sends F9

### Phase 2: System Tray (P1)

**Goal:** User-friendly tray application

**Tasks:**
1. Add system tray integration
2. Create tray icons for each state
3. Implement tray menu
4. Add status display
5. Implement device state machine
6. Add auto-reconnect logic
7. Add logging to file

**Deliverable:** System tray app with status indicators

### Phase 3: Polish & Distribution (P1-P2)

**Goal:** Production-ready application

**Tasks:**
1. macOS keyboard implementation
2. Linux keyboard implementation
3. Configuration file support
4. Auto-start option
5. Installer creation (optional)
6. Code signing (optional)
7. Documentation

**Deliverable:** Cross-platform distributable binary

---

## References

- [DESIGN.md](./DESIGN.md) - Design document with full background
- [DESIGN.md Section 9](./DESIGN.md#9-device-profile-system) - Device Profile System overview
- [DESIGN.md Section 10](./DESIGN.md#10-adding-new-device-profiles) - How to add new device profiles
- [README.md](../README.md) - Investigation documentation
- [gousb documentation](https://pkg.go.dev/github.com/google/gousb)
- [systray documentation](https://github.com/getlantern/systray)

---

## Document History

| Version | Date | Author | Changes |
|---------|------|--------|---------|
| 1.0 | Feb 2025 | Claude | Initial specification |
| 1.1 | Feb 2025 | Claude | Added Device Profile System (Section 5) for supporting different device models (single device operation) |