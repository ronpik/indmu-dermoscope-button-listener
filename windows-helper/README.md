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
                                            │ GET  /health   (JSON) │
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

- **Live preview** streams from the UVC Capture pin at **1024×768** by default, [configurable](#configuration-file-helper-configtxt) up to the device's top MJPEG mode of 1600×1200. The default is tuned for Wi-Fi, where frame *size* rather than frame rate sets what a remote browser can actually display.
- **Still capture** on button press: `GET /still` serves the **device's own still image** from the UVC Still pin at **1600×1200**, independent of the preview resolution. A capture freezes the preview for about 2 s — see "Known issues".
- **Software capture** without a button press: `GET /snapshot` returns the latest preview frame, for an on-screen Capture button.
- **Keystroke** via `SendInput` to the focused window: one **F9** per button press. The web app treats every F9 as "capture". Multi-click gestures (clear / undo / etc.) are not reliable on this hardware — see "Known issues" below and [`../docs/NEXT-SESSION.md`](../docs/NEXT-SESSION.md) for the full post-mortem. Put those gestures in the web app's own UI.

---

## Install

**Recommended: run the installer.** Download `DermoscopeHelper-Setup-X.Y.Z.exe` from the
[latest release](https://github.com/ronpik/indmu-dermoscope-button-listener/releases/latest)
(see [Releases](#releases)) and run it. There's no admin prompt — Windows won't even ask for
elevation, because the installer requests none.

What it does:

- Installs **per-user, no admin, no UAC prompt** into `%LOCALAPPDATA%\Programs\Dermoscope Helper`.
- Adds a Start Menu shortcut (and its own uninstall entry there).
- **Desktop shortcut** — offered, unchecked by default.
- **"Start Dermoscope Helper when I sign in"** — offered, **checked by default**: it drops a
  shortcut in your Startup folder (`shell:startup`), so the tray icon is there without you having
  to launch it by hand. Turn it off later without reinstalling: **Task Manager → Startup apps** →
  disable it, or just delete the shortcut from `shell:startup`.
- Offers to **launch the helper immediately** at the end of setup (checked by default).

**Why per-user instead of Program Files:** the helper writes `helper.log` (rotated to
`helper.log.1`) *next to its own exe*, and a machine-wide `Program Files` install is read-only to
a non-admin process — that would silently break logging for anyone without admin rights, which
is most people on a clinic workstation. Installing under `%LOCALAPPDATA%` keeps the exe (and the
log beside it) writable by the same account that runs the tray app, with no elevation needed.

**Upgrades:** every Setup build shares the same installer identity (fixed AppId), so running a
newer `DermoscopeHelper-Setup-X.Y.Z.exe` installs over the old copy in place — no manual
uninstall first, no duplicate Start Menu entries. If a helper is currently running, Setup (and
Uninstall) will ask you to close it first: right-click the tray icon → **Exit**.

**Uninstall:** **Settings → Apps → Installed apps → Dermoscope Helper → Uninstall**, or the
"Uninstall Dermoscope Helper" entry in its Start Menu group. This removes the exe, the Start Menu
and desktop shortcuts, the Startup-folder entry, and `helper.log` / `helper.log.1`. Nothing else
is touched.

**SmartScreen:** the installer isn't code-signed yet (same as the bare exe — see
[Code signing](#code-signing)), so Windows may show "Windows protected your PC" on first run.
Click **More info** → **Run anyway**.

**Alternative: the portable exe, no install.** If you'd rather not install anything — a one-off
test machine, a USB-stick drop, or you just want to run it from wherever you put it — skip the
installer and use the bare `helper.exe` described below in [Quick start](#quick-start-end-user-on-a-fresh-windows-machine).
Nothing is installed, nothing is registered to start at sign-in, and removing it is just deleting
the file.

---

## Quick start (end user, on a fresh Windows machine)

This section covers the **portable route**: running `helper.exe` directly with no installer. If
you want a Start Menu entry, an uninstaller, and auto-start at sign-in, use the
[installer](#install) instead — this is the manual equivalent of what it sets up for you.

1. Get `helper.exe` onto the target machine — download it from the [latest release](https://github.com/ronpik/indmu-dermoscope-button-listener/releases/latest) (see [Releases](#releases)), or copy a locally built `dist-static/helper.exe` (single file) or `dist/` (folder, exe + DLLs).
2. Plug in the dermoscope.
3. Double-click `helper.exe`. A console window may flash briefly and vanish (the helper detaches it at startup); an icon then shows up in the system tray (notification area), the helper starts capture immediately, and a balloon tells you the result: running, camera busy, device not found, or a generic start failure pointing at `helper.log`.
4. Open `http://localhost:8080/` in a browser for the built-in test page, or open the production web app. (Right-click the tray icon → **Open test page** does the same thing.)
5. Press the dermoscope button → browser (if focused) receives `F9` → fetches `/still` → displays the full-res capture.
6. When you're done, right-click the tray icon → **Exit**. That releases the camera for other apps.

No admin rights, no driver install. The server binds to all interfaces (`INADDR_ANY`) so it's reachable from other machines on the network, not just loopback — Windows Firewall will likely prompt to allow access the first time it runs.

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
| `make static` | `dist-static/helper.exe` | ~3.4 MB single file | No external deps. Drop anywhere and run. Slower to link, larger binary. **This is the build the Releases page ships.** |

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

### Building the installer (`make installer`)

```bash
make installer VERSION=1.2.3
```

Packages `dist-static/helper.exe` into a per-user Inno Setup installer at
`dist-installer/DermoscopeHelper-Setup-1.2.3.exe`. `installer` depends on `static`, so it always
rebuilds `dist-static/helper.exe` for the same `VERSION` first — you never have to remember to run
`make static` yourself, and you can't accidentally ship a Setup exe wrapping a stale build.
`VERSION` has no default here: [`installer/helper.iss`](installer/helper.iss) fails the build
outright if it isn't given one, rather than silently producing an unversioned installer.

Needs **Inno Setup 6**'s command-line compiler, `ISCC.exe` — a build-time-only dependency, not
something the shipped installer or helper need at runtime:

```powershell
winget install JRSoftware.InnoSetup
```

The `ISCC` make variable finds it: `iscc` on `PATH` first, then Inno Setup 6's default install
location (`C:/Program Files (x86)/Inno Setup 6/ISCC.exe`). Installed somewhere else? Override it:

```bash
make installer VERSION=1.2.3 ISCC="D:/Tools/Inno Setup 6/ISCC.exe"
```

`make check-iscc` (a dependency of `installer`) fails with that same install hint if `ISCC` can't
be found. See [`installer/README.md`](installer/README.md) for how `helper.iss` itself is
structured and the reasoning behind each of its settings — per-user install, the fixed AppId, the
Startup-folder shortcut, `AppMutex`, and what breaks if you change any of them.

### The one gotcha: `qedit.dll`

Microsoft deprecated `qedit.dll` long ago but it still ships with Windows 10 and 11. If a future Windows version removes it, `CoCreateInstance(CLSID_SampleGrabber)` will fail and the helper won't start. The replacement would be Media Foundation (`IMFSourceReader` + custom sink) — not done here because qedit still works today.

---

## Releases

End users do not build this. They download a prebuilt asset from the repo's Releases page:

**https://github.com/ronpik/indmu-dermoscope-button-listener/releases/latest**

Each release carries three assets:

| Asset | What it is |
|---|---|
| `DermoscopeHelper-Setup-<version>.exe` | The per-user installer — **the recommended download**. See [Install](#install). |
| `helper.exe` | The bare static exe, unchanged — a stable name that docs and client instructions hard-code, so it must not change between releases. Portable, no install; see [Quick start](#quick-start-end-user-on-a-fresh-windows-machine). |
| `helper-<version>-windows-x64.zip` | The same exe as above, zipped, for browsers and AV products that block a bare `.exe` download. |

See [`CLIENT-HANDOFF.md`](CLIENT-HANDOFF.md) for what to tell a pilot customer.

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
| Installer | Resolves `ISCC.exe` (installing Inno Setup via `choco` if the runner doesn't already have it), then `make installer VERSION=<version> ISCC=<resolved path>` to produce `DermoscopeHelper-Setup-<version>.exe`. |
| Release assets | Attaches `helper.exe`, `helper-<version>-windows-x64.zip`, and `DermoscopeHelper-Setup-<version>.exe` to the published release (release events only). |
| Workflow artifact | Uploads `helper.exe` and the installer exe as a run artifact, `helper-<version>-windows-x64`, on every run — so a build is retrievable from the Actions run even when there is no release. |
| Checksum | Prints the version, byte size and SHA256 of both `helper.exe` and the installer into the run summary. Verify a download with `Get-FileHash .\helper.exe -Algorithm SHA256` (or the installer's filename). |

### Smoke-testing without cutting a release

The workflow also has a `workflow_dispatch` trigger: **Actions** tab → **Release** → **Run workflow**. It takes an optional `version` input (default `0.0.0-dev`), builds the exe, and uploads it as a workflow artifact without creating a tag or touching any release — download the artifact from the run page and test it. This exercises the whole build and packaging path; the only step it does not reach is attaching the assets to a release, which runs on release events only.

Like a real release, this needs the workflow to be on the default branch — `workflow_dispatch` only lists workflows that exist on `main`.

### Code signing

Not wired up yet — the published exe **and** the installer that wraps it are both unsigned, so SmartScreen may warn on first run of either ("More info" → "Run anyway"). There is a commented-out Azure Trusted Signing placeholder in [`.github/workflows/release.yml`](../.github/workflows/release.yml) for when we do it — it signs `helper.exe` before the installer is built (so the installer never embeds an unsigned exe) and signs the installer again afterwards.

**This is more than a SmartScreen warning.** During installer testing, Windows Defender's
real-time protection deleted a freshly *launched* `helper.exe` outright, flagging it as
`Trojan:Win32/Bearfoos.A!ml` — a well-documented machine-learning false positive that
disproportionately hits statically-linked mingw-w64 binaries (exactly what `make static` /
`dist-static/helper.exe` produces). The file at rest on disk wasn't touched; the detection fired
on *execution*, which is also what happens when the installer's `[Run]` postinstall step launches
the app at the end of setup — so a from-scratch install on a machine with default Defender
settings can end with the exe silently deleted seconds after "successful" install. Signing both
the exe and the installer is expected to fix this (a verified publisher identity is exactly what
this class of ML heuristic is checking for) — until then, if a fresh install's tray icon
disappears right after launch or Explorer shows the app "missing" post-install, check
**Windows Security → Protection history** for a Bearfoos.A!ml (or similar) detection before
assuming a build or install bug. See the Defender note in
[`CLIENT-HANDOFF.md`](CLIENT-HANDOFF.md) for what to tell a client hitting this.

---

## Runtime usage

```
helper.exe [--console] [--preview=WxH] [--still=WxH] [port] [debounce_ms]
```

| Arg | Default | What it does |
|---|---|---|
| `--console` | off | Keeps the console window open and logs to **stderr**, as in previous versions. Without it the console is detached and the log goes to `helper.log` next to the exe. The tray icon appears either way. |
| `--preview=WxH` | `1024x768` | Capture-pin (live preview) resolution. Overrides `preview_resolution` in the config file. |
| `--still=WxH` | `1600x1200` | Still-pin resolution — the image `GET /still` serves. Overrides `still_resolution` in the config file. |
| `port` | `8080` | TCP port for the HTTP server. Bound to all interfaces, so other machines on the LAN can reach it. |
| `debounce_ms` | `300` | Suppresses still frames arriving within this window of the previous accepted one. Defense-in-depth only — the device's firmware cooldown between stills is much longer than this anyway. |

Flags don't consume positional slots: `helper.exe --console 9090` runs on port 9090.

### Configuration file (`helper-config.txt`)

Resolutions are settings, not constants. Put `helper-config.txt` **next to `helper.exe`** (the same folder as `helper.log`):

```
# Dermoscope helper settings
preview_resolution = 1024x768
still_resolution   = 1600x1200
```

One `key = value` per line; `#` and `;` start a comment. **A missing file, an unknown key, or an unparseable value all leave the defaults in place** — a machine that has never heard of this file keeps working.

| Key | Default | What it sets |
|---|---|---|
| `preview_resolution` | `1024x768` | Capture-pin resolution: the live `/preview` stream and `/snapshot`. |
| `still_resolution` | `1600x1200` | Still-pin resolution: the image `/still` serves on a button press. |

Precedence is **CLI flag > config file > built-in default**.

Both are read **once at startup**, because changing either rebuilds the DirectShow graph. Edit the file and restart the helper — no rebuild. (This is unlike the log, which is written continuously.)

**Why the preview default is 1024×768.** Over Wi-Fi the link, not the helper, is the bottleneck, and client frame rate is set by bytes-per-frame rather than by how fast frames are published. Measured on an 802.11 link at ~57.8 Mbps, with the publisher offering 6.8 fps at every resolution:

| Preview mode | Frame size | Client fps |
|---|---|---|
| 1600×1200 | ~811 KB | 2.4 |
| 1280×960 | ~453 KB | 4.1 |
| **1024×768** | **~290 KB** | **6.1** |

On a wired or otherwise fast link that constraint disappears, and `preview_resolution = 1600x1200` is a one-line change. Note this affects the **preview only** — `/still` is unaffected and keeps serving full-resolution device stills.

**Check what actually happened.** The camera advertises a fixed list of modes, and `configure_format` silently falls back to the nearest one it offers rather than failing. `GET /health` therefore reports the requested and the negotiated mode side by side, so a typo is visible instead of silent:

```
preview_resolution = 999x999   ->   "configured_width": 999,  "capture_width": 1280
                                     "configured_height": 999, "capture_height": 1024
```

If `configured_*` and `capture_*` disagree, the mode you asked for does not exist on that camera. There is deliberately no hardcoded list of valid modes to check against — it would rot against a different camera. The Still pin's list is the capture list **minus 1024×768**, so `still_resolution = 1024x768` will quietly land on a neighbouring mode; `still_pin_width`/`still_pin_height` will show it.

### Log output

Every accepted still is logged with byte size and "sending F9"; debounced triggers are logged with a `DEBOUNCED` tag. Where that goes depends on the mode:

- **`--console`** — **stderr**, same format as before.
- **tray mode (the default)** — `helper.log` in the same folder as `helper.exe`, flushed line by line so it's still useful if the process is killed. If the file has grown past roughly 1 MB it's rotated once at startup to `helper.log.1`; only that one previous log is kept.

### Single instance

Only one helper runs at a time. Launching a second `helper.exe` exits immediately and quietly, and the instance already running shows a balloon to say so. Use its tray icon rather than starting another copy.

---

## HTTP API (for integrating a real web app)

All endpoints are under `http://localhost:<port>/`, and also reachable at `http://<this machine's LAN IP>:<port>/` — the server binds to all interfaces (`INADDR_ANY`), not just loopback, so other machines on the network can reach them too.

> **There is no access control beyond the network itself.** Every endpoint sends `Access-Control-Allow-Origin: *`, so any web page open in any browser that can reach the helper's port — on this machine, or on any other machine on the same network — can stream `/preview` and fetch `/still` cross-origin, with no authentication and no per-origin restriction. That includes a third-party ad iframe on an unrelated page, or any device on the LAN, deliberately or not. In practice the dermoscope's live video is readable by anything that can route to this host and port, for as long as the helper is running. The tray **Stop** and **Exit** commands close the port as well as releasing the camera, and Windows Firewall (or restricting the network the machine is on) is the other mitigation available today. Tightening this (an `Origin` allowlist, a token in the URL, or binding back to loopback with a reverse proxy for the cases that need LAN access) would change the HTTP contract, so it is a deliberate decision for the integrating team rather than something the helper decides.

### `GET /preview`

Multipart MJPEG stream (`multipart/x-mixed-replace; boundary=frame`) at the [configured preview resolution](#configuration-file-helper-configtxt) — **1024×768** by default. The HT-B30S delivers **~6.8 fps at every resolution**; the 15 fps its driver advertises is not achievable at any mode, so don't design around it. Intended to be consumed as the `src` of an `<img>` tag:

```html
<img src="http://localhost:8080/preview" alt="dermoscope preview">
```

Browser support is universal. You can also read the same URL with `fetch()` and parse the multipart stream yourself if you want the raw frames in JS, but for rendering there's no reason to. It is **not** usable with `EventSource` — that requires `text/event-stream`, and this is `multipart/x-mixed-replace`.

Response headers include `Access-Control-Allow-Origin: *` so the endpoint can be consumed from any origin.

Query strings are stripped before routing, so the common cache-busting pattern `/preview?t=${Date.now()}` works as expected — see "Recovering from a Stop/Start cycle" below for why you'd want that. They are *ignored* everywhere except `/still`, which reads `?after=<seq>` out of the query string to long-poll (see below); a cache-buster there is harmless as long as you keep `after=` intact.

### `GET /still`

Returns the **device's own still image** from the last hardware button press — the bytes the camera's Still pin produced, at `still_resolution` (1600×1200 by default). Content type is `image/jpeg`. If no button has been pressed yet this session, returns **`204 No Content`** (empty body) — **not** `404`. (Prior to the `/health` endpoint below, this returned `404`; that was a breaking change for any client keying off `404` specifically — see `still_available` on `/health` if you need to check without triggering a fetch of nothing.)

These are real device stills, not a frozen preview frame. That distinction is the whole point: the preview pin is deliberately run at a lower resolution for streaming smoothness, while `/still` stays at full resolution for diagnostic quality. `still_width`/`still_height` on `/health` are parsed from the returned JPEG's own SOF header, so they report what the device actually produced rather than what the pin was asked for.

Every response — including the `204` — carries the sequence number of the still it represents:

```
X-Still-Seq: 5
Access-Control-Expose-Headers: X-Still-Seq
```

The `Expose-Headers` line matters: **without it a cross-origin `fetch()` cannot read `X-Still-Seq` at all**, and `resp.headers.get()` returns `null` with no error to explain why.

#### `GET /still?after=<seq>` — long-poll

Blocks until a still **newer than `<seq>`** arrives, then returns it as above. If none arrives within **8 seconds**, returns `204`. The wait is a real condition-variable wait, not a polling loop, and it is checked in 250 ms slices so an aborted request (a cancelled retake) releases its thread promptly rather than holding it for the full 8 s. `/health` and `/snapshot` stay fully responsive while a long-poll is blocked.

In practice this returns immediately, because the ordering is guaranteed by construction: the helper stores the bytes, bumps `still_seq`, and only **then** synthesises the F9 keystroke. So by the time your F9 handler runs, the image is already there. A long-poll that actually blocks means the keystroke was lost (the browser wasn't focused) — the capture still happened, and this is how you collect it.

```js
// seq is what /health last reported, or 0. Reset it whenever run_id changes.
const resp = await fetch(`http://localhost:8080/still?after=${seq}`, { cache: 'no-store' });
if (resp.status === 204) {
  // No new still within 8 s — the press produced nothing (see the note on
  // swallowed clicks under "Known issues"), or there was no press.
} else if (resp.ok) {
  seq = Number(resp.headers.get('X-Still-Seq'));
  const img = new Image();
  img.src = URL.createObjectURL(await resp.blob());
}
```

**F9 is sent only when new bytes were actually stored.** If the device delivers an empty or malformed buffer, `/still` keeps the previous image and no keystroke is emitted — so a failed capture presents as "the button did nothing" rather than silently re-saving the previous lesion. Do not assume every physical press produces an F9.

Response includes `Access-Control-Allow-Origin: *`.

### `GET /snapshot`

Returns the **latest preview frame** as `image/jpeg`, at the capture-pin resolution (1024×768 by default, ~274 KB). Never blocks and needs no button press — this is the endpoint for an on-screen "Capture" button, as opposed to `/still`'s hardware button. Returns `204 No Content` only if no preview frame has been received yet.

Because it serves the preview pin, it is unaffected by the Still pin's cooldown, and it costs the device nothing extra — it is a copy of a frame that was already captured for the stream.

Response includes `Access-Control-Allow-Origin: *`.

### `GET /health`

Returns `200 application/json` whenever the helper's HTTP server is up — the right endpoint for a web app to detect "is the helper running", as opposed to `/still`'s "has anything been captured".

```json
{
  "status": "running",
  "version": "1.2.3",
  "device": "USB Camera",
  "run_id": 1788557650209,
  "configured_width": 1024,
  "configured_height": 768,
  "capture_width": 1024,
  "capture_height": 768,
  "still_pin_width": 1600,
  "still_pin_height": 1200,
  "preview_frames": 4821,
  "preview_fps": 6.82,
  "preview_input_fps": 6.82,
  "preview_clients": 1,
  "still_seq": 3,
  "still_available": true,
  "still_width": 1600,
  "still_height": 1200,
  "still_last_ms_ago": 20343,
  "preview_max_gap_ms_since_still": 2049,
  "port": 8080,
  "uptime_s": 57
}
```

| Field | Meaning |
|---|---|
| `status` | Always `"running"` — the HTTP server (and so `/health`) exists only while capture is running; when the helper is stopped or not running, the connection is refused instead of returning a different `status` value. |
| `version` | The stamped version (see [Version and resources](#version-and-resources-helperrc)); `0.0.0` for an unstamped local build. |
| `device` | DirectShow friendly name of the camera the helper attached to. |
| `run_id` | Identifies this run of the helper (ms epoch at startup). **A client holding a `still_seq` across a helper restart must drop it when `run_id` changes** — the counter resets to 0, so a stale `?after=` would otherwise wait forever. |
| `configured_width` / `_height` | The preview resolution that was **requested** (config file or `--preview`). |
| `capture_width` / `_height` | The capture-pin mode actually **negotiated**. Differs from `configured_*` when the camera doesn't offer that mode — see [Configuration file](#configuration-file-helper-configtxt). |
| `still_pin_width` / `_height` | The Still-pin mode actually negotiated, for the same reason. |
| `preview_frames` | Running count of preview frames **published** since this capture session started. |
| `preview_fps` | Measured rate at which frames are being published. |
| `preview_input_fps` | Measured rate at which the camera is **delivering** frames. Compare with `preview_fps` to see whether the helper is dropping any. |
| `preview_frames_in`, `_skipped`, `_dropped_busy`, `_rejected` | Frame accounting: received, deliberately skipped, dropped because the buffer was locked, and rejected as malformed. |
| `preview_clients` | Number of `/preview` streams currently attached. |
| `capture_frame_interval_100ns`, `capture_advertised_fps` | What the driver *claims* the capture pin runs at. **Advertised fps is not reliable on this device** (it reports 15 fps at every mode while delivering ~6.8); trust `preview_input_fps` instead. |
| `still_seq` | Running count of accepted (non-debounced) button presses since this session started. Increments **before** the F9 keystroke is sent. |
| `still_available` | Whether `/still` currently has an image to serve — `false` until the first button press of this session. |
| `still_width` / `_height` | Dimensions parsed from the stored still's **own JPEG SOF header**, so this is what the device actually produced rather than what the pin was asked for. `0` when nothing has been captured yet. |
| `still_last_ms_ago` | Milliseconds since the last still was stored; `-1` if there hasn't been one. |
| `preview_max_gap_ms_since_still` | Largest gap between preview frames since the last still, reset on every still. The device freezes the preview for roughly **2 s** while it produces a still, so shortly after a press this reads ~2000; in steady state it sits near the nominal frame interval (~147 ms at 6.8 fps). |
| `port` | The TCP port the helper is actually listening on. |
| `uptime_s` | Seconds since this capture session started (the last Start, including the automatic one at launch). |

**A refused/failed connection to `/health` — not a particular status code from it — means the helper is stopped or not running at all.** That is the "not connected" signal for a web app to show; treat it as distinct from a `204` on `/still`, which just means nothing has been captured yet.

Response includes `Access-Control-Allow-Origin: *`.

### CORS preflight (`OPTIONS`)

`OPTIONS` on `/`, `/index.html`, `/preview`, `/still`, `/snapshot`, or `/health` returns `204 No Content` with `Access-Control-Allow-Origin: *`, `Access-Control-Allow-Methods: GET, OPTIONS`, `Access-Control-Allow-Headers: *`, and `Access-Control-Max-Age: 600`, so a browser's CORS preflight (triggered by a non-simple request, e.g. a custom header) never fails. Every endpoint here is otherwise `GET`-only.

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
   Default resolution is 1024×768; apply CSS for display size. If the preview looks laggy — most often over Wi-Fi, where bytes per frame rather than frame rate is the limit — change `preview_resolution` in [`helper-config.txt`](#configuration-file-helper-configtxt) and restart. No rebuild, and it does not affect `/still`.

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

`SendInput` only reaches the foreground window of the current interactive user session. Running the helper as a Windows service would break this. Auto-start is a Startup-folder shortcut under the interactive user, not a service — which is exactly what the [installer](#install)'s "Start Dermoscope Helper when I sign in" option sets up. Running the bare exe manually (no installer) registers nothing; you'd have to make your own Startup-folder shortcut if you want the same effect.

### A capture freezes the preview for about 2 seconds

While the device produces a still, its firmware stops delivering frames on the capture pin, so the live preview freezes. This is the device's behaviour, not a helper stall, and it is the price of real full-resolution device stills.

Measured on the HT-B30S with the Still pin at 1600×1200, across four clean presses:

| | |
|---|---|
| Freeze after a press | **1904 ms** (1903 / 1905 / 1904 / 1903 — a 2 ms spread) |
| Recovery once it ends | **immediate** — the next frame is already at the nominal interval |

There is no gradual ramp: frame 1 after the still shows the ~2 s gap, frame 2 is back to ~144 ms (nominal is 147 ms at 6.8 fps), so a "recovering" UI state isn't warranted. `preview_max_gap_ms_since_still` on `/health` reports this per capture.

The image itself is ready at the **start** of that window — the bytes are stored and `still_seq` is bumped before the F9 keystroke is sent — so an app that renders the captured still immediately hides the freeze entirely.

Note this affects `/snapshot` too, since it serves preview frames: for ~2 s after a press it returns the frame from just before the capture.

### A press during the cooldown is silently swallowed

If the button is pressed while the device is still in that ~2 s window, the firmware discards it: `StillCB::BufferCB` is never called, so there is **no still, no `still_seq` increment and no F9**, and the freeze extends by roughly one more cooldown period. This was observed once in five presses during the measurement above (a 4000 ms freeze instead of 1904 ms).

It is a device limitation, not something the helper can work around — the arrival of a Still-pin sample *is* the trigger, and there is no sample to react to. The practical fix is in the UI: disable the capture affordance for ~2 s after each press, which turns an impossible input into one that simply isn't accepted. A `?after=` long-poll timing out at 8 s is the other signal for the same condition.

Presses spaced 4–6 s apart were accepted 5 out of 5.

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
│   ├── helper.ico          -- app icon, resource ID 101 (tracked source, not build output)
│   ├── helper.png          -- 256px PNG render of the same icon, for docs/installer wizard images
│   ├── make-helper-icon.py -- regenerates helper.ico/helper.png (stdlib-only, no Pillow)
│   └── ICON-LICENSE.txt    -- icon provenance: original artwork, no third-party assets used
├── installer/
│   ├── helper.iss     -- Inno Setup 6 script, built by `make installer`
│   └── README.md      -- how the installer is built and structured, and why
├── build/             -- intermediate objects incl. windres output    [git-ignored]
├── dist/              -- shared-build output (`make shared`)          [git-ignored]
│   ├── helper.exe
│   ├── libstdc++-6.dll
│   ├── libgcc_s_seh-1.dll
│   └── libwinpthread-1.dll
├── dist-static/       -- static-build output (`make static`)          [git-ignored]
│   └── helper.exe
└── dist-installer/    -- installer output (`make installer`)          [git-ignored]
    └── DermoscopeHelper-Setup-X.Y.Z.exe
```

The `[git-ignored]` directories are listed explicitly in the repo root [`.gitignore`](../.gitignore); no built binary is ever committed. Released exes live on the [Releases page](https://github.com/ronpik/indmu-dermoscope-button-listener/releases), not in the tree.

Everything the helper reads or writes at runtime lives **next to `helper.exe` itself** — the exe's own folder, not the current working directory. That is true wherever `helper.exe` came from, including a copy downloaded from Releases:

| File | Direction | Notes |
|---|---|---|
| [`helper-config.txt`](#configuration-file-helper-configtxt) | read at startup | Optional. Absent means built-in defaults. |
| `helper.log` | written in tray mode | Rotated once to `helper.log.1` past ~1 MB. |

Neither is a build output, so neither appears in the tree above; create `helper-config.txt` yourself if you need it.

---

## Related docs

| File | When to read |
|---|---|
| [`../docs/INVESTIGATION.md`](../docs/INVESTIGATION.md) | Why the design is what it is — full chronology of probes, dead ends, and breakthroughs |
| [`../docs/NEXT-SESSION.md`](../docs/NEXT-SESSION.md) | Post-mortem of the multi-click investigation (why we went single-click only) |
| [`../docs/DESIGN.md`](../docs/DESIGN.md) | Original project design (mostly superseded by INVESTIGATION.md for Windows) |
| [`../dermoscope-helper/README.md`](../dermoscope-helper/README.md) | The original Go helper (works on Linux/macOS with caveats; **broken on Windows**) |
