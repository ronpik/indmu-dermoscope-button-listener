package app

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
)

// Test NewStateMachine starts in STARTUP state
func TestNewStateMachine_StartsInStartupState(t *testing.T) {
	sm := NewStateMachine(nil)

	if sm == nil {
		t.Fatal("NewStateMachine() returned nil")
	}

	current := sm.Current()
	if current != StateStartup {
		t.Errorf("Current() = %v, want StateStartup", current)
	}
}

// Test NewStateMachine with onChange callback
func TestNewStateMachine_AcceptsCallback(t *testing.T) {
	callbackCalled := false
	callback := func(old, new AppState) {
		callbackCalled = true
	}

	sm := NewStateMachine(callback)

	if sm == nil {
		t.Fatal("NewStateMachine() returned nil")
	}

	// Callback should not be called on creation
	if callbackCalled {
		t.Error("Callback should not be called on NewStateMachine()")
	}
}

// Test valid transition STARTUP -> SEARCHING succeeds
func TestStateMachine_Transition_StartupToSearching_Succeeds(t *testing.T) {
	sm := NewStateMachine(nil)

	err := sm.Transition(StateSearching)

	if err != nil {
		t.Errorf("Transition(StateSearching) returned error: %v", err)
	}
	if sm.Current() != StateSearching {
		t.Errorf("Current() = %v, want StateSearching", sm.Current())
	}
}

// Test valid transition SEARCHING -> CLAIMING succeeds
func TestStateMachine_Transition_SearchingToClaiming_Succeeds(t *testing.T) {
	sm := NewStateMachine(nil)

	// First transition to SEARCHING
	if err := sm.Transition(StateSearching); err != nil {
		t.Fatalf("Setup transition failed: %v", err)
	}

	// Now transition to CLAIMING
	err := sm.Transition(StateClaiming)

	if err != nil {
		t.Errorf("Transition(StateClaiming) returned error: %v", err)
	}
	if sm.Current() != StateClaiming {
		t.Errorf("Current() = %v, want StateClaiming", sm.Current())
	}
}

// Test valid transition CLAIMING -> MONITORING succeeds
func TestStateMachine_Transition_ClaimingToMonitoring_Succeeds(t *testing.T) {
	sm := NewStateMachine(nil)

	// Setup: STARTUP -> SEARCHING -> CLAIMING
	_ = sm.Transition(StateSearching)
	_ = sm.Transition(StateClaiming)

	// Now transition to MONITORING
	err := sm.Transition(StateMonitoring)

	if err != nil {
		t.Errorf("Transition(StateMonitoring) returned error: %v", err)
	}
	if sm.Current() != StateMonitoring {
		t.Errorf("Current() = %v, want StateMonitoring", sm.Current())
	}
}

// Test invalid transition STARTUP -> MONITORING fails
func TestStateMachine_Transition_StartupToMonitoring_Fails(t *testing.T) {
	sm := NewStateMachine(nil)

	err := sm.Transition(StateMonitoring)

	if err == nil {
		t.Error("Transition(StateMonitoring) should return error from STARTUP state")
	}

	// Should return ErrInvalidTransition
	if !errors.Is(err, ErrInvalidTransition) {
		t.Errorf("Error should wrap ErrInvalidTransition, got: %v", err)
	}

	// State should remain STARTUP
	if sm.Current() != StateStartup {
		t.Errorf("Current() = %v, want StateStartup (unchanged after failed transition)", sm.Current())
	}
}

// Test invalid transition STARTUP -> CLAIMING fails
func TestStateMachine_Transition_StartupToClaiming_Fails(t *testing.T) {
	sm := NewStateMachine(nil)

	err := sm.Transition(StateClaiming)

	if err == nil {
		t.Error("Transition(StateClaiming) should return error from STARTUP state")
	}
	if !errors.Is(err, ErrInvalidTransition) {
		t.Errorf("Error should wrap ErrInvalidTransition, got: %v", err)
	}
}

// Test invalid transition STARTUP -> DISCONNECTED fails
func TestStateMachine_Transition_StartupToDisconnected_Fails(t *testing.T) {
	sm := NewStateMachine(nil)

	err := sm.Transition(StateDisconnected)

	if err == nil {
		t.Error("Transition(StateDisconnected) should return error from STARTUP state")
	}
	if !errors.Is(err, ErrInvalidTransition) {
		t.Errorf("Error should wrap ErrInvalidTransition, got: %v", err)
	}
}

