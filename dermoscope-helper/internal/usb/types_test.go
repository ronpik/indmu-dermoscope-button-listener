package usb

import (
	"testing"
	"time"
)

// Test DeviceState String method
func TestDeviceState_String(t *testing.T) {
	tests := []struct {
		name     string
		state    DeviceState
		expected string
	}{
		{"disconnected", StateDisconnected, "disconnected"},
		{"connected", StateConnected, "connected"},
		{"monitoring", StateMonitoring, "monitoring"},
		{"error", StateError, "error"},
		{"unknown", DeviceState(99), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.state.String(); got != tt.expected {
				t.Errorf("DeviceState.String() = %v, want %v", got, tt.expected)
			}
		})
	}
}

// Test DeviceState constants have expected iota values
func TestDeviceState_Constants(t *testing.T) {
	if StateDisconnected != 0 {
		t.Errorf("StateDisconnected = %d, want 0", StateDisconnected)
	}
	if StateConnected != 1 {
		t.Errorf("StateConnected = %d, want 1", StateConnected)
	}
	if StateMonitoring != 2 {
		t.Errorf("StateMonitoring = %d, want 2", StateMonitoring)
	}
	if StateError != 3 {
		t.Errorf("StateError = %d, want 3", StateError)
	}
}

// Test DeviceProfile.MatchesProfile with matching VID/PID
func TestDeviceProfile_MatchesProfile_Matching(t *testing.T) {
	profile := &DeviceProfile{
		ID:        "test-device",
		VendorID:  0x1234,
		ProductID: 0x5678,
	}

	if !profile.MatchesProfile(0x1234, 0x5678) {
		t.Error("MatchesProfile() should return true for matching VID/PID")
	}
}

// Test DeviceProfile.MatchesProfile with non-matching VID/PID
func TestDeviceProfile_MatchesProfile_NonMatching(t *testing.T) {
	profile := &DeviceProfile{
		ID:        "test-device",
		VendorID:  0x1234,
		ProductID: 0x5678,
	}

	tests := []struct {
		name string
		vid  uint16
		pid  uint16
	}{
		{"different VID", 0x9999, 0x5678},
		{"different PID", 0x1234, 0x9999},
		{"both different", 0x9999, 0x9999},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if profile.MatchesProfile(tt.vid, tt.pid) {
				t.Errorf("MatchesProfile(%#x, %#x) should return false", tt.vid, tt.pid)
			}
		})
	}
}

// Test DeviceProfile.MatchesProfile with zero VID/PID values
func TestDeviceProfile_MatchesProfile_ZeroValues(t *testing.T) {
	// Profile with zero values should match zero inputs
	zeroProfile := &DeviceProfile{
		ID:        "zero-device",
		VendorID:  0x0000,
		ProductID: 0x0000,
	}

	if !zeroProfile.MatchesProfile(0x0000, 0x0000) {
		t.Error("MatchesProfile() should return true for zero VID/PID matching zero profile")
	}

	if zeroProfile.MatchesProfile(0x1234, 0x5678) {
		t.Error("MatchesProfile() should return false for non-zero VID/PID against zero profile")
	}

	// Non-zero profile should not match zero inputs
	nonZeroProfile := &DeviceProfile{
		ID:        "non-zero-device",
		VendorID:  0x1234,
		ProductID: 0x5678,
	}

	if nonZeroProfile.MatchesProfile(0x0000, 0x0000) {
		t.Error("MatchesProfile() should return false for zero VID/PID against non-zero profile")
	}
}

// Test DeviceProfile.MatchesButtonPress with valid press pattern
func TestDeviceProfile_MatchesButtonPress_ValidPattern(t *testing.T) {
	profile := &DeviceProfile{
		ID:                 "test-device",
		ButtonPressPattern: []byte{0x01, 0x02, 0x03},
	}

	if !profile.MatchesButtonPress([]byte{0x01, 0x02, 0x03}) {
		t.Error("MatchesButtonPress() should return true for matching pattern")
	}
}

// Test DeviceProfile.MatchesButtonPress with release pattern (should fail)
func TestDeviceProfile_MatchesButtonPress_ReleasePattern(t *testing.T) {
	profile := &DeviceProfile{
		ID:                   "test-device",
		ButtonPressPattern:   []byte{0x01, 0x02, 0x03},
		ButtonReleasePattern: []byte{0x04, 0x05, 0x06},
	}

	// Using release pattern should fail for press check
	if profile.MatchesButtonPress([]byte{0x04, 0x05, 0x06}) {
		t.Error("MatchesButtonPress() should return false for release pattern")
	}
}

