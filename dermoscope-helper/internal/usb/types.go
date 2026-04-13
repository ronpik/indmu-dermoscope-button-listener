// Package usb provides USB device management for dermoscope button monitoring.
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

// String returns the string representation of the device state
func (s DeviceState) String() string {
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

// bytesEqual compares two byte slices for equality
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

// USB Constants shared across all UVC-based devices
const (
	VideoControlClass     = 0x0E // UVC Video class
	VideoControlSubclass  = 0x01 // Video Control subclass
	InterruptEndpointType = 0x03
	EndpointDirectionIn   = 0x80
)

// Timing constants
const (
	ReadTimeoutMs    = 500
	DebounceMs       = 250
	ReconnectDelayMs = 2000
	DevicePollMs     = 1000
)
