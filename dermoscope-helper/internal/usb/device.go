package usb

import (
	"errors"
	"sync"

	"github.com/google/gousb"
)

// Common errors
var (
	ErrNoDeviceFound       = errors.New("no supported device found")
	ErrNoProfile           = errors.New("no profile set for device")
	ErrClaimFailed         = errors.New("failed to claim interface")
	ErrInterfaceNotClaimed = errors.New("interface not claimed")
	ErrEndpointFailed      = errors.New("failed to get endpoint")
	ErrNoEndpoint          = errors.New("no interrupt endpoint found")
	ErrProfileNotFound     = errors.New("profile not found")
)

// DeviceManager handles USB device connection and lifecycle
type DeviceManager struct {
	ctx      *gousb.Context
	device   *gousb.Device
	config   *gousb.Config
	intf     *gousb.Interface
	endpoint *gousb.InEndpoint
	state    DeviceState
	profile  *DeviceProfile // The matched profile for this device
	registry *ProfileRegistry
	mu       sync.RWMutex
}

// NewDeviceManager creates a new device manager with profile support
func NewDeviceManager(registry *ProfileRegistry) *DeviceManager {
	return &DeviceManager{
		registry: registry,
		state:    StateDisconnected,
	}
}

// FindDevice searches for any supported dermoscope device
// Iterates through all connected USB devices and matches against registered profiles
func (dm *DeviceManager) FindDevice() (*DeviceInfo, error) {
	dm.mu.Lock()
	defer dm.mu.Unlock()

	// Create USB context if not exists
	if dm.ctx == nil {
		dm.ctx = gousb.NewContext()
	}

	// Enumerate all USB devices
	devices, err := dm.ctx.OpenDevices(func(desc *gousb.DeviceDesc) bool {
		// Check each device against all profiles
		_, found := dm.registry.FindMatchingProfile(uint16(desc.Vendor), uint16(desc.Product))
		return found
	})

	if err != nil {
		return nil, err
	}

	if len(devices) == 0 {
		return nil, ErrNoDeviceFound
	}

	// Use first matching device
	dm.device = devices[0]

	// Close other devices if any
	for i := 1; i < len(devices); i++ {
		devices[i].Close()
	}

	// Find matching profile
	profile, _ := dm.registry.FindMatchingProfile(
		uint16(dm.device.Desc.Vendor),
		uint16(dm.device.Desc.Product),
	)
	dm.profile = profile
	dm.state = StateConnected

	// Get device info
	manufacturer, _ := dm.device.Manufacturer()
	product, _ := dm.device.Product()
	serial, _ := dm.device.SerialNumber()

	return &DeviceInfo{
		Profile:      profile,
		VendorID:     uint16(dm.device.Desc.Vendor),
		ProductID:    uint16(dm.device.Desc.Product),
		Manufacturer: manufacturer,
		Product:      product,
		SerialNumber: serial,
	}, nil
}

// FindDeviceByProfile searches for a specific device profile
func (dm *DeviceManager) FindDeviceByProfile(profileID string) (*DeviceInfo, error) {
	dm.mu.Lock()
	defer dm.mu.Unlock()

	// Look up the profile by ID
	targetProfile, found := dm.registry.GetByID(profileID)
	if !found {
		return nil, ErrProfileNotFound
	}

	// Create USB context if not exists
	if dm.ctx == nil {
		dm.ctx = gousb.NewContext()
	}

	// Enumerate all USB devices and find one matching the specific profile
	devices, err := dm.ctx.OpenDevices(func(desc *gousb.DeviceDesc) bool {
		return targetProfile.MatchesProfile(uint16(desc.Vendor), uint16(desc.Product))
	})

	if err != nil {
		return nil, err
	}

	if len(devices) == 0 {
		return nil, ErrNoDeviceFound
	}

	// Use first matching device
	dm.device = devices[0]

	// Close other devices if any
	for i := 1; i < len(devices); i++ {
		devices[i].Close()
	}

	// Set the matched profile
	dm.profile = targetProfile
	dm.state = StateConnected

	// Get device info
	manufacturer, _ := dm.device.Manufacturer()
	product, _ := dm.device.Product()
	serial, _ := dm.device.SerialNumber()

	return &DeviceInfo{
		Profile:      targetProfile,
		VendorID:     uint16(dm.device.Desc.Vendor),
		ProductID:    uint16(dm.device.Desc.Product),
		Manufacturer: manufacturer,
		Product:      product,
		SerialNumber: serial,
	}, nil
}

