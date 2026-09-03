# Multi-Click Post-Mortem — Resolved as Single-Click Only (Option B)

> This file used to be a briefing for picking up the multi-click tuning issue.
> That work is now done and the outcome is documented here. For the surrounding
> context and why the helper's architecture is shaped the way it is, see
> [`INVESTIGATION.md`](INVESTIGATION.md). For build / run / integration of the
> shipping helper, see [`../windows-helper/README.md`](../windows-helper/README.md).

## TL;DR

Multi-click (double / triple) detection on the hardware dermoscope button cannot be made reliable on the HT-B30S at any Capture-pin resolution above **320×240**. A ~4–8 second effective cooldown between Still-pin samples appears whenever the Capture pin is configured above 320×240, and no software lever available to us (resolution binary search, fps override, SampleGrabber removal) gets past it. We've committed to **Option B**: Capture pin at 1600×1200, Still pin at 320×240 used purely as a button trigger, **single F9 per press**, captures delivered by snapshotting the preview frame. Multi-click UX moves into the web app (buttons / keyboard shortcuts / gestures).

## Root cause

The HT-B30S's UVC driver bucket-picks its USB isochronous alt-setting based on the Capture-pin resolution in a discrete, binary way:

| Capture-pin MJPG | Multi-click? | Why |
|---|---|---|
| 320×240 | ✓ reliable | Low-bandwidth alt-setting; Still-pin IRPs have room on the bus |
| 352×288 through 1600×1200 | ✗ broken | High-bandwidth alt-setting reserves enough isochronous budget to crowd out the Still-pin's delivery path |

The observable symptom in the broken regime: one `still trigger accepted (q=1)` per click burst, then a 4–8 second gap before the next accepted still, regardless of how many times the user clicked. `StillCB::BufferCB` is genuinely not called for clicks 2/3 — the device's samples aren't arriving at user-mode. This is upstream of anything we control: USB bus / driver / device firmware.

None of the software mitigations we tried moves the threshold — see the per-experiment log below.

---

## What we tested this session

Every experiment was a one-line or one-function change to `windows-helper/helper.cpp`, rebuilt via `make all`, then empirically tested by hammering the hardware button while watching the helper's stderr log for `q=2` / `q=3` lines (indicating clicks 2 and 3 of a burst reached `StillCB::BufferCB`).

### 1. Capture pin at 320×240 (baseline verification → working hypothesis)

- **Idea.** Drop the Capture pin to its lowest MJPG resolution. Briefing §1.
- **Reasoning.** At 1600×1200 the Capture pin continuously pushes ~9 MB/s over USB. Hypothesis: this saturates the bus enough that Still-pin deliveries for clicks 2/3 get dropped. Lowering to 320×240 cuts continuous traffic to ~50 KB/s.
- **Result.** Multi-click works perfectly. `q=1` / `q=2` / `q=3` arrivals with intra-burst gaps of ~188–515 ms; `CLICK PATTERN: raw=N` firing correctly.
- **Verdict.** Hypothesis confirmed: the continuous Capture-pin traffic is the upstream choke. But 320×240 is visually too low for dermoscopy captures (since `/still` is a snapshot of preview), so we can't just stop here.

### 2. Capture pin at 1280×960 (~2.5 MB/s)

- **Idea.** Binary-search upward for the highest working resolution. Briefing §2.
- **Reasoning.** 1280×960 MJPG is ~3.6× lower bandwidth than 1600×1200. If the bottleneck is bandwidth proportional to actual payload, 1280×960 should fit comfortably.
- **Result.** Broken. `q=1` only, 4–11 second inter-click gaps. Same symptom as 1600×1200.
- **Verdict.** The bottleneck is not linear in bandwidth. Something discrete is happening between 320×240 and 1280×960. Continue the search down.

### 3. Capture pin at 1024×768 (~1.5 MB/s)

- **Idea.** Continue the binary search further down.
- **Reasoning.** ~40% lower continuous bandwidth than 1280×960. Should definitely work if bandwidth is the limit.
- **Result.** Broken. Same pattern. Additional observation: after the first click's preview snapshot (195 KB), subsequent snapshots collapsed to ~14 KB, consistent with the UVC driver's firmware dropping JPEG quality aggressively under sustained bus pressure.
- **Verdict.** The cliff is lower than expected. Go more aggressive.

### 4. Capture pin at 640×480 (~0.6 MB/s)

- **Idea.** Big jump below the threshold where bandwidth could plausibly saturate even conservative USB allocations.
- **Reasoning.** 640×480 MJPG is ~15× less bandwidth than 1600×1200. If continuous bus bandwidth were the mechanism, this would work.
- **Result.** Broken. First capture ~100–170 KB, later captures dropping to ~27 KB (same firmware quality-drop). Inter-click gaps 3.8–6.4 s.
- **Verdict.** The cliff is right at the 320→352×288 step, not at any "bandwidth" threshold. This points at discrete bucketing, not proportional contention.

### 5. Capture pin at 320×240 (baseline re-verification)

- **Idea.** Sanity check — rule out an unrelated code regression from all the back-and-forth edits.
- **Reasoning.** Before concluding "resolution is the only lever", confirm the known-working baseline still works.
- **Result.** Works. `q=1` / `q=2` / `q=3` with ~500 ms gaps. The `first_in_burst → snapshot preview` logic is innocent.
- **Verdict.** All behavior differences are purely driven by Capture-pin resolution. No code regression.

### 6. Capture pin at 352×288

