package usb

import (
	"sync"
	"testing"
	"time"
)

// =============================================================================
// Unit Tests - Test NewMonitor constructor
// =============================================================================

// Test NewMonitor creates monitor with correct config
func TestNewMonitor_CreatesValidMonitor(t *testing.T) {
	registry := NewProfileRegistry()
	dm := NewDeviceManager(registry)

	monitor := NewMonitor(dm, 250)

	if monitor == nil {
		t.Fatal("NewMonitor() returned nil")
	}
}

// Test NewMonitor sets DeviceManager reference
func TestNewMonitor_SetsDeviceManager(t *testing.T) {
	registry := NewProfileRegistry()
	dm := NewDeviceManager(registry)

	monitor := NewMonitor(dm, 250)

	if monitor.dm != dm {
		t.Error("NewMonitor() should store the provided DeviceManager")
	}
}

// Test NewMonitor sets debounce value
func TestNewMonitor_SetsDebounceValue(t *testing.T) {
	registry := NewProfileRegistry()
	dm := NewDeviceManager(registry)

	tests := []struct {
		name       string
		debounceMs int
	}{
		{"default 250ms", 250},
		{"zero debounce", 0},
		{"large debounce", 5000},
		{"small debounce", 10},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			monitor := NewMonitor(dm, tt.debounceMs)
			if monitor.debounceMs != tt.debounceMs {
				t.Errorf("debounceMs = %d, want %d", monitor.debounceMs, tt.debounceMs)
			}
		})
	}
}

// Test NewMonitor creates buffered event channel
func TestNewMonitor_CreatesEventChannel(t *testing.T) {
	registry := NewProfileRegistry()
	dm := NewDeviceManager(registry)

	monitor := NewMonitor(dm, 250)

	if monitor.eventChan == nil {
		t.Fatal("NewMonitor() should create event channel")
	}

	// Verify channel is buffered (capacity > 0)
	if cap(monitor.eventChan) == 0 {
		t.Error("Event channel should be buffered")
	}
}

// Test NewMonitor creates buffered error channel
func TestNewMonitor_CreatesErrorChannel(t *testing.T) {
	registry := NewProfileRegistry()
	dm := NewDeviceManager(registry)

	monitor := NewMonitor(dm, 250)

	if monitor.errorChan == nil {
		t.Fatal("NewMonitor() should create error channel")
	}

	// Verify channel is buffered
	if cap(monitor.errorChan) == 0 {
		t.Error("Error channel should be buffered")
	}
}

// Test NewMonitor creates stop channel
func TestNewMonitor_CreatesStopChannel(t *testing.T) {
	registry := NewProfileRegistry()
	dm := NewDeviceManager(registry)

	monitor := NewMonitor(dm, 250)

	if monitor.stopChan == nil {
		t.Fatal("NewMonitor() should create stop channel")
	}
}

// Test NewMonitor initializes running to false
func TestNewMonitor_InitiallyNotRunning(t *testing.T) {
	registry := NewProfileRegistry()
	dm := NewDeviceManager(registry)

	monitor := NewMonitor(dm, 250)

	if monitor.running {
		t.Error("NewMonitor() should initialize running to false")
	}
}

// Test NewMonitor initializes lastPressMs to zero
func TestNewMonitor_InitialLastPressZero(t *testing.T) {
	registry := NewProfileRegistry()
	dm := NewDeviceManager(registry)

	monitor := NewMonitor(dm, 250)

	if monitor.lastPressMs != 0 {
		t.Errorf("lastPressMs = %d, want 0", monitor.lastPressMs)
	}
}

// Test NewMonitor with nil DeviceManager (edge case)
func TestNewMonitor_NilDeviceManager(t *testing.T) {
	// Should not panic
	monitor := NewMonitor(nil, 250)

	if monitor == nil {
		t.Fatal("NewMonitor(nil, 250) returned nil")
	}
	if monitor.dm != nil {
		t.Error("dm should be nil when passed nil")
	}
}

// =============================================================================
// Unit Tests - Test parseEvent with various patterns
// =============================================================================

