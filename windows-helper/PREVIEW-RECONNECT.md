# `/preview` doesn't recover when the connection drops — needs a reconnect strategy

**Audience:** the web-app team integrating with the helper's `/preview` endpoint.

**Status:** confirmed gap, not yet fixed on the web-app side. Nothing on the helper side can close this — it needs client-side (web-app) reconnect logic. The helper's own test page already implements the fix; use it as the reference.

---

## Context

`GET http://localhost:8080/preview` (served by the Windows helper, [`helper.cpp`](helper.cpp)) is a single long-lived HTTP/1.0 connection streaming `multipart/x-mixed-replace; boundary=frame` — the standard way to push an MJPEG live feed to an `<img src="...">` tag. It is **not** re-established automatically by anything on the server side; it's one TCP connection for the lifetime of that `<img>` element.

## The gap

If that connection ever closes, the browser does not automatically reopen it. An `<img>` tag fetches its `src` once; there's no built-in retry for a streaming resource like this.

**Confirmed behavior today:** the preview **freezes on the last successfully rendered frame** with no signal that anything went wrong — `naturalWidth`/`naturalHeight` and `img.complete` stay as they were, and neither a `load` nor an `error` event fires. This is standard browser behaviour for `multipart/x-mixed-replace`: a stream that quietly ends looks identical to one that is idle between frames. **`onerror` is not a usable signal.** It stays frozen until the page is manually reloaded.

## Why this matters now

Two things changed on the helper side that make a clean connection close the *normal* failure mode rather than an edge case:

1. **The helper is now a system-tray app** (PR #5). The clinician can Stop and Start the camera from the tray menu at any time, with no browser involvement. Every Stop closes the `/preview` connection. Add helper restarts, device unplug, and sleep/wake on top of that.
2. **The stream itself was hardened.** Previously a slow client read could cause a partial TCP write that the old code didn't detect, silently corrupting the stream while leaving the connection technically open — never erroring, never closing, just permanently garbled. `send_all()` now retries short writes and the helper drops the connection cleanly on a real failure (also PR #5); this branch adds a JPEG-validity check so a bad frame is skipped rather than emitted. Net effect: `/preview` is now either perfectly framed or cleanly closed. Freeze-on-close is the only failure mode left, and it was never handled on the client side.

## What to do on the web-app side

The reliable pattern is documented in [`README.md` → "Recovering from a Stop/Start cycle"](README.md#recovering-from-a-stopstart-cycle), and the helper's built-in test page (`INDEX_HTML` in [`helper.cpp`](helper.cpp), function `checkPreviewHealth()`) is a working implementation. In short:

1. **Poll a liveness endpoint**, e.g. `GET /` every 2–3 s. It only responds while the server is up; `/preview` itself can't tell you anything.
2. **When it starts responding again after having failed, force a fresh connection** by reassigning `img.src` with a cache-buster: `img.src = 'http://localhost:8080/preview?t=' + Date.now()`. Reassigning the *same* `src` does not reconnect a stalled stream. Query strings are stripped server-side before routing, so `?t=` is safe on every endpoint.
3. **Surface the state in the UI** — "camera disconnected / reconnecting" — rather than leaving a silently frozen frame in front of the clinician.

Do **not** build on `onerror`, `onload`, or `img.complete`; none of them fire or change when the stream ends.

The F9 → `fetch('/still')` capture path needs no such handling — it's stateless per request; a `fetch('/still')` while the helper is stopped simply rejects.

---

## Related docs

| File | When to read |
|---|---|
| [`README.md`](README.md) | Full HTTP API reference; the "Recovering from a Stop/Start cycle" section is the canonical write-up of this behaviour |
| [`CLIENT-HANDOFF.md`](CLIENT-HANDOFF.md) | What to send the client and the base preview/capture integration (`<img src="/preview">`, F9 → `fetch('/still')`) |
| [`../docs/INVESTIGATION.md`](../docs/INVESTIGATION.md) | Why the helper architecture looks the way it does |
