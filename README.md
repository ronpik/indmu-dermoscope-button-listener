# Dermoscope Capture Driver

A native helper that translates USB button presses on a dermoscope device into an `F9` keystroke, so a browser-based web app (TrichoAI) can trigger hands-free image capture while the clinician holds the device with both hands on the patient.

Primary target device: **HT-B30S** (Sonix / Indmu, VID `0xAB02`, PID `0xAB01`). The architecture supports additional compatible models via a device-profile registry.

## The Problem in One Paragraph

The dermoscope's physical capture button does **not** emit a keyboard event and does **not** expose itself as a HID device. The button instead sends a 4-byte UVC (USB Video Class) *status interrupt* event on the Video Control interface (press = `[0x02, 0x01, 0x00, 0x00]`, release = `[0x02, 0x01, 0x00, 0x01]`). Browsers cannot read USB interrupt endpoints, so a native helper is required to bridge the hardware button to the web app.

## Solution Summary

Windows 11 is the primary platform, and the shipping Windows implementation is the native C++ helper under `windows-helper/`: it owns the camera via DirectShow, serves preview and still-capture over local HTTP, and turns the hardware button into an `F9` keystroke. Prebuilt `helper.exe` binaries are published on the [Releases page](https://github.com/ronpik/indmu-dermoscope-button-listener/releases/latest).

The earlier cross-platform Go helper (`dermoscope-helper/`) takes a different route — it claims the UVC Video Control interface and reads its interrupt endpoint directly. That works on Linux and, with caveats, on macOS, where claiming the interface is exclusive at the kernel level so the camera becomes unavailable to browsers while the helper runs. It does **not** work on Windows with this device.

## Repo Layout

| Path | Purpose |
|------|---------|
| `windows-helper/` | **Production Windows implementation** — single-file C++ helper, built with mingw-w64 (`make static`) and shipped from the Releases page as `helper.exe`. See its own `README.md`. |
| `dermoscope-helper/` | Original Go helper — Linux / macOS only, **broken on Windows** with this device. See its own `README.md` for install, usage, config, and supported devices. |
| `docs/DESIGN.md` | Full design document: background, USB protocol analysis, alternatives considered, platform trade-offs, architecture, device-profile system. |
| `docs/GO-SPECS.md` | Implementation spec for the Go helper: modules, dependencies, build targets, functional requirements (P0 / P1). |
| `dermoscope_helper.py`, `dermoscope_helper_v2.py`, `dermoscope_helper_v3.py` | Early Python prototypes (kept for historical reference — see below). |

## Chain of Events That Led Here

1. **v1 — `dermoscope_helper.py`** — Direct interrupt-endpoint read on the Video Control interface. Proved the button protocol on macOS, but claiming the interface blocks the camera for the browser.
2. **v2 — `dermoscope_helper_v2.py`** — Tried time-sliced "claim / read / release" cycles and a no-claim control-polling mode, aiming to keep the camera usable. Still blocked the camera on macOS.
3. **v3 — `dermoscope_helper_v3.py`** — Interactive troubleshooter that tries reading without detaching the kernel driver, raw libusb reads, and UVC control-transfer monitoring. All failed on macOS due to `AppleUSBVideoControl` exclusivity.
4. **Conclusion** — macOS kernel-driver exclusivity is the root limitation; Windows is the practical primary target. The investigation is written up in `docs/DESIGN.md`.
5. **Go helper** — Rewrite in Go for a single static binary, good USB support (`gousb`), cross-platform tray support, and easy cross-compilation. See `dermoscope-helper/` and `docs/GO-SPECS.md`.
6. **Windows C++ helper** — `gousb` could not claim the interface on Windows (usbvideo.sys owns it), so the Windows path was rebuilt natively on DirectShow: the helper owns the camera and serves preview / stills over local HTTP. This is what ships. See `windows-helper/` and `docs/INVESTIGATION.md`.

## Where to Read Next

- Installing / running on Windows → [`windows-helper/README.md`](windows-helper/README.md) and the [Releases page](https://github.com/ronpik/indmu-dermoscope-button-listener/releases/latest)
- Handing the Windows helper to a pilot customer → [`windows-helper/CLIENT-HANDOFF.md`](windows-helper/CLIENT-HANDOFF.md)
- Using / installing the Go helper (Linux / macOS) → [`dermoscope-helper/README.md`](dermoscope-helper/README.md)
- Why the Windows design is what it is → [`docs/INVESTIGATION.md`](docs/INVESTIGATION.md)
- Why it works this way, USB protocol details, alternatives that failed → [`docs/DESIGN.md`](docs/DESIGN.md)
- How the Go app is structured and what each module does → [`docs/GO-SPECS.md`](docs/GO-SPECS.md)