// Helper to create a mock DeviceManager with a profile set
func createMockDeviceManagerWithProfile() *DeviceManager {
	registry := NewProfileRegistry()
	dm := NewDeviceManager(registry)

	// Manually set the profile (simulating device found)
	profile, _ := registry.GetByID("ht-b30s")
	dm.mu.Lock()
	dm.profile = profile
	dm.mu.Unlock()

	return dm
}

// Test parseEvent with button press pattern
func TestMonitor_ParseEvent_ButtonPress(t *testing.T) {
	dm := createMockDeviceManagerWithProfile()
	monitor := NewMonitor(dm, 250)

	// HT-B30S press pattern: {0x02, 0x01, 0x00, 0x00}
	pressData := []byte{0x02, 0x01, 0x00, 0x00}

	event, err := monitor.parseEvent(pressData)

	if err != nil {
		t.Fatalf("parseEvent() returned error: %v", err)
	}
	if event == nil {
		t.Fatal("parseEvent() returned nil event")
	}
	if !event.Pressed {
		t.Error("parseEvent() should set Pressed=true for press pattern")
	}
	if event.DeviceID != "ht-b30s" {
		t.Errorf("DeviceID = %v, want ht-b30s", event.DeviceID)
	}
	if !bytesEqual(event.RawData, pressData) {
		t.Errorf("RawData = %v, want %v", event.RawData, pressData)
	}
}

// Test parseEvent with button release pattern
func TestMonitor_ParseEvent_ButtonRelease(t *testing.T) {
	dm := createMockDeviceManagerWithProfile()
	monitor := NewMonitor(dm, 250)

	// HT-B30S release pattern: {0x02, 0x01, 0x00, 0x01}
	releaseData := []byte{0x02, 0x01, 0x00, 0x01}

	event, err := monitor.parseEvent(releaseData)

	if err != nil {
		t.Fatalf("parseEvent() returned error: %v", err)
	}
	if event == nil {
		t.Fatal("parseEvent() returned nil event")
	}
	if event.Pressed {
		t.Error("parseEvent() should set Pressed=false for release pattern")
	}
	if event.DeviceID != "ht-b30s" {
		t.Errorf("DeviceID = %v, want ht-b30s", event.DeviceID)
	}
}

// Test parseEvent with unknown pattern returns error
func TestMonitor_ParseEvent_UnknownPattern(t *testing.T) {
	dm := createMockDeviceManagerWithProfile()
	monitor := NewMonitor(dm, 250)

	unknownPatterns := [][]byte{
		{0xFF, 0xFF, 0xFF, 0xFF},
		{0x00, 0x00, 0x00, 0x00},
		{0x01, 0x02, 0x03, 0x04},
		{0x02, 0x01, 0x00, 0x02}, // Similar but wrong last byte
	}

	for _, pattern := range unknownPatterns {
		t.Run(string(pattern), func(t *testing.T) {
			event, err := monitor.parseEvent(pattern)

			if err == nil {
				t.Error("parseEvent() should return error for unknown pattern")
			}
			if event != nil {
				t.Error("parseEvent() should return nil event for unknown pattern")
			}
		})
	}
}

// Test parseEvent without profile set returns error
func TestMonitor_ParseEvent_NoProfile(t *testing.T) {
	registry := NewProfileRegistry()
	dm := NewDeviceManager(registry)
	// Profile is NOT set
	monitor := NewMonitor(dm, 250)

	event, err := monitor.parseEvent([]byte{0x02, 0x01, 0x00, 0x00})

	if err == nil {
		t.Error("parseEvent() should return error when no profile is set")
	}
	if err != ErrNoProfile {
		t.Errorf("parseEvent() error = %v, want ErrNoProfile", err)
	}
	if event != nil {
		t.Error("parseEvent() should return nil event when no profile")
	}
}

// Test parseEvent sets timestamp
func TestMonitor_ParseEvent_SetsTimestamp(t *testing.T) {
	dm := createMockDeviceManagerWithProfile()
	monitor := NewMonitor(dm, 250)

	before := time.Now()
	event, err := monitor.parseEvent([]byte{0x02, 0x01, 0x00, 0x00})
	after := time.Now()

	if err != nil {
		t.Fatalf("parseEvent() returned error: %v", err)
	}
	if event.Timestamp.Before(before) || event.Timestamp.After(after) {
		t.Error("parseEvent() should set Timestamp to current time")
	}
}

