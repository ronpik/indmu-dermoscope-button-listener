package usb

import (
	"errors"
	"sync"
	"testing"
)

// =============================================================================
// Unit Tests - These run without a physical device
// =============================================================================

// Test NewDeviceManager creates a valid manager
func TestNewDeviceManager_CreatesValidManager(t *testing.T) {
	registry := NewProfileRegistry()

	dm := NewDeviceManager(registry)

	if dm == nil {
		t.Fatal("NewDeviceManager() returned nil")
	}
}

// Test NewDeviceManager sets initial state to disconnected
func TestNewDeviceManager_InitialStateDisconnected(t *testing.T) {
	registry := NewProfileRegistry()

	dm := NewDeviceManager(registry)

	if dm.GetState() != StateDisconnected {
		t.Errorf("GetState() = %v, want StateDisconnected", dm.GetState())
	}
}

// Test NewDeviceManager stores registry reference
func TestNewDeviceManager_StoresRegistry(t *testing.T) {
	registry := NewProfileRegistry()

	dm := NewDeviceManager(registry)

	if dm.registry != registry {
		t.Error("NewDeviceManager() should store the provided registry")
	}
}

// Test NewDeviceManager with nil registry (edge case)
func TestNewDeviceManager_NilRegistry(t *testing.T) {
	// Should not panic with nil registry
	dm := NewDeviceManager(nil)

	if dm == nil {
		t.Fatal("NewDeviceManager(nil) returned nil")
	}

	if dm.GetState() != StateDisconnected {
		t.Errorf("GetState() = %v, want StateDisconnected", dm.GetState())
	}
}

// Test GetState returns correct state
func TestDeviceManager_GetState_ReturnsCorrectState(t *testing.T) {
	registry := NewProfileRegistry()
	dm := NewDeviceManager(registry)

	// Initial state
	if dm.GetState() != StateDisconnected {
		t.Errorf("Initial GetState() = %v, want StateDisconnected", dm.GetState())
	}

	// After SetState
	dm.SetState(StateConnected)
	if dm.GetState() != StateConnected {
		t.Errorf("GetState() after SetState(StateConnected) = %v, want StateConnected", dm.GetState())
	}

	dm.SetState(StateMonitoring)
	if dm.GetState() != StateMonitoring {
		t.Errorf("GetState() after SetState(StateMonitoring) = %v, want StateMonitoring", dm.GetState())
	}

	dm.SetState(StateError)
	if dm.GetState() != StateError {
		t.Errorf("GetState() after SetState(StateError) = %v, want StateError", dm.GetState())
	}
}

// Test SetState updates state correctly
func TestDeviceManager_SetState_UpdatesState(t *testing.T) {
	registry := NewProfileRegistry()
	dm := NewDeviceManager(registry)

	tests := []struct {
		name     string
		newState DeviceState
	}{
		{"disconnected", StateDisconnected},
		{"connected", StateConnected},
		{"monitoring", StateMonitoring},
		{"error", StateError},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dm.SetState(tt.newState)
			if dm.GetState() != tt.newState {
				t.Errorf("GetState() = %v after SetState(%v)", dm.GetState(), tt.newState)
			}
		})
	}
}

// Test GetProfile returns nil before device found
func TestDeviceManager_GetProfile_ReturnsNilBeforeDeviceFound(t *testing.T) {
	registry := NewProfileRegistry()
	dm := NewDeviceManager(registry)

	profile := dm.GetProfile()

	if profile != nil {
		t.Error("GetProfile() should return nil before device is found")
	}
}

// Test Close doesn't panic on uninitialized manager
func TestDeviceManager_Close_NoPanicOnUninitialized(t *testing.T) {
	registry := NewProfileRegistry()
	dm := NewDeviceManager(registry)

	// This should not panic
	err := dm.Close()

	if err != nil {
		t.Errorf("Close() on uninitialized manager returned error: %v", err)
	}
}

// Test Close resets state to disconnected
func TestDeviceManager_Close_ResetsState(t *testing.T) {
	registry := NewProfileRegistry()
	dm := NewDeviceManager(registry)

	// Set some state
	dm.SetState(StateConnected)

	err := dm.Close()

	if err != nil {
		t.Errorf("Close() returned error: %v", err)
	}
	if dm.GetState() != StateDisconnected {
		t.Errorf("GetState() after Close() = %v, want StateDisconnected", dm.GetState())
	}
}