// Test DeviceProfile.MatchesButtonRelease with valid release pattern
func TestDeviceProfile_MatchesButtonRelease_ValidPattern(t *testing.T) {
	profile := &DeviceProfile{
		ID:                   "test-device",
		ButtonReleasePattern: []byte{0x04, 0x05, 0x06},
	}

	if !profile.MatchesButtonRelease([]byte{0x04, 0x05, 0x06}) {
		t.Error("MatchesButtonRelease() should return true for matching pattern")
	}
}

// Test DeviceProfile.MatchesButtonRelease with press pattern (should fail)
func TestDeviceProfile_MatchesButtonRelease_PressPattern(t *testing.T) {
	profile := &DeviceProfile{
		ID:                   "test-device",
		ButtonPressPattern:   []byte{0x01, 0x02, 0x03},
		ButtonReleasePattern: []byte{0x04, 0x05, 0x06},
	}

	// Using press pattern should fail for release check
	if profile.MatchesButtonRelease([]byte{0x01, 0x02, 0x03}) {
		t.Error("MatchesButtonRelease() should return false for press pattern")
	}
}

// Test bytesEqual with equal arrays
func TestBytesEqual_Equal(t *testing.T) {
	tests := []struct {
		name string
		a    []byte
		b    []byte
	}{
		{"single byte", []byte{0x01}, []byte{0x01}},
		{"multiple bytes", []byte{0x01, 0x02, 0x03}, []byte{0x01, 0x02, 0x03}},
		{"longer array", []byte{0x00, 0x11, 0x22, 0x33, 0x44, 0x55}, []byte{0x00, 0x11, 0x22, 0x33, 0x44, 0x55}},
		{"all zeros", []byte{0x00, 0x00, 0x00}, []byte{0x00, 0x00, 0x00}},
		{"all max values", []byte{0xFF, 0xFF, 0xFF}, []byte{0xFF, 0xFF, 0xFF}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !bytesEqual(tt.a, tt.b) {
				t.Errorf("bytesEqual(%v, %v) = false, want true", tt.a, tt.b)
			}
		})
	}
}

// Test bytesEqual with different lengths
func TestBytesEqual_DifferentLengths(t *testing.T) {
	tests := []struct {
		name string
		a    []byte
		b    []byte
	}{
		{"longer a", []byte{0x01, 0x02, 0x03}, []byte{0x01, 0x02}},
		{"longer b", []byte{0x01, 0x02}, []byte{0x01, 0x02, 0x03}},
		{"empty vs non-empty", []byte{}, []byte{0x01}},
		{"non-empty vs empty", []byte{0x01}, []byte{}},
		{"prefix match", []byte{0x01, 0x02, 0x03}, []byte{0x01}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if bytesEqual(tt.a, tt.b) {
				t.Errorf("bytesEqual(%v, %v) = true, want false for different lengths", tt.a, tt.b)
			}
		})
	}
}

// Test bytesEqual with same length but different content
func TestBytesEqual_SameLengthDifferentContent(t *testing.T) {
	tests := []struct {
		name string
		a    []byte
		b    []byte
	}{
		{"first byte different", []byte{0x01, 0x02, 0x03}, []byte{0xFF, 0x02, 0x03}},
		{"middle byte different", []byte{0x01, 0x02, 0x03}, []byte{0x01, 0xFF, 0x03}},
		{"last byte different", []byte{0x01, 0x02, 0x03}, []byte{0x01, 0x02, 0xFF}},
		{"all bytes different", []byte{0x01, 0x02, 0x03}, []byte{0x04, 0x05, 0x06}},
		{"single byte different", []byte{0x00}, []byte{0x01}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if bytesEqual(tt.a, tt.b) {
				t.Errorf("bytesEqual(%v, %v) = true, want false for different content", tt.a, tt.b)
			}
		})
	}
}

// Test bytesEqual with empty arrays (edge case)
func TestBytesEqual_EmptyArrays(t *testing.T) {
	// Two empty arrays should be equal
	if !bytesEqual([]byte{}, []byte{}) {
		t.Error("bytesEqual([], []) should return true")
	}
}

// Test bytesEqual with nil arrays (edge case)
func TestBytesEqual_NilArrays(t *testing.T) {
	// Two nil slices should be equal (both have length 0)
	if !bytesEqual(nil, nil) {
		t.Error("bytesEqual(nil, nil) should return true")
	}

	// nil and empty slice should be equal (both have length 0)
	if !bytesEqual(nil, []byte{}) {
		t.Error("bytesEqual(nil, []) should return true")
	}

	if !bytesEqual([]byte{}, nil) {
		t.Error("bytesEqual([], nil) should return true")
	}
}