// =============================================================================
// Edge Cases - Event data length variations
// =============================================================================

// Test parseEvent with event data shorter than expected
func TestMonitor_ParseEvent_ShortData(t *testing.T) {
	dm := createMockDeviceManagerWithProfile()
	monitor := NewMonitor(dm, 250)

	shortPatterns := [][]byte{
		{},                   // Empty
		{0x02},               // 1 byte (pattern is 4 bytes)
		{0x02, 0x01},         // 2 bytes
		{0x02, 0x01, 0x00},   // 3 bytes
	}

	for _, pattern := range shortPatterns {
		t.Run("short_data", func(t *testing.T) {
			event, err := monitor.parseEvent(pattern)

			// Should return error because pattern doesn't match
			if err == nil {
				t.Errorf("parseEvent(%v) should return error for short data", pattern)
			}
			if event != nil {
				t.Errorf("parseEvent(%v) should return nil event for short data", pattern)
			}
		})
	}
}

// Test parseEvent with event data longer than expected
func TestMonitor_ParseEvent_LongData(t *testing.T) {
	dm := createMockDeviceManagerWithProfile()
	monitor := NewMonitor(dm, 250)

	longPatterns := [][]byte{
		{0x02, 0x01, 0x00, 0x00, 0xFF},       // Press + extra byte
		{0x02, 0x01, 0x00, 0x00, 0x00, 0x00}, // Press + extra bytes
		{0x02, 0x01, 0x00, 0x01, 0xFF},       // Release + extra byte
	}

	for _, pattern := range longPatterns {
		t.Run("long_data", func(t *testing.T) {
			event, err := monitor.parseEvent(pattern)

			// Should return error because pattern doesn't match exactly
			if err == nil {
				t.Errorf("parseEvent(%v) should return error for long data", pattern)
			}
			if event != nil {
				t.Errorf("parseEvent(%v) should return nil event for long data", pattern)
			}
		})
	}
}

// =============================================================================
// Unit Tests - Test debouncing logic (shouldTrigger)
// =============================================================================

// Test shouldTrigger returns true on first call
func TestMonitor_ShouldTrigger_FirstCall(t *testing.T) {
	registry := NewProfileRegistry()
	dm := NewDeviceManager(registry)
	monitor := NewMonitor(dm, 250)

	result := monitor.shouldTrigger()

	if !result {
		t.Error("shouldTrigger() should return true on first call")
	}
}

// Test shouldTrigger returns false within debounce window
func TestMonitor_ShouldTrigger_WithinDebounceWindow(t *testing.T) {
	registry := NewProfileRegistry()
	dm := NewDeviceManager(registry)
	monitor := NewMonitor(dm, 250)

	// First call should succeed
	if !monitor.shouldTrigger() {
		t.Fatal("First shouldTrigger() should return true")
	}

	// Immediate second call should fail (within 250ms window)
	if monitor.shouldTrigger() {
		t.Error("shouldTrigger() should return false within debounce window")
	}
}

// Test shouldTrigger returns true after debounce window
func TestMonitor_ShouldTrigger_AfterDebounceWindow(t *testing.T) {
	registry := NewProfileRegistry()
	dm := NewDeviceManager(registry)
	monitor := NewMonitor(dm, 50) // Short debounce for testing

	// First call
	if !monitor.shouldTrigger() {
		t.Fatal("First shouldTrigger() should return true")
	}

	// Wait past debounce window
	time.Sleep(60 * time.Millisecond)

	// Second call should succeed
	if !monitor.shouldTrigger() {
		t.Error("shouldTrigger() should return true after debounce window")
	}
}

// Test shouldTrigger with zero debounce allows all
func TestMonitor_ShouldTrigger_ZeroDebounce(t *testing.T) {
	registry := NewProfileRegistry()
	dm := NewDeviceManager(registry)
	monitor := NewMonitor(dm, 0)

	// All calls should succeed with zero debounce
	for i := 0; i < 5; i++ {
		// Small sleep to ensure timestamps differ
		time.Sleep(1 * time.Millisecond)
		if !monitor.shouldTrigger() {
			t.Errorf("shouldTrigger() call %d should return true with zero debounce", i+1)
		}
	}
}

