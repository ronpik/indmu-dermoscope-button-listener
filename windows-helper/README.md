# Dermoscope Windows Helper

A small Windows-only native app that owns the dermoscope camera, serves its video and still-captures over HTTP to a web app, and translates the dermoscope's hardware button into keystrokes the web app can listen for.

This is the **working Windows path** produced by the investigation documented in [`../docs/INVESTIGATION.md`](../docs/INVESTIGATION.md). The gousb-based Go helper under [`../dermoscope-helper/`](../dermoscope-helper/) does not work on Windows with this device (see §3 of the investigation doc).

---

## What it does

```
┌─────────────────────────────┐          ┌─────────────────────────────┐
│   Dermoscope (HT-B30S)      │   USB    │   helper.exe                │
│   VID:AB02 PID:AB01         │◄────────►│   Owns the camera           │
│   (UVC, usbvideo.sys)       │          │   DirectShow graph          │
└─────────────────────────────┘          └──────────────┬──────────────┘
                                                        │
                                            ┌───────────┴───────────┐
                                            │ HTTP server (:8080)   │
                                            │ GET  /preview  (MJPEG)│
                                            │ GET  /still    (JPEG) │
                                            │ GET  /         (HTML) │
                                            └───────────┬───────────┘
                                                        │  localhost
                                            ┌───────────▼───────────┐
                                            │  Browser / web app    │
                                            │  <img src="/preview"> │
                                            │  F9 keydown           │
                                            │  fetch('/still')      │
                                            └───────────────────────┘
                                                        ▲
                                                        │  SendInput
                                            (hardware button press)
```

- **Live preview** streams at the device's highest MJPEG resolution (1600×1200) from the UVC Capture pin.
- **Still capture** on button press: we snapshot the most recent preview frame into a dedicated buffer — that's what `GET /still` serves. Captures match the preview quality (1600×1200). The Still-pin stream is used *only* as the hardware trigger; its own bytes are discarded.
- **Keystroke** via `SendInput` to the focused window: one **F9** per button press. The web app treats every F9 as "capture". Multi-click gestures (clear / undo / etc.) are not reliable on this hardware — see "Known issues" below and [`../docs/NEXT-SESSION.md`](../docs/NEXT-SESSION.md) for the full post-mortem. Put those gestures in the web app's own UI.

---

## Quick start (end user, on a fresh Windows machine)

1. Copy either `dist/` (folder) **or** `dist-static/helper.exe` (single file) to the target machine.
2. Plug in the dermoscope.
3. Run `helper.exe` from a terminal (Command Prompt, PowerShell, or Windows Terminal). It will stay in the foreground and log to stderr.
4. Open `http://localhost:8080/` in a browser for the built-in test page, or open the production web app.
5. Press the dermoscope button → browser (if focused) receives `F9` → fetches `/still` → displays the full-res capture.

No admin rights, no driver install, no firewall prompt (loopback-only bind).

---

## Build

Requires MSYS2 with the mingw-w64 toolchain. Same setup that builds the Go helper:

```bash
# in MSYS2 MINGW64 shell
pacman -S --needed mingw-w64-x86_64-gcc make
cd windows-helper
make              # builds both shared and static
# or:
make shared       # small exe + 3 DLLs in dist/
make static       # single fat exe in dist-static/
make clean
```

### What `shared` vs `static` produce

| Target | Output | Size | Notes |
|---|---|---|---|
| `make shared` | `dist/helper.exe` + `libstdc++-6.dll`, `libgcc_s_seh-1.dll`, `libwinpthread-1.dll` | ~800 KB exe + ~2.7 MB DLLs | Ship the whole folder. DLLs are the mingw-w64 C/C++ runtime. |
| `make static` | `dist-static/helper.exe` | ~3.4 MB single file | No external deps. Drop anywhere and run. Slower to link, larger binary. |

Both link against **only inbox Windows DLLs** at runtime:
- `KERNEL32.dll`, `USER32.dll` — base Win32
- `ole32.dll`, `oleaut32.dll` — COM (DirectShow is COM-based)
- `strmiids` symbols — compile-time only; resolved into the binary
- `ws2_32.dll` — Winsock 2
- `quartz.dll` — DirectShow graph manager
- `qedit.dll` — `CLSID_SampleGrabber`, `CLSID_NullRenderer`