// GetProfile returns the matched profile for the connected device
func (dm *DeviceManager) GetProfile() *DeviceProfile {
	dm.mu.RLock()
	defer dm.mu.RUnlock()
	return dm.profile
}

// ClaimInterface claims the Video Control interface based on profile settings
func (dm *DeviceManager) ClaimInterface() error {
	dm.mu.Lock()
	defer dm.mu.Unlock()

	if dm.device == nil {
		return ErrNoDeviceFound
	}

	if dm.profile == nil {
		return ErrNoProfile
	}

	// Find and claim the Video Control interface
	cfg, err := dm.device.Config(1)
	if err != nil {
		return err
	}
	dm.config = cfg

	// Find interface matching the profile
	for _, ifaceDesc := range cfg.Desc.Interfaces {
		for _, altSetting := range ifaceDesc.AltSettings {
			if altSetting.Class == gousb.Class(dm.profile.InterfaceClass) &&
				altSetting.SubClass == gousb.Class(dm.profile.InterfaceSubclass) {
				intf, err := cfg.Interface(ifaceDesc.Number, 0)
				if err != nil {
					continue
				}
				dm.intf = intf
				return nil
			}
		}
	}

	return ErrClaimFailed
}

// ReleaseInterface releases the claimed interface
func (dm *DeviceManager) ReleaseInterface() error {
	dm.mu.Lock()
	defer dm.mu.Unlock()

	if dm.intf != nil {
		dm.intf.Close()
		dm.intf = nil
	}
	dm.endpoint = nil
	return nil
}

// GetEndpoint returns the interrupt IN endpoint
func (dm *DeviceManager) GetEndpoint() (*gousb.InEndpoint, error) {
	dm.mu.Lock()
	defer dm.mu.Unlock()

	if dm.intf == nil {
		return nil, ErrInterfaceNotClaimed
	}

	// Find interrupt IN endpoint
	for _, ep := range dm.intf.Setting.Endpoints {
		if ep.TransferType == gousb.TransferTypeInterrupt && ep.Direction == gousb.EndpointDirectionIn {
			endpoint, err := dm.intf.InEndpoint(ep.Number)
			if err != nil {
				return nil, err
			}
			dm.endpoint = endpoint
			return endpoint, nil
		}
	}

	return nil, ErrNoEndpoint
}

// GetState returns the current device state
func (dm *DeviceManager) GetState() DeviceState {
	dm.mu.RLock()
	defer dm.mu.RUnlock()
	return dm.state
}

// SetState sets the current device state
func (dm *DeviceManager) SetState(state DeviceState) {
	dm.mu.Lock()
	defer dm.mu.Unlock()
	dm.state = state
}

// Close releases all resources
func (dm *DeviceManager) Close() error {
	dm.mu.Lock()
	defer dm.mu.Unlock()

	if dm.intf != nil {
		dm.intf.Close()
		dm.intf = nil
	}

	if dm.config != nil {
		dm.config.Close()
		dm.config = nil
	}

	if dm.device != nil {
		dm.device.Close()
		dm.device = nil
	}

	if dm.ctx != nil {
		dm.ctx.Close()
		dm.ctx = nil
	}

	dm.endpoint = nil
	dm.profile = nil
	dm.state = StateDisconnected

	return nil
}
