package tray

import _ "embed"

// Icon resources embedded using go:embed
// The icons are simple 16x16 PNG files with solid colors:
// - connected: green
// - disconnected: gray
// - monitoring: blue
// - error: red

//go:embed assets/icon-connected.png
var iconConnected []byte

//go:embed assets/icon-disconnected.png
var iconDisconnected []byte

//go:embed assets/icon-monitoring.png
var iconMonitoring []byte

//go:embed assets/icon-error.png
var iconError []byte

// GetIcon returns the icon bytes for a given state
func GetIcon(state State) []byte {
	switch state {
	case StateConnected:
		return iconConnected
	case StateDisconnected:
		return iconDisconnected
	case StateMonitoring:
		return iconMonitoring
	case StateError:
		return iconError
	default:
		return iconDisconnected
	}
}
