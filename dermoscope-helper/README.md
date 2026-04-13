# Dermoscope Button Helper

A native Go application that monitors supported dermoscope devices for button press events and simulates F9 keypresses to trigger image capture in the TrichoAI web application.

## Overview

Medical clinicians use dermoscopes for scalp imaging during hair loss consultations. The clinical workflow requires hands-free image capture while the clinician positions the dermoscope with both hands on the patient.

The dermoscope has a physical capture button, but it does not emit standard keyboard events or expose itself as a HID device. Instead, the button sends UVC (USB Video Class) status interrupt events via the Video Control interface. Standard browser APIs cannot access these events, necessitating this native helper application.

This helper bridges the gap between the hardware button and the TrichoAI web application by:
1. Monitoring the dermoscope's USB interrupt endpoint for button events
2. Translating button presses into F9 keyboard events
3. The browser-based TrichoAI application listens for F9 to trigger image capture

## Installation

### Windows (Primary Platform)

Windows is the primary supported platform. The application works without blocking camera access.

**Pre-built Binary:**

1. Download `dermoscope-helper-X.X.X-windows-amd64.exe` from the [Releases](https://github.com/trichoai/dermoscope-helper/releases) page
2. Save to a convenient location (e.g., `C:\Program Files\DermoscopeHelper\`)
3. Optionally, create a shortcut to run at startup

**Running the Application:**

```cmd
dermoscope-helper.exe
```

The application will start minimized to the system tray. Look for the dermoscope icon in the system tray notification area.

**Administrator Privileges:**

In most cases, the application runs without administrator privileges. If you encounter USB access errors, try running as Administrator:
- Right-click the executable
- Select "Run as administrator"

**WinUSB Driver (If Needed):**

Most dermoscopes work with the default Windows drivers. If the device is not detected:

1. Download [Zadig](https://zadig.akeo.ie/)
2. Connect the dermoscope
3. Open Zadig and select the dermoscope from the device list
4. Select "WinUSB" as the target driver
5. Click "Install Driver"

Note: Installing WinUSB may affect the device's use with other software.

### macOS (Secondary Platform)

macOS is supported with an important limitation: **button monitoring blocks camera access**. This is due to macOS kernel driver exclusivity - when the helper claims the USB interface for button monitoring, the camera becomes unavailable to browsers.

**Workaround:** Use the helper only when you need button capture, then quit the helper to use the camera normally.

**Pre-built Binary:**

1. Download the appropriate binary:
   - Apple Silicon (M1/M2/M3): `dermoscope-helper-X.X.X-darwin-arm64`
   - Intel Mac: `dermoscope-helper-X.X.X-darwin-amd64`
2. Move to `/Applications` or another location
3. Make executable: `chmod +x dermoscope-helper-*`

**System Requirements:**

- libusb must be installed:
  ```bash
  brew install libusb
  ```

**Accessibility Permissions:**

The application requires Accessibility permissions to simulate keyboard events:

1. Open **System Preferences** > **Security & Privacy** > **Privacy** tab
2. Select **Accessibility** in the left sidebar
3. Click the lock icon and authenticate
4. Add the `dermoscope-helper` application to the list
5. Ensure the checkbox next to it is enabled

If you move the application, you may need to re-grant permissions.

**Running the Application:**

```bash
# May require sudo for USB access
sudo ./dermoscope-helper-X.X.X-darwin-arm64

# Or with debug logging
sudo ./dermoscope-helper-X.X.X-darwin-arm64 --debug
```

### Linux

Linux is supported for X11-based desktop environments.

**Pre-built Binary:**

1. Download `dermoscope-helper-X.X.X-linux-amd64` from the Releases page
2. Make executable: `chmod +x dermoscope-helper-X.X.X-linux-amd64`

**System Requirements:**

- libusb-1.0:
  ```bash
  # Debian/Ubuntu
  sudo apt-get install libusb-1.0-0-dev

  # Fedora/RHEL
  sudo dnf install libusb1-devel

  # Arch Linux
  sudo pacman -S libusb
  ```

- xdotool (required for keyboard simulation):
  ```bash
  # Debian/Ubuntu
  sudo apt-get install xdotool

  # Fedora/RHEL
  sudo dnf install xdotool

  # Arch Linux
  sudo pacman -S xdotool
  ```

**USB Permissions (udev Rules):**

To run without root privileges, add a udev rule:

```bash
# Create udev rule file
sudo tee /etc/udev/rules.d/99-dermoscope.rules << 'EOF'
# HT-B30S Dermoscope
SUBSYSTEM=="usb", ATTR{idVendor}=="ab02", ATTR{idProduct}=="ab01", MODE="0666", GROUP="plugdev"
EOF

# Reload udev rules
sudo udevadm control --reload-rules
sudo udevadm trigger

# Add your user to plugdev group (logout required)
sudo usermod -a -G plugdev $USER
```

**Running the Application:**

```bash
# After setting up udev rules (no sudo needed)
./dermoscope-helper-X.X.X-linux-amd64

# Without udev rules (requires root)
sudo ./dermoscope-helper-X.X.X-linux-amd64
```

**Note:** Linux keyboard simulation requires X11. Wayland users may need to run in an X11 session or use XWayland.

## Usage

### Basic Usage

```bash
# Start the application (runs in system tray)
./dermoscope-helper

# Start with debug logging
./dermoscope-helper --debug

# List supported device profiles
./dermoscope-helper --list-profiles

# Use a custom configuration file
./dermoscope-helper --config /path/to/config.json

# Print version
./dermoscope-helper --version
```

### Command Line Options

| Flag | Description |
|------|-------------|
| `--debug` | Enable debug logging to console |
| `--config PATH` | Path to JSON configuration file |
| `--list-profiles` | List all supported device profiles and exit |
| `--version` | Print version information and exit |

### Configuration File

Create a `config.json` file to customize behavior:

```json
{
  "debounce_ms": 250,
  "reconnect_delay_ms": 2000,
  "read_timeout_ms": 500,
  "trigger_key": 120,
  "log_file": "dermoscope-helper.log",
  "log_level": "info",
  "start_minimized": false,
  "auto_start": false
}
```

| Option | Default | Description |
|--------|---------|-------------|
| `debounce_ms` | 250 | Minimum time between button events (prevents double-triggers) |
| `reconnect_delay_ms` | 2000 | Delay before attempting reconnection after device loss |
| `read_timeout_ms` | 500 | USB read timeout |
| `trigger_key` | 120 | Virtual key code to send (120 = F9) |
| `log_file` | "" | Path to log file (empty = no file logging) |
| `log_level` | "info" | Log level: "debug", "info", "warn", "error" |
| `start_minimized` | false | Start minimized to system tray |
| `auto_start` | false | Auto-start monitoring on launch |

### System Tray

The application runs as a system tray utility with visual status indicators.

**Tray Icon States:**

| Icon State | Meaning |
|------------|---------|
| Gray/Disconnected | No dermoscope device connected |
| Blue/Connected | Device connected but not monitoring |
| Green/Monitoring | Actively monitoring for button presses |
| Red/Error | An error occurred (check logs) |

**Tray Menu Options:**

| Menu Item | Action |
|-----------|--------|
| Status line (grayed) | Shows current status (informational) |
| Start/Stop Monitoring | Toggle button monitoring on/off |
| Exit | Close the application |

**Tooltips:** Hover over the tray icon to see the current status message.

## Supported Devices

| Device | Vendor ID | Product ID | Status |
|--------|-----------|------------|--------|
| HT-B30S Dermoscope | 0xAB02 | 0xAB01 | Verified |

### Identifying Your Device

To check if your dermoscope is supported:

**Windows:**
1. Open Device Manager
2. Find your dermoscope under "Imaging devices" or "Cameras"
3. Right-click > Properties > Details > Hardware Ids
4. Look for VID (Vendor ID) and PID (Product ID)

**macOS:**
```bash
system_profiler SPUSBDataType | grep -A 10 -i dermoscope
```

**Linux:**
```bash
lsusb | grep -i dermoscope
# Or list all devices
lsusb
```

The device VID:PID should appear in format like `ID ab02:ab01`.

## Adding New Devices

To add support for a new dermoscope model, you need to:

1. **Identify the USB device:**
   - Find the Vendor ID (VID) and Product ID (PID)
   - Confirm it's a UVC device (USB Video Class)

2. **Capture button patterns:**
   - Use a USB protocol analyzer (e.g., Wireshark with USBPcap)
   - Record the byte patterns for button press and release events
   - Patterns are typically 4 bytes on the Video Control interrupt endpoint

3. **Add profile to `internal/usb/profiles.go`:**
   ```go
   {
       ID:                   "device-model",
       Name:                 "Device Model Name",
       Manufacturer:         "Manufacturer Name",
       VendorID:             0x1234,  // Your device's VID
       ProductID:            0x5678,  // Your device's PID
       InterfaceClass:       0x0E,    // Video class
       InterfaceSubclass:    0x01,    // Video Control
       ButtonPressPattern:   []byte{0x02, 0x01, 0x00, 0x00},
       ButtonReleasePattern: []byte{0x02, 0x01, 0x00, 0x01},
       Notes:                "Added YYYY-MM-DD",
   },
   ```

4. **Rebuild and test:**
   ```bash
   make build
   ./dermoscope-helper --list-profiles  # Verify profile appears
   ./dermoscope-helper --debug          # Test with device
   ```

## Troubleshooting

### Windows Issues

**Device not detected:**
- Ensure the dermoscope is connected and powered on
- Try a different USB port (prefer USB 2.0 ports for compatibility)
- Check Device Manager for driver issues
- If needed, install WinUSB driver using Zadig (see Installation)

**USB access error:**
- Run the application as Administrator
- Check if another application is using the device exclusively

**F9 keypress not received by browser:**
- Ensure the TrichoAI browser window/tab is focused
- Check that no other application is capturing F9 globally
- Try clicking in the browser before pressing the dermoscope button

**Application crashes on startup:**
- Run with `--debug` flag to see detailed error messages
- Check Windows Event Viewer for crash information

### macOS Issues

**"Cannot access USB device" error:**
- Run with `sudo` for USB permissions
- Check System Preferences > Security & Privacy for any blocked components

**Keyboard simulation fails:**
- Grant Accessibility permissions (see Installation section)
- After granting permissions, restart the application
- If moved from Downloads, re-grant permissions

**Camera not working when helper is running:**
- This is a known limitation on macOS (see Known Limitations)
- Quit the helper application to release the USB interface
- The camera will become available again

**Application not appearing in system tray:**
- macOS may hide tray icons; check the menu bar expansion area
- Grant Full Disk Access if prompted

### Linux Issues

**"xdotool not found" error:**
- Install xdotool: `sudo apt-get install xdotool`
- Ensure xdotool is in your PATH

**"libusb" errors:**
- Install libusb: `sudo apt-get install libusb-1.0-0-dev`
- Verify installation: `pkg-config --libs libusb-1.0`

**Permission denied accessing USB:**
- Set up udev rules (see Installation section)
- Alternative: run with `sudo`
- After adding udev rules, logout and login or run `sudo udevadm trigger`

**Keyboard events not working:**
- Ensure you're running an X11 session
- Wayland may require XWayland or alternative keyboard simulation
- Test xdotool directly: `xdotool key F9`

**System tray not appearing:**
- Ensure your desktop environment supports system trays
- GNOME users may need a tray extension like "AppIndicator Support"

### General Issues

**Button press causes multiple captures:**
- The debounce setting may need adjustment
- Create a config file and increase `debounce_ms` (e.g., 300 or 500)

**Button press not detected:**
- Run with `--debug` to see USB communication details
- Verify the device appears in `--list-profiles`
- Check that button patterns match (may need USB protocol analysis)

**Application uses too much CPU:**
- Increase `read_timeout_ms` in config file
- Check for USB communication errors in debug output

## Known Limitations

### macOS Camera Blocking
On macOS, claiming the USB interface for button monitoring prevents simultaneous camera access. This is a fundamental limitation of how macOS handles USB Video Class devices - the kernel driver requires exclusive access. **Workaround:** Quit the helper when you need to use the camera, restart it when you need button capture.

### Single Device at a Time
The application monitors only one dermoscope at a time. If multiple supported devices are connected, the first one detected is used.

### No Graphical Configuration
Configuration is done via command-line flags and JSON config files. There is no graphical settings interface.

### X11 Required on Linux
Keyboard simulation on Linux uses xdotool, which requires X11. Native Wayland keyboard simulation is not currently supported.

### No Auto-Update
The application does not auto-update. Check the Releases page for new versions.

### Keyboard Focus Required
The F9 keypress is sent to the currently focused application. The TrichoAI browser window must be in focus to receive the capture trigger.

## Building from Source

### Prerequisites

- Go 1.21 or later
- libusb development headers (see platform-specific installation above)
- Git

### Build Steps

```bash
# Clone the repository
git clone https://github.com/trichoai/dermoscope-helper.git
cd dermoscope-helper

# Download dependencies
make deps

# Build for current platform
make build

# Build for all platforms
make build-all

# Run tests
make test

# Run tests with coverage
make test-coverage
```

### Makefile Targets

| Target | Description |
|--------|-------------|
| `make build` | Build for current platform |
| `make dev` | Development build (no optimizations) |
| `make build-windows` | Cross-compile for Windows (amd64) |
| `make build-darwin` | Build for macOS (Intel + Apple Silicon) |
| `make build-linux` | Cross-compile for Linux (amd64) |
| `make build-all` | Build for all platforms |
| `make test` | Run all tests |
| `make test-coverage` | Run tests with coverage report |
| `make vet` | Run Go vet |
| `make fmt` | Format code with gofmt |
| `make clean` | Remove build artifacts |
| `make list-profiles` | Build and list supported profiles |

### Cross-Compilation Notes

Cross-compilation from macOS requires additional setup due to CGO dependencies:

- **For Windows:** Install mingw-w64: `brew install mingw-w64`
- **For Linux:** Install musl-cross: `brew install FiloSottile/musl-cross/musl-cross`

For CI/CD, it's recommended to build on native runners for each target platform.

### Output Binaries

Built binaries are placed in the `dist/` directory:
- `dermoscope-helper-X.X.X-windows-amd64.exe`
- `dermoscope-helper-X.X.X-darwin-amd64`
- `dermoscope-helper-X.X.X-darwin-arm64`
- `dermoscope-helper-X.X.X-linux-amd64`

## Project Structure

```
dermoscope-helper/
├── cmd/
│   └── dermoscope-helper/
│       └── main.go              # Application entry point
├── internal/
│   ├── usb/
│   │   ├── device.go            # USB device management
│   │   ├── monitor.go           # Interrupt endpoint monitoring
│   │   ├── profiles.go          # Device profile registry
│   │   └── types.go             # USB-related types
│   ├── keyboard/
│   │   ├── simulator.go         # Keyboard interface
│   │   ├── windows.go           # Windows implementation
│   │   ├── darwin.go            # macOS implementation
│   │   └── linux.go             # Linux implementation
│   ├── tray/
│   │   ├── tray.go              # System tray management
│   │   ├── icons.go             # Embedded icon resources
│   │   └── menu.go              # Tray menu handling
│   ├── app/
│   │   ├── app.go               # Main application logic
│   │   ├── state.go             # State machine implementation
│   │   └── config.go            # Configuration management
│   └── logger/
│       └── logger.go            # Logging setup
├── assets/                       # Icon placeholder directory
├── dist/                         # Built binaries (generated)
├── scripts/
│   └── build-all.sh             # Cross-platform build script
├── go.mod
├── go.sum
├── Makefile
└── README.md
```

## License

Proprietary - TrichoAI

## Related Documentation

- [DESIGN.md](../docs/DESIGN.md) - Design document with USB protocol analysis
- [GO-SPECS.md](../docs/GO-SPECS.md) - Implementation specification

## Support

For issues or questions, please contact the TrichoAI development team or open an issue on the repository.