// Test Current() returns correct state
func TestStateMachine_Current_ReturnsCorrectState(t *testing.T) {
	tests := []struct {
		name          string
		transitions   []AppState
		expectedState AppState
	}{
		{
			name:          "initial state",
			transitions:   []AppState{},
			expectedState: StateStartup,
		},
		{
			name:          "after SEARCHING",
			transitions:   []AppState{StateSearching},
			expectedState: StateSearching,
		},
		{
			name:          "after CLAIMING",
			transitions:   []AppState{StateSearching, StateClaiming},
			expectedState: StateClaiming,
		},
		{
			name:          "after MONITORING",
			transitions:   []AppState{StateSearching, StateClaiming, StateMonitoring},
			expectedState: StateMonitoring,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sm := NewStateMachine(nil)

			for _, state := range tt.transitions {
				if err := sm.Transition(state); err != nil {
					t.Fatalf("Setup transition to %v failed: %v", state, err)
				}
			}

			current := sm.Current()
			if current != tt.expectedState {
				t.Errorf("Current() = %v, want %v", current, tt.expectedState)
			}
		})
	}
}

// Test onChange callback is invoked on transition
func TestStateMachine_Transition_CallbackInvoked(t *testing.T) {
	callbackCalled := false
	var capturedOld, capturedNew AppState

	callback := func(old, new AppState) {
		callbackCalled = true
		capturedOld = old
		capturedNew = new
	}

	sm := NewStateMachine(callback)

	err := sm.Transition(StateSearching)

	if err != nil {
		t.Fatalf("Transition() returned error: %v", err)
	}
	if !callbackCalled {
		t.Error("Callback should be invoked on successful transition")
	}
	if capturedOld != StateStartup {
		t.Errorf("Callback old state = %v, want StateStartup", capturedOld)
	}
	if capturedNew != StateSearching {
		t.Errorf("Callback new state = %v, want StateSearching", capturedNew)
	}
}

// Test onChange callback is not invoked on failed transition
func TestStateMachine_Transition_CallbackNotInvokedOnFailure(t *testing.T) {
	callbackCalled := false
	callback := func(old, new AppState) {
		callbackCalled = true
	}

	sm := NewStateMachine(callback)

	// Try invalid transition
	_ = sm.Transition(StateMonitoring)

	if callbackCalled {
		t.Error("Callback should not be invoked on failed transition")
	}
}

// Test onChange callback invoked multiple times
func TestStateMachine_Transition_CallbackInvokedMultipleTimes(t *testing.T) {
	callCount := 0
	transitions := make([]struct{ old, new AppState }, 0)

	callback := func(old, new AppState) {
		callCount++
		transitions = append(transitions, struct{ old, new AppState }{old, new})
	}

	sm := NewStateMachine(callback)

	// Perform multiple valid transitions
	_ = sm.Transition(StateSearching)
	_ = sm.Transition(StateClaiming)
	_ = sm.Transition(StateMonitoring)

	if callCount != 3 {
		t.Errorf("Callback call count = %d, want 3", callCount)
	}

	expected := []struct{ old, new AppState }{
		{StateStartup, StateSearching},
		{StateSearching, StateClaiming},
		{StateClaiming, StateMonitoring},
	}

	for i, exp := range expected {
		if i >= len(transitions) {
			t.Errorf("Missing transition %d", i)
			continue
		}
		if transitions[i].old != exp.old || transitions[i].new != exp.new {
			t.Errorf("Transition %d: got {%v, %v}, want {%v, %v}",
				i, transitions[i].old, transitions[i].new, exp.old, exp.new)
		}
	}
}

// Test String() returns human-readable state names
func TestAppState_String(t *testing.T) {
	tests := []struct {
		state    AppState
		expected string
	}{
		{StateStartup, "startup"},
		{StateSearching, "searching"},
		{StateClaiming, "claiming"},
		{StateMonitoring, "monitoring"},
		{StateDisconnected, "disconnected"},
		{StateStopping, "stopping"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := tt.state.String()
			if result != tt.expected {
				t.Errorf("String() = %q, want %q", result, tt.expected)
			}
		})
	}
}

// Test String() returns "unknown" for invalid state
func TestAppState_String_UnknownState(t *testing.T) {
	invalidState := AppState(99)

	result := invalidState.String()

	if result != "unknown" {
		t.Errorf("String() for invalid state = %q, want \"unknown\"", result)
	}
}

