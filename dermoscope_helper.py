#!/usr/bin/env python3
"""
Dermoscope Button Helper Application
=====================================

Monitors the HT-B30S dermoscope capture button and simulates F9 keypress
when the button is pressed. This bridges the hardware button to the web app.

USAGE:
    sudo python dermoscope_helper.py

REQUIREMENTS:
    - Must run with sudo (to claim USB interface)
    - Accessibility permissions for keyboard simulation
    - pyusb installed: pip install pyusb

The web app should listen for F9 key to trigger image capture:
    document.addEventListener('keydown', (e) => {
        if (e.key === 'F9') captureImage();
    });
"""

import sys
import time
import subprocess
import signal

VENDOR_ID = 0xAB02   # 43778
PRODUCT_ID = 0xAB01  # 43777

# Button event pattern from UVC Status Interrupt
# [2, 1, 0, 0] = button pressed, [2, 1, 0, 1] = button released
BUTTON_PRESS_PATTERN = [2, 1, 0, 0]

class DermoscopeButtonMonitor:
    def __init__(self):
        self.running = False
        self.dev = None
        self.int_ep = None
        self.vc_intf = None
        self.last_press_time = 0
        self.debounce_ms = 200  # Prevent double triggers

    def simulate_f9(self):
        """Simulate F9 keypress using AppleScript."""
        # Debounce check
        now = time.time() * 1000
        if now - self.last_press_time < self.debounce_ms:
            return
        self.last_press_time = now

        script = '''
        tell application "System Events"
            key code 101
        end tell
        '''
        try:
            subprocess.run(
                ["osascript", "-e", script],
                capture_output=True,
                timeout=2
            )
            print("   → F9 keypress sent to system")
        except Exception as e:
            print(f"   → Failed to send keypress: {e}")

    def connect(self):
        """Connect to the dermoscope and claim the interface."""
        import usb.core
        import usb.util

        print("Connecting to dermoscope...")
        self.dev = usb.core.find(idVendor=VENDOR_ID, idProduct=PRODUCT_ID)

        if not self.dev:
            print("ERROR: Dermoscope not found!")
            print("  - Check USB connection")
            print("  - Verify device in System Profiler")
            return False

        print(f"  Device found: VID=0x{VENDOR_ID:04X}, PID=0x{PRODUCT_ID:04X}")

        # Find Video Control interface and interrupt endpoint
        for cfg in self.dev:
            for intf in cfg:
                if intf.bInterfaceClass == 14 and intf.bInterfaceSubClass == 1:
                    self.vc_intf = intf
                    for ep in intf:
                        ep_type = usb.util.endpoint_type(ep.bmAttributes)
                        ep_dir = usb.util.endpoint_direction(ep.bEndpointAddress)
                        if ep_type == usb.util.ENDPOINT_TYPE_INTR and ep_dir == usb.util.ENDPOINT_IN:
                            self.int_ep = ep
                            break

        if not self.int_ep:
            print("ERROR: Interrupt endpoint not found!")
            return False

        print(f"  Interrupt endpoint: 0x{self.int_ep.bEndpointAddress:02X}")

        # Claim the interface
        try:
            if self.dev.is_kernel_driver_active(self.vc_intf.bInterfaceNumber):
                print("  Detaching kernel driver...")
                self.dev.detach_kernel_driver(self.vc_intf.bInterfaceNumber)

            import usb.util
            usb.util.claim_interface(self.dev, self.vc_intf.bInterfaceNumber)
            print("  Interface claimed successfully")
        except Exception as e:
            print(f"ERROR: Could not claim interface: {e}")
            print("  - Make sure to run with sudo")
            return False

        return True

    def disconnect(self):
        """Release the USB interface."""
        if self.dev and self.vc_intf:
            try:
                import usb.util
                usb.util.release_interface(self.dev, self.vc_intf.bInterfaceNumber)
                print("Interface released")
            except:
                pass

    def run(self):
        """Main monitoring loop."""
        import usb.core

        if not self.connect():
            return False

        self.running = True
        print("\n" + "=" * 50)
        print("DERMOSCOPE BUTTON HELPER RUNNING")
        print("=" * 50)
        print("Press the dermoscope button to trigger F9 keypress")
        print("Press Ctrl+C to stop")
        print("-" * 50)

        button_presses = 0

        try:
            while self.running:
                try:
                    data = self.dev.read(
                        self.int_ep.bEndpointAddress,
                        self.int_ep.wMaxPacketSize,
                        timeout=500
                    )
                    data_list = list(data)

                    # Check for button press event
                    if data_list == BUTTON_PRESS_PATTERN:
                        button_presses += 1
                        timestamp = time.strftime("%H:%M:%S")
                        print(f"\n[{timestamp}] Button press #{button_presses} detected!")
                        self.simulate_f9()

                except usb.core.USBTimeoutError:
                    # Normal - just no data yet
                    pass
                except usb.core.USBError as e:
                    if "timeout" not in str(e).lower():
                        print(f"USB Error: {e}")
                        time.sleep(1)

        except KeyboardInterrupt:
            print("\n\nStopping...")
        finally:
            self.running = False
            self.disconnect()

        print(f"\nTotal button presses detected: {button_presses}")
        return True


def check_accessibility():
    """Check if accessibility permissions are granted."""
    script = '''
    tell application "System Events"
        return true
    end tell
    '''
    try:
        result = subprocess.run(
            ["osascript", "-e", script],
            capture_output=True,
            timeout=5
        )
        if result.returncode != 0:
            print("WARNING: Accessibility permissions may not be granted")
            print("  Go to System Preferences > Security & Privacy > Privacy > Accessibility")
            print("  Add Terminal (or your terminal app) to the allowed list")
            return False
    except:
        pass
    return True


def main():
    print("=" * 50)
    print("DERMOSCOPE BUTTON HELPER")
    print("=" * 50)
    print(f"Target device: VID=0x{VENDOR_ID:04X}, PID=0x{PRODUCT_ID:04X}")
    print()

    # Check for root
    import os
    if os.geteuid() != 0:
        print("ERROR: This script must be run with sudo")
        print("Usage: sudo python dermoscope_helper.py")
        return 1

    # Check accessibility
    check_accessibility()

    # Run the monitor
    monitor = DermoscopeButtonMonitor()

    # Handle signals for clean shutdown
    def signal_handler(sig, frame):
        monitor.running = False

    signal.signal(signal.SIGINT, signal_handler)
    signal.signal(signal.SIGTERM, signal_handler)

    if not monitor.run():
        return 1

    return 0


if __name__ == "__main__":
    sys.exit(main())