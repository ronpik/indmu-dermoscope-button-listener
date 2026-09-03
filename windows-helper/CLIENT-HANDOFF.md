# Client Handoff — Dermoscope Helper

What to send to the clinician's workstation (or QA / pilot user) to run the helper and test it against the web app.

---

## What to send

**One file:** `DermoscopeHelper-Setup-<version>.exe` (the installer), downloaded from the repo's Releases page:

**https://github.com/ronpik/indmu-dermoscope-button-listener/releases/latest**

Send the client a link to the release rather than emailing the exe, if you can — mail gateways strip executables. This is a per-user installer: no admin rights needed, no UAC prompt, and it leaves the client with a normal Start Menu entry and uninstaller instead of a loose exe they have to remember where they put.

**Checksum.** The release workflow prints the SHA256 of both the installer and `helper.exe` into its run summary (Actions → the `Release` run for that tag). To confirm the client got the right file:

```powershell
Get-FileHash .\DermoscopeHelper-Setup-<version>.exe -Algorithm SHA256
```

**Not code-signed yet.** There is no Authenticode signature on the installer or the `helper.exe` it installs, so Windows SmartScreen may show "Windows protected your PC" the first time either one runs — the user clicks **More info** → **Run anyway**. The exe does carry version metadata (company `Indmu`, description `Dermoscope Helper`, file/product version — visible under Properties → Details), which makes it look less anonymous and helps some reputation heuristics, but it is not a substitute for signing. Warn the client about this before they open either file.

**Windows Defender may go further than a warning.** Because `helper.exe` is a statically-linked mingw-w64 binary, Windows Defender's real-time protection can flag it on execution as `Trojan:Win32/Bearfoos.A!ml` — a known machine-learning false positive for this kind of build, not an actual threat — and **delete the file**, not just warn about it. This was observed happening to a freshly-installed copy seconds after the installer's own end-of-setup launch, and again on a manual launch of the installed exe afterward. If a client reports the tray icon vanishing right after install, or Explorer showing the app missing, have them check **Windows Security → Protection history** before assuming the install failed — a `Bearfoos.A!ml` (or similarly-named) entry there confirms this, not a real infection.

Until the exe is signed, tell the client to add an exclusion *before* installing (a detection that already fired can still have deleted the file even after the exclusion is added — see below):

