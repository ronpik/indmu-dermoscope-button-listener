#!/usr/bin/env python3
"""
Dermoscope Button Helper v2 - Non-blocking Camera
==================================================

This version uses a time-sliced approach to monitor the button
WITHOUT blocking the camera stream.

Strategy:
- Briefly claim interface, check for button event, release immediately
- Camera driver regains control between checks
- Small latency tradeoff for working video

USAGE:
    sudo python dermoscope_helper_v2.py

Alternative: Use --poll-only mode which doesn't claim the interface
    sudo python dermoscope_helper_v2.py --poll-only
"""

import sys
import time
import subprocess
import signal
import argparse

VENDOR_ID = 0xAB02
PRODUCT_ID = 0xAB01

# Button event: [status_type=2, originator=1, event=0, selector=0]
BUTTON_PRESS_SELECTOR = 0  # selector=0 means pressed, selector=1 means released


class DermoscopeButtonMonitorV2:
    def __init__(self, poll_interval_ms=50):
        self.running = False
        self.poll_interval = poll_interval_ms / 1000.0
        self.last_press_time = 0
        self.debounce_ms = 300
        self.button_presses = 0

    def simulate_f9(self):
        """Simulate F9 keypress using AppleScript."""
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
            subprocess.run(["osascript", "-e", script], capture_output=True, timeout=2)
            print("   → F9 keypress sent")
        except Exception as e:
            print(f"   → Failed to send keypress: {e}")

    def quick_check_button(self):
        """
        Quickly claim interface, check for button event, release.
        Returns True if button was pressed.
        """
        import usb.core
        import usb.util

        dev = usb.core.find(idVendor=VENDOR_ID, idProduct=PRODUCT_ID)
        if not dev:
            return None  # Device disconnected

        # Find Video Control interface and interrupt endpoint
        vc_intf = None
        int_ep = None

        for cfg in dev:
            for intf in cfg:
                if intf.bInterfaceClass == 14 and intf.bInterfaceSubClass == 1:
                    vc_intf = intf
                    for ep in intf:
                        ep_type = usb.util.endpoint_type(ep.bmAttributes)
                        ep_dir = usb.util.endpoint_direction(ep.bEndpointAddress)
                        if ep_type == usb.util.ENDPOINT_TYPE_INTR and ep_dir == usb.util.ENDPOINT_IN:
                            int_ep = ep
                            break

        if not int_ep or not vc_intf:
            return None

        button_pressed = False

        try:
            # Quick claim
            try:
                if dev.is_kernel_driver_active(vc_intf.bInterfaceNumber):
                    dev.detach_kernel_driver(vc_intf.bInterfaceNumber)
            except:
                pass

            usb.util.claim_interface(dev, vc_intf.bInterfaceNumber)

            # Quick read with very short timeout
            try:
                data = dev.read(int_ep.bEndpointAddress, int_ep.wMaxPacketSize, timeout=10)
                data_list = list(data)

                # Check for button press: [2, 1, 0, 0]
                if len(data_list) >= 4:
                    if data_list[0] == 2 and data_list[2] == 0 and data_list[3] == BUTTON_PRESS_SELECTOR:
                        button_pressed = True

            except usb.core.USBTimeoutError:
                pass  # No data, that's fine
            except usb.core.USBError:
                pass

        except Exception as e:
            pass
        finally:
            # Always release quickly
            try:
                usb.util.release_interface(dev, vc_intf.bInterfaceNumber)
            except:
                pass
            # Try to re-attach kernel driver
            try:
                dev.attach_kernel_driver(vc_intf.bInterfaceNumber)
            except:
                pass

        return button_pressed

    def run_quick_poll(self):
        """
        Poll mode: quickly check and release, allowing camera to work.
        """
        print("\n" + "=" * 50)
        print("DERMOSCOPE HELPER v2 (Quick Poll Mode)")
        print("=" * 50)
        print(f"Poll interval: {self.poll_interval * 1000:.0f}ms")
        print("Camera should continue working between polls.")
        print("Press Ctrl+C to stop")
        print("-" * 50)

        self.running = True
        consecutive_none = 0

        try:
            while self.running:
                result = self.quick_check_button()

                if result is None:
                    consecutive_none += 1
                    if consecutive_none > 10:
                        print("\nDevice may be disconnected. Waiting...")
                        time.sleep(2)
                        consecutive_none = 0
                else:
                    consecutive_none = 0
                    if result:
                        self.button_presses += 1
                        timestamp = time.strftime("%H:%M:%S")
                        print(f"\n[{timestamp}] Button press #{self.button_presses}!")
                        self.simulate_f9()

                time.sleep(self.poll_interval)

        except KeyboardInterrupt:
            print("\n\nStopping...")

        self.running = False
        print(f"\nTotal button presses: {self.button_presses}")


