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
                                            │  F9/F10/F11 keydown   │
                                            │  fetch('/still')      │
                                            └───────────────────────┘
                                                        ▲
                                                        │  SendInput
                                            (hardware button press)
```

- **Live preview** streams at the device's highest MJPEG resolution (1600×1200) from the UVC Capture pin.
- **Still capture** on button press: we snapshot the most recent preview frame into a dedicated buffer — that's what `GET /still` serves. Captures match the preview quality (1600×1200). The Still-pin stream is used *only* as the hardware trigger; its own bytes are discarded (small still samples, so the device's firmware cooldown is short and rapid clicks can be detected).
- **Keystrokes** via `SendInput` to the focused window. Helper buffers the click burst, counts, clamps to 3, and emits that many **F9** keystrokes in quick succession (~40 ms apart). The web app counts F9s within a short window and dispatches single / double / triple actions.

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
helper.exe [port] [debounce_ms] [group_ms]
```

| Arg | Default | What it does |
|---|---|---|
| `port` | `8080` | TCP port for the local HTTP server (bound to `127.0.0.1`). |
| `debounce_ms` | `150` | Suppresses still frames arriving within this window of the previous accepted one (anti-bounce). Real clicks that arrive within `debounce_ms` are **dropped**; keep it well under 500 ms. |
| `group_ms` | `800` | After the most recent accepted still, wait this long for more before deciding the click burst is over. Must exceed the device's inter-still throttle (observed ~515 ms when multi-click was working at low still-pin resolution). Trade-off: also the latency between the last click and the keystroke firing. **Note:** this setting is moot until the multi-click-not-detected bug is resolved — see "Known issues". |

Example: tolerate a wider multi-click window at the cost of a bit more latency:
```
helper.exe 8080 150 1200
```

### Log output

The helper prints to **stderr**. Every still that arrives is logged with a `+Xms` gap and an accepted-or-debounced status; every click-pattern decision (`n=1/2/3 → F9/F10/F11`) is logged. Useful for tuning and diagnosing.

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

Implementation note: this is *not* a still-pin frame — DirectShow's Still pin on this device is used only as the click trigger, and keeping it at low resolution is what makes rapid multi-click detection possible. For capture quality we snapshot the high-res preview-pin frame at trigger time and serve that.

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
- Listens for F9/F10/F11 and handles capture/no-op/clear
- Fetches `/still` on F9 to render the full-res JPEG into a canvas

Useful as a reference implementation and as a smoke test.

### Keystrokes (not HTTP, but part of the contract)

The helper waits for the button-click burst to finish (default `group_ms` = 800 ms of silence after the last click), counts the presses, **clamps at 3** (4+ presses are treated as 3), and sends **that many `F9` keystrokes** into the focused window via `SendInput`, spaced ~40 ms apart.

| Click pattern | Keystroke(s) sent | Intended web-app behavior |
|---|---|---|
| Single click | `F9` once | Capture → fetch `/still` |
| Double click | `F9 F9` (~40 ms apart) | No-op (preserve last capture) |
| Triple click (or more) | `F9 F9 F9` (~80 ms total) | Clear last capture |

**Why only F9 and not F10/F11:** both `F10` and `F11` have meaningful Windows behaviors (`F10` activates the window menu bar; `F11` toggles browser fullscreen). Sending them would fight with the browser. A single key with sequence counting is cleaner, and that's what your web app already handles.

**Web-app expectation:** buffer `F9` keydown events and dispatch after a short quiet window (~300–500 ms). See the `<script>` block served at `GET /` for a working example (`F9_WINDOW_MS = 400`).

The keys only arrive at the window that has focus. For this to work, the browser tab using the dermoscope must be focused when the user presses the button.

---

## Integrating with your web app

Minimum integration — add two things to the web app that currently uses `getUserMedia`:

1. **Replace the live video source.** Instead of `navigator.mediaDevices.getUserMedia({ video: ... })`, point the preview surface at the helper's MJPEG URL:
   ```html
   <img id="preview" src="http://localhost:8080/preview">
   ```
   Native resolution is 1600×1200; apply CSS for display size. If the preview looks laggy on a slow USB bus, you can drop the preview resolution by editing `configure_format(..., &PIN_CATEGORY_CAPTURE, 9999, 9999)` in `helper.cpp` — e.g. pass `1024, 768` — and rebuild.

