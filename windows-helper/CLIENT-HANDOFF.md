# Client Handoff — Dermoscope Helper

What to send to the clinician's workstation (or QA / pilot user) to run the helper and test it against the web app.

---

## What to send

**One file:** [`dist-static/helper.exe`](dist-static/helper.exe) (~3.4 MB, no dependencies).

Built with `make static` from the source in this folder. Single self-contained Windows executable — no DLLs, no installer, no admin rights, no Visual C++ Redistributable required. Links against only inbox Windows DLLs (`kernel32`, `user32`, `ole32`, `oleaut32`, `ws2_32`, `quartz`, `qedit`), all present on every Windows 10/11 install.

If you need a smaller exe, `dist/helper.exe` (~800 KB) ships with three mingw runtime DLLs alongside (`libstdc++-6.dll`, `libgcc_s_seh-1.dll`, `libwinpthread-1.dll`) — send the whole `dist/` folder in that case. For a first customer drop, the single-file static build is the simpler experience.

---

## What the client does

1. Plug the dermoscope into a USB port.
2. Double-click `helper.exe`. A console window flashes briefly and vanishes — that is expected; the helper detaches it and keeps running in the background. An icon then appears in the system tray (notification area), capture starts on its own, and a balloon reports the result.
   - **Windows 11 hides new tray icons by default.** The first time, look in the overflow flyout behind the `^` chevron on the taskbar. Drag the icon onto the taskbar to pin it.
3. Open `http://localhost:8080/` in a browser — the built-in test page shows live preview. (Right-click the tray icon → **Open test page** does the same thing.) Press the hardware button on the dermoscope → full-res capture appears in the canvas on the right. That's the smoke test.
4. If it works, point the production web app at `http://localhost:8080/preview` and `http://localhost:8080/still`.
5. When finished, right-click the tray icon → **Exit**. That is how you quit the helper and release the camera for other apps — there is no window to close.

Right-clicking the tray icon gives four commands: **Start**, **Stop**, **Open test page**, **Exit**. Left double-click toggles Start / Stop. Hover the icon for a tooltip showing the current state (`running`, `stopped`, `device not found`, `camera busy`, `error`).

The log goes to `helper.log`, in the same folder as `helper.exe`. That is the first place to look if anything misbehaves. To get the old foreground-in-a-terminal behaviour with the log on stderr instead, run `helper.exe --console`.

No admin rights, no driver install, no firewall prompt (loopback-only bind to `127.0.0.1`). Only one copy runs at a time — launching a second `helper.exe` exits immediately and the running one shows a balloon saying so.

---

## Caveats to flag up front

- **Close any other app that might be using the camera** before running — Teams, Zoom, Skype, Windows Camera app, another browser tab doing `getUserMedia`. The dermoscope is a **single-consumer** USB device; if anything else holds it open, the helper fails to start (`MediaControl::Run` returns `ERROR_NO_SYSTEM_RESOURCES`). Log will say so.
- **The browser tab must be focused** when the user presses the hardware button. `SendInput` only delivers the F9 keystroke to the foreground window — if the clinician clicks off to Outlook and then presses the button, F9 goes to Outlook, not the web app.
- **Mixed-content rules.** If the production web app is served over HTTPS, modern browsers treat `http://localhost` as a **secure context**, so `<img src="http://localhost:8080/preview">` and `fetch("http://localhost:8080/still")` work from an HTTPS page. Verify on the target browser before shipping — behavior is uniform on current Chrome/Edge/Firefox/Safari, but test anyway.
- **Single button gesture.** One press = one capture. Multi-click (double / triple) is **not** supported on this hardware — see the post-mortem at [`../docs/NEXT-SESSION.md`](../docs/NEXT-SESSION.md) for the details. Any "clear / undo / re-take / navigate" gestures should live in the web-app UI (buttons or keyboard shortcuts), not on the hardware button.

---

## What the web-app team needs to add (two touches)

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
  if (!resp.ok) return;
  const blob = await resp.blob();
  // render or upload the blob -- it's a ~1600x1200 JPEG
});
```

Both endpoints send `Access-Control-Allow-Origin: *`, so cross-origin from any web app on any origin is allowed.

---

## Optional extras to send along

| File | Send if |
|---|---|
| [`README.md`](README.md) | The integrating team wants full HTTP API docs, build/run details, and known issues. |
| [`../docs/NEXT-SESSION.md`](../docs/NEXT-SESSION.md) | They ask "why only single-click?" — it's the full multi-click post-mortem. |
| [`../docs/INVESTIGATION.md`](../docs/INVESTIGATION.md) | They want the full design chronology (why DirectShow / why one consumer / why F9). Usually not needed at handoff; good to have linked. |

---

## How to verify before sending

```bash
# In MSYS2 MINGW64:
cd windows-helper
make static                        # builds dist-static/helper.exe
./dist-static/helper.exe --console 8080
# --console keeps the log on stderr where you can watch it. Open
# http://localhost:8080/ in a browser, press the hardware button, confirm the
# capture shows up in the canvas. Ctrl-C exits cleanly and releases the camera.
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