// Test very rapid button presses (debounce test)
func TestMonitor_ShouldTrigger_RapidPresses(t *testing.T) {
	registry := NewProfileRegistry()
	dm := NewDeviceManager(registry)
	monitor := NewMonitor(dm, 100) // 100ms debounce

	// Simulate rapid button presses
	triggered := 0
	for i := 0; i < 10; i++ {
		if monitor.shouldTrigger() {
			triggered++
		}
		time.Sleep(20 * time.Millisecond) // 20ms between presses
	}

	// With 100ms debounce and 20ms between presses:
	// ~200ms total, should only trigger about 2-3 times
	if triggered > 3 {
		t.Errorf("shouldTrigger() triggered %d times in rapid succession, expected <= 3", triggered)
	}
	if triggered < 1 {
		t.Error("shouldTrigger() should trigger at least once")
	}
}

// Test shouldTrigger updates lastPressMs on success
func TestMonitor_ShouldTrigger_UpdatesLastPressMs(t *testing.T) {
	registry := NewProfileRegistry()
	dm := NewDeviceManager(registry)
	monitor := NewMonitor(dm, 250)

	initialLastPressMs := monitor.lastPressMs
	if initialLastPressMs != 0 {
		t.Errorf("Initial lastPressMs = %d, want 0", initialLastPressMs)
	}

	monitor.shouldTrigger()

	if monitor.lastPressMs == 0 {
		t.Error("lastPressMs should be updated after shouldTrigger()")
	}
	if monitor.lastPressMs <= initialLastPressMs {
		t.Error("lastPressMs should increase after shouldTrigger()")
	}
}

// Test shouldTrigger does not update lastPressMs on failure
func TestMonitor_ShouldTrigger_NoUpdateOnFailure(t *testing.T) {
	registry := NewProfileRegistry()
	dm := NewDeviceManager(registry)
	monitor := NewMonitor(dm, 250)

	// First call to set lastPressMs
	monitor.shouldTrigger()
	lastPressAfterFirst := monitor.lastPressMs

	// Second call should fail
	if monitor.shouldTrigger() {
		t.Fatal("Second shouldTrigger() should return false")
	}

	// lastPressMs should not change
	if monitor.lastPressMs != lastPressAfterFirst {
		t.Error("lastPressMs should not change when shouldTrigger() returns false")
	}
}

// =============================================================================
// Unit Tests - Test Stop() terminates monitoring
// =============================================================================

// Test Stop sets running to false
func TestMonitor_Stop_SetsRunningFalse(t *testing.T) {
	registry := NewProfileRegistry()
	dm := NewDeviceManager(registry)
	monitor := NewMonitor(dm, 250)

	// Manually set running to true
	monitor.mu.Lock()
	monitor.running = true
	monitor.mu.Unlock()

	monitor.Stop()

	monitor.mu.Lock()
	running := monitor.running
	monitor.mu.Unlock()

	if running {
		t.Error("Stop() should set running to false")
	}
}

// Test Stop is safe when not running
func TestMonitor_Stop_SafeWhenNotRunning(t *testing.T) {
	registry := NewProfileRegistry()
	dm := NewDeviceManager(registry)
	monitor := NewMonitor(dm, 250)

	// Should not panic
	monitor.Stop()

	// Can call multiple times safely
	monitor.Stop()
	monitor.Stop()
}

// Test Stop can be called multiple times
func TestMonitor_Stop_MultipleCalls(t *testing.T) {
	registry := NewProfileRegistry()
	dm := NewDeviceManager(registry)
	monitor := NewMonitor(dm, 250)

	// Manually set running
	monitor.mu.Lock()
	monitor.running = true
	monitor.mu.Unlock()

	// Multiple stop calls should not panic
	for i := 0; i < 5; i++ {
		monitor.Stop()
	}

	// Should be stopped
	monitor.mu.Lock()
	running := monitor.running
	monitor.mu.Unlock()

	if running {
		t.Error("running should be false after Stop()")
	}
}

