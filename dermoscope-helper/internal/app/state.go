package app

import (
	"errors"
	"fmt"
	"sync"
)

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

// String returns the string representation of the state
func (s AppState) String() string {
	switch s {
	case StateStartup:
		return "startup"
	case StateSearching:
		return "searching"
	case StateClaiming:
		return "claiming"
	case StateMonitoring:
		return "monitoring"
	case StateDisconnected:
		return "disconnected"
	case StateStopping:
		return "stopping"
	default:
		return "unknown"
	}
}

// validTransitions defines which state transitions are allowed.
// The key is the current state, the value is a set of valid target states.
var validTransitions = map[AppState]map[AppState]bool{
	StateStartup: {
		StateSearching: true,
		StateStopping:  true, // ANY → STOPPING
	},
	StateSearching: {
		StateClaiming:     true, // device found
		StateDisconnected: true, // timeout/error
		StateStopping:     true, // ANY → STOPPING
	},
	StateClaiming: {
		StateMonitoring: true, // success
		StateSearching:  true, // failed
		StateStopping:   true, // ANY → STOPPING
	},
	StateMonitoring: {
		StateMonitoring:   true, // button press - stays in monitoring
		StateDisconnected: true, // device lost
		StateStopping:     true, // ANY → STOPPING
	},
	StateDisconnected: {
		StateSearching: true, // polling for reconnect
		StateStopping:  true, // ANY → STOPPING
	},
	StateStopping: {
		// No valid transitions from STOPPING
	},
}

// ErrInvalidTransition is returned when an invalid state transition is attempted
var ErrInvalidTransition = errors.New("invalid state transition")

// StateMachine manages application state transitions
type StateMachine struct {
	current  AppState
	mu       sync.RWMutex
	onChange func(old, new AppState)
}

// NewStateMachine creates a new state machine starting in STARTUP state
func NewStateMachine(onChange func(old, new AppState)) *StateMachine {
	return &StateMachine{
		current:  StateStartup,
		onChange: onChange,
	}
}

// Transition attempts to transition to a new state.
// Returns an error if the transition is not valid.
func (sm *StateMachine) Transition(newState AppState) error {
	sm.mu.Lock()

	old := sm.current

	// Check if transition is valid
	if !IsValidTransition(old, newState) {
		sm.mu.Unlock()
		return fmt.Errorf("%w: %s -> %s", ErrInvalidTransition, old, newState)
	}

	// Perform the transition
	sm.current = newState

	// Get callback reference before releasing lock
	callback := sm.onChange

	// Release lock before calling callback to avoid blocking
	sm.mu.Unlock()

	// Invoke callback if set and state actually changed
	if callback != nil && old != newState {
		callback(old, newState)
	}

	return nil
}

// Current returns the current state
func (sm *StateMachine) Current() AppState {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.current
}

// IsValidTransition checks if a transition from one state to another is valid
func IsValidTransition(from, to AppState) bool {
	targets, exists := validTransitions[from]
	if !exists {
		return false
	}
	return targets[to]
}