1. Open **Windows Security** (search it from the Start menu).
2. **Virus & threat protection** → under "Virus & threat protection settings", click **Manage settings**.
3. Scroll to **Exclusions** → **Add or remove exclusions** → **Add an exclusion** → **Folder**.
4. Add `%LOCALAPPDATA%\Programs\Dermoscope Helper` (paste that literally into the folder picker's address bar — it expands to the real path).

If the file was already deleted before the exclusion was added, either add the exclusion and reinstall, or open **Windows Security → Protection history**, find the detection, and choose **Actions → Restore** (then still add the exclusion, or the next run will just get flagged again). Code signing is expected to resolve this the same way it resolves SmartScreen, at which point this whole section goes away.

**Alternative: the bare exe, no install.** For a portable drop — a one-off test box, or a client who'd rather not install anything — send `helper.exe` (~3.4 MB, no dependencies) instead, from the same release page. It's the identical static build the installer wraps: no DLLs, no installer, no admin rights, no Visual C++ Redistributable. `helper-<version>-windows-x64.zip` (same binary, zipped) is attached alongside it for people whose browser or antivirus blocks a bare `.exe` download. Links against only inbox Windows DLLs (`kernel32`, `user32`, `ole32`, `oleaut32`, `ws2_32`, `quartz`, `qedit`), all present on every Windows 10/11 install. This is the right choice when there's no interest in a Start Menu entry, an uninstaller, or auto-start at sign-in — just double-click and go, covered in the bare-exe steps in [`README.md`](README.md#quick-start-end-user-on-a-fresh-windows-machine).

The shared build (`make shared` → `dist/helper.exe`, ~870 KB) needs three mingw runtime DLLs beside it, so it's never a good handoff pick and is never attached to a release — mentioned here only so you don't go looking for it.

---

## What the client does

1. Download `DermoscopeHelper-Setup-<version>.exe` and run it. On first run SmartScreen may interrupt: **More info** → **Run anyway** (see "What to send" — it's not signed yet).
2. Click through the wizard — no admin prompt appears. The two checkboxes it shows are already set sensibly for a clinic workstation: **desktop shortcut** is off, **"Start Dermoscope Helper when I sign in"** is on. The client can change either before clicking Install.
3. At the end of setup, "Launch Dermoscope Helper" is checked by default — leave it, and the helper starts right there. An icon appears in the system tray (notification area), capture starts on its own, and a balloon reports the result.
   - **Windows 11 hides new tray icons by default.** The first time, look in the overflow flyout behind the `^` chevron on the taskbar. Drag the icon onto the taskbar to pin it.
4. Open `http://localhost:8080/` in a browser — the built-in test page shows live preview. (Right-click the tray icon → **Open test page** does the same thing.) Press the hardware button on the dermoscope → full-res capture appears in the canvas on the right. That's the smoke test.
5. If it works, point the production web app at `http://localhost:8080/preview` and `http://localhost:8080/still`.
6. When finished, right-click the tray icon → **Exit**. That is how you quit the helper and release the camera for other apps — there is no window to close. Because "start at sign-in" is on by default, it'll be back in the tray next time the client logs in, with no need to launch it by hand.

Right-clicking the tray icon gives four commands: **Start**, **Stop**, **Open test page**, **Exit**. Left double-click toggles Start / Stop. Hover the icon for a tooltip showing the current state (`running`, `stopped`, `device not found`, `camera busy`, `error`).

The log goes to `helper.log`, in the same folder the installer put the exe (`%LOCALAPPDATA%\Programs\Dermoscope Helper`) — that is the first place to look if anything misbehaves. To get the old foreground-in-a-terminal behaviour with the log on stderr instead, run the installed `helper.exe --console` from that folder.

No admin rights, no driver install. The server listens on all network interfaces, not just `127.0.0.1`, so it's reachable from other machines on the network — Windows Firewall will likely prompt to allow access the first time it runs. Only one copy runs at a time — launching a second `helper.exe` exits immediately and the running one shows a balloon saying so; Setup itself will also notice a running helper and ask to close it before installing or uninstalling.

**Uninstalling:** **Settings → Apps → Installed apps → Dermoscope Helper → Uninstall**, or the "Uninstall Dermoscope Helper" entry in its Start Menu group. This removes the exe, both shortcuts, the Startup-folder entry, and `helper.log` / `helper.log.1` — nothing is left behind.

---

## Caveats to flag up front

- **Close any other app that might be using the camera** before running — Teams, Zoom, Skype, Windows Camera app, another browser tab doing `getUserMedia`. The dermoscope is a **single-consumer** USB device; if anything else holds it open, the helper fails to start (`MediaControl::Run` returns `ERROR_NO_SYSTEM_RESOURCES`). Log will say so.
- **The browser tab must be focused** when the user presses the hardware button. `SendInput` only delivers the F9 keystroke to the foreground window — if the clinician clicks off to Outlook and then presses the button, F9 goes to Outlook, not the web app.
- **Mixed-content rules.** If the production web app is served over HTTPS, modern browsers treat `http://localhost` as a **secure context**, so `<img src="http://localhost:8080/preview">` and `fetch("http://localhost:8080/still")` work from an HTTPS page. Verify on the target browser before shipping — behavior is uniform on current Chrome/Edge/Firefox/Safari, but test anyway.
- **Single button gesture.** One press = one capture. Multi-click (double / triple) is **not** supported on this hardware — see the post-mortem at [`../docs/NEXT-SESSION.md`](../docs/NEXT-SESSION.md) for the details. Any "clear / undo / re-take / navigate" gestures should live in the web-app UI (buttons or keyboard shortcuts), not on the hardware button.

---

## What the web-app team needs to add (three touches)

### 1. Preview source

```html
<img id="preview" src="http://localhost:8080/preview">
```

Native resolution is 1600×1200. Apply CSS for display size. No `getUserMedia` needed — the helper owns the camera and streams MJPEG to this tag.

### 2. Capture on F9

```js
document.addEventListener('keydown', async e => {
  if (e.key !== 'F9' && e.code !== 'F9') return;
  e.preventDefault();
  const resp = await fetch('http://localhost:8080/still', { cache: 'no-store' });
  if (resp.status === 204) return;   // nothing captured yet this session -- not an error
  if (!resp.ok) return;
  const blob = await resp.blob();
  // render or upload the blob -- it's a ~1600x1200 JPEG
});
```

`/still` returns **`204 No Content`** (not `404`) before the first button press of a session — that's a normal "nothing captured yet" state, so don't treat it as "helper not connected".

### 3. Connectivity check: `GET /health`

Poll `http://localhost:8080/health` to know whether the helper is running — **that's** the "is the helper connected" signal, not `/still`. It responds `200 application/json` (`{"status":"running", ...}`) whenever the helper's HTTP server is up. **A refused/failed connection — not a particular response from it — means the helper is stopped or not running at all:**

```js
async function helperIsUp() {
  try {
    const resp = await fetch('http://localhost:8080/health', { cache: 'no-store' });
    return resp.ok;
  } catch {
    return false;   // connection refused: helper is stopped or not running
  }
}
```

See [`README.md`](README.md#get-health) for the full response shape (version, device name, frame counters, `still_available`, `uptime_s`).

---

All three endpoints send `Access-Control-Allow-Origin: *`, so cross-origin from any web app on any origin is allowed.

---

## Optional extras to send along

| File | Send if |
|---|---|
| [`README.md`](README.md) | The integrating team wants full HTTP API docs, build/run details, and known issues. |
| [`../docs/NEXT-SESSION.md`](../docs/NEXT-SESSION.md) | They ask "why only single-click?" — it's the full multi-click post-mortem. |
| [`../docs/INVESTIGATION.md`](../docs/INVESTIGATION.md) | They want the full design chronology (why DirectShow / why one consumer / why F9). Usually not needed at handoff; good to have linked. |

---

## How to verify before sending

**Normal path — check the released installer.** Download `DermoscopeHelper-Setup-<version>.exe` from the [latest release](https://github.com/ronpik/indmu-dermoscope-button-listener/releases/latest), compare `Get-FileHash .\DermoscopeHelper-Setup-<version>.exe -Algorithm SHA256` against the SHA256 in that release's workflow run summary, then run it end to end on a Windows box with the dermoscope plugged in: run the installer, let it launch the helper at the end, open `http://localhost:8080/` in a browser, press the hardware button, confirm the capture shows up in the canvas. Then uninstall it (**Settings → Apps → Installed apps → Dermoscope Helper**) and confirm the exe, shortcuts, Startup entry, and `helper.log*` are all gone.

Also spot-check the bare `helper.exe` from the same release the same way it's used standalone:

```powershell
.\helper.exe 8080
# Runs in tray mode: a console window flashes and vanishes, then a tray icon
# appears and a balloon reports the result. Open http://localhost:8080/ in a
# browser, press the hardware button, confirm the capture shows up in the
# canvas, then right-click the tray icon -> Exit when done.
```

**Fallback — build it yourself.** Only needed when you are testing a change that has not been released yet (see [`README.md`](README.md) for the full build and release docs):

```bash
# In MSYS2 MINGW64:
cd windows-helper
make static                        # builds dist-static/helper.exe
./dist-static/helper.exe --console 8080
# --console keeps the log on stderr where you can watch it. Open
# http://localhost:8080/ in a browser, press the hardware button, confirm the
# capture shows up in the canvas. Ctrl-C exits cleanly and releases the camera.

make installer VERSION=0.0.1-dev   # needs ISCC -- see README.md's "Building the installer" section
# -> dist-installer/DermoscopeHelper-Setup-0.0.1-dev.exe -- run it to test the install flow itself.
```

Verify the shipping (tray) behaviour too: double-click `dist-static/helper.exe`, confirm the tray icon appears and the balloon says it is running, then right-click → **Exit** and confirm the camera is free afterwards (the Windows Camera app should open the dermoscope).

Expected log on a good run — on stderr with `--console`, otherwise in `helper.log` next to the exe:

```
[HH:MM:SS.mmm] Config: port=8080 debounce_ms=300 mode=console
[HH:MM:SS.mmm] Looking for dermoscope (vid_ab02)...
[HH:MM:SS.mmm] Selected device: USB Camera
[HH:MM:SS.mmm] Setting format MJPG 1600x1200 on pin
[HH:MM:SS.mmm] Setting format MJPG 320x240 on pin
[HH:MM:SS.mmm] MediaControl::Run HR=0x00000001
[HH:MM:SS.mmm] Graph state: 2 (2=Running)
[HH:MM:SS.mmm] HTTP server listening on http://localhost:8080/
[HH:MM:SS.mmm] Helper ready. Open http://localhost:8080/ in your browser.
[HH:MM:SS.mmm] Hardware button -> F9 -> web app fetches /still.
# ... on button press:
[HH:MM:SS.mmm]   still trigger -> /still 413392 bytes, sending F9
```

If `MediaControl::Run` returns anything other than `0x00000000` or `0x00000001`, or if `Graph state` is not `2`, something else is holding the camera — close Teams / Zoom / browser cam tabs / other helper instances and retry. In tray mode the same condition shows up as a `camera busy` tooltip and balloon.