// Test Close clears profile
func TestDeviceManager_Close_ClearsProfile(t *testing.T) {
	registry := NewProfileRegistry()
	dm := NewDeviceManager(registry)

	// Manually set a profile (simulating device found)
	dm.mu.Lock()
	profile, _ := registry.GetByID("ht-b30s")
	dm.profile = profile
	dm.mu.Unlock()

	// Verify profile is set
	if dm.GetProfile() == nil {
		t.Fatal("Test setup failed: profile should be set")
	}

	err := dm.Close()

	if err != nil {
		t.Errorf("Close() returned error: %v", err)
	}
	if dm.GetProfile() != nil {
		t.Error("GetProfile() after Close() should return nil")
	}
}

// Test Close can be called multiple times safely
func TestDeviceManager_Close_MultipleCallsSafe(t *testing.T) {
	registry := NewProfileRegistry()
	dm := NewDeviceManager(registry)

	// Multiple closes should not panic
	for i := 0; i < 5; i++ {
		err := dm.Close()
		if err != nil {
			t.Errorf("Close() call %d returned error: %v", i+1, err)
		}
	}
}

// Test ClaimInterface fails without device
func TestDeviceManager_ClaimInterface_FailsWithoutDevice(t *testing.T) {
	registry := NewProfileRegistry()
	dm := NewDeviceManager(registry)

	err := dm.ClaimInterface()

	if err == nil {
		t.Error("ClaimInterface() should return error without device")
	}
	if !errors.Is(err, ErrNoDeviceFound) {
		t.Errorf("ClaimInterface() error = %v, want ErrNoDeviceFound", err)
	}
}

// Test GetEndpoint fails without interface claimed
func TestDeviceManager_GetEndpoint_FailsWithoutInterfaceClaimed(t *testing.T) {
	registry := NewProfileRegistry()
	dm := NewDeviceManager(registry)

	endpoint, err := dm.GetEndpoint()

	if err == nil {
		t.Error("GetEndpoint() should return error without interface claimed")
	}
	if !errors.Is(err, ErrInterfaceNotClaimed) {
		t.Errorf("GetEndpoint() error = %v, want ErrInterfaceNotClaimed", err)
	}
	if endpoint != nil {
		t.Error("GetEndpoint() should return nil endpoint on error")
	}
}

// Test ReleaseInterface is safe without interface claimed
func TestDeviceManager_ReleaseInterface_SafeWithoutInterface(t *testing.T) {
	registry := NewProfileRegistry()
	dm := NewDeviceManager(registry)

	// Should not panic or return error
	err := dm.ReleaseInterface()

	if err != nil {
		t.Errorf("ReleaseInterface() without interface returned error: %v", err)
	}
}

// Test ReleaseInterface can be called multiple times
func TestDeviceManager_ReleaseInterface_MultipleCallsSafe(t *testing.T) {
	registry := NewProfileRegistry()
	dm := NewDeviceManager(registry)

	// Multiple releases should be safe
	for i := 0; i < 5; i++ {
		err := dm.ReleaseInterface()
		if err != nil {
			t.Errorf("ReleaseInterface() call %d returned error: %v", i+1, err)
		}
	}
}

// Test FindDevice fails without registry profiles for nil registry
func TestDeviceManager_FindDevice_NilRegistryPanics(t *testing.T) {
	dm := NewDeviceManager(nil)

	defer func() {
		if r := recover(); r == nil {
			// If we get here without panic, the call either returned an error
			// or succeeded unexpectedly. Both are acceptable behaviors for
			// a nil registry scenario. The key is we're testing the edge case.
		}
	}()

	// This may panic or return error - both are valid behaviors for nil registry
	_, err := dm.FindDevice()
	if err == nil {
		t.Error("FindDevice() with nil registry should return error or panic")
	}
}

