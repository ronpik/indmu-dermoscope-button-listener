package usb

import (
	"sync"
	"testing"
)

// Test NewProfileRegistry returns registry with built-in profiles
func TestNewProfileRegistry_ReturnsBuiltInProfiles(t *testing.T) {
	registry := NewProfileRegistry()

	if registry == nil {
		t.Fatal("NewProfileRegistry() returned nil")
	}

	count := registry.Count()
	if count == 0 {
		t.Error("NewProfileRegistry() should return registry with built-in profiles")
	}
}

// Test GetAll returns all profiles
func TestProfileRegistry_GetAll(t *testing.T) {
	registry := NewProfileRegistry()

	profiles := registry.GetAll()

	if len(profiles) == 0 {
		t.Error("GetAll() should return at least one profile")
	}

	// Verify HT-B30S is in the list
	found := false
	for _, p := range profiles {
		if p.ID == "ht-b30s" {
			found = true
			break
		}
	}
	if !found {
		t.Error("GetAll() should include ht-b30s profile")
	}
}

// Test GetAll returns a copy (not the internal slice)
func TestProfileRegistry_GetAll_ReturnsCopy(t *testing.T) {
	registry := NewProfileRegistry()

	profiles1 := registry.GetAll()
	profiles2 := registry.GetAll()

	// Modify profiles1 and verify profiles2 is not affected
	if len(profiles1) > 0 {
		profiles1[0].ID = "modified-id"
		if profiles2[0].ID == "modified-id" {
			t.Error("GetAll() should return a copy, not the internal slice")
		}
	}
}

// Test GetByID returns HT-B30S profile
func TestProfileRegistry_GetByID_HTB30S(t *testing.T) {
	registry := NewProfileRegistry()

	profile, found := registry.GetByID("ht-b30s")

	if !found {
		t.Fatal("GetByID(\"ht-b30s\") should return true for found")
	}
	if profile == nil {
		t.Fatal("GetByID(\"ht-b30s\") should return non-nil profile")
	}
	if profile.ID != "ht-b30s" {
		t.Errorf("profile.ID = %v, want ht-b30s", profile.ID)
	}
	if profile.VendorID != 0xAB02 {
		t.Errorf("profile.VendorID = %#x, want 0xAB02", profile.VendorID)
	}
	if profile.ProductID != 0xAB01 {
		t.Errorf("profile.ProductID = %#x, want 0xAB01", profile.ProductID)
	}
}

// Test GetByID returns not found for unknown profile
func TestProfileRegistry_GetByID_Unknown(t *testing.T) {
	registry := NewProfileRegistry()

	profile, found := registry.GetByID("unknown")

	if found {
		t.Error("GetByID(\"unknown\") should return false for found")
	}
	if profile != nil {
		t.Error("GetByID(\"unknown\") should return nil profile")
	}
}

// Test GetByID returns not found for empty string
func TestProfileRegistry_GetByID_EmptyString(t *testing.T) {
	registry := NewProfileRegistry()

	profile, found := registry.GetByID("")

	if found {
		t.Error("GetByID(\"\") should return false for found")
	}
	if profile != nil {
		t.Error("GetByID(\"\") should return nil profile")
	}
}

// Test FindMatchingProfile returns HT-B30S for correct VID/PID
func TestProfileRegistry_FindMatchingProfile_HTB30S(t *testing.T) {
	registry := NewProfileRegistry()

	profile, found := registry.FindMatchingProfile(0xAB02, 0xAB01)

	if !found {
		t.Fatal("FindMatchingProfile(0xAB02, 0xAB01) should return true for found")
	}
	if profile == nil {
		t.Fatal("FindMatchingProfile(0xAB02, 0xAB01) should return non-nil profile")
	}
	if profile.ID != "ht-b30s" {
		t.Errorf("profile.ID = %v, want ht-b30s", profile.ID)
	}
}

// Test FindMatchingProfile returns not found for unknown VID/PID
func TestProfileRegistry_FindMatchingProfile_Unknown(t *testing.T) {
	registry := NewProfileRegistry()

	profile, found := registry.FindMatchingProfile(0x0000, 0x0000)

	if found {
		t.Error("FindMatchingProfile(0x0000, 0x0000) should return false for found")
	}
	if profile != nil {
		t.Error("FindMatchingProfile(0x0000, 0x0000) should return nil profile")
	}
}

