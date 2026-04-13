# Dermoscope Capture Driver

A native helper that translates USB button presses on a dermoscope device into an `F9` keystroke, so a browser-based web app (TrichoAI) can trigger hands-free image capture while the clinician holds the device with both hands on the patient.

Primary target device: **HT-B30S** (Sonix / Indmu, VID `0xAB02`, PID `0xAB01`). The architecture supports additional compatible models via a device-profile registry.

## The Problem in One Paragraph

The dermoscope's physical capture button does **not** emit a keyboard event and does **not** expose itself as a HID device. The button instead sends a 4-byte UVC (USB Video Class) *status interrupt* event on the Video Control interface (press = `[0x02, 0x01, 0x00, 0x00]`, release = `[0x02, 0x01, 0x00, 0x01]`). Browsers cannot read USB interrupt endpoints, so a native helper is required to bridge the hardware button to the web app.

## Solution Summary

A small cross-platform Go application that:

1. Detects the dermoscope by VID/PID.
2. Claims the Video Control interface and reads its interrupt endpoint.
3. Parses the 4-byte UVC button event and debounces it.
4. Emits a virtual `F9` keypress which the TrichoAI web app listens for.
5. Runs in the system tray with auto-reconnect and status indicators.

Windows 11 is the primary platform. macOS is secondary and has a known limitation: claiming the Video Control interface is exclusive at the kernel level, so while the helper runs the camera is unavailable to browsers.

## Repo Layout

| Path | Purpose |
|------|---------|
| `dermoscope-helper/` | **Production Go implementation** — the helper app (cmd + internal modules + Makefile + build scripts). See its own `README.md` for install, usage, config, and supported devices. |
| `docs/DESIGN.md` | Full design document: background, USB protocol analysis, alternatives considered, platform trade-offs, architecture, device-profile system. |
| `docs/GO-SPECS.md` | Implementation spec for the Go helper: modules, dependencies, build targets, functional requirements (P0 / P1). |
| `dermoscope_helper.py`, `dermoscope_helper_v2.py`, `dermoscope_helper_v3.py` | Early Python prototypes (kept for historical reference — see below). |
| `dermoscope_investigation/` | Python virtualenv used during the investigation (no notes inside; the investigation findings are written up in `docs/DESIGN.md`). |
| `impl/go/` | Empty placeholder from the previous repo layout — the real Go code lives in `dermoscope-helper/`. |

## Chain of Events That Led Here

1. **v1 — `dermoscope_helper.py`** — Direct interrupt-endpoint read on the Video Control interface. Proved the button protocol on macOS, but claiming the interface blocks the camera for the browser.
2. **v2 — `dermoscope_helper_v2.py`** — Tried time-sliced "claim / read / release" cycles and a no-claim control-polling mode, aiming to keep the camera usable. Still blocked the camera on macOS.
3. **v3 — `dermoscope_helper_v3.py`** — Interactive troubleshooter that tries reading without detaching the kernel driver, raw libusb reads, and UVC control-transfer monitoring. All failed on macOS due to `AppleUSBVideoControl` exclusivity.
4. **Conclusion** — macOS kernel-driver exclusivity is the root limitation; Windows is the practical primary target. The investigation is written up in `docs/DESIGN.md`.
5. **Go helper** — Rewrite in Go for a single static binary, good USB support (`gousb`), cross-platform tray support, and easy cross-compilation. See `dermoscope-helper/` and `docs/GO-SPECS.md`.

## Where to Read Next

- Using / installing the helper → [`dermoscope-helper/README.md`](dermoscope-helper/README.md)
- Why it works this way, USB protocol details, alternatives that failed → [`docs/DESIGN.md`](docs/DESIGN.md)
- How the Go app is structured and what each module does → [`docs/GO-SPECS.md`](docs/GO-SPECS.md)