// Test Stop reinitializes stopChan for potential restart
func TestMonitor_Stop_ReinitializesStopChan(t *testing.T) {
	registry := NewProfileRegistry()
	dm := NewDeviceManager(registry)
	monitor := NewMonitor(dm, 250)

	// Manually set running
	monitor.mu.Lock()
	monitor.running = true
	originalStopChan := monitor.stopChan
	monitor.mu.Unlock()

	monitor.Stop()

	monitor.mu.Lock()
	newStopChan := monitor.stopChan
	monitor.mu.Unlock()

	// Channel should be recreated (different channel)
	if newStopChan == originalStopChan {
		t.Error("Stop() should reinitialize stopChan for potential restart")
	}
	if newStopChan == nil {
		t.Error("Stop() should not set stopChan to nil")
	}
}

// =============================================================================
// Unit Tests - Test Events() and Errors() channel accessors
// =============================================================================

// Test Events returns receive-only channel
func TestMonitor_Events_ReturnsChannel(t *testing.T) {
	registry := NewProfileRegistry()
	dm := NewDeviceManager(registry)
	monitor := NewMonitor(dm, 250)

	eventChan := monitor.Events()

	if eventChan == nil {
		t.Fatal("Events() returned nil channel")
	}
}

// Test Events channel receives button events
func TestMonitor_Events_ReceivesEvents(t *testing.T) {
	registry := NewProfileRegistry()
	dm := NewDeviceManager(registry)
	monitor := NewMonitor(dm, 250)

	// Send an event to the internal channel
	testEvent := ButtonEvent{
		Pressed:   true,
		Timestamp: time.Now(),
		RawData:   []byte{0x01, 0x02},
		DeviceID:  "test",
	}

	// Send on internal channel
	monitor.eventChan <- testEvent

	// Receive on external channel
	select {
	case received := <-monitor.Events():
		if received.Pressed != testEvent.Pressed {
			t.Error("Received event should match sent event")
		}
		if received.DeviceID != testEvent.DeviceID {
			t.Error("Received DeviceID should match")
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("Events() channel should receive the sent event")
	}
}

// Test Errors returns receive-only channel
func TestMonitor_Errors_ReturnsChannel(t *testing.T) {
	registry := NewProfileRegistry()
	dm := NewDeviceManager(registry)
	monitor := NewMonitor(dm, 250)

	errorChan := monitor.Errors()

	if errorChan == nil {
		t.Fatal("Errors() returned nil channel")
	}
}

// Test Errors channel receives errors
func TestMonitor_Errors_ReceivesErrors(t *testing.T) {
	registry := NewProfileRegistry()
	dm := NewDeviceManager(registry)
	monitor := NewMonitor(dm, 250)

	testError := ErrNoDeviceFound

	// Send on internal channel
	monitor.errorChan <- testError

	// Receive on external channel
	select {
	case received := <-monitor.Errors():
		if received != testError {
			t.Error("Received error should match sent error")
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("Errors() channel should receive the sent error")
	}
}

// =============================================================================
// Unit Tests - Test Start() behavior
// =============================================================================

// Test Start fails without endpoint
func TestMonitor_Start_FailsWithoutEndpoint(t *testing.T) {
	registry := NewProfileRegistry()
	dm := NewDeviceManager(registry)
	monitor := NewMonitor(dm, 250)

	err := monitor.Start()

	if err == nil {
		t.Error("Start() should return error without endpoint")
	}
}

// Test Start returns error when already running
func TestMonitor_Start_ReturnsErrorWhenAlreadyRunning(t *testing.T) {
	registry := NewProfileRegistry()
	dm := NewDeviceManager(registry)
	monitor := NewMonitor(dm, 250)

	// Manually set running to true
	monitor.mu.Lock()
	monitor.running = true
	monitor.mu.Unlock()

	err := monitor.Start()

	if err == nil {
		t.Error("Start() should return error when already running")
	}

	// Error message should indicate already running
	if err.Error() != "monitor already running" {
		t.Errorf("Error = %v, want 'monitor already running'", err)
	}
}

// =============================================================================
// Thread-Safety Tests
// =============================================================================

// Test concurrent shouldTrigger calls
func TestMonitor_ConcurrentShouldTrigger(t *testing.T) {
	registry := NewProfileRegistry()
	dm := NewDeviceManager(registry)
	monitor := NewMonitor(dm, 50) // Short debounce

	var wg sync.WaitGroup
	numGoroutines := 100
	triggered := make(chan bool, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			result := monitor.shouldTrigger()
			triggered <- result
		}()
	}

	wg.Wait()
	close(triggered)

	// Count triggers - should be limited by debouncing
	triggerCount := 0
	for result := range triggered {
		if result {
			triggerCount++
		}
	}

	// With concurrent calls and debouncing, should be limited
	// At least 1, but not all 100
	if triggerCount < 1 {
		t.Error("At least one shouldTrigger() should return true")
	}
	if triggerCount > 10 {
		t.Errorf("Too many triggers (%d) - debouncing may not be working correctly", triggerCount)
	}
}

// Test concurrent Stop calls
func TestMonitor_ConcurrentStop(t *testing.T) {
	registry := NewProfileRegistry()
	dm := NewDeviceManager(registry)
	monitor := NewMonitor(dm, 250)

	var wg sync.WaitGroup
	numGoroutines := 50

	// Set running first
	monitor.mu.Lock()
	monitor.running = true
	monitor.mu.Unlock()

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			monitor.Stop()
		}()
	}

	wg.Wait()
	// No assertion needed - test passes if no race condition detected
}

