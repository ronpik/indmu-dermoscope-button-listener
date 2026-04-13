package usb

import (
	"errors"
	"fmt"
	"sync"
)

// ProfileRegistry maintains the list of supported device profiles
type ProfileRegistry struct {
	profiles []DeviceProfile
	mu       sync.RWMutex
}

// NewProfileRegistry creates a registry with built-in profiles
func NewProfileRegistry() *ProfileRegistry {
	return &ProfileRegistry{
		profiles: builtInProfiles,
	}
}

// builtInProfiles contains all known device profiles
// Add new profiles here when supporting new devices
var builtInProfiles = []DeviceProfile{
	{
		ID:                   "ht-b30s",
		Name:                 "HT-B30S Dermoscope",
		Manufacturer:         "Sonix Technology",
		VendorID:             0xAB02,
		ProductID:            0xAB01,
		InterfaceClass:       0x0E, // Video
		InterfaceSubclass:    0x01, // Video Control
		ButtonPressPattern:   []byte{0x02, 0x01, 0x00, 0x00},
		ButtonReleasePattern: []byte{0x02, 0x01, 0x00, 0x01},
		Notes:                "Primary clinic device - verified working on Windows",
	},
	// === ADD NEW DEVICE PROFILES BELOW THIS LINE ===
	//
	// Example:
	// {
	//     ID:                   "device-model",
	//     Name:                 "Device Model Name",
	//     Manufacturer:         "Manufacturer",
	//     VendorID:             0x1234,
	//     ProductID:            0x5678,
	//     InterfaceClass:       0x0E,
	//     InterfaceSubclass:    0x01,
	//     ButtonPressPattern:   []byte{0x02, 0x01, 0x00, 0x00},
	//     ButtonReleasePattern: []byte{0x02, 0x01, 0x00, 0x01},
	//     Notes:                "Added YYYY-MM-DD",
	// },
}

// GetAll returns all registered profiles
func (r *ProfileRegistry) GetAll() []DeviceProfile {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]DeviceProfile, len(r.profiles))
	copy(result, r.profiles)
	return result
}

// GetByID returns a profile by its ID
func (r *ProfileRegistry) GetByID(id string) (*DeviceProfile, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for i := range r.profiles {
		if r.profiles[i].ID == id {
			return &r.profiles[i], true
		}
	}
	return nil, false
}

// FindMatchingProfile finds a profile that matches the given VID/PID
func (r *ProfileRegistry) FindMatchingProfile(vid, pid uint16) (*DeviceProfile, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for i := range r.profiles {
		if r.profiles[i].MatchesProfile(vid, pid) {
			return &r.profiles[i], true
		}
	}
	return nil, false
}

// Register adds a new profile to the registry (for runtime additions)
func (r *ProfileRegistry) Register(profile DeviceProfile) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Check for duplicate ID or VID/PID
	for _, p := range r.profiles {
		if p.ID == profile.ID {
			return fmt.Errorf("profile with ID %s already exists", profile.ID)
		}
		if p.VendorID == profile.VendorID && p.ProductID == profile.ProductID {
			return fmt.Errorf("profile with VID:PID %04x:%04x already exists", profile.VendorID, profile.ProductID)
		}
	}

	r.profiles = append(r.profiles, profile)
	return nil
}

// Count returns the number of registered profiles
func (r *ProfileRegistry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.profiles)
}

// Validate validates all profiles in the registry
func (r *ProfileRegistry) Validate() []error {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var errs []error

	ids := make(map[string]bool)
	vidpids := make(map[string]bool)

	for _, p := range r.profiles {
		// Check for duplicate IDs
		if ids[p.ID] {
			errs = append(errs, fmt.Errorf("duplicate profile ID: %s", p.ID))
		}
		ids[p.ID] = true

		// Check for duplicate VID:PID
		key := fmt.Sprintf("%04x:%04x", p.VendorID, p.ProductID)
		if vidpids[key] {
			errs = append(errs, fmt.Errorf("duplicate VID:PID: %s", key))
		}
		vidpids[key] = true

		// Validate required fields
		if p.ID == "" {
			errs = append(errs, errors.New("profile missing ID"))
		}
		if len(p.ButtonPressPattern) == 0 {
			errs = append(errs, fmt.Errorf("profile %s missing ButtonPressPattern", p.ID))
		}
	}

	return errs
}
