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

1. Get `helper.exe` onto the target machine — download it from the [latest release](https://github.com/ronpik/indmu-dermoscope-button-listener/releases/latest) (see [Releases](#releases)), or copy a locally built `dist-static/helper.exe` (single file) or `dist/` (folder, exe + DLLs).
2. Plug in the dermoscope.
3. Double-click `helper.exe`. A console window may flash briefly and vanish (the helper detaches it at startup); an icon then shows up in the system tray (notification area), the helper starts capture immediately, and a balloon tells you the result: running, camera busy, device not found, or a generic start failure pointing at `helper.log`.
4. Open `http://localhost:8080/` in a browser for the built-in test page, or open the production web app. (Right-click the tray icon → **Open test page** does the same thing.)
5. Press the dermoscope button → browser (if focused) receives `F9` → fetches `/still` → displays the full-res capture.
6. When you're done, right-click the tray icon → **Exit**. That releases the camera for other apps.

No admin rights, no driver install, no firewall prompt (loopback-only bind).

If you want the old foreground-in-a-terminal behaviour with the log on stderr, run `helper.exe --console` — see [Runtime usage](#runtime-usage).

---

## The tray icon

The helper has no window; everything lives on the tray icon. (The icon appears in both modes — `--console` only changes where the log goes and whether the console window stays.)

**On launch** it adds the icon, starts capture straight away, and shows a balloon with the outcome.

> **Windows 11 hides new tray icons by default.** The first time you run the helper its icon goes into the
> overflow flyout behind the `^` chevron rather than the visible taskbar strip. Open the flyout and drag the
> icon onto the taskbar to pin it. This is standard Windows behaviour for any new tray app, not a fault in the
> helper — the balloon on launch still appears either way.

**Right-click** for the menu:

| Item | What it does |
|---|---|
| **Start** | Opens the camera and starts the HTTP server. Greyed out while already running. |
| **Stop** | Stops the server and fully releases the camera. Greyed out while stopped. |
| **Open test page** | Opens `http://localhost:<port>/` in the default browser, using the port this instance is actually running on. |
| **Exit** | Stops capture, removes the icon, quits. |

**Left double-click** the icon toggles Start / Stop.

**Hover** the icon for the tooltip. The tooltip is the authoritative state indicator:

| Tooltip | Meaning |
|---|---|
| `Dermoscope Helper: running` | Camera open and the HTTP server listening, as of the last successful Start. If the dermoscope is unplugged while running the helper notices, stops, switches the tooltip to `device not found` and starts auto-retrying — so replugging recovers on its own. It cannot promise to catch *every* stall, though: a graph that wedges without raising a DirectShow event still reads as `running`. If the preview has frozen, check `helper.log`. |
| `Dermoscope Helper: stopped` | Idle — you stopped it, or it hasn't been started. |
| `Dermoscope Helper: device not found` | No `VID_AB02` device present. The helper keeps retrying every few seconds on its own, so plugging the dermoscope in is enough — no need to click Start. Retries go to the log, not to balloons. |
| `Dermoscope Helper: camera busy` | Another app has the camera (see "Single-consumer device" below). Deliberately **not** auto-retried — that would fight the other app. Close it, then click **Start**. |
| `Dermoscope Helper: error` | Start failed for a reason other than a missing or busy device — most often the port is already in use by something else, or DirectShow / `qedit.dll` failed to create the graph. **Not** auto-retried. `helper.log` records the specific failure; fix that and click **Start**. |

The icon graphic is the app's own icon when the executable carries one (resource `101`) and a stock Windows icon otherwise; a build may additionally grey the icon out while the helper isn't running. Either way, read the tooltip for the real state.

**Stop and Exit normally release the camera straight afterwards.** The DirectShow graph is torn down and every device handle is released, so the Windows Camera app — or a browser tab doing `getUserMedia`, or anything else — can open the dermoscope. Nothing needs to be unplugged. Teardown takes a few seconds (the tray menu is unresponsive while it runs), so give it a moment before starting the other app. Teardown can also fail: if a wedged USB driver refuses to stop the graph, the helper logs a `WARNING: graph ... may not have been released` line. If a later app still reports the camera as busy, check `helper.log` for that warning. If a web app is consuming `/preview` while you Stop and Start, its `<img>` will freeze silently with no error — see [Recovering from a Stop/Start cycle](#recovering-from-a-stopstart-cycle).

---

## Build

Requires MSYS2 with the mingw-w64 toolchain. Same setup that builds the Go helper:

```bash
# in MSYS2 MINGW64 shell
pacman -S --needed mingw-w64-x86_64-gcc mingw-w64-x86_64-binutils make
cd windows-helper
make              # builds both shared and static
# or:
make shared       # small exe + 3 DLLs in dist/
make static       # single fat exe in dist-static/
make static VERSION=1.2.3   # same, but stamps 1.2.3 into the version resource
make clean
```

### Version and resources (`helper.rc`)

`VERSION` sets the exe's Win32 version resource — the file version, product version, company and description that Explorer shows under **Properties → Details**, and that Windows reputation heuristics look at on an unsigned binary. It **defaults to `0.0.0`**, so a plain `make static` produces an exe that reports `0.0.0`; release builds always pass the real number (see [Releases](#releases)).

```bash
make static VERSION=1.2.3
```

| Given | Stamped |
|---|---|
| *(nothing)* | `0.0.0` |
| `VERSION=1.2.3` | `1.2.3` |
| `VERSION=v1.2.3` | `1.2.3` — a leading `v` is stripped, so a tag name can be passed straight through |
| `VERSION=1.2.3-rc1` | `1.2.3-rc1` in the displayed strings, `1.2.3.0` in the numeric `FILEVERSION` fields |

Changing only `VERSION` is enough to force a re-stamp: the resource object also depends on `build/version.stamp`, which records the requested version and is rewritten only when that value actually changes. No `make clean` needed.

The resource script is [`helper.rc`](helper.rc). Besides the version block it embeds the application icon from [`assets/helper.ico`](assets/helper.ico) under **resource ID 101** (`IDI_APP`). That ID is fixed by contract — anything that needs the icon at runtime loads it with `LoadIcon(hInstance, MAKEINTRESOURCE(101))` — so do not renumber it. `helper.rc` is compiled by `windres` (from `mingw-w64-x86_64-binutils`) into `build/helper_res.o`, which the link step folds into the exe; `make` fails early with an install hint if `g++` or `windres` is missing. `helper.rc` and `assets/helper.ico` are tracked source files; `build/`, `dist/` and `dist-static/` are git-ignored.

### What `shared` vs `static` produce

| Target | Output | Size | Notes |
|---|---|---|---|
| `make shared` | `dist/helper.exe` + `libstdc++-6.dll`, `libgcc_s_seh-1.dll`, `libwinpthread-1.dll` | ~870 KB exe + ~2.7 MB DLLs | Ship the whole folder. DLLs are the mingw-w64 C/C++ runtime. Local builds only — never published to a release. |
| `make static` | `dist-static/helper.exe` | ~3.4 MB single file, built locally | No external deps. Drop anywhere and run. Slower to link, larger binary. **This is the build the Releases page ships**, though the CI-built asset is smaller (~1.5 MB) — the runner's mingw-w64 links a smaller static C++ runtime. |

Both link against **only inbox Windows DLLs** at runtime:
- `KERNEL32.dll`, `USER32.dll` — base Win32
- `ole32.dll`, `oleaut32.dll` — COM (DirectShow is COM-based)
- `strmiids` symbols — compile-time only; resolved into the binary
- `ws2_32.dll` — Winsock 2
- `quartz.dll` — DirectShow graph manager
- `qedit.dll` — `CLSID_SampleGrabber`, `CLSID_NullRenderer`
- `shell32.dll` — the tray icon (`Shell_NotifyIcon`) and **Open test page** (`ShellExecute`)
- `gdi32.dll` — loaded dynamically at startup only, to build the greyed-out icon variant. If it cannot be loaded the helper simply uses one icon for every state.

All of those are present on every Windows 7/8/10/11 installation. No Visual C++ Redistributable required.

### The one gotcha: `qedit.dll`

Microsoft deprecated `qedit.dll` long ago but it still ships with Windows 10 and 11. If a future Windows version removes it, `CoCreateInstance(CLSID_SampleGrabber)` will fail and the helper won't start. The replacement would be Media Foundation (`IMFSourceReader` + custom sink) — not done here because qedit still works today.

---

## Releases

End users do not build this. They download the prebuilt exe from the repo's Releases page:

**https://github.com/ronpik/indmu-dermoscope-button-listener/releases/latest**

The asset is named exactly `helper.exe` — a stable name that docs and client instructions hard-code, so it must not change between releases. `helper-<version>-windows-x64.zip`, holding the same exe at its root, is attached alongside it for browsers and AV products that block a bare `.exe` download. See [`CLIENT-HANDOFF.md`](CLIENT-HANDOFF.md) for what to tell a pilot customer.

### Cutting a release

`release.yml` must already be on the default branch (`main`). GitHub reads both the `release` and `workflow_dispatch` triggers from there, so merge the workflow before cutting the first release — on a feature branch neither trigger exists.

Tags are `vX.Y.Z`. Tag, push, and publish a GitHub release from that tag:

```bash
git tag v1.2.3
git push origin v1.2.3
# then publish a release from that tag in the GitHub UI, or in one step:
gh release create v1.2.3 --generate-notes
```

**Publishing the release is what fires the workflow.** Pushing the tag on its own does nothing, and a *draft* release does nothing either — the build only starts when the release is actually published. If you created a draft, publish it to trigger the build.

The version handed to `make` is the tag with the leading `v` stripped: tag `v1.2.3` → `make static VERSION=1.2.3`.

### What the workflow does

[`.github/workflows/release.yml`](../.github/workflows/release.yml), on a `windows-latest` runner:

| Step | Detail |
|---|---|
| Toolchain | MSYS2 with the MINGW64 mingw-w64 toolchain — the same compiler a local build uses. |
| Build | `make static VERSION=<tag without the leading v>`, so the published exe carries the release's version in its resource metadata. |
| Release assets | Attaches `helper.exe` and `helper-<version>-windows-x64.zip` to the published release (release events only). |
| Workflow artifact | Uploads `helper.exe` as a run artifact, `helper-<version>-windows-x64`, on every run — so a build is retrievable from the Actions run even when there is no release. |
| Checksum | Prints the version, byte size and SHA256 of `helper.exe` into the run summary. Verify a download with `Get-FileHash .\helper.exe -Algorithm SHA256`. |

### Smoke-testing without cutting a release

The workflow also has a `workflow_dispatch` trigger: **Actions** tab → **Release** → **Run workflow**. It takes an optional `version` input (default `0.0.0-dev`), builds the exe, and uploads it as a workflow artifact without creating a tag or touching any release — download the artifact from the run page and test it. This exercises the whole build and packaging path; the only step it does not reach is attaching the assets to a release, which runs on release events only.

Like a real release, this needs the workflow to be on the default branch — `workflow_dispatch` only lists workflows that exist on `main`.

### Code signing

Not wired up yet — the published exe is unsigned, so SmartScreen may warn on first run ("More info" → "Run anyway"). There is a commented-out Azure Trusted Signing placeholder in [`.github/workflows/release.yml`](../.github/workflows/release.yml) for when we do it.

---

## Runtime usage

```
helper.exe [--console] [port] [debounce_ms]
```

| Arg | Default | What it does |
|---|---|---|
| `--console` | off | Keeps the console window open and logs to **stderr**, as in previous versions. Without it the console is detached and the log goes to `helper.log` next to the exe. The tray icon appears either way. |
| `port` | `8080` | TCP port for the local HTTP server (bound to `127.0.0.1`). |
| `debounce_ms` | `300` | Suppresses still frames arriving within this window of the previous accepted one. Defense-in-depth only — the device's firmware cooldown between stills is much longer than this anyway. |

Flags don't consume positional slots: `helper.exe --console 9090` runs on port 9090.

### Log output

Every accepted still is logged with byte size and "sending F9"; debounced triggers are logged with a `DEBOUNCED` tag. Where that goes depends on the mode:

- **`--console`** — **stderr**, same format as before.
- **tray mode (the default)** — `helper.log` in the same folder as `helper.exe`, flushed line by line so it's still useful if the process is killed. If the file has grown past roughly 1 MB it's rotated once at startup to `helper.log.1`; only that one previous log is kept.

### Single instance

Only one helper runs at a time. Launching a second `helper.exe` exits immediately and quietly, and the instance already running shows a balloon to say so. Use its tray icon rather than starting another copy.

---

## HTTP API (for integrating a real web app)

All endpoints are under `http://localhost:<port>/` and bound to the loopback interface only, so other machines on the network cannot reach them.

> **What the loopback bind does and does not protect.** It stops *other machines*, not *other websites*. Every endpoint sends `Access-Control-Allow-Origin: *`, so while the helper is running, any web page open in any browser on that same machine can stream `/preview` and fetch `/still` cross-origin — including a third-party ad iframe on an unrelated page. In practice that means the dermoscope's live video is readable by local software and by any site the user happens to have open, for as long as the helper is running. The tray **Stop** and **Exit** commands close the port as well as releasing the camera, which is the mitigation available today. Tightening this (an `Origin` allowlist, or a token in the URL) would change the HTTP contract, so it is a deliberate decision for the integrating team rather than something the helper decides.

### `GET /preview`

Multipart MJPEG stream (`multipart/x-mixed-replace; boundary=frame`) at **1600×1200** (the HT-B30S's top MJPEG mode), typically ~15 fps. Intended to be consumed as the `src` of an `<img>` tag:

```html
<img src="http://localhost:8080/preview" alt="dermoscope preview">
```

Browser support is universal. You can also read the same URL with `fetch()` and parse the multipart stream yourself if you want the raw frames in JS, but for rendering there's no reason to. It is **not** usable with `EventSource` — that requires `text/event-stream`, and this is `multipart/x-mixed-replace`.

Response headers include `Access-Control-Allow-Origin: *` so the endpoint can be consumed from any origin.

Query strings on any endpoint are accepted and ignored (stripped before routing), so the common cache-busting pattern `/preview?t=${Date.now()}` works as expected — see "Recovering from a Stop/Start cycle" below for why you'd want that.

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

### Recovering from a Stop/Start cycle

The clinician can Stop and Start the camera at any time from the tray menu — a browser tab
does not need to be involved for that to happen. When the helper stops, an `<img>` already
pointed at `/preview` **freezes on its last frame and gives no signal that anything went
wrong**: `naturalWidth`/`naturalHeight` and `img.complete` stay exactly as they were, and
neither a `load` nor an `error` event fires. This is standard browser behaviour for
`multipart/x-mixed-replace` — a stream that quietly ends looks identical to one that is
merely idle between frames. `onerror` is **not** a usable signal here.

The reliable way to detect it is to poll an endpoint that only responds while the server is
up, e.g. `GET /`. When it starts responding again after having failed, re-request `/preview`
to reconnect — reusing the existing `<img>`'s `src` will not reconnect a stalled stream, so
either append a cache-busting query string (query strings are stripped server-side, see
above, so `/preview?t=...` is safe) or clear `src` before reassigning it:

```js
const img = document.getElementById('preview');
let wasUp = true;
setInterval(async () => {
  const isUp = await fetch('http://localhost:8080/', { cache: 'no-store' })
                      .then(r => r.ok).catch(() => false);
  if (isUp && !wasUp) {
    img.src = 'http://localhost:8080/preview?t=' + Date.now();  // force a fresh connection
  }
  wasUp = isUp;
}, 3000);
```

This is unrelated to the F9/`/still` capture path, which is stateless per request and needs
no such handling — a `fetch('/still')` issued while the server is stopped simply rejects.

### Mixed-content caveat

If your production web app is served over HTTPS and the helper serves HTTP at `http://localhost:8080/`, browsers *may* block the loads as mixed content. As of recent versions Chrome/Edge/Firefox/Safari treat `http://localhost` and `http://127.0.0.1` as **secure contexts**, which permits fetch/XHR/img loads from HTTPS pages — but test on your target browsers before committing. If blocked, the workarounds are (a) self-signed cert on the helper, (b) a WebSocket connection (secure-context rules differ), or (c) shipping an extension/PWA that relaxes the policy.

---

## Known issues (worth reading before building on this)

### Multi-click (double / triple) is not supported by design

The hardware button fires one still per press and that's all we get. We investigated exhaustively whether double/triple-click gestures could be detected reliably; the short answer is no — the device's UVC driver picks a high-bandwidth USB alt-setting at any Capture-pin resolution above 320×240, which crowds out the Still-pin deliveries for clicks 2 and 3 of a rapid burst. The 2nd/3rd clicks never arrive at user-mode. Lowering the Capture pin to 320×240 fixes it but reduces capture quality to below dermoscopy-usable. See [`../docs/NEXT-SESSION.md`](../docs/NEXT-SESSION.md) for the full per-experiment post-mortem.

**Practical consequence:** "clear", "undo", "re-take", "navigate" etc. need to live in the web app's own UI (buttons / keyboard shortcuts / gestures), not on the hardware button.

### Single-consumer device

Only one app can stream from the dermoscope at a time on Windows. If anything else (Teams, Skype, Zoom, a browser tab doing `getUserMedia`) has the camera open, `MediaControl::Run` returns `ERROR_NO_SYSTEM_RESOURCES` and the helper fails to start — the tray tooltip reads `camera busy`. Close the other app and click **Start**. (A second `helper.exe` can't cause this any more; it exits on startup — see "Single instance".) This is why the whole architecture is "helper owns the camera, web app consumes via HTTP" — sharing at the DirectShow level isn't possible on this device.

### Service mode not supported

`SendInput` only reaches the foreground window of the current interactive user session. Running the helper as a Windows service would break this. If you need auto-start, register it as a Startup-folder shortcut or a Task Scheduler task under the interactive user, not as a service. Shipping that as a feature — a "run at login" option and an installer — is a separate, later task; the tray build does not install or register itself.

### Preview and capture share one buffer

The captured image (`/still`) is a snapshot of the Preview pin at the moment the user pressed the button. If you lower the Capture-pin resolution, captured images drop in resolution too. Historically this trade-off existed to keep the Still pin tiny for multi-click detection; now that multi-click is out of scope, keeping it this way is just the simplest design. If you ever want to diverge preview and capture resolution on a future device, the cleanest switch is reading Still-pin bytes directly in `StillCB::BufferCB`.

---

## File layout

```
windows-helper/
├── README.md          -- this file
├── CLIENT-HANDOFF.md  -- what to send a pilot customer, and how they run it
├── Makefile           -- shared + static build targets, VERSION stamping
├── helper.cpp         -- single-file implementation
├── helper.rc          -- Win32 resources: version info + app icon (ID 101)
├── assets/
│   └── helper.ico     -- app icon, resource ID 101 (tracked source, not build output)
├── build/             -- intermediate objects incl. windres output    [git-ignored]
├── dist/              -- shared-build output (`make shared`)          [git-ignored]
│   ├── helper.exe
│   ├── libstdc++-6.dll
│   ├── libgcc_s_seh-1.dll
│   └── libwinpthread-1.dll
└── dist-static/       -- static-build output (`make static`)          [git-ignored]
    └── helper.exe
```

The three `[git-ignored]` directories are listed explicitly in the repo root [`.gitignore`](../.gitignore); no built binary is ever committed. Released exes live on the [Releases page](https://github.com/ronpik/indmu-dermoscope-button-listener/releases), not in the tree.

At runtime, tray mode writes `helper.log` (and, after a rotation, `helper.log.1`) next to `helper.exe` itself — the exe's own folder, not the current working directory. That is true wherever `helper.exe` came from, including a copy downloaded from Releases.

---

## Related docs

| File | When to read |
|---|---|
| [`../docs/INVESTIGATION.md`](../docs/INVESTIGATION.md) | Why the design is what it is — full chronology of probes, dead ends, and breakthroughs |
| [`../docs/NEXT-SESSION.md`](../docs/NEXT-SESSION.md) | Post-mortem of the multi-click investigation (why we went single-click only) |
| [`../docs/DESIGN.md`](../docs/DESIGN.md) | Original project design (mostly superseded by INVESTIGATION.md for Windows) |
| [`../dermoscope-helper/README.md`](../dermoscope-helper/README.md) | The original Go helper (works on Linux/macOS with caveats; **broken on Windows**) |