// Test FindMatchingProfile with matching VID but wrong PID
func TestProfileRegistry_FindMatchingProfile_PartialMatch(t *testing.T) {
	registry := NewProfileRegistry()

	tests := []struct {
		name string
		vid  uint16
		pid  uint16
	}{
		{"matching VID wrong PID", 0xAB02, 0x9999},
		{"wrong VID matching PID", 0x9999, 0xAB01},
		{"both wrong", 0x1234, 0x5678},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			profile, found := registry.FindMatchingProfile(tt.vid, tt.pid)
			if found || profile != nil {
				t.Errorf("FindMatchingProfile(%#x, %#x) should return not found", tt.vid, tt.pid)
			}
		})
	}
}

// Test Register adds new profile successfully
func TestProfileRegistry_Register_Success(t *testing.T) {
	registry := NewProfileRegistry()
	initialCount := registry.Count()

	newProfile := DeviceProfile{
		ID:                   "new-device",
		Name:                 "New Test Device",
		Manufacturer:         "Test Manufacturer",
		VendorID:             0x1234,
		ProductID:            0x5678,
		ButtonPressPattern:   []byte{0x01, 0x02},
		ButtonReleasePattern: []byte{0x03, 0x04},
	}

	err := registry.Register(newProfile)

	if err != nil {
		t.Fatalf("Register() returned unexpected error: %v", err)
	}
	if registry.Count() != initialCount+1 {
		t.Errorf("Count() = %d, want %d", registry.Count(), initialCount+1)
	}

	// Verify the profile can be retrieved
	retrieved, found := registry.GetByID("new-device")
	if !found {
		t.Fatal("Registered profile should be retrievable by ID")
	}
	if retrieved.VendorID != 0x1234 {
		t.Errorf("retrieved.VendorID = %#x, want 0x1234", retrieved.VendorID)
	}
}

// Test Register rejects duplicate ID
func TestProfileRegistry_Register_DuplicateID(t *testing.T) {
	registry := NewProfileRegistry()

	duplicateProfile := DeviceProfile{
		ID:                   "ht-b30s", // This ID already exists
		Name:                 "Duplicate Device",
		VendorID:             0x9999, // Different VID/PID
		ProductID:            0x9999,
		ButtonPressPattern:   []byte{0x01},
		ButtonReleasePattern: []byte{0x02},
	}

	err := registry.Register(duplicateProfile)

	if err == nil {
		t.Error("Register() should return error for duplicate ID")
	}
}

// Test Register rejects duplicate VID/PID
func TestProfileRegistry_Register_DuplicateVIDPID(t *testing.T) {
	registry := NewProfileRegistry()

	duplicateProfile := DeviceProfile{
		ID:                   "different-id",  // Different ID
		Name:                 "Duplicate VID/PID Device",
		VendorID:             0xAB02,           // Same VID/PID as HT-B30S
		ProductID:            0xAB01,
		ButtonPressPattern:   []byte{0x01},
		ButtonReleasePattern: []byte{0x02},
	}

	err := registry.Register(duplicateProfile)

	if err == nil {
		t.Error("Register() should return error for duplicate VID/PID")
	}
}

// Test Count returns correct number
func TestProfileRegistry_Count(t *testing.T) {
	registry := NewProfileRegistry()

	initialCount := registry.Count()
	if initialCount < 1 {
		t.Error("Count() should return at least 1 for built-in profiles")
	}

	// Add a new profile
	newProfile := DeviceProfile{
		ID:                   "count-test-device",
		VendorID:             0x1111,
		ProductID:            0x2222,
		ButtonPressPattern:   []byte{0x01},
		ButtonReleasePattern: []byte{0x02},
	}
	_ = registry.Register(newProfile)

	if registry.Count() != initialCount+1 {
		t.Errorf("Count() = %d after Register, want %d", registry.Count(), initialCount+1)
	}
}

// Test Validate with valid profiles returns no errors
func TestProfileRegistry_Validate_ValidProfiles(t *testing.T) {
	registry := NewProfileRegistry()

	errors := registry.Validate()

	if len(errors) > 0 {
		t.Errorf("Validate() returned errors for valid built-in profiles: %v", errors)
	}
}

