#!/usr/bin/env python3
"""
Dermoscope Button Helper v3 - Interrupt Reading Without Full Claim
===================================================================

This version attempts to read the interrupt endpoint without
detaching the kernel driver, which should allow the camera to work.

USAGE:
    sudo python dermoscope_helper_v3.py
"""

import sys
import time
import subprocess
import signal
import threading

VENDOR_ID = 0xAB02
PRODUCT_ID = 0xAB01

# Global flag for clean shutdown
shutdown_flag = threading.Event()


def simulate_f9():
    """Simulate F9 keypress using AppleScript."""
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


def signal_handler(sig, frame):
    """Handle Ctrl+C gracefully."""
    print("\n\nReceived shutdown signal...")
    shutdown_flag.set()


def try_read_without_detach():
    """
    Attempt to read interrupt endpoint WITHOUT detaching kernel driver.
    """
    import usb.core
    import usb.util

    print("\n" + "=" * 60)
    print("MODE 1: Read Without Detaching Kernel Driver")
    print("=" * 60)

    dev = usb.core.find(idVendor=VENDOR_ID, idProduct=PRODUCT_ID)
    if not dev:
        print("ERROR: Device not found!")
        return False

    print(f"Device found: {dev.manufacturer} - {dev.product}")

    # Find interrupt endpoint
    int_ep = None
    vc_intf_num = None

    for cfg in dev:
        for intf in cfg:
            if intf.bInterfaceClass == 14 and intf.bInterfaceSubClass == 1:
                vc_intf_num = intf.bInterfaceNumber
                for ep in intf:
                    ep_type = usb.util.endpoint_type(ep.bmAttributes)
                    ep_dir = usb.util.endpoint_direction(ep.bEndpointAddress)
                    if ep_type == usb.util.ENDPOINT_TYPE_INTR and ep_dir == usb.util.ENDPOINT_IN:
                        int_ep = ep
                        break

    if not int_ep:
        print("ERROR: Interrupt endpoint not found!")
        return False

    print(f"Interrupt endpoint: 0x{int_ep.bEndpointAddress:02X}")
    print(f"Interface: {vc_intf_num}")

    # Check if kernel driver is active
    try:
        is_active = dev.is_kernel_driver_active(vc_intf_num)
        print(f"Kernel driver active: {is_active}")
    except:
        print("Could not check kernel driver status")

    # Try to claim WITHOUT detaching
    print("\nAttempting to claim interface without detaching driver...")
    try:
        usb.util.claim_interface(dev, vc_intf_num)
        print("✓ Interface claimed (driver may share access)")
    except usb.core.USBError as e:
        print(f"✗ Could not claim: {e}")
        print("  This mode won't work - kernel driver has exclusive access")
        return False

    print("\nMonitoring for button events...")
    print("Press dermoscope button. Press Ctrl+C to stop.")
    print("-" * 60)

    button_presses = 0
    last_press_time = 0

    try:
        while not shutdown_flag.is_set():
            try:
                data = dev.read(int_ep.bEndpointAddress, int_ep.wMaxPacketSize, timeout=200)
                data_list = list(data)

                # Check for button press: [2, 1, 0, 0]
                if len(data_list) >= 4 and data_list[0] == 2 and data_list[2] == 0:
                    now = time.time() * 1000
                    if data_list[3] == 0 and (now - last_press_time) > 300:  # Press with debounce
                        last_press_time = now
                        button_presses += 1
                        timestamp = time.strftime("%H:%M:%S")
                        print(f"\n[{timestamp}] Button press #{button_presses}! Data: {data_list}")
                        simulate_f9()

            except usb.core.USBTimeoutError:
                pass
            except usb.core.USBError as e:
                if "timeout" not in str(e).lower():
                    print(f"USB Error: {e}")
                    time.sleep(0.5)

    finally:
        try:
            usb.util.release_interface(dev, vc_intf_num)
            print("Interface released")
        except:
            pass

    print(f"\nTotal button presses: {button_presses}")
    return True