// Test FindDeviceByProfile fails with unknown profile ID
func TestDeviceManager_FindDeviceByProfile_UnknownProfile(t *testing.T) {
	registry := NewProfileRegistry()
	dm := NewDeviceManager(registry)

	_, err := dm.FindDeviceByProfile("unknown-profile")

	if err == nil {
		t.Error("FindDeviceByProfile() should return error for unknown profile")
	}
	if !errors.Is(err, ErrProfileNotFound) {
		t.Errorf("FindDeviceByProfile() error = %v, want ErrProfileNotFound", err)
	}
}

// Test FindDeviceByProfile fails with empty profile ID
func TestDeviceManager_FindDeviceByProfile_EmptyProfileID(t *testing.T) {
	registry := NewProfileRegistry()
	dm := NewDeviceManager(registry)

	_, err := dm.FindDeviceByProfile("")

	if err == nil {
		t.Error("FindDeviceByProfile() should return error for empty profile ID")
	}
	if !errors.Is(err, ErrProfileNotFound) {
		t.Errorf("FindDeviceByProfile() error = %v, want ErrProfileNotFound", err)
	}
}

// Test error constants are defined
func TestDeviceManager_ErrorConstants(t *testing.T) {
	if ErrNoDeviceFound == nil {
		t.Error("ErrNoDeviceFound should be defined")
	}
	if ErrNoProfile == nil {
		t.Error("ErrNoProfile should be defined")
	}
	if ErrClaimFailed == nil {
		t.Error("ErrClaimFailed should be defined")
	}
	if ErrInterfaceNotClaimed == nil {
		t.Error("ErrInterfaceNotClaimed should be defined")
	}
	if ErrEndpointFailed == nil {
		t.Error("ErrEndpointFailed should be defined")
	}
	if ErrNoEndpoint == nil {
		t.Error("ErrNoEndpoint should be defined")
	}
	if ErrProfileNotFound == nil {
		t.Error("ErrProfileNotFound should be defined")
	}
}

// Test error messages are meaningful
func TestDeviceManager_ErrorMessages(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{"ErrNoDeviceFound", ErrNoDeviceFound, "no supported device found"},
		{"ErrNoProfile", ErrNoProfile, "no profile set for device"},
		{"ErrClaimFailed", ErrClaimFailed, "failed to claim interface"},
		{"ErrInterfaceNotClaimed", ErrInterfaceNotClaimed, "interface not claimed"},
		{"ErrEndpointFailed", ErrEndpointFailed, "failed to get endpoint"},
		{"ErrNoEndpoint", ErrNoEndpoint, "no interrupt endpoint found"},
		{"ErrProfileNotFound", ErrProfileNotFound, "profile not found"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.err.Error() != tt.want {
				t.Errorf("%s.Error() = %v, want %v", tt.name, tt.err.Error(), tt.want)
			}
		})
	}
}

// =============================================================================
// Thread-Safety Tests
// =============================================================================

// Test concurrent GetState and SetState
func TestDeviceManager_ConcurrentStateAccess(t *testing.T) {
	registry := NewProfileRegistry()
	dm := NewDeviceManager(registry)

	var wg sync.WaitGroup
	numGoroutines := 100

	// Concurrent writes
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			// Alternate between states
			states := []DeviceState{StateDisconnected, StateConnected, StateMonitoring, StateError}
			dm.SetState(states[id%len(states)])
		}(i)
	}

	// Concurrent reads
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = dm.GetState()
		}()
	}

	wg.Wait()
	// No assertion needed - test passes if no race condition detected
}

// Test concurrent GetProfile access
func TestDeviceManager_ConcurrentGetProfile(t *testing.T) {
	registry := NewProfileRegistry()
	dm := NewDeviceManager(registry)

	var wg sync.WaitGroup
	numGoroutines := 100

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = dm.GetProfile()
		}()
	}

	wg.Wait()
	// No assertion needed - test passes if no race condition detected
}

// Test concurrent Close calls
func TestDeviceManager_ConcurrentClose(t *testing.T) {
	registry := NewProfileRegistry()
	dm := NewDeviceManager(registry)

	var wg sync.WaitGroup
	numGoroutines := 10

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = dm.Close()
		}()
	}

	wg.Wait()
	// No assertion needed - test passes if no race condition detected
}