// Test MatchesButtonPress with empty pattern arrays
func TestDeviceProfile_MatchesButtonPress_EmptyPattern(t *testing.T) {
	profile := &DeviceProfile{
		ID:                 "test-device",
		ButtonPressPattern: []byte{},
	}

	// Empty data should match empty pattern
	if !profile.MatchesButtonPress([]byte{}) {
		t.Error("MatchesButtonPress([]) should return true for empty pattern")
	}

	// Non-empty data should not match empty pattern
	if profile.MatchesButtonPress([]byte{0x01}) {
		t.Error("MatchesButtonPress([0x01]) should return false for empty pattern")
	}
}

// Test MatchesButtonPress with nil pattern arrays
func TestDeviceProfile_MatchesButtonPress_NilPattern(t *testing.T) {
	profile := &DeviceProfile{
		ID:                 "test-device",
		ButtonPressPattern: nil,
	}

	// nil data should match nil pattern
	if !profile.MatchesButtonPress(nil) {
		t.Error("MatchesButtonPress(nil) should return true for nil pattern")
	}

	// Empty data should match nil pattern (both have length 0)
	if !profile.MatchesButtonPress([]byte{}) {
		t.Error("MatchesButtonPress([]) should return true for nil pattern")
	}

	// Non-empty data should not match nil pattern
	if profile.MatchesButtonPress([]byte{0x01}) {
		t.Error("MatchesButtonPress([0x01]) should return false for nil pattern")
	}
}

// Test MatchesButtonRelease with empty pattern arrays
func TestDeviceProfile_MatchesButtonRelease_EmptyPattern(t *testing.T) {
	profile := &DeviceProfile{
		ID:                   "test-device",
		ButtonReleasePattern: []byte{},
	}

	// Empty data should match empty pattern
	if !profile.MatchesButtonRelease([]byte{}) {
		t.Error("MatchesButtonRelease([]) should return true for empty pattern")
	}

	// Non-empty data should not match empty pattern
	if profile.MatchesButtonRelease([]byte{0x01}) {
		t.Error("MatchesButtonRelease([0x01]) should return false for empty pattern")
	}
}

// Test MatchesButtonRelease with nil pattern arrays
func TestDeviceProfile_MatchesButtonRelease_NilPattern(t *testing.T) {
	profile := &DeviceProfile{
		ID:                   "test-device",
		ButtonReleasePattern: nil,
	}

	// nil data should match nil pattern
	if !profile.MatchesButtonRelease(nil) {
		t.Error("MatchesButtonRelease(nil) should return true for nil pattern")
	}

	// Empty data should match nil pattern
	if !profile.MatchesButtonRelease([]byte{}) {
		t.Error("MatchesButtonRelease([]) should return true for nil pattern")
	}

	// Non-empty data should not match nil pattern
	if profile.MatchesButtonRelease([]byte{0x01}) {
		t.Error("MatchesButtonRelease([0x01]) should return false for nil pattern")
	}
}

// Test partial pattern matches (should fail)
func TestDeviceProfile_PartialPatternMatch(t *testing.T) {
	profile := &DeviceProfile{
		ID:                   "test-device",
		ButtonPressPattern:   []byte{0x01, 0x02, 0x03, 0x04},
		ButtonReleasePattern: []byte{0x05, 0x06, 0x07, 0x08},
	}

	// Partial matches should fail - data is a prefix of pattern
	if profile.MatchesButtonPress([]byte{0x01, 0x02}) {
		t.Error("Partial press pattern match (prefix) should fail")
	}

	if profile.MatchesButtonRelease([]byte{0x05, 0x06}) {
		t.Error("Partial release pattern match (prefix) should fail")
	}

	// Partial matches should fail - pattern is a prefix of data
	if profile.MatchesButtonPress([]byte{0x01, 0x02, 0x03, 0x04, 0x05}) {
		t.Error("Partial press pattern match (superset) should fail")
	}

	if profile.MatchesButtonRelease([]byte{0x05, 0x06, 0x07, 0x08, 0x09}) {
		t.Error("Partial release pattern match (superset) should fail")
	}
}

// Test ButtonEvent struct
func TestButtonEvent_Struct(t *testing.T) {
	now := time.Now()
	rawData := []byte{0x01, 0x02, 0x03}

	event := ButtonEvent{
		Pressed:   true,
		Timestamp: now,
		RawData:   rawData,
		DeviceID:  "test-device",
	}

	if !event.Pressed {
		t.Error("ButtonEvent.Pressed should be true")
	}
	if event.Timestamp != now {
		t.Errorf("ButtonEvent.Timestamp = %v, want %v", event.Timestamp, now)
	}
	if !bytesEqual(event.RawData, rawData) {
		t.Errorf("ButtonEvent.RawData = %v, want %v", event.RawData, rawData)
	}
	if event.DeviceID != "test-device" {
		t.Errorf("ButtonEvent.DeviceID = %v, want test-device", event.DeviceID)
	}
}