class UVCControlPollingMonitor:
    """
    Alternative approach: Poll UVC controls for changes.
    This doesn't require claiming the interface exclusively.
    """

    def __init__(self):
        self.running = False
        self.last_press_time = 0
        self.debounce_ms = 300
        self.button_presses = 0

    def simulate_f9(self):
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
            subprocess.run(["osascript", "-e", script], capture_output=True, timeout=2)
            print("   → F9 keypress sent")
        except Exception as e:
            print(f"   → Failed: {e}")

    def run(self):
        """
        Monitor by polling UVC controls - may work without blocking camera.
        """
        import usb.core
        import usb.util

        print("\n" + "=" * 50)
        print("DERMOSCOPE HELPER (Control Polling Mode)")
        print("=" * 50)
        print("This mode polls UVC controls without claiming interface.")
        print("Camera should work normally.")
        print("Press Ctrl+C to stop")
        print("-" * 50)

        dev = usb.core.find(idVendor=VENDOR_ID, idProduct=PRODUCT_ID)
        if not dev:
            print("ERROR: Device not found!")
            return

        # Find Video Control interface
        vc_intf = None
        for cfg in dev:
            for intf in cfg:
                if intf.bInterfaceClass == 14 and intf.bInterfaceSubClass == 1:
                    vc_intf = intf.bInterfaceNumber
                    break

        if vc_intf is None:
            print("ERROR: No Video Control interface found!")
            return

        print(f"Video Control interface: {vc_intf}")
        print("Monitoring for control value changes...")

        # Monitor multiple controls that might change on button press
        # CT_PRIVACY is selector 0x11 on unit 1
        controls_to_monitor = [
            (1, 0x11, "CT_PRIVACY"),
            (1, 0x01, "CT_SCANNING_MODE"),
        ]

        # Get initial values
        prev_values = {}
        for unit, sel, name in controls_to_monitor:
            try:
                val = list(dev.ctrl_transfer(
                    0xA1, 0x81, sel << 8, (unit << 8) | vc_intf, 4, 100
                ))
                prev_values[(unit, sel)] = val
                print(f"  {name}: {val}")
            except:
                pass

        self.running = True
        check_count = 0

        try:
            while self.running:
                for unit, sel, name in controls_to_monitor:
                    try:
                        val = list(dev.ctrl_transfer(
                            0xA1, 0x81, sel << 8, (unit << 8) | vc_intf, 4, 50
                        ))
                        prev = prev_values.get((unit, sel))
                        if prev and val != prev:
                            self.button_presses += 1
                            timestamp = time.strftime("%H:%M:%S")
                            print(f"\n[{timestamp}] Control change #{self.button_presses}!")
                            print(f"  {name}: {prev} → {val}")
                            self.simulate_f9()
                        prev_values[(unit, sel)] = val
                    except:
                        pass

                check_count += 1
                if check_count % 100 == 0:
                    sys.stdout.write(".")
                    sys.stdout.flush()

                time.sleep(0.02)  # 50 checks per second

        except KeyboardInterrupt:
            print("\n\nStopping...")

        self.running = False
        print(f"\nTotal changes detected: {self.button_presses}")


def check_accessibility():
    """Check if accessibility permissions are granted."""
    script = 'tell application "System Events" to return true'
    try:
        result = subprocess.run(["osascript", "-e", script], capture_output=True, timeout=5)
        if result.returncode != 0:
            print("WARNING: Accessibility permissions may not be granted")
            return False
    except:
        pass
    return True


def main():
    parser = argparse.ArgumentParser(description="Dermoscope Button Helper v2")
    parser.add_argument(
        "--mode",
        choices=["quick", "poll"],
        default="quick",
        help="quick=fast claim/release cycle, poll=monitor control values (default: quick)"
    )
    parser.add_argument(
        "--interval",
        type=int,
        default=50,
        help="Poll interval in milliseconds (default: 50)"
    )
    args = parser.parse_args()

    print("=" * 50)
    print("DERMOSCOPE BUTTON HELPER v2")
    print("=" * 50)
    print(f"Mode: {args.mode}")
    print()

    import os
    if os.geteuid() != 0:
        print("ERROR: This script must be run with sudo")
        print("Usage: sudo python dermoscope_helper_v2.py")
        return 1

    check_accessibility()

    if args.mode == "quick":
        monitor = DermoscopeButtonMonitorV2(poll_interval_ms=args.interval)

        def signal_handler(sig, frame):
            monitor.running = False

        signal.signal(signal.SIGINT, signal_handler)
        signal.signal(signal.SIGTERM, signal_handler)

        monitor.run_quick_poll()
    else:
        monitor = UVCControlPollingMonitor()

        def signal_handler(sig, frame):
            monitor.running = False

        signal.signal(signal.SIGINT, signal_handler)
        signal.signal(signal.SIGTERM, signal_handler)

        monitor.run()

    return 0


if __name__ == "__main__":
    sys.exit(main())