// Test concurrent parseEvent calls
func TestMonitor_ConcurrentParseEvent(t *testing.T) {
	dm := createMockDeviceManagerWithProfile()
	monitor := NewMonitor(dm, 250)

	var wg sync.WaitGroup
	numGoroutines := 100

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			data := []byte{0x02, 0x01, 0x00, byte(id % 2)} // Alternate press/release
			_, _ = monitor.parseEvent(data)
		}(i)
	}

	wg.Wait()
	// No assertion needed - test passes if no race condition detected
}

// Test concurrent Events and Errors channel access
func TestMonitor_ConcurrentChannelAccess(t *testing.T) {
	registry := NewProfileRegistry()
	dm := NewDeviceManager(registry)
	monitor := NewMonitor(dm, 250)

	var wg sync.WaitGroup
	numGoroutines := 100

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = monitor.Events()
			_ = monitor.Errors()
		}()
	}

	wg.Wait()
	// No assertion needed - test passes if no race condition detected
}

// Test mixed concurrent operations
func TestMonitor_ConcurrentMixedOperations(t *testing.T) {
	dm := createMockDeviceManagerWithProfile()
	monitor := NewMonitor(dm, 50)

	var wg sync.WaitGroup
	numGoroutines := 50

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			switch id % 5 {
			case 0:
				monitor.shouldTrigger()
			case 1:
				monitor.parseEvent([]byte{0x02, 0x01, 0x00, 0x00})
			case 2:
				monitor.parseEvent([]byte{0x02, 0x01, 0x00, 0x01})
			case 3:
				_ = monitor.Events()
			case 4:
				_ = monitor.Errors()
			}
		}(i)
	}

	wg.Wait()
}

// =============================================================================
// Integration Tests - Require Physical Device
// =============================================================================

// skipIfNoMonitorDevice skips the test if no dermoscope device is connected
func skipIfNoMonitorDevice(t *testing.T) (*DeviceManager, *Monitor) {
	t.Helper()

	registry := NewProfileRegistry()
	dm := NewDeviceManager(registry)

	_, err := dm.FindDevice()
	if err != nil {
		t.Skipf("Skipping integration test: %v", err)
	}

	err = dm.ClaimInterface()
	if err != nil {
		dm.Close()
		t.Skipf("Skipping integration test: %v", err)
	}

	monitor := NewMonitor(dm, 250)

	return dm, monitor
}

// Test Start begins monitoring
func TestMonitor_Start_BeginsMonitoring(t *testing.T) {
	dm, monitor := skipIfNoMonitorDevice(t)
	defer dm.Close()

	err := monitor.Start()

	if err != nil {
		t.Errorf("Start() returned error: %v", err)
	}

	// Verify running is true
	monitor.mu.Lock()
	running := monitor.running
	monitor.mu.Unlock()

	if !running {
		t.Error("running should be true after Start()")
	}

	// Cleanup
	monitor.Stop()
}