// Test transition to same state (MONITORING -> MONITORING allowed)
func TestStateMachine_Transition_SameState_MonitoringToMonitoring(t *testing.T) {
	sm := NewStateMachine(nil)

	// Setup: STARTUP -> SEARCHING -> CLAIMING -> MONITORING
	_ = sm.Transition(StateSearching)
	_ = sm.Transition(StateClaiming)
	_ = sm.Transition(StateMonitoring)

	// Now transition to MONITORING again (button press - stays in monitoring)
	err := sm.Transition(StateMonitoring)

	if err != nil {
		t.Errorf("Transition(StateMonitoring) from MONITORING should succeed, got error: %v", err)
	}
	if sm.Current() != StateMonitoring {
		t.Errorf("Current() = %v, want StateMonitoring", sm.Current())
	}
}

// Test callback NOT invoked when transitioning to same state (no actual change)
func TestStateMachine_Transition_SameState_CallbackNotInvoked(t *testing.T) {
	callbackCalled := false
	callback := func(old, new AppState) {
		callbackCalled = true
	}

	sm := NewStateMachine(callback)

	// Setup: STARTUP -> SEARCHING -> CLAIMING -> MONITORING
	_ = sm.Transition(StateSearching)
	_ = sm.Transition(StateClaiming)
	_ = sm.Transition(StateMonitoring)

	// Reset flag
	callbackCalled = false

	// Transition to same state
	_ = sm.Transition(StateMonitoring)

	if callbackCalled {
		t.Error("Callback should not be invoked when transitioning to same state")
	}
}

// Test concurrent transitions (race condition test)
func TestStateMachine_ConcurrentTransitions(t *testing.T) {
	sm := NewStateMachine(nil)

	var wg sync.WaitGroup
	numGoroutines := 100

	// All goroutines try to transition from STARTUP to SEARCHING
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = sm.Transition(StateSearching)
		}()
	}

	wg.Wait()

	// State should be SEARCHING (one of them succeeded, rest failed gracefully)
	// Note: All attempts at STARTUP -> SEARCHING should succeed since it's a valid transition
	current := sm.Current()
	if current != StateSearching {
		t.Errorf("Current() = %v, want StateSearching after concurrent transitions", current)
	}
}

// Test concurrent reads and transitions
func TestStateMachine_ConcurrentReadsAndTransitions(t *testing.T) {
	sm := NewStateMachine(nil)

	var wg sync.WaitGroup
	numReaders := 50
	numWriters := 10

	// Start readers
	for i := 0; i < numReaders; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				_ = sm.Current()
			}
		}()
	}

	// Start writers (transitions)
	for i := 0; i < numWriters; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			// Try valid transitions
			_ = sm.Transition(StateSearching)
			_ = sm.Transition(StateStopping)
		}()
	}

	wg.Wait()
	// Test passes if no race condition is detected
}

// Test nil onChange callback
func TestStateMachine_NilCallback_DoesNotPanic(t *testing.T) {
	sm := NewStateMachine(nil)

	// Should not panic when callback is nil
	err := sm.Transition(StateSearching)

	if err != nil {
		t.Errorf("Transition() with nil callback returned error: %v", err)
	}
	if sm.Current() != StateSearching {
		t.Errorf("Current() = %v, want StateSearching", sm.Current())
	}
}

// Test ANY -> STOPPING transition (all states can transition to STOPPING)
func TestStateMachine_Transition_AnyToStopping(t *testing.T) {
	tests := []struct {
		name         string
		setupStates  []AppState
		startingFrom AppState
	}{
		{
			name:         "STARTUP -> STOPPING",
			setupStates:  []AppState{},
			startingFrom: StateStartup,
		},
		{
			name:         "SEARCHING -> STOPPING",
			setupStates:  []AppState{StateSearching},
			startingFrom: StateSearching,
		},
		{
			name:         "CLAIMING -> STOPPING",
			setupStates:  []AppState{StateSearching, StateClaiming},
			startingFrom: StateClaiming,
		},
		{
			name:         "MONITORING -> STOPPING",
			setupStates:  []AppState{StateSearching, StateClaiming, StateMonitoring},
			startingFrom: StateMonitoring,
		},
		{
			name:         "DISCONNECTED -> STOPPING",
			setupStates:  []AppState{StateSearching, StateDisconnected},
			startingFrom: StateDisconnected,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sm := NewStateMachine(nil)

			// Setup to starting state
			for _, state := range tt.setupStates {
				if err := sm.Transition(state); err != nil {
					t.Fatalf("Setup transition to %v failed: %v", state, err)
				}
			}

			// Verify we're in the expected starting state
			if sm.Current() != tt.startingFrom {
				t.Fatalf("Setup failed: Current() = %v, want %v", sm.Current(), tt.startingFrom)
			}

			// Transition to STOPPING
			err := sm.Transition(StateStopping)

			if err != nil {
				t.Errorf("Transition(StateStopping) from %v returned error: %v", tt.startingFrom, err)
			}
			if sm.Current() != StateStopping {
				t.Errorf("Current() = %v, want StateStopping", sm.Current())
			}
		})
	}
}

