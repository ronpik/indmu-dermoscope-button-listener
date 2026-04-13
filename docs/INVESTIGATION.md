# Dermoscope Button Helper — Investigation Notes

End-to-end record of one debugging/discovery session: starting from a Go helper that didn't work on Windows, ending with a working C++ MVP that owns the camera and serves it to a browser while detecting hardware-button presses. Captures every theory tried, what failed, what worked, and the open issues left at the end.

> **Audience:** future-you (or a teammate) who needs to understand *why* the helper looks the way it does on Windows. The original `dermoscope-helper/` Go code is unchanged by this investigation; this document explains what we'd refactor and why.

> **If you're picking up the multi-click tuning work**, start from [`docs/NEXT-SESSION.md`](NEXT-SESSION.md) — it's a self-contained briefing that points back here for context.

## Companion documents

| File | When to read it |
|---|---|
| [`docs/NEXT-SESSION.md`](NEXT-SESSION.md) | Resuming the multi-click tuning issue specifically. Contains the latest empirical log + concrete tuning suggestions |
| [`docs/DESIGN.md`](DESIGN.md) | Original project design (pre-investigation). USB protocol details + alternatives the original author considered. **Note:** the "Windows works without blocking the camera" claim in here is now known to be wrong for this device — see §7 below |
| [`docs/GO-SPECS.md`](GO-SPECS.md) | Implementation spec for the original Go helper |
| [`README.md`](../README.md) | Repo orientation |
| [`dermoscope-helper/README.md`](../dermoscope-helper/README.md) | Usage docs for the original Go helper (gousb-based; broken on Windows — see §3) |
| [`dermoscope-helper/internal/usb/profiles.go`](../dermoscope-helper/internal/usb/profiles.go) | HT-B30S VID/PID/byte-pattern profile in the Go code |
| [`windows-helper/`](../windows-helper/) | **The canonical C++ MVP** (moved from `%TEMP%\probe\`). `helper.cpp` + `Makefile` (shared + static builds) + `README.md` (build/run/integration) |
| `C:\Users\Maya\AppData\Local\Temp\probe\probe5.cpp` | The breakthrough probe (SampleGrabber on Still pin). Left in place for reference; not needed for day-to-day work |
| `C:\Users\Maya\AppData\Local\Temp\probe\probe8_mediatypes.cpp` | Lists pin media types (MJPG/YUY2 × 7 resolutions) |
| `C:\Users\Maya\Downloads\30\30\HotScalp Installation neutral v1.26.exe` | Vendor software we reverse-engineered |
| `C:\Users\Maya\Downloads\30\30\amcap v3.0.9.exe` | Generic Microsoft DirectShow sample — useful for validating the Still-pin mechanism without our own code |

---

## 1. Starting state

- Repo: a Go helper (`dermoscope-helper/`) that uses `gousb`/libusb to claim the dermoscope's USB interface, read button events from the UVC interrupt endpoint, and inject `F9` keystrokes via `keybd_event`/`SendInput`.
- Device: HT-B30S (Sonix), VID `0xAB02`, PID `0xAB01`. Enumerates as a USB composite device with two interfaces:
  - `MI_00` — UVC camera (`usbvideo.sys`)
  - `MI_02` — USB audio mic (`usbaudio.sys`)
- README claim: *"Windows works without blocking camera access."*
- Reality before this session: Go helper compiled but at runtime found the device, then failed with `libusb: not found [code -5]` when trying to claim the Video Control interface.

## 2. Building and running the existing Go code (Windows)

The project had no pre-built binary. The Windows build needs:
- Go 1.21+
- mingw-w64 GCC (CGO requires a C toolchain)
- libusb-1.0 (gousb wraps it)
- pkg-config (gousb's CGO directives use it)

Cleanest install path on Windows: **MSYS2 + pacman**, *not* Git Bash (which ships a stripped MINGW64 with no package manager).

```
winget install --id MSYS2.MSYS2 -e --accept-source-agreements --accept-package-agreements
# In MSYS2 MINGW64 shell:
pacman -Syu
pacman -S --needed mingw-w64-x86_64-go mingw-w64-x86_64-gcc \
                    mingw-w64-x86_64-pkg-config mingw-w64-x86_64-libusb make
```

Build output: `dermoscope-helper.exe` (~5 MB), depends on `libusb-1.0.dll`, `libgcc_s_seh-1.dll`, `libwinpthread-1.dll` from `C:\msys64\mingw64\bin`. To run outside the MSYS2 shell (Explorer, cmd, PowerShell), copy those three DLLs next to the exe.

Other notes from the build:
- The Makefile's `build` target writes `dermoscope-helper` (no `.exe`); `make build-windows` writes `dist/dermoscope-helper-1.0.0-windows-amd64.exe`.
- `assets/icon-*.png` are 81-byte placeholders → systray logs `Unable to set icon: The operation completed successfully` on startup. Cosmetic.

## 3. First failure: gousb can't claim the interface

Running `dermoscope-helper.exe --debug` against the connected device produced:
```
DBG Searching for device...
DBG No device found  error="libusb: not found [code -5]"
```

Confirmation that the device IS present:
```powershell
Get-PnpDevice | Where-Object { $_.InstanceId -match 'VID_AB02' }
# USB Composite Device + USB Microphone (MI_02) + USB Camera (MI_00), all Status=OK
```

`code -5` = `LIBUSB_ERROR_NOT_FOUND`. On Windows, libusb can only talk to devices bound to **WinUSB**, **libusbK**, or **libusb-win32**. The inbox `usbvideo.sys` driver owns `MI_00` exclusively, so libusb can't enumerate it.

The README's "fix" was *"install WinUSB via Zadig"* — but binding WinUSB to `MI_00` would replace the UVC driver, killing camera access in browsers. That contradicts the "doesn't block camera access" promise.

**Conclusion:** the entire Go-helper architecture (gousb-based interrupt-endpoint read) is incompatible with sharing the UVC camera with the browser. Any Windows path must avoid claiming the USB interface directly.

## 4. Reading the Go code in full

Key modules:
- `cmd/dermoscope-helper/main.go` — flags + bootstrap.
- `internal/usb/profiles.go` — registry, one entry: HT-B30S (`AB02:AB01`), interface class `0x0E`/sub `0x01`, button press `02 01 00 00`, release `02 01 00 01`.
- `internal/usb/device.go` — `gousb.Context.OpenDevices` + `Config(1)` + claim by class/subclass.
- `internal/usb/monitor.go` — goroutine reading interrupt endpoint, debounced, emits press events on a channel.
- `internal/keyboard/windows.go` — wraps `keybd_event` (SendInput); `KeyF9 = 0x78`. Has a 2-second `time.Sleep` in `Initialize()`.
- `internal/tray/tray.go` — getlantern/systray; `toggleMonitoring` is a TODO.
- `internal/app/app.go` — state machine `STARTUP → SEARCHING → CLAIMING → MONITORING → DISCONNECTED`.

WIP issues spotted in passing (not blocking, but worth noting):
- `app.toggleMonitoring()` empty.
- No backoff in `searchForDevice` — tight loop when device absent.
- `LogFile`, `LogLevel`, `StartMinimized`, `AutoStart` config fields are dead (never read).
- `monitor.go:82` only emits press events; release events dropped.
- Tray icon assets are placeholders.

## 5. Reverse-engineering the manufacturer's bundled software

Vendor program: `HotScalp Installation neutral v1.26.exe` from `C:\Users\Maya\Downloads\30\30\`. Inno Setup installer (extracted with `innoextract`). Also bundled: `amcap v3.0.9.exe`, `Hotviewer Installation v2.0.11.20.exe`, `HotBeauty Installation v11.42.exe`.

Static analysis of `HotScalp.exe` (mingw `objdump -p`, `strings`, byte-level GUID search):

| Imports | Notes |
|---|---|
| `ole32`, `oleaut32`, `oleacc`, `oledlg` | COM, used implicitly |
| `opencv_highgui249.dll` | Camera capture (DirectShow under the hood) |
| `WBLSDK.dll` | Vendor SDK, **only loaded** native DLL |
| `GetAsyncKeyState`, `GetKeyState`, `SetWindowsHookExA` | Keyboard hooks |
| `cvCreateFileCapture`, `cvSaveImage` | OpenCV video |
| **No** `WinUSB`, `setupapi`, `HID`, `STI`, `WIA`, `KS*` | No direct USB or trigger-event interfaces |

Vendor SDK breakdown:
- `WBLSDK.dll` — leaked PDB path `E:\boliu_wifi\win_sdk\WBLSDK\Release\WBLSDK.pdb`. Class `CWBLSDK` with `startConnect(path, dataCb, eventCb)`. **WiFi-mode SDK** for HT-BW30/BW35, irrelevant to the USB HT-B30S.
- `UAVSDK.dll` — exports like `mjpeg_ndk_*`, `get_ip_port`, `handle_mcu_msg_*` — also WiFi/network. Built with Cygwin.
- `fxKeyMana.dll` — exports `fxOpenKey`, `fxReadSector`, `fxGetSerial` → **USB hardware-key DRM dongle** (FeiXian or similar), not keyboard.
- `XHWY_UKey.dll` — also DRM dongle.
- No driver, no service, no INF/SYS/CAT in any installer.

Byte-level GUID search across `HotScalp.exe` and `amcap.exe`:

| GUID | HotScalp | AMCap |
|---|---|---|
| `KSPROPSETID_VIDCAP_VIDEOCONTROL` | 1 | 1 |
| `IID_ISampleGrabber` | **1** | **1** |
| `CLSID_SampleGrabber` | **1** | **1** |
| `IID_IKsControl` | 0 | 0 |
| `IID_IAMVideoControl` | 0 | 0 |
| `KSEVENTSETID_VIDCAPTOSTI` | 0 | 0 |
| `IID_IWiaDevMgr` | 0 | 0 |

**Both binaries use `SampleGrabber`. Neither uses KS event subscription, IAMVideoControl, or WIA.** That hinted the trigger mechanism doesn't go through "events" at all — it's the *frames themselves*.

## 6. The user's empirical observation that broke my earlier theory

I had concluded HotScalp didn't actually read the dermoscope button. Then the user ran HotScalp and reported: pressing the button **freezes** the camera preview. Then they ran AMCap (a generic Microsoft DirectShow sample with zero device-specific code) and pressing the button **captured a still in a popup window** while preview kept streaming.

That single observation overturned the static-analysis conclusion: a generic DirectShow app responds to the dermoscope button → there's a documented Windows mechanism, not a custom one.

## 7. DirectShow probes — finding the right mechanism

Wrote a series of small C++ probes (under `C:\Users\Maya\AppData\Local\Temp\probe\`), built with mingw-w64 + DirectShow headers (`-lstrmiids -lole32 -loleaut32 -luuid`). Each probe answered one question.

### probe1, probe2, probe3 — KS event subscription
Tried `IKsControl::KsEvent` with `KSEVENTSETID_VIDEOCONTROL` + `KSEVENT_VIDEOCONTROL_VIDEO_TRIGGER`.

| Where | Pre-graph | Post-graph (running) |
|---|---|---|
| Filter | `0x80070492 ERROR_PROPSET_NOT_FOUND` | same |
| Capture pin | `0x80070006 ERROR_INVALID_HANDLE` | `0x80070492` |
| Still pin | `0x80070006` | `0x80070006` |

**Verdict:** This driver doesn't expose the trigger as a KS event subscription. Dead end.

### probe4 — IAMVideoControl polling (diagnostic)
- `GetCaps` on Still pin: `0xC = ExtTrigEnable | Trigger` ← driver advertises trigger support
- `SetMode(Still, ExternalTriggerEnable)` returned `S_OK` after a 2.5-second delay (suspicious)
- `GetMode(Still)` returned `0xC` constantly, never changed when button pressed

**Verdict:** `GetMode` echoes capabilities, not live state. The IAMVideoControl polling path is broken on this device.

### probe5 — SampleGrabber on Still pin (the breakthrough)
Setup: source filter → Preview pin → `NullRenderer`; source filter → Still pin → `SampleGrabber(BufferCB)` → `NullRenderer`. Graph runs continuously.

Result with the user pressing the button several times over 30 seconds:
```
[16:17:30.750] STILL FRAME #1   bytes=678992   first8=FF D8 FF C0 ...
[16:17:31.278] STILL FRAME #2   bytes=581392   first8=FF D8 FF C0 ...
... (11 frames matching button presses)
```

`FF D8 FF C0` = JPEG SOI + SOF0 marker. Every frame is a complete ~600 KB JPEG. **Pressing the hardware button literally causes the device to push a JPEG frame onto the UVC Still pin.** No event subscription needed — the *arrival of a sample* IS the trigger. AMCap and HotScalp both use this mechanism (consistent with our SampleGrabber GUID hits in §5).

This made the design clear: open the camera with DirectShow, attach a SampleGrabber to the Still pin, treat each callback as a button event.

### probe6 — Still-pin alone (coexistence test, part 1)
Hypothesis: maybe we can open just the Still pin without touching Capture, so the browser keeps the Capture pin to itself.

```
RenderStream(STILL -> Grabber -> NullStill)   HR=0x80004005 E_FAIL
```

**Verdict:** This driver requires the Capture pin to be active before the Still pin is openable. Still-pin-only is not possible.

### probe5 again, with browser open (coexistence test, part 2)
Opened a WebRTC camera test in a browser tab (browser holds the Capture pin), then ran probe5:

```
MediaControl::Run   HR=0x800705AA   ERROR_NO_SYSTEM_RESOURCES
Graph state: 0 (Stopped)
0 still frames captured
```

**Verdict:** **The dermoscope only allows one DirectShow consumer of the streaming pipeline at a time.** Helper-and-browser coexistence is not possible via standard DirectShow on this device. This contradicts the README's "doesn't block camera" promise.

### probe7 — WIA enumeration (last-ditch coexistence attempt)
Theory: WIA runs as a system service; UVC drivers can register cameras as WIA devices and fire `WIA_EVENT_DEVICE_USER_REQUEST` on button press, independent of the DirectShow streaming pipeline.

```
EnumDeviceInfo(WIA_DEVINFO_ENUM_LOCAL)   HR=0x00000000 S_OK
No WIA devices found at all.
```

**Verdict:** This dermoscope's inbox `usbvideo.sys` driver doesn't register the device with the WIA service. WIA path is unavailable.

### probe8 — pin media-type enumeration (sanity check)
The Capture pin offers `MJPG` and `YUY2` at 7 resolutions each (320x240 to 1600x1200). The Still pin offers the same. Native MJPG output means we can serve frames to a browser as MJPEG without re-encoding.

## 8. The decision: helper owns the camera (approach C)

After ruling out:
- **gousb / WinUSB** — would break the camera for the browser
- **WIA event subscription** — device not registered with WIA
- **IKsControl KS event** — driver doesn't expose the property set
- **IAMVideoControl polling** — driver returns broken mode values
- **DirectShow coexistence with the browser** — single-consumer device

…the only paths left were:
- **B) Helper owns the camera, exposes a virtual webcam** — needs an OBS-style virtual driver install per workstation
- **C) Helper owns the camera, serves it to the web app over HTTP** — needs web-app changes
- **D) Cooperative handoff** — web app releases camera, helper grabs still, web app re-acquires

User chose **C** as the most acceptable trade-off (no driver install, web-app changes are tractable).

## 9. The MVP (`windows-helper/helper.exe`)

Single self-contained C++ program in [`windows-helper/`](../windows-helper/). Build via the [`windows-helper/Makefile`](../windows-helper/Makefile) — `make shared` (small exe + 3 mingw runtime DLLs in `dist/`), `make static` (single fat exe in `dist-static/`), or `make all`. See [`windows-helper/README.md`](../windows-helper/README.md) for build prereqs, CLI usage, HTTP API details, and web-app integration guidance.

### Architecture (current state, after all the changes in §10)

```
SourceFilter (HT-B30S)
  ├── Capture pin (MJPG 1600x1200)  -> SampleGrabber(PreviewCB) -> NullRenderer
  │                                    feeds /preview (live MJPEG)
  │                                    AND serves as the source of the /still snapshot
  └── Still pin   (MJPG 320x240)    -> SampleGrabber(StillCB)   -> NullRenderer
                                       bytes are discarded; arrival is only the trigger

PreviewCB (on every 2nd frame):
  try_lock g_previewMutex -> memcpy into g_latestPreview -> notify /preview readers

StillCB (on every still-pin sample = button press):
  debounce within g_debounce_ms of previous press
  push timestamp to press_queue (wakes grouper)
  if this is the FIRST press of a burst:
      copy current g_latestPreview into g_latestStill (the /still buffer)

ClickGrouper worker (separate thread):
  wait for press_queue to become non-empty
  wait until g_group_ms has passed since the most recent press
  count presses, clamp to max 3
  send N F9 keystrokes via SendInput (~40 ms apart)

HTTP server (Winsock, port 8080, loopback-only):
  GET /         -> tiny HTML test page (F9-sequence handler + canvas)
  GET /preview  -> multipart/x-mixed-replace MJPEG from g_latestPreview (1600x1200)
  GET /still    -> image/jpeg from g_latestStill (1600x1200 snapshot of preview
                   at the moment the button was pressed)
```

### Why the split is the way it is

Two invariants drive the resolution choice:

1. **Preview pin at high res** so both the live stream and the snapshot are good quality. The snapshot IS a preview frame — we don't use the Still pin's own bytes.
2. **Still pin at low res** because the device's firmware cooldown after emitting a still is resolution-dependent. At 1600×1200 the cooldown is multi-second — we only ever get *one* still per click burst, and double/triple clicks look identical to single clicks. At 320×240 the cooldown is short (earlier traces showed ~515 ms), giving the grouper something to count. Discarding the Still-pin bytes is fine because we get capture quality from the Preview pin instead.

### Keystroke contract (current)

F10 and F11 were tried briefly and dropped — `F10` activates the window menu bar and `F11` toggles browser fullscreen. Neither can be sent without fighting the browser. The current design sends **N consecutive F9 keystrokes** (~40 ms apart, clamped at 3) and lets the web app count them within a short window (~400 ms) to dispatch single/double/triple actions. Matches what the production web app already handles.

### Test HTML page (served from `/`)

- `<img src="/preview">` renders the 1600×1200 MJPEG live
- `<canvas>` displays the last captured still
- F9 keydown handler buffers arrivals (`F9_WINDOW_MS = 400`) and, when quiet, dispatches:
  - `n=1` → `fetch('/still')` → draw blob onto canvas
  - `n=2` → no-op (keep last capture)
  - `n=3` → clear canvas

**End-to-end test result:** Single press → helper logs `still trigger accepted (q=1, first-in-burst -> snapshot preview)` then `captured preview frame into /still buffer: N bytes` → grouper fires `n=1, sending 1 F9` → test page fetches `/still` → canvas fills with the full-res JPEG. **Approach C works end-to-end for single-click capture.** Multi-click is still broken — see §10.

## 10. Multi-click click-pattern detection — current state (not yet finished)

User asked for a richer interaction:
- single click → capture
- double click → no-op (last capture stays)
- triple click → clear last capture

Implemented in `helper.cpp` as a "click grouper" thread:
- Each `StillCB` BufferCB pushes a press timestamp into a queue (debounced if within `g_debounce_ms` of the previous accepted press)
- Worker waits until `g_group_ms` of silence after the most recent press
- Counts presses → sends `F9` (1), `F10` (2), or `F11` (3) via `SendInput`
- Tunable via CLI: `helper.exe <port> <debounce_ms> <group_ms>`

### What we found while testing this

**Each physical click produces exactly one still frame** (confirmed: 6 deliberate clicks at ~3-second intervals produced exactly 6 stills in probe5).

**However, when many rapid clicks are attempted with the helper running, only a fraction reach us.** With the helper's full preview SampleGrabber active, even hammering the button 5+ times in a second often results in 1 still frame, then a 5–13-second gap before the next still is delivered.

Cause: USB bandwidth + SampleGrabber CPU load on the Capture pin chokes the Still pipeline. With `probe5` (which has only a `NullRenderer` on the Capture pin, no grabber), 37 stills in 14 seconds were possible from rapid clicking.

### Optimizations applied to the preview path

1. **Lower preview resolution to 320×240** — cuts MJPEG payload from ~600 KB to ~50 KB per frame, frees ~10× the USB bandwidth for the Still pin.
2. **Lower Still pin resolution to 320×240** as well — each still transfers in a fraction of the time, so the device's still pipeline recovers faster between clicks.
3. **Frame-skip** in `PreviewCB` — process every Nth frame (default `N=2`), halving CPU work in the SampleGrabber callback.
4. **`try_lock` instead of blocking lock** in `PreviewCB` — never blocks the SampleGrabber thread on contention with the HTTP server.
5. **Buffer reuse** — `resize` + `memcpy` instead of `assign` (no per-frame allocation).

### What the optimized helper actually showed (and why detection is unreliable)

After the optimizations a controlled test produced the trace below. Multi-click detection **does work occasionally** — but the user reports it's *"highly difficult"* to land doubles and harder still to land triples. The log explains why.

```
[17:38:19.197]   still (16592, +∞)        q=1  -> n=1   single
[17:38:23.364]   still (16592, +4171ms)   q=1  -> n=1
[17:38:23.875]   still (33176, +516ms)    q=1  -> n=1   ← same physical click as above?
[17:38:26.340]   still (16592, +2453ms)   q=1  -> n=1
[17:38:26.867]   still (33176, +531ms)    q=1  -> n=1   ← pair pattern repeats
[17:38:31.139]   still (16592, +1938ms)   q=1  -> n=1
[17:38:31.667]   still (33176, +515ms)    q=1  -> n=1
[17:38:31.923]   still (16592, +266ms)    q=2  -> n=2   ← only here doubles register
[17:38:54.627]   still (14992, +5062ms)   q=1
[17:38:55.140]   still (31576, +516ms)    q=2
[17:38:55.396]   still (16592, +250ms)    q=3  -> n=3   triple!
[17:38:55.923]   still (33176, +531ms)    q=1
[17:38:56.180]   still (16592, +250ms)    q=2  -> n=2   ← phantom: stragglers from triple
```

Two structural patterns the data forced us to acknowledge:

**Pattern A — every single click emits ~2 stills, ~515 ms apart.** The 515–535 ms gap recurs about 14 times in the log; the stills also alternate two distinct sizes (small ~16 KB, large ~33 KB) consistently in `small → large` order. This looks like the device emits one frame at button-down and another at button-up (or some equivalent two-phase emission). One physical click ≠ one still — it's typically a `(small, large)` pair.

**Pattern B — the 500 ms `group_ms` is *exactly* short enough to break the pair.** With `group_ms=500`, every ~515 ms intra-pair gap closes the group, so a single physical click reports as two `n=1` events. Multi-click detection only succeeds when the user clicks fast enough (≤266 ms inter-click gap) that the pairs from successive clicks blur together.

**Implication at the time:** the assumption "1 still = 1 click" was wrong. We increased `group_ms` to 800 ms (well above the 515 ms intra-pair gap) to keep pairs together.

### Then the picture changed again (still-pin resolution vs. firmware cooldown)

After raising `group_ms`, we wanted higher-quality captures so we raised the Still pin's resolution to 1600×1200. Multi-click detection **collapsed entirely** — only `q=1` per click burst, regardless of clicking speed. The `(small, large)` pair disappeared too.

The cause: **the device's firmware cooldown after emitting a still depends on the still's resolution.** At 320×240 the cooldown was short enough that rapid presses (~250 ms apart) each produced their own still. At 1600×1200 the cooldown stretches to multiple seconds, so all subsequent presses within a click burst are absorbed by the firmware and never reach us. The grouper only ever sees `q=1`.

### Current architecture (as of the last edit of `helper.cpp`)

To recover multi-click while keeping capture quality high, we split the two concerns across the two pins:

| Pin | Resolution | Role |
|---|---|---|
| Capture (Preview) | 1600×1200 MJPG | Live stream for `/preview`; also the *source* of the capture image. On each Still-pin trigger we snapshot the latest preview frame into the `/still` buffer. |
| Still | 320×240 MJPG | Hardware-button trigger only. Sample bytes are discarded; we just count arrivals. Small size keeps the firmware cooldown short. |

The keystroke contract also changed: we no longer send `F9/F10/F11`. The helper now sends **N consecutive `F9` keystrokes** (~40 ms apart, clamped at 3) and the web app counts F9s within a short window to dispatch the action. Rationale: `F10` activates the window menu bar and `F11` toggles fullscreen — both fight with the browser.

### Where this leaves us

Single-click capture is reliable. Double and triple click are still not reliable — even at 320×240 still-pin resolution, with the current architecture the grouper rarely sees `q≥2`. The "which upstream is absorbing the 2nd/3rd clicks" question is still open; see [`docs/NEXT-SESSION.md`](NEXT-SESSION.md) for the current hypotheses and the next experiments to try.

## 11. Final solution (Windows path)

For TrichoAI on Windows with this device, the architecture is:

1. **`helper.exe` runs as a local foreground app on the clinician's machine.** It owns the dermoscope camera via DirectShow with an asymmetric two-pin setup: **Capture pin at 1600×1200** (live preview + source of capture snapshots), **Still pin at 320×240** (hardware-button trigger only; its bytes are discarded).
2. **The web app fetches video from `http://localhost:8080/preview`** as a `multipart/x-mixed-replace` MJPEG stream — drop-in replacement for `<video>` + `getUserMedia`.
3. **On single button press**, the helper snapshots the current preview frame into `/still` and sends one `F9` keystroke. The web app receives F9 and `fetch('/still')` to render the full-res capture.
4. **Multi-click** (double/triple) is mechanically supported — the helper counts stills, clamps at 3, sends N F9 keystrokes 40 ms apart, and the web app counts them in a short window to dispatch single/double/triple actions. **Not yet reliable** (see §10 and [`docs/NEXT-SESSION.md`](NEXT-SESSION.md)); production UX should treat single-click as the only reliable gesture and move clear/undo/navigate to in-app controls.
5. **Linux/macOS paths can keep the original gousb design** (different OS-level driver semantics; not addressed in this session).

The reverse-engineering effort was load-bearing: it produced two facts that nothing else in our search uncovered, and which the README/DESIGN docs got wrong.

| Belief in the original docs | Reality on this device |
|---|---|
| "Windows works without blocking the camera" | False — single-consumer device |
| Button needs custom USB interrupt-endpoint reads | False — the Still pin's sample *arrival* is the trigger; bytes can be discarded |

## 12. What's intentionally not done

- **Refactor of the Go helper** — left as-is; this session produced the design, not the integration.
- **Service / tray packaging** of the C++ helper — the MVP is a console exe.
- **HTTPS / auth** on the local HTTP server — fine for `localhost` MVP, not for production.
- **Multi-click final tuning** — pending the empirical retest above.
- **Camera teardown / restart on device disconnect** — graph is built once at startup.
- **Multi-camera selection** — first match on substring `vid_ab02` is used.
- **macOS / Linux equivalents** of approach C — DirectShow is Windows-only; would need Media Foundation, V4L2, or AVFoundation analogs.

## 13. Useful artifacts left on disk

All under `C:\Users\Maya\AppData\Local\Temp\probe\` (volatile — `/tmp` in MSYS2 maps there):

| File | Purpose |
|---|---|
| `probe.cpp` (probe1) | First KS event subscription attempt |
| `probe2.cpp`, `probe3.cpp` | Adding graph build, IAMVideoControl polling |
| `probe4.cpp` | Diagnostic IAMVideoControl trace |
| `probe5.cpp` | **The breakthrough**: SampleGrabber on Still pin |
| `probe6.cpp` | Still-pin-only (failed: needs Capture active) |
| `probe7.cpp` | WIA enumeration (failed: device not WIA-registered) |
| `probe8_mediatypes.cpp` | Pin media-type enumeration |
| `helper.cpp` | **The MVP**: camera-server + button-trigger |
| `helper.exe` | Built MVP |
| `*.dll` (libstdc++/libgcc/libwinpthread) | mingw runtime, needed when running outside MSYS2 shell |

The MSYS2 install (Go + GCC + libusb + pkg-config + boost + bzip2 + xz + ICU + p7zip + innoextract) is in `C:\msys64\`.

## 14. Useful background facts collected along the way

- Git Bash's bundled "MINGW64" shell ≠ MSYS2's MINGW64 — Git Bash has no `pacman`. The actual MSYS2 has a separate Start-menu entry: **MSYS2 MINGW64**.
- `/tmp` in this MSYS2 install maps to `C:\Users\Maya\AppData\Local\Temp\` (set by `TMP=/tmp`). Convert with `cygpath -w`.
- mingw-built exes that use libstdc++ won't run from PowerShell unless `libstdc++-6.dll`, `libgcc_s_seh-1.dll`, `libwinpthread-1.dll` are on PATH or alongside the exe.
- `qedit.dll` (where `CLSID_SampleGrabber` and `CLSID_NullRenderer` live) is still present on Windows 10/11 inbox; we don't have to ship it.
- DirectShow's `IGraphBuilder::Render` returning `S_FALSE` (`0x00000001`) means "graph reached running but with warnings (e.g. one filter started, one didn't)" — for our use it's fine; what we care about is `IMediaControl::GetState` returning `State_Running`.
- The Inno Setup vendor installer can be unpacked without running it: `pacman -S mingw-w64-x86_64-innoextract; innoextract installer.exe`.
- Static GUID search in PE binaries: dump with `xxd -p`, then grep for the 16-byte little-endian GUID. Far more reliable than string search for COM-based codebases.
