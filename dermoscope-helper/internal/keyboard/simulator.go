// Package keyboard provides cross-platform keyboard simulation.
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

// Key codes (Windows virtual key codes)
const (
	KeyF9  = 0x78
	KeyF10 = 0x79
	KeyF11 = 0x7A
	KeyF12 = 0x7B
)

// NewSimulator returns a platform-specific keyboard simulator.
// The actual implementation is provided by platform-specific files
// (windows.go, darwin.go, linux.go) using build tags.
func NewSimulator() (Simulator, error) {
	return newPlatformSimulator()
}