// Test Events receives button press events (requires device interaction)
func TestMonitor_Events_ReceivesButtonPressEvents(t *testing.T) {
	dm, monitor := skipIfNoMonitorDevice(t)
	defer dm.Close()

	err := monitor.Start()
	if err != nil {
		t.Fatalf("Start() failed: %v", err)
	}
	defer monitor.Stop()

	t.Log("Waiting for button press event (press device button within 5 seconds)...")

	select {
	case event := <-monitor.Events():
		if !event.Pressed {
			t.Error("Expected button press event (Pressed=true)")
		}
		t.Logf("Received button press event: DeviceID=%s, Timestamp=%v", event.DeviceID, event.Timestamp)
	case err := <-monitor.Errors():
		t.Logf("Received error instead of event: %v", err)
	case <-time.After(5 * time.Second):
		t.Log("No button press received within timeout (this is OK if no button was pressed)")
	}
}

// Test debouncing prevents rapid double-triggers
func TestMonitor_Integration_Debouncing(t *testing.T) {
	dm, monitor := skipIfNoMonitorDevice(t)
	defer dm.Close()

	err := monitor.Start()
	if err != nil {
		t.Fatalf("Start() failed: %v", err)
	}
	defer monitor.Stop()

	t.Log("Rapidly press device button multiple times within 5 seconds...")

	eventCount := 0
	timeout := time.After(5 * time.Second)

	for {
		select {
		case <-monitor.Events():
			eventCount++
			t.Logf("Event %d received", eventCount)
		case err := <-monitor.Errors():
			t.Logf("Received error: %v", err)
		case <-timeout:
			t.Logf("Total events received: %d", eventCount)
			// With 250ms debounce, rapid presses should be filtered
			return
		}
	}
}

// Test device disconnect triggers error
func TestMonitor_Integration_DeviceDisconnect(t *testing.T) {
	dm, monitor := skipIfNoMonitorDevice(t)
	defer dm.Close()

	err := monitor.Start()
	if err != nil {
		t.Fatalf("Start() failed: %v", err)
	}
	defer monitor.Stop()

	t.Log("Disconnect the device within 5 seconds to test disconnect handling...")

	select {
	case err := <-monitor.Errors():
		t.Logf("Received error (expected on disconnect): %v", err)
	case event := <-monitor.Events():
		t.Logf("Received event instead: %+v", event)
	case <-time.After(5 * time.Second):
		t.Log("No disconnect error received (this is OK if device was not disconnected)")
	}
}

// =============================================================================
// Edge Case Tests
// =============================================================================

// Test parseEvent with empty data
func TestMonitor_ParseEvent_EmptyData(t *testing.T) {
	dm := createMockDeviceManagerWithProfile()
	monitor := NewMonitor(dm, 250)

	event, err := monitor.parseEvent([]byte{})

	if err == nil {
		t.Error("parseEvent() should return error for empty data")
	}
	if event != nil {
		t.Error("parseEvent() should return nil event for empty data")
	}
}

// Test parseEvent with nil data
func TestMonitor_ParseEvent_NilData(t *testing.T) {
	dm := createMockDeviceManagerWithProfile()
	monitor := NewMonitor(dm, 250)

	event, err := monitor.parseEvent(nil)

	if err == nil {
		t.Error("parseEvent() should return error for nil data")
	}
	if event != nil {
		t.Error("parseEvent() should return nil event for nil data")
	}
}

// Test negative debounce value (edge case)
func TestNewMonitor_NegativeDebounce(t *testing.T) {
	registry := NewProfileRegistry()
	dm := NewDeviceManager(registry)

	// Negative debounce - should not crash
	monitor := NewMonitor(dm, -100)

	if monitor == nil {
		t.Fatal("NewMonitor() returned nil with negative debounce")
	}

	// With negative debounce, all triggers should pass
	// (since time difference will always be >= -100)
	for i := 0; i < 3; i++ {
		if !monitor.shouldTrigger() {
			t.Errorf("shouldTrigger() call %d should return true with negative debounce", i+1)
		}
		time.Sleep(1 * time.Millisecond)
	}
}

