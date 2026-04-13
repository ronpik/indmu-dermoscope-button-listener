package usb

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

// Monitor continuously reads from the interrupt endpoint
type Monitor struct {
	dm          *DeviceManager
	eventChan   chan ButtonEvent
	errorChan   chan error
	stopChan    chan struct{}
	debounceMs  int
	lastPressMs int64
	mu          sync.Mutex
	running     bool
}

// NewMonitor creates a new interrupt endpoint monitor
func NewMonitor(dm *DeviceManager, debounceMs int) *Monitor {
	return &Monitor{
		dm:         dm,
		eventChan:  make(chan ButtonEvent, 10),
		errorChan:  make(chan error, 10),
		stopChan:   make(chan struct{}),
		debounceMs: debounceMs,
	}
}

// Start begins monitoring in a goroutine
func (m *Monitor) Start() error {
	m.mu.Lock()
	if m.running {
		m.mu.Unlock()
		return errors.New("monitor already running")
	}
	m.running = true
	m.mu.Unlock()

	endpoint, err := m.dm.GetEndpoint()
	if err != nil {
		return err
	}

	go func() {
		buf := make([]byte, 64)
		ctx := context.Background()

		for {
			select {
			case <-m.stopChan:
				return
			default:
				// Read from endpoint with timeout
				n, err := endpoint.ReadContext(ctx, buf)
				if err != nil {
					// Check if it's a timeout (normal, continue)
					if errors.Is(err, context.DeadlineExceeded) {
						continue
					}
					select {
					case m.errorChan <- err:
					default:
					}
					continue
				}

				if n > 0 {
					data := make([]byte, n)
					copy(data, buf[:n])
					event, err := m.parseEvent(data)
					if err != nil {
						// Unknown event, ignore
						continue
					}

					// Only emit button press events (not release) for F9 trigger
					if !event.Pressed {
						continue
					}

					// Apply debouncing for press events
					if !m.shouldTrigger() {
						continue
					}

					select {
					case m.eventChan <- *event:
					default:
					}
				}
			}
		}
	}()

	return nil
}

// Stop stops the monitoring goroutine
func (m *Monitor) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.running {
		close(m.stopChan)
		m.running = false
		// Reinitialize stopChan for potential restart
		m.stopChan = make(chan struct{})
	}
}

// Events returns the channel for button events
func (m *Monitor) Events() <-chan ButtonEvent {
	return m.eventChan
}

// Errors returns the channel for errors
func (m *Monitor) Errors() <-chan error {
	return m.errorChan
}

// shouldTrigger checks if enough time has passed since last press (debouncing)
func (m *Monitor) shouldTrigger() bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now().UnixMilli()
	if now-m.lastPressMs < int64(m.debounceMs) {
		return false
	}
	m.lastPressMs = now
	return true
}

// parseEvent parses raw USB data into a ButtonEvent using the device's profile
func (m *Monitor) parseEvent(data []byte) (*ButtonEvent, error) {
	profile := m.dm.GetProfile()
	if profile == nil {
		return nil, ErrNoProfile
	}

	event := &ButtonEvent{
		Timestamp: time.Now(),
		RawData:   data,
		DeviceID:  profile.ID,
	}

	// Match against profile patterns
	if profile.MatchesButtonPress(data) {
		event.Pressed = true
		return event, nil
	}

	if profile.MatchesButtonRelease(data) {
		event.Pressed = false
		return event, nil
	}

	// Unknown event pattern
	return nil, fmt.Errorf("unknown event pattern: %v", data)
}