def try_raw_libusb_read():
    """
    Try using lower-level libusb access.
    """
    import usb.core
    import usb.util
    import usb.backend.libusb1

    print("\n" + "=" * 60)
    print("MODE 2: Raw libusb Read (No Interface Claim)")
    print("=" * 60)

    # Get backend explicitly
    backend = usb.backend.libusb1.get_backend()
    if not backend:
        print("ERROR: libusb backend not found!")
        return False

    dev = usb.core.find(idVendor=VENDOR_ID, idProduct=PRODUCT_ID, backend=backend)
    if not dev:
        print("ERROR: Device not found!")
        return False

    print(f"Device found via libusb backend")

    # Find interrupt endpoint address
    int_ep_addr = None
    max_packet = 16

    for cfg in dev:
        for intf in cfg:
            if intf.bInterfaceClass == 14 and intf.bInterfaceSubClass == 1:
                for ep in intf:
                    ep_type = usb.util.endpoint_type(ep.bmAttributes)
                    ep_dir = usb.util.endpoint_direction(ep.bEndpointAddress)
                    if ep_type == usb.util.ENDPOINT_TYPE_INTR and ep_dir == usb.util.ENDPOINT_IN:
                        int_ep_addr = ep.bEndpointAddress
                        max_packet = ep.wMaxPacketSize
                        break

    if not int_ep_addr:
        print("ERROR: Interrupt endpoint not found!")
        return False

    print(f"Interrupt endpoint: 0x{int_ep_addr:02X}, max packet: {max_packet}")

    # Try direct read without claiming
    print("\nAttempting direct read without claiming interface...")
    print("Press dermoscope button. Press Ctrl+C to stop.")
    print("-" * 60)

    button_presses = 0
    last_press_time = 0
    errors = 0

    while not shutdown_flag.is_set():
        try:
            # Direct read attempt
            data = dev.read(int_ep_addr, max_packet, timeout=200)
            data_list = list(data)

            if len(data_list) >= 4 and data_list[0] == 2 and data_list[2] == 0:
                now = time.time() * 1000
                if data_list[3] == 0 and (now - last_press_time) > 300:
                    last_press_time = now
                    button_presses += 1
                    timestamp = time.strftime("%H:%M:%S")
                    print(f"\n[{timestamp}] Button press #{button_presses}! Data: {data_list}")
                    simulate_f9()

            errors = 0  # Reset error count on success

        except usb.core.USBTimeoutError:
            pass
        except usb.core.USBError as e:
            errors += 1
            if errors == 1:
                print(f"USB Error: {e}")
            if errors > 5:
                print("Too many errors, this mode won't work")
                return False
            time.sleep(0.1)

    print(f"\nTotal button presses: {button_presses}")
    return True