// Test STOPPING has no valid outgoing transitions
func TestStateMachine_Transition_StoppingNoOutgoing(t *testing.T) {
	sm := NewStateMachine(nil)

	// Transition to STOPPING
	_ = sm.Transition(StateStopping)

	// Try all states - all should fail
	states := []AppState{
		StateStartup,
		StateSearching,
		StateClaiming,
		StateMonitoring,
		StateDisconnected,
		StateStopping,
	}

	for _, state := range states {
		err := sm.Transition(state)
		if err == nil {
			t.Errorf("Transition(%v) from STOPPING should return error", state)
		}
		if !errors.Is(err, ErrInvalidTransition) {
			t.Errorf("Transition(%v) error should wrap ErrInvalidTransition, got: %v", state, err)
		}
	}

	// State should remain STOPPING
	if sm.Current() != StateStopping {
		t.Errorf("Current() = %v, want StateStopping", sm.Current())
	}
}

// Test IsValidTransition helper function
func TestIsValidTransition(t *testing.T) {
	tests := []struct {
		from     AppState
		to       AppState
		expected bool
	}{
		// Valid transitions
		{StateStartup, StateSearching, true},
		{StateStartup, StateStopping, true},
		{StateSearching, StateClaiming, true},
		{StateSearching, StateDisconnected, true},
		{StateSearching, StateStopping, true},
		{StateClaiming, StateMonitoring, true},
		{StateClaiming, StateSearching, true},
		{StateClaiming, StateStopping, true},
		{StateMonitoring, StateMonitoring, true},
		{StateMonitoring, StateDisconnected, true},
		{StateMonitoring, StateStopping, true},
		{StateDisconnected, StateSearching, true},
		{StateDisconnected, StateStopping, true},

		// Invalid transitions
		{StateStartup, StateClaiming, false},
		{StateStartup, StateMonitoring, false},
		{StateStartup, StateDisconnected, false},
		{StateSearching, StateStartup, false},
		{StateSearching, StateMonitoring, false},
		{StateClaiming, StateStartup, false},
		{StateClaiming, StateDisconnected, false},
		{StateMonitoring, StateStartup, false},
		{StateMonitoring, StateSearching, false},
		{StateMonitoring, StateClaiming, false},
		{StateDisconnected, StateStartup, false},
		{StateDisconnected, StateClaiming, false},
		{StateDisconnected, StateMonitoring, false},
		{StateStopping, StateStartup, false},
		{StateStopping, StateSearching, false},
		{StateStopping, StateClaiming, false},
		{StateStopping, StateMonitoring, false},
		{StateStopping, StateDisconnected, false},
		{StateStopping, StateStopping, false},
	}

	for _, tt := range tests {
		t.Run(tt.from.String()+"_to_"+tt.to.String(), func(t *testing.T) {
			result := IsValidTransition(tt.from, tt.to)
			if result != tt.expected {
				t.Errorf("IsValidTransition(%v, %v) = %v, want %v",
					tt.from, tt.to, result, tt.expected)
			}
		})
	}
}

// Test IsValidTransition with unknown state
func TestIsValidTransition_UnknownState(t *testing.T) {
	unknownState := AppState(99)

	// Transition from unknown state should be invalid
	result := IsValidTransition(unknownState, StateSearching)
	if result {
		t.Error("IsValidTransition() from unknown state should return false")
	}

	// Transition to unknown state should be invalid
	result = IsValidTransition(StateStartup, unknownState)
	if result {
		t.Error("IsValidTransition() to unknown state should return false")
	}
}