// Test Validate catches duplicate IDs
func TestProfileRegistry_Validate_DuplicateIDs(t *testing.T) {
	// Create a registry with manually set profiles containing duplicates
	registry := &ProfileRegistry{
		profiles: []DeviceProfile{
			{
				ID:                 "duplicate-id",
				VendorID:           0x1111,
				ProductID:          0x1111,
				ButtonPressPattern: []byte{0x01},
			},
			{
				ID:                 "duplicate-id", // Duplicate ID
				VendorID:           0x2222,
				ProductID:          0x2222,
				ButtonPressPattern: []byte{0x02},
			},
		},
	}

	errors := registry.Validate()

	if len(errors) == 0 {
		t.Error("Validate() should return error for duplicate IDs")
	}

	// Check that error message mentions duplicate ID
	foundDuplicateIDError := false
	for _, err := range errors {
		if err != nil && contains(err.Error(), "duplicate profile ID") {
			foundDuplicateIDError = true
			break
		}
	}
	if !foundDuplicateIDError {
		t.Error("Validate() should return error mentioning duplicate profile ID")
	}
}

// Test Validate catches duplicate VID/PIDs
func TestProfileRegistry_Validate_DuplicateVIDPIDs(t *testing.T) {
	// Create a registry with manually set profiles containing duplicate VID/PIDs
	registry := &ProfileRegistry{
		profiles: []DeviceProfile{
			{
				ID:                 "device-1",
				VendorID:           0x1234,
				ProductID:          0x5678,
				ButtonPressPattern: []byte{0x01},
			},
			{
				ID:                 "device-2",
				VendorID:           0x1234, // Same VID/PID
				ProductID:          0x5678,
				ButtonPressPattern: []byte{0x02},
			},
		},
	}

	errors := registry.Validate()

	if len(errors) == 0 {
		t.Error("Validate() should return error for duplicate VID/PIDs")
	}

	// Check that error message mentions duplicate VID:PID
	foundDuplicateVIDPIDError := false
	for _, err := range errors {
		if err != nil && contains(err.Error(), "duplicate VID:PID") {
			foundDuplicateVIDPIDError = true
			break
		}
	}
	if !foundDuplicateVIDPIDError {
		t.Error("Validate() should return error mentioning duplicate VID:PID")
	}
}

// Test Validate catches missing ID
func TestProfileRegistry_Validate_MissingID(t *testing.T) {
	registry := &ProfileRegistry{
		profiles: []DeviceProfile{
			{
				ID:                 "", // Missing ID
				VendorID:           0x1234,
				ProductID:          0x5678,
				ButtonPressPattern: []byte{0x01},
			},
		},
	}

	errors := registry.Validate()

	if len(errors) == 0 {
		t.Error("Validate() should return error for missing ID")
	}

	// Check that error mentions missing ID
	foundMissingIDError := false
	for _, err := range errors {
		if err != nil && contains(err.Error(), "missing ID") {
			foundMissingIDError = true
			break
		}
	}
	if !foundMissingIDError {
		t.Error("Validate() should return error mentioning missing ID")
	}
}

// Test Validate catches missing ButtonPressPattern
func TestProfileRegistry_Validate_MissingButtonPressPattern(t *testing.T) {
	registry := &ProfileRegistry{
		profiles: []DeviceProfile{
			{
				ID:                 "test-device",
				VendorID:           0x1234,
				ProductID:          0x5678,
				ButtonPressPattern: nil, // Missing ButtonPressPattern
			},
		},
	}

	errors := registry.Validate()

	if len(errors) == 0 {
		t.Error("Validate() should return error for missing ButtonPressPattern")
	}

	// Check that error mentions missing ButtonPressPattern
	foundError := false
	for _, err := range errors {
		if err != nil && contains(err.Error(), "ButtonPressPattern") {
			foundError = true
			break
		}
	}
	if !foundError {
		t.Error("Validate() should return error mentioning ButtonPressPattern")
	}
}

// Test empty registry (edge case)
func TestProfileRegistry_EmptyRegistry(t *testing.T) {
	// Create an empty registry directly
	registry := &ProfileRegistry{
		profiles: []DeviceProfile{},
	}

	// Count should be 0
	if registry.Count() != 0 {
		t.Errorf("Count() = %d for empty registry, want 0", registry.Count())
	}

	// GetAll should return empty slice
	profiles := registry.GetAll()
	if len(profiles) != 0 {
		t.Errorf("GetAll() returned %d profiles for empty registry, want 0", len(profiles))
	}

	// GetByID should not find anything
	_, found := registry.GetByID("any-id")
	if found {
		t.Error("GetByID() should return false for empty registry")
	}

	// FindMatchingProfile should not find anything
	_, found = registry.FindMatchingProfile(0x1234, 0x5678)
	if found {
		t.Error("FindMatchingProfile() should return false for empty registry")
	}

	// Validate should return no errors for empty registry
	errors := registry.Validate()
	if len(errors) != 0 {
		t.Errorf("Validate() returned %d errors for empty registry, want 0", len(errors))
	}

	// Register should work on empty registry
	newProfile := DeviceProfile{
		ID:                   "first-profile",
		VendorID:             0x1111,
		ProductID:            0x2222,
		ButtonPressPattern:   []byte{0x01},
		ButtonReleasePattern: []byte{0x02},
	}
	err := registry.Register(newProfile)
	if err != nil {
		t.Errorf("Register() on empty registry returned error: %v", err)
	}
	if registry.Count() != 1 {
		t.Errorf("Count() = %d after Register on empty registry, want 1", registry.Count())
	}
}

