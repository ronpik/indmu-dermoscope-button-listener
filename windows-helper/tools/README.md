# Debugging tools

Small standalone utilities for questions the helper's own logs cannot answer.
Not shipped to users and not part of the installer.

## camprobe

Answers: **is the camera actually free right now, or is something still holding
it?**

The helper logs `Capture stopped; camera released.` when its DirectShow graph
confirms it reached `Stopped`. That is the helper's own view. `camprobe` is an
independent second process that tries to genuinely claim the device, so it can
confirm or contradict that view.

Reach for it when:

* the helper reports `CameraBusy` and you need to know whether the blocker is
  the helper itself, a leftover process, or something else on the machine
* you suspect a Stop did not really release the device
* a still or preview has gone dead and you want to know if the device is
  claimable at all before blaming the helper

### Build and run

```sh
make camprobe                 # from windows-helper/, in an MSYS2 MINGW64 shell
./build/camprobe.exe          # probes the first device matching "USB Camera"
./build/camprobe.exe --list   # just enumerate devices, claim nothing
./build/camprobe.exe EasyCam  # probe a different device by name substring
```

Exit code: `0` available, `1` blocked, `2` probe error.

> **Do not run `camprobe` (without `--list`) while the helper is capturing.**
> It does not merely compete for the device — it corrupts it. On the HT-B30S,
> one probe against a live helper permanently drops the **still pin** to the
> capture pin's resolution (1600x1200 -> 1024x768) for the rest of the session,
> while the pin continues to advertise 1600x1200. Only restarting capture
> recovers it; the helper now rejects such frames outright (see
> `still_frames_rejected` in `/health`), so the visible symptom is that button
> presses stop producing stills.
>
> The cause is that `RenderStream` succeeds on the capture pin even when the
> camera is busy and `Run` fails — the format negotiation lands before the
> failure does. `--list` never builds a graph and is always safe.

Use `--list` first. Reach for the full probe only when the helper is stopped, or
when you have already accepted that capture will need a restart afterwards.

### Reading the output

```
  Run            -> 0x800705AA  ERROR_NO_SYSTEM_RESOURCES -- CAMERA IN USE by another process
RESULT: camera BLOCKED -- could not claim it
```

Two things worth knowing, both mirrored from `helper.cpp`:

* **`Run()` returning `S_FALSE` (`0x00000001`) is normal, not a failure.** This
  device returns it while the graph is still transitioning. `GetState` reporting
  `state=2 (Running)` is the arbiter — `camprobe` and `start_capture()` both
  judge success that way.
* **`0x800705AA` is the only code that means "someone else has the camera".**
  It is the sole HRESULT `helper.cpp:is_camera_busy_hr()` maps to the
  `CameraBusy` state, so the tool and the helper agree by construction. Other
  failures mean something different is wrong — see the table in `camprobe.cpp`.

A successful `BindToObject` proves very little on its own: the USB stream is not
opened until the graph runs, so a busy camera usually binds fine and then fails
at `Run`. That step is reported only to localise a failure.

## Established with these tools

Findings worth not rediscovering. Verified 2026-09-10 against helper
`0.4.6-align` and the HT-B30S dermoscope.

* **Tray Stop genuinely releases the camera.** Probe was BLOCKED
  (`0x800705AA`) while capturing and AVAILABLE on 3/3 attempts immediately
  after a Stop. Teardown took ~320 ms. If it ever *fails* to release, the helper
  logs `graph STILL not Stopped ... the camera may not have been released`
  (`helper.cpp`, in `stop_capture`) — that warning is the thing to grep for.
* **Tray Stop also shuts down the HTTP server.** `stop_capture()` calls
  `http_server_stop()`, so port 8080 closes completely and `/health` becomes
  unreachable rather than reporting zeros. Anything pointed at `localhost:8080`
  dies outright, which can easily be mistaken for the camera still being locked.
* **The helper's window is message-only** (`CreateWindowExW(..., HWND_MESSAGE,
  ...)`). It is therefore not enumerable by `EnumWindows`, and a plain
  `taskkill <pid>` posts a `WM_CLOSE` that is never delivered — only
  `taskkill /F` works from a script. The tray menu's own Stop and Exit are
  unaffected, as is the installer, which uses `AppMutex` to detect a running
  helper.
* **The helper enforces a single instance** via the `DermoscopeHelperSingleInstance`
  mutex, so you cannot start a second `helper.exe` to test whether the camera is
  free. That is the gap `camprobe` fills.