// Test error message includes state names
func TestStateMachine_Transition_ErrorMessage(t *testing.T) {
	sm := NewStateMachine(nil)

	err := sm.Transition(StateMonitoring)

	if err == nil {
		t.Fatal("Transition() should return error")
	}

	errStr := err.Error()
	if !containsSubstring(errStr, "startup") {
		t.Errorf("Error message should contain source state name, got: %q", errStr)
	}
	if !containsSubstring(errStr, "monitoring") {
		t.Errorf("Error message should contain target state name, got: %q", errStr)
	}
}

// Test callback is invoked outside of lock (doesn't deadlock)
func TestStateMachine_Callback_NoDeadlock(t *testing.T) {
	done := make(chan bool, 1)

	var sm *StateMachine
	callback := func(old, new AppState) {
		// This would deadlock if callback is called while holding lock
		// because Current() tries to acquire RLock
		_ = sm.Current()
		done <- true
	}

	sm = NewStateMachine(callback)

	go func() {
		_ = sm.Transition(StateSearching)
	}()

	select {
	case <-done:
		// Success - callback completed without deadlock
	default:
		// Give it a moment
		select {
		case <-done:
			// Success
		}
	}
}

// Test concurrent state transitions don't corrupt state
func TestStateMachine_ConcurrentTransitions_NoCorruption(t *testing.T) {
	sm := NewStateMachine(nil)

	// Setup: move to MONITORING state which allows self-transitions
	_ = sm.Transition(StateSearching)
	_ = sm.Transition(StateClaiming)
	_ = sm.Transition(StateMonitoring)

	var wg sync.WaitGroup
	var successCount int32

	numGoroutines := 100

	// All try to transition MONITORING -> MONITORING (allowed)
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := sm.Transition(StateMonitoring)
			if err == nil {
				atomic.AddInt32(&successCount, 1)
			}
		}()
	}

	wg.Wait()

	// All transitions should succeed (self-transition allowed in MONITORING)
	if successCount != int32(numGoroutines) {
		t.Errorf("Success count = %d, want %d (all should succeed)", successCount, numGoroutines)
	}

	// Final state should be MONITORING
	if sm.Current() != StateMonitoring {
		t.Errorf("Final state = %v, want StateMonitoring", sm.Current())
	}
}

// Test DISCONNECTED -> SEARCHING (reconnect path)
func TestStateMachine_Transition_DisconnectedToSearching(t *testing.T) {
	sm := NewStateMachine(nil)

	// Setup: STARTUP -> SEARCHING -> DISCONNECTED
	_ = sm.Transition(StateSearching)
	_ = sm.Transition(StateDisconnected)

	// Reconnect: DISCONNECTED -> SEARCHING
	err := sm.Transition(StateSearching)

	if err != nil {
		t.Errorf("Transition(StateSearching) from DISCONNECTED should succeed, got error: %v", err)
	}
	if sm.Current() != StateSearching {
		t.Errorf("Current() = %v, want StateSearching", sm.Current())
	}
}

// Test CLAIMING -> SEARCHING (claim failed path)
func TestStateMachine_Transition_ClaimingToSearching(t *testing.T) {
	sm := NewStateMachine(nil)

	// Setup: STARTUP -> SEARCHING -> CLAIMING
	_ = sm.Transition(StateSearching)
	_ = sm.Transition(StateClaiming)

	// Claim failed: CLAIMING -> SEARCHING
	err := sm.Transition(StateSearching)

	if err != nil {
		t.Errorf("Transition(StateSearching) from CLAIMING should succeed, got error: %v", err)
	}
	if sm.Current() != StateSearching {
		t.Errorf("Current() = %v, want StateSearching", sm.Current())
	}
}

// Test MONITORING -> DISCONNECTED (device lost path)
func TestStateMachine_Transition_MonitoringToDisconnected(t *testing.T) {
	sm := NewStateMachine(nil)

	// Setup: STARTUP -> SEARCHING -> CLAIMING -> MONITORING
	_ = sm.Transition(StateSearching)
	_ = sm.Transition(StateClaiming)
	_ = sm.Transition(StateMonitoring)

	// Device lost: MONITORING -> DISCONNECTED
	err := sm.Transition(StateDisconnected)

	if err != nil {
		t.Errorf("Transition(StateDisconnected) from MONITORING should succeed, got error: %v", err)
	}
	if sm.Current() != StateDisconnected {
		t.Errorf("Current() = %v, want StateDisconnected", sm.Current())
	}
}

// Helper function to check if a string contains a substring
func containsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