- **Idea.** Test the only MJPG resolution the device advertises between 320×240 and 640×480 (per `probe8_mediatypes.cpp`).
- **Reasoning.** If the low-bandwidth USB alt-setting bucket includes "small-ish" resolutions in general, 352×288 might squeak in. If the bucket is literally 320×240 only, 352×288 will fail.
- **Result.** Broken. Same 4-second inter-click gaps as 640×480 and above.
- **Verdict.** The low bucket is exactly 320×240. There is no intermediate viable resolution.

### 7. Capture pin at 640×480, forced 5 fps

- **Idea.** Modify `VIDEOINFOHEADER.AvgTimePerFrame` to 5 fps (= 2,000,000 in 100-ns units) before `IAMStreamConfig::SetFormat`. If the driver picks its USB alt-setting from the combination of resolution × fps (actual bandwidth) rather than resolution alone, forcing low fps might drop us back into the low bucket.
- **Reasoning.** UVC drivers can, in principle, negotiate alt-settings based on the negotiated frame interval. If this one does, it's a lever we haven't pulled yet.
- **Result.** Driver ignored the override. Startup log: `driver accepted: 640x480, AvgTimePerFrame=666666 (~15.00 fps)`. This driver advertises exactly one frame interval per resolution and snaps `SetFormat` back to it.
- **Verdict.** Fps lever is dead on this driver. Multi-click still broken.

### 8. Capture pin at 1600×1200 with SampleGrabber removed

- **Idea.** `NEXT-SESSION.md §5` diagnostic. Render the Capture pin directly to NullRenderer with no SampleGrabber in between (probe5's original topology).
- **Reasoning.** Eliminates the user-mode sample handoff, the `PreviewCB::BufferCB` callback, the memcpy into `g_latestPreview`, and all mutex interaction. The USB bandwidth consumed is unchanged (device streams at whatever alt-setting the driver picked, regardless of downstream user-mode filters). If the Still-pin starvation was caused by CPU contention on the streaming thread or the IRP queue being held busy by our callback, this would recover multi-click. If it's bus-level reservation, nothing changes.
- **Result.** Broken. Identical 4–8 second inter-click gaps as before. `/preview` and `/still` were intentionally disabled for this diagnostic.
- **Verdict.** Definitively rules out SampleGrabber CPU / lock / handoff as the cause. Confirms the bottleneck is the USB isochronous alt-setting reservation itself — a resource allocated when the pin is connected, not something we can reduce from user-mode.

---

## Levers we did *not* try this session — and why they weren't worth trying

These were considered and dismissed up-front because prior investigation (see `INVESTIGATION.md` §§3–7) already established they don't work on this device, or because the mechanics make them mechanically incompatible with click-burst timing.

| Lever | Why not |
|---|---|
| Software-trigger a high-res still via `IAMVideoControl::SetMode(Still, ExternalTriggerEnable)` | probe4 showed `SetMode` returns S_OK after a suspicious 2.5-second delay and `GetMode` returns static capability bits, not live state. Driver is broken for this property. |
| Raise the Still pin to 1600×1200 to let it carry captures directly | At 1600×1200 the device's firmware cooldown on the Still pin itself stretches to multi-seconds. Rapid clicks are absorbed by the firmware before they can be emitted. Incompatible with multi-click in principle. |
| Runtime format switch: 320×240 (idle) ↔ 1600×1200 (on first click) | UVC `SetFormat` while running generally requires Stop → reconnect → Run (~200ms–1s). During that gap the *entire graph* is stopped — Still-pin deliveries included. The very operation meant to grab high-res on click #1 would destroy clicks #2 and #3. |
| Open two DirectShow graphs — one low-res for preview, one high-res for stills | probe5-with-browser already established the device is single-consumer: the second graph's `Run()` fails with `0x800705AA ERROR_NO_SYSTEM_RESOURCES`. |
| Dynamic subscription to a WIA / KS event instead of DirectShow | WIA: `EnumDeviceInfo` returns zero devices for this hardware (probe7). KS `KSEVENT_VIDEOCONTROL_VIDEO_TRIGGER`: driver returns `ERROR_PROPSET_NOT_FOUND` (probes 1–3). Neither is exposed. |

---

## Final architecture (Option B)

- **Capture pin**: MJPG 1600×1200 → SampleGrabber (`PreviewCB`, writes `g_latestPreview`) → NullRenderer. Feeds both `/preview` (live) and `/still` (snapshot taken at click moment).
- **Still pin**: MJPG 320×240 → SampleGrabber (`StillCB`) → NullRenderer. Used purely as the hardware-button trigger — bytes are discarded. On each trigger, `StillCB::BufferCB` debounces, copies the current `g_latestPreview` into `g_latestStill`, and sends **one** F9 keystroke via `SendInput`.
- **Web-app contract**: single F9 keydown ⇒ `fetch('/still')` ⇒ render the full-res JPEG. "Clear", "undo", "re-take", "navigate" all live in the web app's own UI (buttons / keyboard shortcuts / gestures).

CLI: `helper.exe [port] [debounce_ms]`. Debounce default 300 ms, defense-in-depth only; firmware cooldown is much longer anyway.

---

## If someone revisits this in the future

If Anthropic / the vendor issues a firmware update that changes the USB alt-setting behavior, or if you're bringing up a different dermoscope SKU, the single empirical test that tells you everything is:

> Set the Capture pin to any resolution ≥ 640×480 and click the hardware button rapidly 3 times. If the helper log shows `q=2` / `q=3` with intra-burst gaps under ~500 ms, multi-click is viable on that hardware and the grouper architecture from earlier commits is worth resurrecting. If you see only `q=1` with multi-second gaps, the bus-reservation bucket is still in play — stay on Option B.

The old grouper / press-queue / `group_ms` logic is preserved in git history (`git log -- windows-helper/helper.cpp`) if you need to revive it.
