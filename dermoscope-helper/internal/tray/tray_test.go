package tray

import (
	"testing"
)

func TestGetIconReturnsCorrectIconForState(t *testing.T) {
	tests := []struct {
		name  string
		state State
		want  []byte
	}{
		{
			name:  "connected state returns connected icon",
			state: StateConnected,
			want:  iconConnected,
		},
		{
			name:  "disconnected state returns disconnected icon",
			state: StateDisconnected,
			want:  iconDisconnected,
		},
		{
			name:  "monitoring state returns monitoring icon",
			state: StateMonitoring,
			want:  iconMonitoring,
		},
		{
			name:  "error state returns error icon",
			state: StateError,
			want:  iconError,
		},
		{
			name:  "unknown state returns disconnected icon",
			state: State(99),
			want:  iconDisconnected,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GetIcon(tt.state)
			if len(got) != len(tt.want) {
				t.Errorf("GetIcon(%v) returned %d bytes, want %d bytes", tt.state, len(got), len(tt.want))
			}
			// Compare byte slices
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("GetIcon(%v) byte %d = %d, want %d", tt.state, i, got[i], tt.want[i])
					break
				}
			}
		})
	}
}

func TestGetIconReturnsNonEmptyIcons(t *testing.T) {
	states := []State{
		StateConnected,
		StateDisconnected,
		StateMonitoring,
		StateError,
	}

	for _, state := range states {
		t.Run(state.String(), func(t *testing.T) {
			icon := GetIcon(state)
			if len(icon) == 0 {
				t.Errorf("GetIcon(%v) returned empty icon", state)
			}
			// Verify PNG signature (first 8 bytes)
			pngSignature := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}
			if len(icon) < 8 {
				t.Errorf("GetIcon(%v) icon too small: %d bytes", state, len(icon))
				return
			}
			for i, b := range pngSignature {
				if icon[i] != b {
					t.Errorf("GetIcon(%v) invalid PNG signature at byte %d: got %02x, want %02x", state, i, icon[i], b)
				}
			}
		})
	}
}

func TestStateString(t *testing.T) {
	tests := []struct {
		state State
		want  string
	}{
		{StateDisconnected, "disconnected"},
		{StateConnected, "connected"},
		{StateMonitoring, "monitoring"},
		{StateError, "error"},
		{State(99), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.state.String(); got != tt.want {
				t.Errorf("State(%d).String() = %q, want %q", tt.state, got, tt.want)
			}
		})
	}
}

func TestNewTrayAppCreatesAppWithCallbacks(t *testing.T) {
	exitCalled := false
	toggleCalled := false

	onExit := func() { exitCalled = true }
	onToggle := func() { toggleCalled = true }

	app := NewTrayApp(onExit, onToggle)

	if app == nil {
		t.Fatal("NewTrayApp returned nil")
	}

	if app.currentState != StateDisconnected {
		t.Errorf("initial state = %v, want %v", app.currentState, StateDisconnected)
	}

	// Test that callbacks are stored (we can't call Run in tests)
	if app.onExit == nil {
		t.Error("onExit callback not stored")
	}
	if app.onToggle == nil {
		t.Error("onToggle callback not stored")
	}

	// Verify callbacks work (without running the tray)
	app.onExit()
	if !exitCalled {
		t.Error("onExit callback was not invoked")
	}

	app.onToggle()
	if !toggleCalled {
		t.Error("onToggle callback was not invoked")
	}
}

func TestTrayAppGetState(t *testing.T) {
	app := NewTrayApp(nil, nil)

	// Initial state should be disconnected
	if got := app.GetState(); got != StateDisconnected {
		t.Errorf("GetState() = %v, want %v", got, StateDisconnected)
	}

	// Set state directly (bypassing SetState to test GetState isolation)
	app.mu.Lock()
	app.currentState = StateMonitoring
	app.mu.Unlock()

	if got := app.GetState(); got != StateMonitoring {
		t.Errorf("GetState() = %v, want %v", got, StateMonitoring)
	}
}

func TestStateToStatusText(t *testing.T) {
	tests := []struct {
		state State
		want  string
	}{
		{StateDisconnected, "No device connected"},
		{StateConnected, "Device connected"},
		{StateMonitoring, "Monitoring for button press"},
		{StateError, "Error - check logs"},
		{State(99), "Unknown state"},
	}

	for _, tt := range tests {
		t.Run(tt.state.String(), func(t *testing.T) {
			if got := stateToStatusText(tt.state); got != tt.want {
				t.Errorf("stateToStatusText(%v) = %q, want %q", tt.state, got, tt.want)
			}
		})
	}
}
