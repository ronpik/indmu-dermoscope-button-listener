# Dermoscope Button Helper - Design Document

**Version:** 1.1
**Date:** February 2025
**Status:** Design Complete, Implementation Pending

---

## Table of Contents

1. [Executive Summary](#1-executive-summary)
2. [Background & Problem Statement](#2-background--problem-statement)
3. [Technical Investigation Summary](#3-technical-investigation-summary)
4. [Solution Approach](#4-solution-approach)
5. [Functional Requirements](#5-functional-requirements)
6. [Alternative Logic Approaches](#6-alternative-logic-approaches)
7. [Platform Considerations](#7-platform-considerations)
8. [Architecture Overview](#8-architecture-overview)
9. [Device Profile System](#9-device-profile-system)
10. [Adding New Device Profiles](#10-adding-new-device-profiles)

---

## 1. Executive Summary

### Purpose

Develop a lightweight, cross-platform desktop application that monitors the physical capture button on supported dermoscope devices and translates button presses into keyboard events (F9) that can be received by the TrichoAI web application running in a browser.

The application supports multiple dermoscope models through a device profile system, allowing the same binary to work with different equipment across clinics.

### Why This Is Needed

The dermoscope's capture button does not emit standard keyboard events or expose itself as a HID device. The button sends UVC (USB Video Class) status interrupt events that require specialized USB access to detect. Standard browser APIs cannot access these events, necessitating a native helper application.

### Target Users

Medical clinicians at BENITAH Clinic using the TrichoAI hair loss diagnosis system who need hands-free image capture while positioning the dermoscope on patients.

---

## 2. Background & Problem Statement

### 2.1 The Clinical Workflow

```
┌─────────────────────────────────────────────────────────────────┐
│                    DESIRED CLINICAL WORKFLOW                     │
├─────────────────────────────────────────────────────────────────┤
│                                                                  │
│   1. Clinician opens TrichoAI web app in browser                │
│   2. Navigates to patient consultation → Image Capture stage    │
│   3. Opens camera modal showing dermoscope live feed            │
│   4. Positions dermoscope on patient's scalp with BOTH hands    │
│   5. Presses physical button on dermoscope to capture           │
│   6. Image is captured and saved to consultation                │
│                                                                  │
│   KEY REQUIREMENT: Hands-free capture (no keyboard/mouse)       │
│                                                                  │
└─────────────────────────────────────────────────────────────────┘
```

### 2.2 The Problem

The dermoscope (HT-B30S by Indmu/Sonix) has a physical capture button, but:

| What We Expected | What Actually Happens |
|------------------|----------------------|
| Button sends keyboard event | ❌ No keyboard event emitted |
| Button exposes as HID device | ❌ No HID interface present |
| Browser can detect button | ❌ No browser API available |
| Manufacturer provides SDK | ❌ No SDK, macOS app has no button support |

### 2.3 Device Identification

| Property | Value |
|----------|-------|
| Model | HT-B30S |
| Manufacturer | Sonix Technology Co., Ltd. |
| Vendor ID | 0xAB02 (43778) |
| Product ID | 0xAB01 (43777) |
| USB Class | Miscellaneous (0xEF) |
| Video Interface | UVC (USB Video Class) 1.1 |

---

## 3. Technical Investigation Summary

### 3.1 Discovery: Button Event Protocol

Through USB protocol analysis, we discovered that the button sends **UVC Status Interrupt Events** via the Video Control interface's interrupt endpoint.

**USB Interface Structure:**
```
Device: HT-B30S Dermoscope
├── Interface 0: Video Control (Class 0x0E, SubClass 0x01)
│   └── Endpoint 0x83: Interrupt IN ← BUTTON EVENTS HERE
├── Interface 1: Video Streaming (Class 0x0E, SubClass 0x02)
│   └── Isochronous endpoints for video frames
└── Interfaces 2-3: Audio (Class 0x01)
```

**Button Event Format (4 bytes):**

| Byte | Field | Press Value | Release Value |
|------|-------|-------------|---------------|
| 0 | Status Type | 0x02 (Streaming) | 0x02 |
| 1 | Originator | 0x01 (Unit 1) | 0x01 |
| 2 | Event | 0x00 (Button) | 0x00 |
| 3 | Selector | 0x00 (Pressed) | 0x01 (Released) |

**Captured Events:**
```
Button Press:   [0x02, 0x01, 0x00, 0x00]
Button Release: [0x02, 0x01, 0x00, 0x01]
```

### 3.2 The macOS Challenge

On macOS, a fundamental limitation was discovered:

```
┌─────────────────────────────────────────────────────────────┐
│                    macOS USB Architecture                    │
├─────────────────────────────────────────────────────────────┤
│                                                              │
│  AppleUSBVideoControl          Our Helper Application       │
│  (Kernel Driver)               (User Space)                 │
│         │                              │                    │
│         │  EXCLUSIVE ACCESS            │  NEEDS TO READ     │
│         ▼                              ▼                    │
│  ┌────────────────────────────────────────────────────┐    │
│  │           Interface 0 (Video Control)               │    │
│  │  • Camera control commands (brightness, etc.)       │    │
│  │  • Endpoint 0x83 (Interrupt - button events)       │    │
│  └────────────────────────────────────────────────────┘    │
│                                                              │
│  PROBLEM: Only ONE process can access at a time!            │
│                                                              │
│  Result: Reading button → Camera freezes                    │
│          Camera works  → Cannot read button                 │
│                                                              │
└─────────────────────────────────────────────────────────────┘
```

### 3.3 Windows Expectation

On Windows, USB driver architecture is more permissive:
- Multiple processes can often share USB interface access
- The manufacturer's Windows software successfully captures button events
- libusb/WinUSB can coexist with camera drivers

**This is why Windows is the primary target platform.**

---

## 4. Solution Approach

### 4.1 Chosen Architecture

```
┌─────────────────┐      USB       ┌───────────────────┐     F9      ┌─────────────────┐
│   Dermoscope    │  Interrupt     │  Helper App       │  Keyboard   │  TrichoAI Web   │
│   Button        │ ────────────►  │  (Native Binary)  │ ─────────►  │  App (Browser)  │
│                 │  Endpoint      │                   │  Simulation │                 │
└─────────────────┘                └───────────────────┘             └─────────────────┘
                                          │
                                          │ Also provides:
                                          ▼
                                   ┌─────────────────┐
                                   │ • System Tray   │
                                   │ • Status API    │
                                   │ • Auto-reconnect│
                                   └─────────────────┘
```

### 4.2 Why This Approach

| Requirement | How Addressed |
|-------------|---------------|
| Hands-free capture | Button press → F9 → Browser captures |
| Works with web app | No browser plugin needed, just keyboard event |
| Cross-platform | Go compiles to native binaries for Win/Mac/Linux |
| User-friendly | System tray app, runs in background |
| Reliable | Auto-reconnect on device disconnect/reconnect |
| Multiple device models | Profile system supports different dermoscope types |

### 4.3 Technology Choice: Go

| Option | Cross-Compile Mac→Win | Binary Size | Complexity |
|--------|----------------------|-------------|------------|
| Python + PyInstaller | ❌ No | ~50MB | Low |
| Rust | ✅ Yes | ~3MB | Medium-High |
| **Go** | ✅ Yes | ~5MB | **Medium** |
| C# / .NET | ✅ Yes | ~15MB | Medium |

**Go was chosen because:**
1. Native cross-compilation with simple commands
2. Single binary with no runtime dependencies
3. Excellent USB library (`gousb`)
4. Good system tray support (`systray`)
5. Simpler than Rust for this use case

---

## 5. Functional Requirements

### 5.1 Core Requirements (P0 - Must Have)

| ID | Requirement | Description |
|----|-------------|-------------|
| C1 | USB Device Detection | Detect supported dermoscope via device profiles (VID/PID matching) |
| C2 | Interface Claim | Claim Video Control interface based on profile settings |
| C3 | Interrupt Read | Read from interrupt endpoint for button events |
| C4 | Event Parsing | Parse button events using profile-specific patterns |
| C5 | Keyboard Simulation | Send F9 keypress to system on button press |
| C6 | Debouncing | Prevent duplicate triggers (200-300ms debounce) |
| C7 | Graceful Shutdown | Clean interface release on exit |
| C8 | Error Handling | Handle USB errors without crashing |

### 5.2 Priority 2 Requirements (P1 - Significant UX Improvement)

| ID | Requirement | Description |
|----|-------------|-------------|
| P1 | System Tray | Run as system tray app with icon |
| P2 | Status Indicator | Tray icon shows: disconnected/connected/monitoring |
| P3 | Auto-Reconnect | Detect device disconnect, reconnect when plugged back |
| P4 | Startup Option | Option to start with Windows/macOS |
| P5 | Menu Options | Tray menu: Status, Start/Stop, Exit |
| P6 | Logging | Log button presses and errors to file |

### 5.3 Nice to Have (P2 - Future Enhancement)

| ID | Requirement | Description |
|----|-------------|-------------|
| N1 | HTTP Status API | Expose `/status` endpoint for web app to query |
| N2 | Configurable Key | Allow user to change from F9 to another key |
| N3 | ~~Multiple Devices~~ | ~~Support multiple dermoscopes simultaneously~~ — **Not planned.** Single device operation; profile system supports different models. |
| N4 | Sound Feedback | Optional beep/sound on button press |
| N5 | Statistics | Track total button presses, uptime |
| N6 | Auto-Update | Check for and install updates |
| N7 | Installer | Windows installer (.msi) / macOS installer (.pkg) |

### 5.4 User Stories

**US1: Basic Capture**
> As a clinician, I want to press the dermoscope button and have the image captured in TrichoAI, so I can work hands-free.

**US2: Background Operation**
> As a clinician, I want the helper to run quietly in the background, so it doesn't interfere with my workflow.

**US3: Visual Feedback**
> As a clinician, I want to see the helper status in the system tray, so I know if it's working.

**US4: Device Reconnection**
> As a clinician, I want the helper to automatically reconnect when I plug the dermoscope back in, so I don't have to restart anything.

---

## 6. Alternative Logic Approaches

During the macOS investigation, several alternative approaches were implemented. While these failed on macOS due to kernel driver exclusivity, they may be useful on other platforms or as fallback strategies.

### 6.1 Approach A: Direct Interrupt Read (Primary - Used in v1)

**File:** `dermoscope_helper.py`

**Strategy:**
1. Find device by VID/PID
2. Find Video Control interface (class 0x0E, subclass 0x01)
3. Find Interrupt IN endpoint (endpoint type 0x03, direction IN)
4. Detach kernel driver if active
5. Claim interface exclusively
6. Read from interrupt endpoint in loop
7. Parse 4-byte UVC status events
8. Trigger F9 on button press pattern

**Pseudocode:**
```
device = find_usb_device(VID=0xAB02, PID=0xAB01)
interface = find_interface(class=VIDEO_CONTROL)
endpoint = find_endpoint(type=INTERRUPT_IN)

detach_kernel_driver(interface)
claim_interface(interface)

while running:
    data = read_endpoint(endpoint, timeout=500ms)
    if data == [0x02, 0x01, 0x00, 0x00]:  # Button press
        if debounce_ok():
            simulate_keypress(F9)

release_interface(interface)
```

**Result:** ✅ Works on macOS (but blocks camera)

### 6.2 Approach B: Quick Claim/Release Cycle (v2 - Quick Mode)

**File:** `dermoscope_helper_v2.py`

**Strategy:**
- Rapidly claim interface, read one event, release immediately
- Camera driver regains control between checks
- Trade latency for camera functionality

**Pseudocode:**
```
while running:
    device = find_device()
    detach_kernel_driver()
    claim_interface()

    data = read_endpoint(timeout=10ms)  # Very short timeout
    if data == BUTTON_PRESS:
        simulate_keypress(F9)

    release_interface()
    reattach_kernel_driver()

    sleep(poll_interval)  # 50-100ms
```

**Result:** ❌ Failed on macOS (still blocked camera, missed events)

### 6.3 Approach C: Read Without Detaching Driver (v3 - Mode 1)

**File:** `dermoscope_helper_v3.py`

**Strategy:**
- Attempt to claim interface WITHOUT detaching kernel driver
- Hope driver allows shared access

**Pseudocode:**
```
device = find_device()
# Skip: detach_kernel_driver()
claim_interface()  # May fail if driver has exclusive lock
read_endpoint()
```

**Result:** ❌ Failed on macOS - "Access denied (insufficient permissions)"

### 6.4 Approach D: Raw libusb Read Without Claim (v3 - Mode 2)

**File:** `dermoscope_helper_v3.py`

**Strategy:**
- Attempt to read directly from endpoint without claiming interface
- Use low-level libusb backend

**Pseudocode:**
```
device = find_device(backend=libusb1)
# Skip: claim_interface()
read_endpoint()  # Direct read attempt
```

**Result:** ❌ Failed on macOS - "Access denied (insufficient permissions)"

### 6.5 Approach E: UVC Control Polling (v3 - Mode 3)

**File:** `dermoscope_helper_v3.py`

**Strategy:**
- Poll UVC control values via control transfers
- Detect if any control changes when button is pressed
- Control transfers don't require interface claim

**Pseudocode:**
```
device = find_device()
controls = probe_available_controls()

prev_values = read_all_controls()

while running:
    current_values = read_all_controls()
    if current_values != prev_values:
        simulate_keypress(F9)
    prev_values = current_values
    sleep(20ms)
```

**Result:** ❌ Failed on macOS - "No readable controls found"
- Button doesn't modify any UVC control values
- Button only sends interrupt events

### 6.6 Summary Table

| Approach | Description | macOS Result | Windows Expected |
|----------|-------------|--------------|------------------|
| A: Direct Read | Claim interface, read interrupt | ✅ Works (blocks camera) | ✅ Likely works |
| B: Quick Cycle | Rapid claim/release | ❌ Blocks camera | 🔍 May work |
| C: No Detach | Claim without detaching driver | ❌ Access denied | 🔍 May work |
| D: Raw Read | Read without claiming | ❌ Access denied | 🔍 May work |
| E: Control Poll | Poll UVC control values | ❌ No controls found | ❌ Won't work |

**Recommendation for Windows:** Start with Approach A. If camera blocking occurs, try approaches B, C, D in order.

---

## 7. Platform Considerations

### 7.1 Primary Target: Windows

**Why Windows First:**
1. Manufacturer's software works on Windows (proves feasibility)
2. Clinic computers likely run Windows
3. Windows USB drivers more permissive
4. Cross-compilation from Mac is supported

**Windows-Specific Implementation:**
- Keyboard simulation: Windows `SendInput` API
- System tray: Standard Windows notification area
- USB access: `libusb` or `WinUSB`
- Permissions: May need to run as Administrator

### 7.2 Secondary Target: macOS

**Challenges:**
- Exclusive kernel driver access (documented in investigation)
- May only work with "Ready to Capture" workflow
- Requires Accessibility permissions for keyboard simulation

**macOS-Specific Implementation:**
- Keyboard simulation: CGEventCreateKeyboardEvent or AppleScript
- System tray: NSStatusItem
- USB access: libusb (requires detaching kernel driver)
- Permissions: Requires sudo for USB, Accessibility for keyboard

### 7.3 Future: Linux

**Expected to work well:**
- Linux USB permissions are configurable via udev rules
- No exclusive kernel driver issues expected
- Can run without sudo with proper udev configuration

**Linux-Specific Implementation:**
- Keyboard simulation: uinput or xdotool
- System tray: Various implementations (AppIndicator, etc.)
- USB access: libusb with udev rules
- Permissions: udev rule for USB, no special permissions for uinput

### 7.4 Cross-Platform Build Commands

```bash
# Build for Windows (from Mac)
GOOS=windows GOARCH=amd64 go build -o dermoscope-helper.exe

# Build for macOS
GOOS=darwin GOARCH=amd64 go build -o dermoscope-helper-mac
GOOS=darwin GOARCH=arm64 go build -o dermoscope-helper-mac-arm

# Build for Linux
GOOS=linux GOARCH=amd64 go build -o dermoscope-helper-linux
```

---

## 8. Architecture Overview

### 8.1 Component Diagram

```
┌─────────────────────────────────────────────────────────────────┐
│                    Dermoscope Helper Application                 │
├─────────────────────────────────────────────────────────────────┤
│                                                                  │
│  ┌──────────────┐    ┌──────────────┐    ┌──────────────┐       │
│  │   USB        │    │   Event      │    │   Keyboard   │       │
│  │   Monitor    │───►│   Processor  │───►│   Simulator  │       │
│  │              │    │              │    │              │       │
│  │ • Find device│    │ • Parse UVC  │    │ • Send F9    │       │
│  │ • Claim intf │    │ • Debounce   │    │ • Platform-  │       │
│  │ • Read intr  │    │ • Validate   │    │   specific   │       │
│  └──────────────┘    └──────────────┘    └──────────────┘       │
│         │                                                        │
│         │ Events                                                 │
│         ▼                                                        │
│  ┌──────────────┐    ┌──────────────┐    ┌──────────────┐       │
│  │   Device     │    │   System     │    │   Logger     │       │
│  │   Manager    │    │   Tray       │    │              │       │
│  │              │    │              │    │ • File log   │       │
│  │ • Hotplug    │    │ • Icon       │    │ • Console    │       │
│  │ • Reconnect  │    │ • Menu       │    │ • Rotation   │       │
│  │ • State mgmt │    │ • Status     │    │              │       │
│  └──────────────┘    └──────────────┘    └──────────────┘       │
│                                                                  │
└─────────────────────────────────────────────────────────────────┘
```

### 8.2 State Machine

```
                    ┌─────────────┐
                    │   STARTUP   │
                    └──────┬──────┘
                           │
                           ▼
            ┌──────────────────────────┐
            │     SEARCHING_DEVICE     │◄──────────────┐
            │  (Matching profiles)     │               │
            └──────────────┬───────────┘               │
                           │ Device Found              │
                           ▼                           │
            ┌──────────────────────────┐               │
            │    CLAIMING_INTERFACE    │               │
            │  (Detach driver, claim)  │               │
            └──────────────┬───────────┘               │
                           │ Success                   │
                           ▼                           │
            ┌──────────────────────────┐               │
    ┌──────►│       MONITORING         │───────┐       │
    │       │  (Reading interrupt EP)  │       │       │
    │       └──────────────┬───────────┘       │       │
    │                      │                   │       │
    │       Button Press   │   Device          │       │
    │       ───────────────┤   Disconnected    │       │
    │                      │   ────────────────┼───────┤
    │                      ▼                   │       │
    │       ┌──────────────────────────┐       │       │
    │       │    SENDING_KEYPRESS      │       │       │
    │       │  (Simulate F9, debounce) │       │       │
    │       └──────────────┬───────────┘       │       │
    │                      │                   │       │
    └──────────────────────┘                   │       │
                                               │       │
            ┌──────────────────────────┐       │       │
            │      DISCONNECTED        │◄──────┘       │
            │  (Waiting for reconnect) │───────────────┘
            └──────────────┬───────────┘  Device Plugged In
                           │
                           │ User Exit / Signal
                           ▼
            ┌──────────────────────────┐
            │       STOPPING           │
            │  (Release, cleanup)      │
            └──────────────────────────┘
```

**States:**
| State | Description |
|-------|-------------|
| STARTUP | Application initializing, loading profiles |
| SEARCHING_DEVICE | Scanning USB for devices matching any profile |
| CLAIMING_INTERFACE | Found device, claiming USB interface |
| MONITORING | Actively reading interrupt endpoint |
| SENDING_KEYPRESS | Transient: simulating F9 (returns to MONITORING) |
| DISCONNECTED | Device lost, polling for reconnection |
| STOPPING | Graceful shutdown in progress |

### 8.3 Error Handling Strategy

| Error Type | Handling |
|------------|----------|
| Device not found | Enter SEARCHING_DEVICE state, retry every 2s |
| Claim failed | Log error, retry with backoff |
| Read timeout | Normal, continue loop |
| Read error | Log, attempt reconnect |
| Keyboard sim failed | Log error, don't block monitoring |
| Unexpected exception | Log, attempt recovery, don't crash |

---

## 9. Device Profile System

### 9.1 Overview

The Dermoscope Helper supports **different dermoscope models** through a **Device Profile System**. Each profile contains the USB identification and button event signature for a specific device model.

**Key point:** The application monitors one device at a time. The profile system allows the same compiled binary to work with various dermoscope models—useful when different clinics have different equipment.

### 9.2 Profile Structure

A device profile consists of:

| Field | Description | Example |
|-------|-------------|---------|
| `ID` | Unique identifier for the profile | `"ht-b30s"` |
| `Name` | Human-readable device name | `"HT-B30S Dermoscope"` |
| `Manufacturer` | Device manufacturer | `"Sonix Technology"` |
| `VendorID` | USB Vendor ID (VID) | `0xAB02` |
| `ProductID` | USB Product ID (PID) | `0xAB01` |
| `InterfaceClass` | USB interface class to claim | `0x0E` (Video) |
| `InterfaceSubclass` | USB interface subclass | `0x01` (Video Control) |
| `ButtonPressPattern` | Byte pattern for button press | `[0x02, 0x01, 0x00, 0x00]` |
| `ButtonReleasePattern` | Byte pattern for button release | `[0x02, 0x01, 0x00, 0x01]` |
| `Notes` | Optional notes about the device | `"Primary clinic device"` |

### 9.3 Built-in Profiles

The application ships with the following built-in profiles:

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                         BUILT-IN DEVICE PROFILES                            │
├──────────────┬─────────────────────┬───────────────┬────────────────────────┤
│ Profile ID   │ Device              │ VID:PID       │ Status                 │
├──────────────┼─────────────────────┼───────────────┼────────────────────────┤
│ ht-b30s      │ HT-B30S Dermoscope  │ AB02:AB01     │ ✅ Verified            │
│              │ (Sonix/Indmu)       │               │                        │
├──────────────┼─────────────────────┼───────────────┼────────────────────────┤
│ (future)     │ Other devices       │ TBD           │ Add as discovered      │
└──────────────┴─────────────────────┴───────────────┴────────────────────────┘
```

### 9.4 Device Detection Flow

```
                    Application Startup
                           │
                           ▼
              ┌────────────────────────┐
              │  Load Device Profiles  │
              │  (Built-in Registry)   │
              └───────────┬────────────┘
                          │
                          ▼
              ┌────────────────────────┐
              │   Enumerate USB        │
              │   Devices              │
              └───────────┬────────────┘
                          │
                          ▼
              ┌────────────────────────┐
              │  For each USB device:  │
              │  Match VID:PID against │
              │  all profiles          │
              └───────────┬────────────┘
                          │
            ┌─────────────┴─────────────┐
            │                           │
            ▼                           ▼
    ┌───────────────┐           ┌───────────────┐
    │ Match Found   │           │ No Match      │
    │ → Use profile │           │ → Skip device │
    │ → Claim intf  │           │               │
    │ → Monitor     │           │               │
    └───────────────┘           └───────────────┘
```

### 9.5 Single Device Operation

The system monitors **one device at a time**. The profile system enables support for different dermoscope models, not simultaneous multi-device operation.

```
┌─────────────────┐                          ┌─────────────────┐
│ Dermoscope      │       USB                │ Dermoscope      │
│ (Any supported  │  ──────────────────►     │ Helper          │
│  profile)       │                          │                 │
└─────────────────┘                          │ • Detect device │
                                             │ • Match profile │
                                             │ • Monitor button│
                                             │ • Send F9       │
                                             └─────────────────┘
```

**If multiple devices are connected:**
- The first detected device matching any profile is used
- Other devices are ignored
- Switching devices requires disconnecting the current one

**Typical use case:**
- Clinic has one dermoscope connected at a time
- Different clinics may have different dermoscope models
- Same application binary works with any supported model

---

## 10. Adding New Device Profiles

### 10.1 Overview

To add support for a new dermoscope device, you need to:
1. **Discover** the device's USB identifiers (VID/PID)
2. **Capture** the button event byte patterns
3. **Add** a new profile to the registry
4. **Rebuild** the application

### 10.2 Prerequisites

**Tools needed:**
- The target dermoscope device
- A computer with USB ports
- USB analysis tools (see below per platform)

**Platform-specific tools:**

| Platform | Tool | Purpose |
|----------|------|---------|
| macOS | `system_profiler SPUSBDataType` | List USB devices |
| macOS | `ioreg -p IOUSB -l` | Detailed USB info |
| Windows | Device Manager | View VID/PID |
| Windows | USBDeview / USBTreeView | Detailed USB info |
| Linux | `lsusb -v` | List USB devices with details |
| All | Wireshark + USBPcap | Capture USB traffic |

### 10.3 Step 1: Discover USB Identifiers

#### macOS
```bash
# List all USB devices
system_profiler SPUSBDataType

# Look for your dermoscope in the output:
#   Product ID: 0xab01
#   Vendor ID: 0xab02 (Sonix Technology Co., Ltd.)
```

#### Windows
```
1. Open Device Manager
2. Find the dermoscope under "Imaging devices" or "Cameras"
3. Right-click → Properties → Details → Hardware IDs
4. Look for: USB\VID_AB02&PID_AB01
```

#### Linux
```bash
# List USB devices
lsusb

# Get detailed info for specific device
lsusb -d AB02:AB01 -v
```

**Record:**
- Vendor ID (VID): e.g., `0xAB02`
- Product ID (PID): e.g., `0xAB01`
- Manufacturer name
- Product name

### 10.4 Step 2: Identify the Interrupt Endpoint

Most dermoscopes use UVC (USB Video Class) with a Video Control interface that has an interrupt endpoint for button events.

#### Using lsusb (Linux/macOS with libusb)
```bash
lsusb -d VID:PID -v | grep -A 20 "VideoControl"

# Look for:
#   bInterfaceClass        14 Video
#   bInterfaceSubClass      1 Video Control
#   ...
#   bEndpointAddress     0x83  EP 3 IN
#   bmAttributes            3  Transfer Type: Interrupt
```

#### Key Information to Record:
- Interface number (usually 0)
- Interface class: `0x0E` (Video)
- Interface subclass: `0x01` (Video Control)
- Endpoint address (e.g., `0x83` = Endpoint 3 IN)
- Endpoint type: Interrupt (`0x03`)

### 10.5 Step 3: Capture Button Event Patterns

This is the critical step: you need to capture the actual byte patterns sent when the button is pressed and released.

#### Method A: Python Script (Recommended)

Create a test script to capture raw events:

```python
#!/usr/bin/env python3
"""
Button Event Capture Script
Usage: sudo python3 capture_button_events.py VID PID
Example: sudo python3 capture_button_events.py 0xAB02 0xAB01
"""

import sys
import usb.core
import usb.util

def capture_events(vid, pid):
    # Find device
    dev = usb.core.find(idVendor=vid, idProduct=pid)
    if dev is None:
        print(f"Device {vid:04x}:{pid:04x} not found")
        return

    print(f"Found: {dev.manufacturer} - {dev.product}")

    # Find Video Control interface
    for cfg in dev:
        for intf in cfg:
            if intf.bInterfaceClass == 0x0E and intf.bInterfaceSubClass == 0x01:
                print(f"Found Video Control interface: {intf.bInterfaceNumber}")

                # Find interrupt endpoint
                for ep in intf:
                    if usb.util.endpoint_type(ep.bmAttributes) == usb.util.ENDPOINT_TYPE_INTR:
                        if usb.util.endpoint_direction(ep.bEndpointAddress) == usb.util.ENDPOINT_IN:
                            print(f"Found interrupt endpoint: 0x{ep.bEndpointAddress:02x}")

                            # Detach kernel driver
                            if dev.is_kernel_driver_active(intf.bInterfaceNumber):
                                dev.detach_kernel_driver(intf.bInterfaceNumber)
                                print("Detached kernel driver")

                            # Claim interface
                            usb.util.claim_interface(dev, intf.bInterfaceNumber)
                            print("Claimed interface")

                            print("\n" + "="*50)
                            print("PRESS AND RELEASE THE BUTTON MULTIPLE TIMES")
                            print("Press Ctrl+C to stop")
                            print("="*50 + "\n")

                            try:
                                while True:
                                    try:
                                        data = dev.read(ep.bEndpointAddress, ep.wMaxPacketSize, timeout=1000)
                                        hex_str = " ".join(f"0x{b:02x}" for b in data)
                                        print(f"Event: [{hex_str}] (length: {len(data)})")
                                    except usb.core.USBTimeoutError:
                                        pass  # Normal, no event
                            except KeyboardInterrupt:
                                print("\nStopped")
                            finally:
                                usb.util.release_interface(dev, intf.bInterfaceNumber)
                            return

    print("No Video Control interface with interrupt endpoint found")

if __name__ == "__main__":
    if len(sys.argv) != 3:
        print("Usage: sudo python3 capture_button_events.py VID PID")
        print("Example: sudo python3 capture_button_events.py 0xAB02 0xAB01")
        sys.exit(1)

    vid = int(sys.argv[1], 16) if sys.argv[1].startswith("0x") else int(sys.argv[1])
    pid = int(sys.argv[2], 16) if sys.argv[2].startswith("0x") else int(sys.argv[2])

    capture_events(vid, pid)
```

**Run the script:**
```bash
# Install pyusb if needed
pip install pyusb

# Run with sudo (required for USB access)
sudo python3 capture_button_events.py 0xAB02 0xAB01
```

**Example output:**
```
Found: Sonix Technology Co., Ltd. - USB 2.0 Camera
Found Video Control interface: 0
Found interrupt endpoint: 0x83
Detached kernel driver
Claimed interface

==================================================
PRESS AND RELEASE THE BUTTON MULTIPLE TIMES
Press Ctrl+C to stop
==================================================

Event: [0x02 0x01 0x00 0x00] (length: 4)   ← Button PRESS
Event: [0x02 0x01 0x00 0x01] (length: 4)   ← Button RELEASE
Event: [0x02 0x01 0x00 0x00] (length: 4)   ← Button PRESS
Event: [0x02 0x01 0x00 0x01] (length: 4)   ← Button RELEASE
```

#### Method B: Wireshark USB Capture

For more complex analysis or when Python doesn't work:

1. **Install Wireshark with USB support**
   - Windows: Install USBPcap during Wireshark installation
   - Linux: `sudo modprobe usbmon`
   - macOS: Requires special setup (see Wireshark docs)

2. **Capture USB traffic**
   - Start Wireshark
   - Select USB interface (e.g., `usbmon1` on Linux)
   - Filter by device: `usb.idVendor == 0xAB02 && usb.idProduct == 0xAB01`

3. **Press the button and look for interrupt transfers**
   - Filter: `usb.transfer_type == 0x01` (Interrupt)
   - Look for small packets (4-8 bytes) that appear on button press

### 10.6 Step 4: Add Profile to Registry

Once you have all the information, add a new profile to the device registry.

**File to edit:** `internal/usb/profiles.go`

```go
var BuiltInProfiles = []DeviceProfile{
    // Existing profile
    {
        ID:                   "ht-b30s",
        Name:                 "HT-B30S Dermoscope",
        Manufacturer:         "Sonix Technology",
        VendorID:             0xAB02,
        ProductID:            0xAB01,
        InterfaceClass:       0x0E,
        InterfaceSubclass:    0x01,
        ButtonPressPattern:   []byte{0x02, 0x01, 0x00, 0x00},
        ButtonReleasePattern: []byte{0x02, 0x01, 0x00, 0x01},
        Notes:                "Primary clinic device, verified working",
    },

    // ADD YOUR NEW PROFILE HERE
    {
        ID:                   "your-device-id",
        Name:                 "Your Device Name",
        Manufacturer:         "Manufacturer Name",
        VendorID:             0x1234,  // Replace with actual VID
        ProductID:            0x5678,  // Replace with actual PID
        InterfaceClass:       0x0E,    // Usually Video class
        InterfaceSubclass:    0x01,    // Usually Video Control
        ButtonPressPattern:   []byte{0x02, 0x01, 0x00, 0x00},  // From capture
        ButtonReleasePattern: []byte{0x02, 0x01, 0x00, 0x01},  // From capture
        Notes:                "Added on YYYY-MM-DD, tested on Windows",
    },
}
```

### 10.7 Step 5: Rebuild and Test

```bash
# Rebuild for your platform
make build-windows   # or build-darwin, build-linux

# Test with the new device
./dermoscope-helper --debug
```

**Verify:**
1. Device is detected on startup
2. Correct profile is selected
3. Button press triggers F9
4. Button release is ignored (no double-trigger)
5. Debouncing works correctly

### 10.8 Profile Discovery Checklist

Use this checklist when adding a new device:

```
□ Device Information
  □ Manufacturer: _________________
  □ Model: _________________
  □ Vendor ID (VID): 0x____
  □ Product ID (PID): 0x____

□ USB Interface
  □ Interface Number: ____
  □ Interface Class: 0x____ (expected: 0x0E for Video)
  □ Interface Subclass: 0x____ (expected: 0x01 for Video Control)

□ Endpoint
  □ Endpoint Address: 0x____ (e.g., 0x83)
  □ Endpoint Type: Interrupt (0x03)
  □ Direction: IN (0x80)

□ Button Events
  □ Press Pattern: [0x__, 0x__, 0x__, 0x__]
  □ Release Pattern: [0x__, 0x__, 0x__, 0x__]
  □ Event Length: ____ bytes

□ Testing
  □ Device detected by helper
  □ Button press sends F9
  □ No double-triggers
  □ Reconnection works
  □ Tested on platform: ________________
```

### 10.9 Troubleshooting New Devices

| Issue | Possible Cause | Solution |
|-------|---------------|----------|
| Device not detected | Wrong VID/PID | Double-check with `lsusb` or Device Manager |
| "Access denied" | Permissions | Run as admin/sudo, or add udev rules (Linux) |
| No events captured | Wrong interface | Try other interfaces with interrupt endpoints |
| Events but no pattern match | Different format | Check byte order, try longer patterns |
| Camera stops working | Driver conflict | Expected on macOS; works on Windows |
| Erratic behavior | No debouncing | Verify debounce timing in config |

---

## References

- [GO-SPECS.md](./GO-SPECS.md) - Go implementation specification
- [GO-SPECS.md Section 5](./GO-SPECS.md#5-device-profile-system) - Device Profile System implementation details
- [README.md](../README.md) - Full investigation documentation
- [dermoscope_helper.py](../dermoscope_helper.py) - Primary Python implementation
- [dermoscope_helper_v2.py](../dermoscope_helper_v2.py) - Quick poll implementation
- [dermoscope_helper_v3.py](../dermoscope_helper_v3.py) - Multi-mode implementation
- [UVC 1.1 Specification](https://www.usb.org/document-library/video-class-v11-document-set) - USB Video Class spec

---

## Document History

| Version | Date | Author | Changes |
|---------|------|--------|---------|
| 1.0 | Feb 2025 | Claude | Initial design document |
| 1.1 | Feb 2025 | Claude | Added Device Profile System (Section 9) and Adding New Profiles guide (Section 10). Single device operation, profile system for supporting different models. |
