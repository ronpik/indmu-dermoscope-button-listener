# Next Session — Multi-Click (Double / Triple) Detection Is Not Working

> **Read this first.** It's the briefing for picking up the single remaining bug on the Windows helper. Full back-story is in [`docs/INVESTIGATION.md`](INVESTIGATION.md). For a clean overview of what the helper does and how to build/run it, see [`windows-helper/README.md`](../windows-helper/README.md).

## TL;DR

There is a working C++ helper (`windows-helper/helper.exe`) that owns the dermoscope camera, serves an MJPEG preview and a full-resolution still snapshot over HTTP, and injects `F9` keystrokes on hardware-button press. **Single-click capture is reliable.** The helper code for grouping rapid presses into double/triple clicks is in place (it counts stills, clamps at 3, emits N F9s, and the web app is expected to count F9s in a short window). **Currently, double and triple clicks are never observed — every click burst reaches the helper as a single still frame, regardless of clicking cadence.** The job for this session is to figure out which upstream layer is absorbing the 2nd/3rd clicks, and either fix it or give up and redesign the UX.

## What you need to know to start

- **Project root:** `C:\Users\Maya\workspace\indmu-dermoscope-button-listener\`
- **Helper source + build:** [`windows-helper/helper.cpp`](../windows-helper/helper.cpp), [`windows-helper/Makefile`](../windows-helper/Makefile). `make all` produces `dist/helper.exe` (+ 3 mingw runtime DLLs) and `dist-static/helper.exe` (single fat exe).
- **Device:** HT-B30S dermoscope (Sonix), VID `0xAB02`, PID `0xAB01`, plugged in via USB. Enumerates as `USB Camera` (UVC) under Windows' inbox `usbvideo.sys`. The button is reported as a sample on the UVC Still pin.
- **Hosting:** Windows 11. C++ toolchain via MSYS2 MINGW64 (`C:\msys64\`). DirectShow APIs.
- **Why this design (and why nothing simpler works):** see [`docs/INVESTIGATION.md`](INVESTIGATION.md) §§4–8. Short version: the device is a single-consumer UVC, so we can't share with a `getUserMedia` browser tab. The helper has to own the camera and serve the video to the web app.

## Current architecture in one paragraph

`helper.exe` builds a DirectShow filter graph: source filter → **Capture pin at MJPG 1600×1200** → `SampleGrabber` (`PreviewCB` copies every 2nd frame to `g_latestPreview`) → NullRenderer; source filter → **Still pin at MJPG 320×240** → `SampleGrabber` (`StillCB` is the button trigger) → NullRenderer. When `StillCB` fires and this is the first trigger of a burst, we snapshot `g_latestPreview` into `g_latestStill` (that's what `GET /still` returns — a full-res snapshot of the click moment). The click-grouper worker waits `g_group_ms` (800 ms) of silence after the last trigger, counts presses, **clamps at 3**, and sends **N consecutive F9 keystrokes** (~40 ms apart) via `SendInput`. The web app counts F9s within a short window (~400 ms) and dispatches single / double / triple actions.

## The exact problem

The user clicks the hardware button N times in rapid succession. Helper logs show **exactly one** `still trigger accepted (q=1)` line per burst, even when the user hammers the button 5+ times in a second. The grouper always fires `n=1, sending 1 F9`. The web app therefore always interprets the gesture as a single click.

This is **not** the same bug as the earlier "`group_ms` too small" issue. The earlier problem was that genuine multi-click bursts were being split mid-burst by the grouper. The current problem is that **the 2nd/3rd clicks never arrive at the grouper at all** — `StillCB::BufferCB` is simply not called more than once per burst.

User's words: *"double and triple clicks are not detected (only single, no matter how many time I click and in what pace)."*

## What's been ruled in / out so far

### Ruled in as possible bottlenecks

- **USB bandwidth contention.** Preview pin currently streams 1600×1200 MJPEG continuously (~9 MB/s at 15 fps). While we're copying a preview frame, the USB bus may not have room to deliver a second still's interrupt/payload in time, and the device may drop it.
- **Device firmware cooldown.** We already observed empirically that still-pin resolution changes the cooldown — 1600×1200 still gave multi-second cooldown, 320×240 gave ~515 ms cooldown. Even at 320×240, something is introducing a multi-second cooldown now.
- **SampleGrabber callback blocking.** `PreviewCB::BufferCB` runs on the DirectShow source-filter streaming thread. If it holds resources the Still pin needs (even briefly), deliveries can be dropped.

### Ruled out

- **Grouper logic error.** `StillCB::BufferCB` logs *every* arrival before doing anything else. We see only one log line per burst → the callback is genuinely called only once. The grouper's count is correct given its input.
- **Debounce eating clicks.** The log shows the first still is *accepted* (`q=1`), not `DEBOUNCED`. A 2nd still within `g_debounce_ms` (150 ms) would log as `DEBOUNCED`. We don't see those lines either — so the 2nd click isn't arriving at `BufferCB` at all.
- **Still-pin resolution too high.** We lowered it back to 320×240 after this bug appeared; the symptom didn't change. So still-pin bytes-per-sample alone isn't the cause right now.

### What changed between "multi-click was working" and "now it's not"

Between the 17:38 log (when we saw `q=2` and `q=3`) and now, the concrete changes to `helper.cpp` are:

| Change | Direction | Likely effect on multi-click |
|---|---|---|
| Capture pin 320×240 → **1600×1200** | ↑ USB bandwidth, ↑ SampleGrabber work | Most likely culprit |
| Still pin 320×240 → 1600×1200 → **320×240** again | net zero | Neutral now |
| `StillCB::BufferCB` now also snapshots preview into `g_latestStill` on first-in-burst | adds ~600 KB memcpy on every burst-start | Possibly contributes |
| F9/F10/F11 → N × F9 | downstream only | Irrelevant to upstream drops |

The **only substantial upstream change** is that Capture-pin traffic went from ~50 KB/s (320×240 at 15 fps after frame-skip) to ~9 MB/s (1600×1200 at 15 fps). That's a 180× increase in bytes flowing through the SampleGrabber and the USB bus. The correlation with "multi-click stops working" is very strong.

## Suggested solutions to try (in order of cheapness)

### 1. Drop preview resolution and confirm the hypothesis (cheapest)

Edit `configure_format(pSrc, pBuilder, &PIN_CATEGORY_CAPTURE, 9999, 9999)` in `helper.cpp` to:

```cpp
configure_format(pSrc, pBuilder, &PIN_CATEGORY_CAPTURE, 320, 240);
```

Rebuild (`make all`), rerun, retry multi-click. If `q=2`/`q=3` reappear → **confirmed**: high-res preview is the choke. Then we know the solution space, and tuning is a straight tradeoff.

If multi-click still fails at 320×240 preview → the bottleneck is somewhere else (device firmware, DirectShow internals, etc.) and we need a different approach.

### 2. Find a middle-ground preview resolution

Assuming #1 shows high-res preview is the problem: binary-search for the largest preview resolution that still allows reliable multi-click.

The device's available MJPG modes (per [`probe8_mediatypes.cpp`](../C:\Users\Maya\AppData\Local\Temp\probe\probe8_mediatypes.cpp) — actually at `C:\Users\Maya\AppData\Local\Temp\probe\probe8_mediatypes.cpp`): 320×240, 352×288, 640×480, 800×600, 1024×768, 1280×960, 1280×1024, 1600×1200.

Try in this order and stop at the first one that works: 640×480, 800×600, 1024×768, 1280×960. Change the `configure_format` args, rebuild, test.

Acceptable for dermoscopy preview quality: anything ≥ 640×480 looks fine for framing. 1024×768 or 1280×960 would be great. If the lowest that works is 640×480, that's a live quality hit — but captures still come from that same pin, so `/still` would also be 640×480. (A hardware-button click does not fetch the still-pin bytes anymore; it snapshots whatever the preview pin is currently streaming.)

### 3. Increase `PreviewCB` frame skip

Currently `FRAME_SKIP=2` (process every 2nd frame). USB still *delivers* every frame; we just drop half in software. That doesn't reduce bus traffic, but it does reduce memcpy/CPU load. If the bottleneck is CPU/lock contention inside the DirectShow pipeline (not pure USB bandwidth), raising `FRAME_SKIP` to 3 or 4 might help. Cheap to try.

Edit the `static const LONG FRAME_SKIP = 2;` line in `PreviewCB` and rebuild.

### 4. Avoid the per-click preview copy

On every `first_in_burst` press, we do a 600 KB memcpy from `g_latestPreview` to `g_latestStill`. If that memcpy happens to overlap with the Still pin's kernel-streaming queue check, it could cost us the next still.

Option A: do the copy under a shorter lock, or defer it to a worker thread (buffer the pointer, copy later).

Option B: serve `/still` directly from `g_latestPreview` — no snapshot at all — and accept that the served image is the preview frame at the moment of the *fetch*, not the click. Downside: by the time the web app calls `/still`, the user may have moved the dermoscope. Latency from last click to web-app fetch is `group_ms (800)` + send-F9s (40–80) + web-app window (~400) ≈ 1.2 s. That's a lot of drift.

Option C (best of both): snapshot into `g_latestStill` asynchronously — in `StillCB::BufferCB`, push a "capture now" signal to a separate thread, return immediately, let that thread do the actual copy while DirectShow continues to stream. The Still-pin's callback stays in the kernel-streaming path for the minimum possible time.

### 5. Re-check whether the device's cooldown is preview-related

A diagnostic to rule out whether the preview pipeline is really the cause: temporarily configure the Capture pin to use a **NullRenderer with no SampleGrabber** (literally the probe5 layout). Keep the Still pin SampleGrabber as is. Re-test multi-click.

- If multi-click comes back → it's the SampleGrabber on Capture, not USB itself. Fix = process preview frames more cheaply (solutions 3, 4).
- If multi-click still fails → it's USB bandwidth or deeper. Fix = drop preview resolution (solutions 1, 2).

Concretely: replace the block that adds `pPrevGrab` + `RenderStream(CAPTURE -> pPrevGrab -> pNullPrev)` with `RenderStream(CAPTURE -> NULL -> pNullPrev)` (skip the grabber). The `/preview` endpoint will have nothing to serve, but that's fine for this diagnostic.

### 6. Accept the limitation and redesign the UX

If experiments show multi-click cannot be made reliable at any useful preview quality, drop it entirely:

- Helper always sends a single F9 on button press (no F10/F11, no sequence).
- Web app treats every F9 as "capture".
- "Clear", "undo", "re-take" etc. become in-app UI buttons or regular keyboard shortcuts.

This is the safest fallback for production. Update `windows-helper/README.md`'s "Known issues" and `docs/INVESTIGATION.md` §11 if we go this route.

## Where to start

**Start with solution 1** — it's a one-line change, one rebuild, one test, and it tells us definitively whether the preview-pin load is the cause. Everything else branches from that answer.

If solution 1 fixes it, do solution 2 to find the highest preview resolution that still allows reliable multi-click. Capture the number; that's the new default.

If solution 1 doesn't fix it, do solution 5 to locate the bottleneck (SampleGrabber vs. USB vs. deeper).

## Relevant files

- **Helper source** — where all the edits happen: [`windows-helper/helper.cpp`](../windows-helper/helper.cpp). The line to edit for solution 1 is the `configure_format(pSrc, pBuilder, &PIN_CATEGORY_CAPTURE, 9999, 9999)` call in `main()`. `PreviewCB::BufferCB` and `StillCB::BufferCB` are the SampleGrabber callbacks referenced by solutions 3–4. `run_grouper` is the click-count worker (no changes expected).
- **Build** — [`windows-helper/Makefile`](../windows-helper/Makefile). `make all` to rebuild both variants.
- **Test page** — the `INDEX_HTML[]` literal in `helper.cpp`. Already implements the F9-sequence handler + `/still` fetch, so you can test with just a browser at `http://localhost:8080/`.
- **Investigation doc** — [`docs/INVESTIGATION.md`](INVESTIGATION.md) for the full back-story: why DirectShow / why Still pin / why one consumer / why F9-only. §10 specifically documents the previous state of the multi-click investigation and the two-phase `(small, large)` pair pattern that turned out to be a red herring once we moved to the current architecture.
- **README** — [`windows-helper/README.md`](../windows-helper/README.md) for build / run / HTTP API / web-app integration.

## Things to NOT do

- Don't try to bring back the gousb / WinUSB / libusb path on Windows. Documented exhaustively in `INVESTIGATION.md` §§3–7.
- Don't raise the Still pin resolution above 320×240. At 1600×1200 it took multiple seconds between accepted stills — multi-click is mechanically impossible.
- Don't add HTTPS / auth on the local HTTP server yet. Fine for MVP; worry about it only when we ship to production.
- Don't restructure the grouper. It is not the bug. Every time we've instrumented it, it correctly processed whatever input it received. The bug is upstream — stills aren't arriving.