// Test concurrent access (basic race condition test)
func TestProfileRegistry_ConcurrentAccess(t *testing.T) {
	registry := NewProfileRegistry()

	// Use a WaitGroup to coordinate goroutines
	var wg sync.WaitGroup
	numGoroutines := 100

	// Concurrent reads
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = registry.GetAll()
			_, _ = registry.GetByID("ht-b30s")
			_, _ = registry.FindMatchingProfile(0xAB02, 0xAB01)
			_ = registry.Count()
			_ = registry.Validate()
		}()
	}

	wg.Wait()
}

// Test concurrent reads and writes (race condition test)
func TestProfileRegistry_ConcurrentReadsAndWrites(t *testing.T) {
	registry := NewProfileRegistry()

	var wg sync.WaitGroup
	numReaders := 50
	numWriters := 10

	// Start readers
	for i := 0; i < numReaders; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				_ = registry.GetAll()
				_, _ = registry.GetByID("ht-b30s")
				_ = registry.Count()
			}
		}()
	}

	// Start writers (some will fail due to duplicates, that's expected)
	for i := 0; i < numWriters; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			profile := DeviceProfile{
				ID:                   "concurrent-device-" + string(rune('a'+id)),
				VendorID:             uint16(0x1000 + id),
				ProductID:            uint16(0x2000 + id),
				ButtonPressPattern:   []byte{byte(id)},
				ButtonReleasePattern: []byte{byte(id + 1)},
			}
			_ = registry.Register(profile) // May fail, that's ok
		}(i)
	}

	wg.Wait()
}

// Test HT-B30S profile has correct button patterns
func TestProfileRegistry_HTB30S_ButtonPatterns(t *testing.T) {
	registry := NewProfileRegistry()

	profile, found := registry.GetByID("ht-b30s")
	if !found {
		t.Fatal("HT-B30S profile not found")
	}

	expectedPress := []byte{0x02, 0x01, 0x00, 0x00}
	expectedRelease := []byte{0x02, 0x01, 0x00, 0x01}

	if !bytesEqual(profile.ButtonPressPattern, expectedPress) {
		t.Errorf("ButtonPressPattern = %v, want %v", profile.ButtonPressPattern, expectedPress)
	}

	if !bytesEqual(profile.ButtonReleasePattern, expectedRelease) {
		t.Errorf("ButtonReleasePattern = %v, want %v", profile.ButtonReleasePattern, expectedRelease)
	}
}

// Test HT-B30S profile has correct interface class/subclass
func TestProfileRegistry_HTB30S_InterfaceConfig(t *testing.T) {
	registry := NewProfileRegistry()

	profile, found := registry.GetByID("ht-b30s")
	if !found {
		t.Fatal("HT-B30S profile not found")
	}

	if profile.InterfaceClass != 0x0E {
		t.Errorf("InterfaceClass = %#x, want 0x0E (Video)", profile.InterfaceClass)
	}

	if profile.InterfaceSubclass != 0x01 {
		t.Errorf("InterfaceSubclass = %#x, want 0x01 (Video Control)", profile.InterfaceSubclass)
	}
}

// Test HT-B30S profile metadata
func TestProfileRegistry_HTB30S_Metadata(t *testing.T) {
	registry := NewProfileRegistry()

	profile, found := registry.GetByID("ht-b30s")
	if !found {
		t.Fatal("HT-B30S profile not found")
	}

	if profile.Name == "" {
		t.Error("HT-B30S profile should have a name")
	}

	if profile.Manufacturer == "" {
		t.Error("HT-B30S profile should have a manufacturer")
	}

	if profile.Notes == "" {
		t.Error("HT-B30S profile should have notes")
	}
}

// Helper function to check if a string contains a substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && findSubstring(s, substr)))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