def try_control_transfer_monitor():
    """
    Monitor via control transfers - doesn't need interface claim.
    Checks if any UVC control values change when button is pressed.
    """
    import usb.core
    import usb.util

    print("\n" + "=" * 60)
    print("MODE 3: UVC Control Transfer Monitor")
    print("=" * 60)

    dev = usb.core.find(idVendor=VENDOR_ID, idProduct=PRODUCT_ID)
    if not dev:
        print("ERROR: Device not found!")
        return False

    # Find Video Control interface
    vc_intf = None
    for cfg in dev:
        for intf in cfg:
            if intf.bInterfaceClass == 14 and intf.bInterfaceSubClass == 1:
                vc_intf = intf.bInterfaceNumber
                break

    if vc_intf is None:
        print("ERROR: Video Control interface not found!")
        return False

    print(f"Video Control interface: {vc_intf}")

    # Probe for available controls
    print("\nProbing available UVC controls...")

    GET_CUR = 0x81
    GET_INFO = 0x86

    available_controls = []

    # Check Camera Terminal (unit 1) and Processing Unit (unit 2) controls
    for unit in [1, 2]:
        for selector in range(1, 32):
            try:
                # Check if control exists and is readable
                info = dev.ctrl_transfer(
                    0xA1,  # bmRequestType: Device-to-host, class, interface
                    GET_INFO,
                    selector << 8,
                    (unit << 8) | vc_intf,
                    1,
                    timeout=50
                )
                if info and (info[0] & 0x01):  # GET supported
                    try:
                        val = dev.ctrl_transfer(0xA1, GET_CUR, selector << 8, (unit << 8) | vc_intf, 4, 50)
                        available_controls.append((unit, selector, list(val)))
                        print(f"  Unit {unit}, Sel {selector:02X}: {list(val)}")
                    except:
                        pass
            except:
                pass

    if not available_controls:
        print("No readable controls found!")
        return False

    print(f"\nFound {len(available_controls)} readable controls")
    print("\nMonitoring for value changes...")
    print("Press dermoscope button. Press Ctrl+C to stop.")
    print("-" * 60)

    # Store initial values
    prev_values = {(u, s): v for u, s, v in available_controls}

    button_presses = 0
    last_press_time = 0
    dot_count = 0

    while not shutdown_flag.is_set():
        changed = False

        for unit, selector, _ in available_controls:
            try:
                val = list(dev.ctrl_transfer(0xA1, GET_CUR, selector << 8, (unit << 8) | vc_intf, 4, 30))
                prev = prev_values.get((unit, selector))

                if prev and val != prev:
                    now = time.time() * 1000
                    if (now - last_press_time) > 300:
                        last_press_time = now
                        button_presses += 1
                        timestamp = time.strftime("%H:%M:%S")
                        print(f"\n[{timestamp}] Control changed #{button_presses}!")
                        print(f"  Unit {unit}, Sel {selector:02X}: {prev} → {val}")
                        simulate_f9()
                        changed = True

                prev_values[(unit, selector)] = val
            except:
                pass

        if not changed:
            dot_count += 1
            if dot_count % 50 == 0:
                sys.stdout.write(".")
                sys.stdout.flush()

        time.sleep(0.02)

    print(f"\n\nTotal changes detected: {button_presses}")
    return True


def main():
    print("=" * 60)
    print("DERMOSCOPE BUTTON HELPER v3")
    print("=" * 60)
    print(f"Device: VID=0x{VENDOR_ID:04X}, PID=0x{PRODUCT_ID:04X}")
    print()

    import os
    if os.geteuid() != 0:
        print("ERROR: Must run with sudo")
        print("Usage: sudo python dermoscope_helper_v3.py")
        return 1

    # Set up signal handlers
    signal.signal(signal.SIGINT, signal_handler)
    signal.signal(signal.SIGTERM, signal_handler)

    print("Select mode:")
    print("  1. Read without detaching driver (may work if driver shares)")
    print("  2. Raw libusb read (no interface claim)")
    print("  3. UVC control monitor (polls control values)")
    print("  4. Try all modes in sequence")
    print()

    try:
        choice = input("Choice [1-4, default=4]: ").strip() or "4"
    except EOFError:
        choice = "4"

    if choice == "1":
        try_read_without_detach()
    elif choice == "2":
        try_raw_libusb_read()
    elif choice == "3":
        try_control_transfer_monitor()
    else:
        print("\nTrying all modes...")

        if not shutdown_flag.is_set():
            success = try_read_without_detach()
            if success:
                return 0

        if not shutdown_flag.is_set():
            print("\nMode 1 didn't work. Trying mode 2...")
            success = try_raw_libusb_read()
            if success:
                return 0

        if not shutdown_flag.is_set():
            print("\nMode 2 didn't work. Trying mode 3...")
            try_control_transfer_monitor()

    return 0


if __name__ == "__main__":
    sys.exit(main())