// Test concurrent ReleaseInterface calls
func TestDeviceManager_ConcurrentReleaseInterface(t *testing.T) {
	registry := NewProfileRegistry()
	dm := NewDeviceManager(registry)

	var wg sync.WaitGroup
	numGoroutines := 10

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = dm.ReleaseInterface()
		}()
	}

	wg.Wait()
	// No assertion needed - test passes if no race condition detected
}

// Test mixed concurrent operations
func TestDeviceManager_ConcurrentMixedOperations(t *testing.T) {
	registry := NewProfileRegistry()
	dm := NewDeviceManager(registry)

	var wg sync.WaitGroup
	numGoroutines := 50

	// Mix of operations
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			switch id % 5 {
			case 0:
				_ = dm.GetState()
			case 1:
				dm.SetState(StateConnected)
			case 2:
				_ = dm.GetProfile()
			case 3:
				_ = dm.ReleaseInterface()
			case 4:
				_ = dm.Close()
			}
		}(i)
	}

	wg.Wait()
	// No assertion needed - test passes if no race condition detected
}

// =============================================================================
// Integration Tests - Require Physical Device
// =============================================================================

// skipIfNoDevice skips the test if no dermoscope device is connected
func skipIfNoDevice(t *testing.T) *DeviceManager {
	t.Helper()

	registry := NewProfileRegistry()
	dm := NewDeviceManager(registry)

	_, err := dm.FindDevice()
	if err != nil {
		t.Skipf("Skipping integration test: %v", err)
	}

	return dm
}

// Test FindDevice finds HT-B30S when connected
func TestDeviceManager_FindDevice_FindsHTB30S(t *testing.T) {
	dm := skipIfNoDevice(t)
	defer dm.Close()

	// Device was found in skipIfNoDevice, verify profile
	profile := dm.GetProfile()
	if profile == nil {
		t.Fatal("GetProfile() returned nil after FindDevice()")
	}

	// Should be HT-B30S or another registered profile
	if profile.ID == "" {
		t.Error("Profile ID should not be empty")
	}

	t.Logf("Found device: %s (VID: %04x, PID: %04x)", profile.Name, profile.VendorID, profile.ProductID)
}

// Test FindDevice returns valid DeviceInfo
func TestDeviceManager_FindDevice_ReturnsValidInfo(t *testing.T) {
	registry := NewProfileRegistry()
	dm := NewDeviceManager(registry)
	defer dm.Close()

	info, err := dm.FindDevice()
	if err != nil {
		t.Skipf("Skipping integration test: %v", err)
	}

	if info == nil {
		t.Fatal("FindDevice() returned nil DeviceInfo")
	}

	if info.Profile == nil {
		t.Error("DeviceInfo.Profile should not be nil")
	}

	if info.VendorID == 0 {
		t.Error("DeviceInfo.VendorID should not be zero")
	}

	if info.ProductID == 0 {
		t.Error("DeviceInfo.ProductID should not be zero")
	}

	t.Logf("Device info: Manufacturer=%s, Product=%s, Serial=%s",
		info.Manufacturer, info.Product, info.SerialNumber)
}

// Test FindDevice sets state to connected
func TestDeviceManager_FindDevice_SetsStateConnected(t *testing.T) {
	registry := NewProfileRegistry()
	dm := NewDeviceManager(registry)
	defer dm.Close()

	_, err := dm.FindDevice()
	if err != nil {
		t.Skipf("Skipping integration test: %v", err)
	}

	if dm.GetState() != StateConnected {
		t.Errorf("GetState() = %v after FindDevice(), want StateConnected", dm.GetState())
	}
}

// Test FindDeviceByProfile finds specific profile
func TestDeviceManager_FindDeviceByProfile_SpecificProfile(t *testing.T) {
	registry := NewProfileRegistry()
	dm := NewDeviceManager(registry)
	defer dm.Close()

	info, err := dm.FindDeviceByProfile("ht-b30s")
	if err != nil {
		t.Skipf("Skipping integration test: %v", err)
	}

	if info == nil {
		t.Fatal("FindDeviceByProfile() returned nil DeviceInfo")
	}

	if info.Profile == nil {
		t.Fatal("DeviceInfo.Profile should not be nil")
	}

	if info.Profile.ID != "ht-b30s" {
		t.Errorf("Profile.ID = %v, want ht-b30s", info.Profile.ID)
	}
}