// Test DeviceInfo struct
func TestDeviceInfo_Struct(t *testing.T) {
	profile := &DeviceProfile{
		ID:        "test-profile",
		VendorID:  0x1234,
		ProductID: 0x5678,
	}

	info := DeviceInfo{
		Profile:      profile,
		VendorID:     0x1234,
		ProductID:    0x5678,
		Manufacturer: "Test Manufacturer",
		Product:      "Test Product",
		SerialNumber: "SN12345",
	}

	if info.Profile != profile {
		t.Error("DeviceInfo.Profile should match assigned profile")
	}
	if info.VendorID != 0x1234 {
		t.Errorf("DeviceInfo.VendorID = %#x, want 0x1234", info.VendorID)
	}
	if info.ProductID != 0x5678 {
		t.Errorf("DeviceInfo.ProductID = %#x, want 0x5678", info.ProductID)
	}
	if info.Manufacturer != "Test Manufacturer" {
		t.Errorf("DeviceInfo.Manufacturer = %v, want Test Manufacturer", info.Manufacturer)
	}
	if info.Product != "Test Product" {
		t.Errorf("DeviceInfo.Product = %v, want Test Product", info.Product)
	}
	if info.SerialNumber != "SN12345" {
		t.Errorf("DeviceInfo.SerialNumber = %v, want SN12345", info.SerialNumber)
	}
}

// Test DeviceInfo with nil Profile (edge case)
func TestDeviceInfo_NilProfile(t *testing.T) {
	info := DeviceInfo{
		Profile:  nil,
		VendorID: 0x1234,
	}

	if info.Profile != nil {
		t.Error("DeviceInfo.Profile should be nil")
	}
}

// Test USB constants
func TestUSBConstants(t *testing.T) {
	if VideoControlClass != 0x0E {
		t.Errorf("VideoControlClass = %#x, want 0x0E", VideoControlClass)
	}
	if VideoControlSubclass != 0x01 {
		t.Errorf("VideoControlSubclass = %#x, want 0x01", VideoControlSubclass)
	}
	if InterruptEndpointType != 0x03 {
		t.Errorf("InterruptEndpointType = %#x, want 0x03", InterruptEndpointType)
	}
	if EndpointDirectionIn != 0x80 {
		t.Errorf("EndpointDirectionIn = %#x, want 0x80", EndpointDirectionIn)
	}
}

// Test timing constants
func TestTimingConstants(t *testing.T) {
	if ReadTimeoutMs != 500 {
		t.Errorf("ReadTimeoutMs = %d, want 500", ReadTimeoutMs)
	}
	if DebounceMs != 250 {
		t.Errorf("DebounceMs = %d, want 250", DebounceMs)
	}
	if ReconnectDelayMs != 2000 {
		t.Errorf("ReconnectDelayMs = %d, want 2000", ReconnectDelayMs)
	}
	if DevicePollMs != 1000 {
		t.Errorf("DevicePollMs = %d, want 1000", DevicePollMs)
	}
}

// Test DeviceProfile struct completeness
func TestDeviceProfile_AllFields(t *testing.T) {
	profile := DeviceProfile{
		ID:                   "ht-b30s",
		Name:                 "Hairtech B30S",
		Manufacturer:         "Hairtech",
		VendorID:             0x0AC8,
		ProductID:            0x3610,
		InterfaceClass:       0x0E,
		InterfaceSubclass:    0x01,
		ButtonPressPattern:   []byte{0x01},
		ButtonReleasePattern: []byte{0x00},
		Notes:                "Primary supported device",
	}

	if profile.ID != "ht-b30s" {
		t.Errorf("profile.ID = %v, want ht-b30s", profile.ID)
	}
	if profile.Name != "Hairtech B30S" {
		t.Errorf("profile.Name = %v, want Hairtech B30S", profile.Name)
	}
	if profile.Manufacturer != "Hairtech" {
		t.Errorf("profile.Manufacturer = %v, want Hairtech", profile.Manufacturer)
	}
	if profile.VendorID != 0x0AC8 {
		t.Errorf("profile.VendorID = %#x, want 0x0AC8", profile.VendorID)
	}
	if profile.ProductID != 0x3610 {
		t.Errorf("profile.ProductID = %#x, want 0x3610", profile.ProductID)
	}
	if profile.InterfaceClass != 0x0E {
		t.Errorf("profile.InterfaceClass = %#x, want 0x0E", profile.InterfaceClass)
	}
	if profile.InterfaceSubclass != 0x01 {
		t.Errorf("profile.InterfaceSubclass = %#x, want 0x01", profile.InterfaceSubclass)
	}
	if profile.Notes != "Primary supported device" {
		t.Errorf("profile.Notes = %v, want Primary supported device", profile.Notes)
	}
}