2. **Buffer F9 keydowns and dispatch on count.** The helper sends 1, 2, or 3 F9s in quick succession (~40 ms apart) based on click count. Your handler counts them within a short window and picks the action:
   ```js
   const F9_WINDOW_MS = 400;
   let f9Count = 0;
   let f9Timer = null;

   async function dispatch() {
     const n = Math.min(f9Count, 3);
     f9Count = 0;
     if (n === 1) {
       const resp = await fetch('http://localhost:8080/still', { cache: 'no-store' });
       if (resp.ok) {
         const blob = await resp.blob();
         // ...render or upload the blob...
       }
     } else if (n === 2) {
       // double-click action (e.g. no-op / keep previous capture)
     } else if (n >= 3) {
       // triple-click action (e.g. clear)
     }
   }

   document.addEventListener('keydown', e => {
     if (e.key === 'F9' || e.code === 'F9') {
       e.preventDefault();
       f9Count++;
       if (f9Timer) clearTimeout(f9Timer);
       f9Timer = setTimeout(dispatch, F9_WINDOW_MS);
     }
   });
   ```

For reference, `helper.cpp`'s embedded `INDEX_HTML` string is a complete working example of both.

### Mixed-content caveat

If your production web app is served over HTTPS and the helper serves HTTP at `http://localhost:8080/`, browsers *may* block the loads as mixed content. As of recent versions Chrome/Edge/Firefox/Safari treat `http://localhost` and `http://127.0.0.1` as **secure contexts**, which permits fetch/XHR/img loads from HTTPS pages — but test on your target browsers before committing. If blocked, the workarounds are (a) self-signed cert on the helper, (b) a WebSocket connection (secure-context rules differ), or (c) shipping an extension/PWA that relaxes the policy.

---

## Known issues (worth reading before building on this)

### Multi-click (double / triple) is currently not detected

This is the biggest open bug. With the current configuration (preview 1600×1200, still 320×240, `debounce_ms=150`, `group_ms=800`), rapid clicks are registered as **a single click every time** — only one `still trigger accepted` log line per click burst, no matter the cadence. Single click + F9 capture works reliably; double/triple do not.

We've confirmed the root cause is **not** the click-grouping logic: the Still-pin SampleGrabber itself is only called once per burst, so there's nothing for the grouper to count. Something upstream — the device firmware, DirectShow, or USB-bandwidth contention with the high-res Preview stream — is absorbing the 2nd/3rd rapid presses.

A full write-up of what we know, what we tried, and the concrete experiments to run next is in [`../docs/NEXT-SESSION.md`](../docs/NEXT-SESSION.md). Short version: the most promising next step is lowering preview resolution (e.g. 640×480) and re-testing. If multi-click starts working at low preview res, the bottleneck is USB bandwidth; if it still doesn't, the device firmware is the limit and the UX needs to drop multi-click entirely.

**Practical workaround until this is fixed:** rely on **single click only** for capture. Put clear / undo / navigate into the web app's own UI (buttons, keyboard shortcuts, gestures).

### Single-consumer device

Only one app can stream from the dermoscope at a time on Windows. If anything else (Teams, Skype, Zoom, a browser tab doing `getUserMedia`, another instance of `helper.exe`) has the camera open, `MediaControl::Run` returns `ERROR_NO_SYSTEM_RESOURCES` and the helper fails to start. This is why the whole architecture is "helper owns the camera, web app consumes via HTTP" — sharing at the DirectShow level isn't possible on this device.

### Service mode not supported

`SendInput` only reaches the foreground window of the current interactive user session. Running the helper as a Windows service would break this. If you need auto-start, register it as a Startup-folder shortcut or a Task Scheduler task under the interactive user, not as a service.

### Preview and capture share one buffer

The captured image (`/still`) is a snapshot of the Preview pin at the moment the user pressed the button. If you lower the Capture-pin resolution to reduce USB load, captured images drop in resolution too. This is a deliberate trade-off: it lets us keep the Still pin tiny (fast multi-click) while still delivering high-res captures from the Preview pin. If you want high-res captures but low-res preview, you'd need to switch back to reading Still-pin bytes — which re-introduces the multi-click-choke problem we just fixed.

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
| [`../docs/NEXT-SESSION.md`](../docs/NEXT-SESSION.md) | If you're picking up the multi-click tuning issue specifically |
| [`../docs/DESIGN.md`](../docs/DESIGN.md) | Original project design (mostly superseded by INVESTIGATION.md for Windows) |
| [`../dermoscope-helper/README.md`](../dermoscope-helper/README.md) | The original Go helper (works on Linux/macOS with caveats; **broken on Windows**) |