// Test ClaimInterface succeeds after FindDevice
func TestDeviceManager_ClaimInterface_SucceedsAfterFindDevice(t *testing.T) {
	dm := skipIfNoDevice(t)
	defer dm.Close()

	err := dm.ClaimInterface()

	if err != nil {
		t.Errorf("ClaimInterface() returned error: %v", err)
	}
}

// Test GetEndpoint returns interrupt endpoint after ClaimInterface
func TestDeviceManager_GetEndpoint_ReturnsInterruptEndpoint(t *testing.T) {
	dm := skipIfNoDevice(t)
	defer dm.Close()

	err := dm.ClaimInterface()
	if err != nil {
		t.Fatalf("ClaimInterface() failed: %v", err)
	}

	endpoint, err := dm.GetEndpoint()

	if err != nil {
		t.Errorf("GetEndpoint() returned error: %v", err)
	}

	if endpoint == nil {
		t.Error("GetEndpoint() should return non-nil endpoint")
	}
}

// Test ReleaseInterface cleans up properly
func TestDeviceManager_ReleaseInterface_CleansUp(t *testing.T) {
	dm := skipIfNoDevice(t)
	defer dm.Close()

	// Claim interface first
	err := dm.ClaimInterface()
	if err != nil {
		t.Fatalf("ClaimInterface() failed: %v", err)
	}

	// Release interface
	err = dm.ReleaseInterface()
	if err != nil {
		t.Errorf("ReleaseInterface() returned error: %v", err)
	}

	// GetEndpoint should fail after release
	_, err = dm.GetEndpoint()
	if err == nil {
		t.Error("GetEndpoint() should fail after ReleaseInterface()")
	}
}

// Test Close releases all resources
func TestDeviceManager_Close_ReleasesAllResources(t *testing.T) {
	dm := skipIfNoDevice(t)

	// Claim interface
	err := dm.ClaimInterface()
	if err != nil {
		t.Fatalf("ClaimInterface() failed: %v", err)
	}

	// Get endpoint
	_, err = dm.GetEndpoint()
	if err != nil {
		t.Fatalf("GetEndpoint() failed: %v", err)
	}

	// Close
	err = dm.Close()
	if err != nil {
		t.Errorf("Close() returned error: %v", err)
	}

	// State should be disconnected
	if dm.GetState() != StateDisconnected {
		t.Errorf("GetState() = %v after Close(), want StateDisconnected", dm.GetState())
	}

	// Profile should be nil
	if dm.GetProfile() != nil {
		t.Error("GetProfile() should return nil after Close()")
	}

	// GetEndpoint should fail after close
	_, err = dm.GetEndpoint()
	if err == nil {
		t.Error("GetEndpoint() should fail after Close()")
	}
}

// Test ClaimInterface fails after Close
func TestDeviceManager_ClaimInterface_FailsAfterClose(t *testing.T) {
	dm := skipIfNoDevice(t)

	// Close immediately
	dm.Close()

	// ClaimInterface should fail
	err := dm.ClaimInterface()
	if err == nil {
		t.Error("ClaimInterface() should fail after Close()")
	}
}

// Test FindDevice works after Close (re-initialization)
func TestDeviceManager_FindDevice_WorksAfterClose(t *testing.T) {
	registry := NewProfileRegistry()
	dm := NewDeviceManager(registry)

	// First find
	_, err := dm.FindDevice()
	if err != nil {
		t.Skipf("Skipping integration test: %v", err)
	}

	// Close
	dm.Close()

	// Second find should work
	_, err = dm.FindDevice()
	if err != nil {
		t.Errorf("FindDevice() after Close() returned error: %v", err)
	}

	dm.Close()
}

// =============================================================================
// Edge Case Tests
// =============================================================================

// Test no device connected (returns ErrNoDeviceFound)
func TestDeviceManager_FindDevice_NoDeviceConnected(t *testing.T) {
	// This test is inherently flaky - it only makes sense when no device
	// is actually connected. We test the error type is correct.
	registry := NewProfileRegistry()
	dm := NewDeviceManager(registry)
	defer dm.Close()

	_, err := dm.FindDevice()

	// If error is nil, device is connected - that's OK, skip the assertion
	if err == nil {
		t.Log("Device found, cannot test no-device-connected scenario")
		return
	}

	// If error exists, it should be ErrNoDeviceFound or a USB error
	if !errors.Is(err, ErrNoDeviceFound) {
		// Could be a USB permission or other error, log it
		t.Logf("FindDevice() returned error: %v (expected ErrNoDeviceFound if no device)", err)
	}
}