All of those are present on every Windows 7/8/10/11 installation. No Visual C++ Redistributable required.

### The one gotcha: `qedit.dll`

Microsoft deprecated `qedit.dll` long ago but it still ships with Windows 10 and 11. If a future Windows version removes it, `CoCreateInstance(CLSID_SampleGrabber)` will fail and the helper won't start. The replacement would be Media Foundation (`IMFSourceReader` + custom sink) — not done here because qedit still works today.

---

## Runtime usage

```
helper.exe [port] [debounce_ms]
```

| Arg | Default | What it does |
|---|---|---|
| `port` | `8080` | TCP port for the local HTTP server (bound to `127.0.0.1`). |
| `debounce_ms` | `300` | Suppresses still frames arriving within this window of the previous accepted one. Defense-in-depth only — the device's firmware cooldown between stills is much longer than this anyway. |

### Log output

The helper prints to **stderr**. Every accepted still is logged with byte size and "sending F9"; debounced triggers are logged with a `DEBOUNCED` tag.

---

## HTTP API (for integrating a real web app)

All endpoints are under `http://localhost:<port>/` and bound to the loopback interface only (not accessible over the network).

### `GET /preview`

Multipart MJPEG stream (`multipart/x-mixed-replace; boundary=frame`) at **1600×1200** (the HT-B30S's top MJPEG mode), typically ~15 fps. Intended to be consumed as the `src` of an `<img>` tag:

```html
<img src="http://localhost:8080/preview" alt="dermoscope preview">
```

Browser support is universal. The same URL also works in `fetch()`/`EventSource` if you want to decode frames in JS yourself, but for rendering there's no reason to.

Response headers include `Access-Control-Allow-Origin: *` so the endpoint can be consumed from any origin.

### `GET /still`

Returns the **most recent full-resolution JPEG** snapshot — a copy of the preview frame that was live at the moment the user pressed the hardware button. Content type is `image/jpeg`. Resolution matches the preview (1600×1200 unless you've edited the source). If no button has been pressed yet, returns `404`.

Implementation note: this is *not* a still-pin frame — DirectShow's Still pin on this device is used only as the click trigger and kept at 320×240 (its bytes are discarded). For capture quality we snapshot the high-res preview-pin frame at trigger time and serve that.

```js
// After receiving F9 (button-click-driven):
const resp = await fetch('http://localhost:8080/still', { cache: 'no-store' });
const blob = await resp.blob();
const img = new Image();
img.src = URL.createObjectURL(blob);
// img is 1600x1200 JPEG by default
```

Response includes `Access-Control-Allow-Origin: *`.

### `GET /`

Minimal HTML test page that:
- Renders `<img src="/preview">` live
- Listens for F9 and fetches `/still` to draw the full-res JPEG into a canvas
- Has a "Clear" button to wipe the canvas

Useful as a reference implementation and as a smoke test.

### Keystroke (not HTTP, but part of the contract)

On each accepted button press, the helper sends **one `F9`** keystroke into the focused window via `SendInput`. That's the whole contract.

**Why F9** (and not F10/F11): both `F10` and `F11` have meaningful Windows behaviors (`F10` activates the window menu bar; `F11` toggles browser fullscreen). Sending them would fight with the browser. F9 is unused by default in most apps.

**Web-app expectation:** listen for F9 `keydown` and `fetch('/still')`. No sequence buffering, no timers. The keys only arrive at the window that has focus — for this to work, the browser tab using the dermoscope must be focused when the user presses the button.

---

## Integrating with your web app

Minimum integration — add two things to the web app that currently uses `getUserMedia`:

1. **Replace the live video source.** Instead of `navigator.mediaDevices.getUserMedia({ video: ... })`, point the preview surface at the helper's MJPEG URL:
   ```html
   <img id="preview" src="http://localhost:8080/preview">
   ```
   Native resolution is 1600×1200; apply CSS for display size. If the preview looks laggy on a slow USB bus, you can drop the preview resolution by editing `configure_format(..., &PIN_CATEGORY_CAPTURE, 9999, 9999)` in `helper.cpp` — e.g. pass `1024, 768` — and rebuild.

2. **Listen for F9 keydown and fetch `/still`.** One F9 per button press.
   ```js
   document.addEventListener('keydown', async e => {
     if (e.key !== 'F9' && e.code !== 'F9') return;
     e.preventDefault();
     const resp = await fetch('http://localhost:8080/still', { cache: 'no-store' });
     if (!resp.ok) return;
     const blob = await resp.blob();
     // ...render or upload the blob...
   });
   ```

3. **Put "clear / undo / re-take" in the web app's own UI.** Those gestures used to be bound to double/triple-click on the hardware button; on the HT-B30S that turned out to be mechanically unreliable (see [`../docs/NEXT-SESSION.md`](../docs/NEXT-SESSION.md) for the full post-mortem). UI buttons or keyboard shortcuts are the right home.

For reference, `helper.cpp`'s embedded `INDEX_HTML` string is a complete working example.

### Mixed-content caveat

If your production web app is served over HTTPS and the helper serves HTTP at `http://localhost:8080/`, browsers *may* block the loads as mixed content. As of recent versions Chrome/Edge/Firefox/Safari treat `http://localhost` and `http://127.0.0.1` as **secure contexts**, which permits fetch/XHR/img loads from HTTPS pages — but test on your target browsers before committing. If blocked, the workarounds are (a) self-signed cert on the helper, (b) a WebSocket connection (secure-context rules differ), or (c) shipping an extension/PWA that relaxes the policy.

---

## Known issues (worth reading before building on this)

### Multi-click (double / triple) is not supported by design

The hardware button fires one still per press and that's all we get. We investigated exhaustively whether double/triple-click gestures could be detected reliably; the short answer is no — the device's UVC driver picks a high-bandwidth USB alt-setting at any Capture-pin resolution above 320×240, which crowds out the Still-pin deliveries for clicks 2 and 3 of a rapid burst. The 2nd/3rd clicks never arrive at user-mode. Lowering the Capture pin to 320×240 fixes it but reduces capture quality to below dermoscopy-usable. See [`../docs/NEXT-SESSION.md`](../docs/NEXT-SESSION.md) for the full per-experiment post-mortem.

**Practical consequence:** "clear", "undo", "re-take", "navigate" etc. need to live in the web app's own UI (buttons / keyboard shortcuts / gestures), not on the hardware button.

### Single-consumer device

Only one app can stream from the dermoscope at a time on Windows. If anything else (Teams, Skype, Zoom, a browser tab doing `getUserMedia`, another instance of `helper.exe`) has the camera open, `MediaControl::Run` returns `ERROR_NO_SYSTEM_RESOURCES` and the helper fails to start. This is why the whole architecture is "helper owns the camera, web app consumes via HTTP" — sharing at the DirectShow level isn't possible on this device.

### Service mode not supported

`SendInput` only reaches the foreground window of the current interactive user session. Running the helper as a Windows service would break this. If you need auto-start, register it as a Startup-folder shortcut or a Task Scheduler task under the interactive user, not as a service.

### Preview and capture share one buffer

The captured image (`/still`) is a snapshot of the Preview pin at the moment the user pressed the button. If you lower the Capture-pin resolution, captured images drop in resolution too. Historically this trade-off existed to keep the Still pin tiny for multi-click detection; now that multi-click is out of scope, keeping it this way is just the simplest design. If you ever want to diverge preview and capture resolution on a future device, the cleanest switch is reading Still-pin bytes directly in `StillCB::BufferCB`.

---

## File layout

```
windows-helper/
├── README.md          -- this file
├── Makefile           -- shared + static build targets
├── helper.cpp         -- single-file implementation (~500 lines)
├── dist/              -- shared-build output (created by `make shared`)
│   ├── helper.exe
│   ├── libstdc++-6.dll
│   ├── libgcc_s_seh-1.dll
│   └── libwinpthread-1.dll
└── dist-static/       -- static-build output (created by `make static`)
    └── helper.exe
```

---

## Related docs

| File | When to read |
|---|---|
| [`../docs/INVESTIGATION.md`](../docs/INVESTIGATION.md) | Why the design is what it is — full chronology of probes, dead ends, and breakthroughs |
| [`../docs/NEXT-SESSION.md`](../docs/NEXT-SESSION.md) | Post-mortem of the multi-click investigation (why we went single-click only) |
| [`../docs/DESIGN.md`](../docs/DESIGN.md) | Original project design (mostly superseded by INVESTIGATION.md for Windows) |
| [`../dermoscope-helper/README.md`](../dermoscope-helper/README.md) | The original Go helper (works on Linux/macOS with caveats; **broken on Windows**) |