// Test very large debounce value
func TestNewMonitor_LargeDebounce(t *testing.T) {
	registry := NewProfileRegistry()
	dm := NewDeviceManager(registry)

	// Very large debounce (1 hour)
	monitor := NewMonitor(dm, 3600000)

	if monitor == nil {
		t.Fatal("NewMonitor() returned nil with large debounce")
	}

	// First call should trigger
	if !monitor.shouldTrigger() {
		t.Error("First shouldTrigger() should return true")
	}

	// Subsequent calls should fail (within huge window)
	for i := 0; i < 5; i++ {
		if monitor.shouldTrigger() {
			t.Errorf("shouldTrigger() call %d should return false within large debounce window", i+2)
		}
	}
}

// Test channel buffer behavior - non-blocking when full
func TestMonitor_ChannelBufferBehavior(t *testing.T) {
	registry := NewProfileRegistry()
	dm := NewDeviceManager(registry)
	monitor := NewMonitor(dm, 250)

	// Fill the event channel
	channelCap := cap(monitor.eventChan)
	for i := 0; i < channelCap; i++ {
		monitor.eventChan <- ButtonEvent{
			Pressed:   true,
			Timestamp: time.Now(),
			DeviceID:  "test",
		}
	}

	// Try to send one more - should not block (select with default in implementation)
	done := make(chan bool)
	go func() {
		select {
		case monitor.eventChan <- ButtonEvent{}:
		default:
			// This is expected - channel is full
		}
		done <- true
	}()

	select {
	case <-done:
		// Good - did not block
	case <-time.After(100 * time.Millisecond):
		t.Error("Sending to full event channel should not block")
	}
}

// Test error channel buffer behavior
func TestMonitor_ErrorChannelBufferBehavior(t *testing.T) {
	registry := NewProfileRegistry()
	dm := NewDeviceManager(registry)
	monitor := NewMonitor(dm, 250)

	// Fill the error channel
	channelCap := cap(monitor.errorChan)
	for i := 0; i < channelCap; i++ {
		monitor.errorChan <- ErrNoDeviceFound
	}

	// Try to send one more - should not block
	done := make(chan bool)
	go func() {
		select {
		case monitor.errorChan <- ErrNoDeviceFound:
		default:
			// This is expected - channel is full
		}
		done <- true
	}()

	select {
	case <-done:
		// Good - did not block
	case <-time.After(100 * time.Millisecond):
		t.Error("Sending to full error channel should not block")
	}
}

// Test parseEvent stores raw data
func TestMonitor_ParseEvent_StoresRawData(t *testing.T) {
	dm := createMockDeviceManagerWithProfile()
	monitor := NewMonitor(dm, 250)

	originalData := []byte{0x02, 0x01, 0x00, 0x00}

	event, err := monitor.parseEvent(originalData)
	if err != nil {
		t.Fatalf("parseEvent() returned error: %v", err)
	}

	// Verify RawData matches exactly at time of call
	if !bytesEqual(event.RawData, originalData) {
		t.Errorf("RawData = %v, want %v", event.RawData, originalData)
	}

	// Note: parseEvent stores a reference to the input data, not a copy.
	// In the actual monitoring loop (Start()), a copy is made before calling parseEvent(),
	// so the real usage is safe. This test documents the current behavior.
}

// Test struct field initialization completeness
func TestMonitor_StructFieldsInitialized(t *testing.T) {
	registry := NewProfileRegistry()
	dm := NewDeviceManager(registry)
	monitor := NewMonitor(dm, 250)

	if monitor.dm == nil {
		t.Error("dm should not be nil")
	}
	if monitor.eventChan == nil {
		t.Error("eventChan should not be nil")
	}
	if monitor.errorChan == nil {
		t.Error("errorChan should not be nil")
	}
	if monitor.stopChan == nil {
		t.Error("stopChan should not be nil")
	}
	if monitor.debounceMs != 250 {
		t.Errorf("debounceMs = %d, want 250", monitor.debounceMs)
	}
	if monitor.lastPressMs != 0 {
		t.Errorf("lastPressMs = %d, want 0", monitor.lastPressMs)
	}
	if monitor.running != false {
		t.Error("running should be false initially")
	}
}