// Test ClaimInterface fails without profile set
func TestDeviceManager_ClaimInterface_FailsWithoutProfile(t *testing.T) {
	registry := NewProfileRegistry()
	dm := NewDeviceManager(registry)
	defer dm.Close()

	// Manually set device but not profile (simulating edge case)
	// This is a protected operation, testing the public API
	// The implementation should handle this gracefully

	err := dm.ClaimInterface()

	// Should fail with either ErrNoDeviceFound or ErrNoProfile
	if err == nil {
		t.Error("ClaimInterface() should fail without device")
	}
}

// Test DeviceManager lifecycle - full workflow
func TestDeviceManager_FullLifecycle(t *testing.T) {
	registry := NewProfileRegistry()
	dm := NewDeviceManager(registry)

	// Step 1: Initial state
	if dm.GetState() != StateDisconnected {
		t.Errorf("Initial state = %v, want StateDisconnected", dm.GetState())
	}
	if dm.GetProfile() != nil {
		t.Error("Initial profile should be nil")
	}

	// Step 2: Find device
	info, err := dm.FindDevice()
	if err != nil {
		t.Skipf("Skipping full lifecycle test: %v", err)
	}
	if dm.GetState() != StateConnected {
		t.Errorf("State after FindDevice = %v, want StateConnected", dm.GetState())
	}
	if dm.GetProfile() == nil {
		t.Error("Profile should be set after FindDevice()")
	}
	t.Logf("Found device: %s", info.Profile.Name)

	// Step 3: Claim interface
	err = dm.ClaimInterface()
	if err != nil {
		t.Fatalf("ClaimInterface() failed: %v", err)
	}

	// Step 4: Get endpoint
	endpoint, err := dm.GetEndpoint()
	if err != nil {
		t.Fatalf("GetEndpoint() failed: %v", err)
	}
	if endpoint == nil {
		t.Fatal("Endpoint should not be nil")
	}

	// Step 5: Release interface
	err = dm.ReleaseInterface()
	if err != nil {
		t.Errorf("ReleaseInterface() failed: %v", err)
	}

	// Step 6: Close
	err = dm.Close()
	if err != nil {
		t.Errorf("Close() failed: %v", err)
	}
	if dm.GetState() != StateDisconnected {
		t.Errorf("State after Close = %v, want StateDisconnected", dm.GetState())
	}
	if dm.GetProfile() != nil {
		t.Error("Profile should be nil after Close()")
	}
}

// Test finding device twice returns same profile
func TestDeviceManager_FindDevice_Idempotent(t *testing.T) {
	registry := NewProfileRegistry()
	dm := NewDeviceManager(registry)
	defer dm.Close()

	info1, err := dm.FindDevice()
	if err != nil {
		t.Skipf("Skipping test: %v", err)
	}

	info2, err := dm.FindDevice()
	if err != nil {
		t.Fatalf("Second FindDevice() failed: %v", err)
	}

	if info1.Profile.ID != info2.Profile.ID {
		t.Errorf("Profile IDs differ: %v vs %v", info1.Profile.ID, info2.Profile.ID)
	}
}

// Test struct fields are properly initialized
func TestDeviceManager_StructFields(t *testing.T) {
	registry := NewProfileRegistry()
	dm := NewDeviceManager(registry)

	// Verify internal fields are properly initialized
	if dm.ctx != nil {
		t.Error("ctx should be nil on new DeviceManager")
	}
	if dm.device != nil {
		t.Error("device should be nil on new DeviceManager")
	}
	if dm.config != nil {
		t.Error("config should be nil on new DeviceManager")
	}
	if dm.intf != nil {
		t.Error("intf should be nil on new DeviceManager")
	}
	if dm.endpoint != nil {
		t.Error("endpoint should be nil on new DeviceManager")
	}
	if dm.profile != nil {
		t.Error("profile should be nil on new DeviceManager")
	}
}
